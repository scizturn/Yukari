package repository

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/kyou-id/yukari/internal/domain"
)

type BirthdayVoucherConfig struct {
	Code               string              `json:"code"`
	CodePrefix         string              `json:"code_prefix"`
	CodeTemplate       string              `json:"code_template"`
	Name               string              `json:"name"`
	Description        *string             `json:"description"`
	Type               string              `json:"type"`
	Amount             flexibleInt         `json:"amount"`
	MaxDiscount        flexibleNullInt     `json:"max_discount"`
	MinPurchase        flexibleInt         `json:"min_purchase"`
	Distribution       string              `json:"distribution"`
	RequiresClaim      flexibleBool        `json:"requires_claim"`
	MaxClaimTotal      flexibleNullInt     `json:"max_claim_total"`
	UsageLimitTotal    flexibleNullInt     `json:"usage_limit_total"`
	UsageLimitPerUser  flexibleInt         `json:"usage_limit_per_user"`
	ClaimAnimation     *string             `json:"claim_animation"`
	DurationDays       flexibleInt         `json:"duration_days"`
	PricingVoucherID   flexibleNullInt64   `json:"pricing_voucher_id"`
	PricingVoucherCode string              `json:"pricing_voucher_code"`
	Rules              []VoucherRuleConfig `json:"rules"`
	BusinessRules      []VoucherRuleConfig `json:"business_rules"`
	BasicInfo          *VoucherBasicInfo   `json:"basic_info"`
}

type VoucherRuleConfig struct {
	Type     string `json:"type"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

type VoucherBasicInfo struct {
	ItemTypes       []string `json:"item_types"`
	UserRoles       []string `json:"user_roles"`
	IsPartner       int      `json:"is_partner"`
	SpecificUserIDs string   `json:"specific_user_ids"`
	AccountAgeMin   string   `json:"account_age_min"`
	AccountAgeMax   string   `json:"account_age_max"`
}

type MySQLVoucherCreator struct {
	db         *sql.DB
	cfg        BirthdayVoucherConfig
	codeSecret string
}

func OpenMySQLVoucherCreator(dsn string, cfg BirthdayVoucherConfig, codeSecret string) (*MySQLVoucherCreator, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &MySQLVoucherCreator{db: db, cfg: cfg.withDefaults(), codeSecret: codeSecret}, nil
}

func LoadBirthdayVoucherConfig(path string) (BirthdayVoucherConfig, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return BirthdayVoucherConfig{}, err
	}
	var cfg BirthdayVoucherConfig
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return BirthdayVoucherConfig{}, err
	}
	return cfg.withDefaults(), nil
}

func (c *MySQLVoucherCreator) Close() error {
	return c.db.Close()
}

func (c *MySQLVoucherCreator) CreateBirthdayVoucher(ctx context.Context, user domain.User, birthdayDate time.Time, itemIDs []string) (domain.Voucher, error) {
	if !c.cfg.PricingVoucherID.Valid && strings.TrimSpace(c.cfg.PricingVoucherCode) == "" {
		return domain.Voucher{}, fmt.Errorf("birthday pricing voucher id or code is required")
	}
	if strings.TrimSpace(user.ID) == "" {
		return domain.Voucher{}, fmt.Errorf("birthday voucher user id is required")
	}

	code := c.voucherCode(user.ID, birthdayDate)
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Voucher{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	existing, found, err := voucherByCode(ctx, tx, code)
	if err != nil {
		return domain.Voucher{}, err
	}
	if found {
		if err = tx.Commit(); err != nil {
			return domain.Voucher{}, err
		}
		existing.Existed = true
		return existing, nil
	}

	pricingVoucherID := c.cfg.PricingVoucherID.Value
	if !c.cfg.PricingVoucherID.Valid {
		err = tx.QueryRowContext(ctx, `
SELECT id
FROM vouchers
WHERE code = ?
LIMIT 1`, c.cfg.PricingVoucherCode).Scan(&pricingVoucherID)
		if err != nil {
			if err == sql.ErrNoRows {
				return domain.Voucher{}, fmt.Errorf("pricing voucher %q not found", c.cfg.PricingVoucherCode)
			}
			return domain.Voucher{}, err
		}
	}

	startAt := birthdayDate
	expiredAt := birthdayDate.AddDate(0, 0, c.cfg.DurationDays.Value)
	requiresClaim := boolInt(c.cfg.RequiresClaim.Value)
	result, err := tx.ExecContext(ctx, `
INSERT INTO vouchers (
  code,
  name,
  description,
  type,
  amount,
  max_discount,
  min_purchase,
  distribution,
  requires_claim,
  max_claim_total,
  start_at,
  expired_at,
  usage_limit_total,
  usage_limit_per_user,
  is_active,
  claim_animation,
  created_at,
  updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, NOW(), NOW())`,
		code,
		c.voucherName(),
		stringPtrValue(c.cfg.Description),
		c.cfg.Type,
		c.cfg.Amount.Value,
		c.cfg.MaxDiscount.sqlValue(),
		c.cfg.MinPurchase.Value,
		c.cfg.Distribution,
		requiresClaim,
		c.cfg.MaxClaimTotal.sqlValue(),
		startAt,
		expiredAt,
		c.cfg.UsageLimitTotal.sqlValue(),
		c.cfg.UsageLimitPerUser.Value,
		stringPtrValue(c.cfg.ClaimAnimation),
	)
	if err != nil {
		return domain.Voucher{}, err
	}
	voucherID, err := result.LastInsertId()
	if err != nil {
		return domain.Voucher{}, err
	}

	_, err = tx.ExecContext(ctx, `
INSERT INTO voucher_pricing_aliases (
  voucher_id,
  pricing_voucher_id,
  alias_mode,
  created_at,
  updated_at
) VALUES (?, ?, 'pricing_only', NOW(), NOW())`, voucherID, pricingVoucherID)
	if err != nil {
		return domain.Voucher{}, err
	}

	if err = insertRule(ctx, tx, voucherID, "user", []string{user.ID}, "include"); err != nil {
		return domain.Voucher{}, err
	}
	if err = c.insertConfiguredRules(ctx, tx, voucherID, user.ID, itemIDs); err != nil {
		return domain.Voucher{}, err
	}
	if err = c.insertConfiguredBusinessRules(ctx, tx, voucherID, user.ID, itemIDs); err != nil {
		return domain.Voucher{}, err
	}
	if err = c.insertBasicInfoRules(ctx, tx, voucherID); err != nil {
		return domain.Voucher{}, err
	}
	if err = tx.Commit(); err != nil {
		return domain.Voucher{}, err
	}
	return domain.Voucher{ID: voucherID, Code: code}, nil
}

func (c *MySQLVoucherCreator) CreateWishlistBackInVoucher(ctx context.Context, user domain.User, campaignDate time.Time, itemIDs []string) (domain.Voucher, error) {
	if !c.cfg.PricingVoucherID.Valid && strings.TrimSpace(c.cfg.PricingVoucherCode) == "" {
		return domain.Voucher{}, fmt.Errorf("wishlist back in pricing voucher id or code is required")
	}
	if strings.TrimSpace(user.ID) == "" {
		return domain.Voucher{}, fmt.Errorf("wishlist back in voucher user id is required")
	}

	code := c.wishlistBackInVoucherCode(user.ID, campaignDate)
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Voucher{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Anti-spam: if this user still has a live (unexpired) WBI voucher they have
	// NOT used yet, don't mint a new one — just widen its item scope to cover the
	// newly restocked items. A used or expired voucher is not reusable, so a fresh
	// voucher is issued then (even before 14 days: a one-shot voucher that was
	// already redeemed is spent).
	reusable, reuse, err := reusableWishlistBackInVoucher(ctx, tx, user.ID)
	if err != nil {
		return domain.Voucher{}, err
	}
	if reuse {
		if err = extendItemIDRule(ctx, tx, reusable.ID, itemIDs); err != nil {
			return domain.Voucher{}, err
		}
		if err = tx.Commit(); err != nil {
			return domain.Voucher{}, err
		}
		reusable.Existed = true
		return reusable, nil
	}

	existing, found, err := voucherByCode(ctx, tx, code)
	if err != nil {
		return domain.Voucher{}, err
	}
	if found {
		if err = tx.Commit(); err != nil {
			return domain.Voucher{}, err
		}
		existing.Existed = true
		return existing, nil
	}

	pricingVoucherID := c.cfg.PricingVoucherID.Value
	if !c.cfg.PricingVoucherID.Valid {
		err = tx.QueryRowContext(ctx, `
SELECT id
FROM vouchers
WHERE code = ?
LIMIT 1`, c.cfg.PricingVoucherCode).Scan(&pricingVoucherID)
		if err != nil {
			if err == sql.ErrNoRows {
				return domain.Voucher{}, fmt.Errorf("pricing voucher %q not found", c.cfg.PricingVoucherCode)
			}
			return domain.Voucher{}, err
		}
	}

	startAt := campaignDate
	expiredAt := campaignDate.AddDate(0, 0, c.cfg.DurationDays.Value)
	requiresClaim := boolInt(c.cfg.RequiresClaim.Value)
	result, err := tx.ExecContext(ctx, `
INSERT INTO vouchers (
  code, name, description, type, amount, max_discount, min_purchase,
  distribution, requires_claim, max_claim_total, start_at, expired_at,
  usage_limit_total, usage_limit_per_user, is_active, claim_animation,
  created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, NOW(), NOW())`,
		code, c.voucherName(), stringPtrValue(c.cfg.Description), c.cfg.Type,
		c.cfg.Amount.Value, c.cfg.MaxDiscount.sqlValue(), c.cfg.MinPurchase.Value,
		c.cfg.Distribution, requiresClaim, c.cfg.MaxClaimTotal.sqlValue(), startAt,
		expiredAt, c.cfg.UsageLimitTotal.sqlValue(), c.cfg.UsageLimitPerUser.Value,
		stringPtrValue(c.cfg.ClaimAnimation),
	)
	if err != nil {
		return domain.Voucher{}, err
	}
	voucherID, err := result.LastInsertId()
	if err != nil {
		return domain.Voucher{}, err
	}

	_, err = tx.ExecContext(ctx, `
INSERT INTO voucher_pricing_aliases (
  voucher_id, pricing_voucher_id, alias_mode, created_at, updated_at
) VALUES (?, ?, 'pricing_only', NOW(), NOW())`, voucherID, pricingVoucherID)
	if err != nil {
		return domain.Voucher{}, err
	}
	if err = insertRule(ctx, tx, voucherID, "user", []string{user.ID}, "include"); err != nil {
		return domain.Voucher{}, err
	}
	if err = c.insertConfiguredRules(ctx, tx, voucherID, user.ID, itemIDs); err != nil {
		return domain.Voucher{}, err
	}
	if err = c.insertConfiguredBusinessRules(ctx, tx, voucherID, user.ID, itemIDs); err != nil {
		return domain.Voucher{}, err
	}
	if err = c.insertBasicInfoRules(ctx, tx, voucherID); err != nil {
		return domain.Voucher{}, err
	}
	if err = tx.Commit(); err != nil {
		return domain.Voucher{}, err
	}
	return domain.Voucher{ID: voucherID, Code: code}, nil
}

// reusableWishlistBackInVoucher finds this user's live, still-unused wishlist-
// back-in voucher (WBI code, active, not expired, no claim marked used_at). Such
// a voucher is reused (with its item scope widened) instead of minting a new one.
func reusableWishlistBackInVoucher(ctx context.Context, tx *sql.Tx, userID string) (domain.Voucher, bool, error) {
	var v domain.Voucher
	err := tx.QueryRowContext(ctx, `
SELECT v.id, v.code, v.created_at
FROM vouchers v
JOIN voucher_rules ur ON ur.voucher_id = v.id AND ur.rule_type = 'user' AND ur.rule_value = ?
WHERE v.code LIKE 'WBI%'
  AND v.is_active = 1
  AND (v.expired_at IS NULL OR v.expired_at > NOW())
  AND NOT EXISTS (
    SELECT 1 FROM voucher_claims vc
    WHERE vc.voucher_id = v.id AND vc.used_at IS NOT NULL
  )
ORDER BY v.expired_at DESC
LIMIT 1`, userID).Scan(&v.ID, &v.Code, &v.CreatedAt)
	if err == sql.ErrNoRows {
		return domain.Voucher{}, false, nil
	}
	if err != nil {
		return domain.Voucher{}, false, err
	}
	return v, true, nil
}

// extendItemIDRule unions newItemIDs into a voucher's existing item_id include
// rule (creating it if absent), so a reused voucher covers newly restocked items.
func extendItemIDRule(ctx context.Context, tx *sql.Tx, voucherID int64, newItemIDs []string) error {
	var current sql.NullString
	err := tx.QueryRowContext(ctx, `
SELECT rule_value FROM voucher_rules
WHERE voucher_id = ? AND rule_type = 'item_id'
LIMIT 1`, voucherID).Scan(&current)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	seen := map[string]bool{}
	var union []string
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		union = append(union, id)
	}
	for _, id := range parseRuleValue(current.String) {
		add(id)
	}
	for _, id := range newItemIDs {
		add(id)
	}

	if err == sql.ErrNoRows {
		return insertRule(ctx, tx, voucherID, "item_id", union, "include")
	}
	_, err = tx.ExecContext(ctx, `
UPDATE voucher_rules SET rule_value = ?, updated_at = NOW()
WHERE voucher_id = ? AND rule_type = 'item_id'`, ruleValue(union), voucherID)
	return err
}

// parseRuleValue reads a voucher_rules.rule_value that is either a bare scalar
// ("123") or a JSON array ("[123,456]" / "[\"123\"]").
func parseRuleValue(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if strings.HasPrefix(value, "[") {
		var arr []json.RawMessage
		if json.Unmarshal([]byte(value), &arr) == nil {
			out := make([]string, 0, len(arr))
			for _, e := range arr {
				out = append(out, strings.Trim(strings.TrimSpace(string(e)), `"`))
			}
			return out
		}
	}
	return []string{value}
}

func (c *MySQLVoucherCreator) CreateAnniversaryVoucher(ctx context.Context, user domain.User, anniversaryDate time.Time, itemIDs []string) (domain.Voucher, error) {
	if !c.cfg.PricingVoucherID.Valid && strings.TrimSpace(c.cfg.PricingVoucherCode) == "" {
		return domain.Voucher{}, fmt.Errorf("anniversary pricing voucher id or code is required")
	}
	if strings.TrimSpace(user.ID) == "" {
		return domain.Voucher{}, fmt.Errorf("anniversary voucher user id is required")
	}

	code := c.anniversaryVoucherCode(user.ID, anniversaryDate)
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Voucher{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	existing, found, err := voucherByCode(ctx, tx, code)
	if err != nil {
		return domain.Voucher{}, err
	}
	if found {
		if err = tx.Commit(); err != nil {
			return domain.Voucher{}, err
		}
		existing.Existed = true
		return existing, nil
	}

	pricingVoucherID := c.cfg.PricingVoucherID.Value
	if !c.cfg.PricingVoucherID.Valid {
		err = tx.QueryRowContext(ctx, `
SELECT id
FROM vouchers
WHERE code = ?
LIMIT 1`, c.cfg.PricingVoucherCode).Scan(&pricingVoucherID)
		if err != nil {
			if err == sql.ErrNoRows {
				return domain.Voucher{}, fmt.Errorf("pricing voucher %q not found", c.cfg.PricingVoucherCode)
			}
			return domain.Voucher{}, err
		}
	}

	startAt := anniversaryDate
	expiredAt := anniversaryDate.AddDate(0, 0, 14) // Hardcoded 2 weeks for anniversary
	requiresClaim := boolInt(c.cfg.RequiresClaim.Value)
	result, err := tx.ExecContext(ctx, `
INSERT INTO vouchers (
  code,
  name,
  description,
  type,
  amount,
  max_discount,
  min_purchase,
  distribution,
  requires_claim,
  max_claim_total,
  start_at,
  expired_at,
  usage_limit_total,
  usage_limit_per_user,
  is_active,
  claim_animation,
  created_at,
  updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, NOW(), NOW())`,
		code,
		c.anniversaryVoucherName(user),
		stringPtrValue(c.cfg.Description),
		c.cfg.Type,
		c.cfg.Amount.Value,
		c.cfg.MaxDiscount.sqlValue(),
		c.cfg.MinPurchase.Value,
		c.cfg.Distribution,
		requiresClaim,
		c.cfg.MaxClaimTotal.sqlValue(),
		startAt,
		expiredAt,
		c.cfg.UsageLimitTotal.sqlValue(),
		c.cfg.UsageLimitPerUser.Value,
		stringPtrValue(c.cfg.ClaimAnimation),
	)
	if err != nil {
		return domain.Voucher{}, err
	}
	voucherID, err := result.LastInsertId()
	if err != nil {
		return domain.Voucher{}, err
	}

	_, err = tx.ExecContext(ctx, `
INSERT INTO voucher_pricing_aliases (
  voucher_id,
  pricing_voucher_id,
  alias_mode,
  created_at,
  updated_at
) VALUES (?, ?, 'pricing_only', NOW(), NOW())`, voucherID, pricingVoucherID)
	if err != nil {
		return domain.Voucher{}, err
	}

	if err = insertRule(ctx, tx, voucherID, "user", []string{user.ID}, "include"); err != nil {
		return domain.Voucher{}, err
	}
	if err = c.insertConfiguredRules(ctx, tx, voucherID, user.ID, itemIDs); err != nil {
		return domain.Voucher{}, err
	}
	if err = c.insertConfiguredBusinessRules(ctx, tx, voucherID, user.ID, itemIDs); err != nil {
		return domain.Voucher{}, err
	}
	if err = c.insertBasicInfoRules(ctx, tx, voucherID); err != nil {
		return domain.Voucher{}, err
	}
	if err = tx.Commit(); err != nil {
		return domain.Voucher{}, err
	}
	return domain.Voucher{ID: voucherID, Code: code}, nil
}

func (c *MySQLVoucherCreator) CreateWinbackVoucher(ctx context.Context, user domain.User, winbackDate time.Time, itemIDs []string) (domain.Voucher, error) {
	if !c.cfg.PricingVoucherID.Valid && strings.TrimSpace(c.cfg.PricingVoucherCode) == "" {
		return domain.Voucher{}, fmt.Errorf("winback pricing voucher id or code is required")
	}
	if strings.TrimSpace(user.ID) == "" {
		return domain.Voucher{}, fmt.Errorf("winback voucher user id is required")
	}

	code := c.winbackVoucherCode(user.ID, winbackDate)
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Voucher{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	existing, found, err := voucherByCode(ctx, tx, code)
	if err != nil {
		return domain.Voucher{}, err
	}
	if found {
		if err = tx.Commit(); err != nil {
			return domain.Voucher{}, err
		}
		existing.Existed = true
		return existing, nil
	}

	pricingVoucherID := c.cfg.PricingVoucherID.Value
	if !c.cfg.PricingVoucherID.Valid {
		err = tx.QueryRowContext(ctx, `
SELECT id
FROM vouchers
WHERE code = ?
LIMIT 1`, c.cfg.PricingVoucherCode).Scan(&pricingVoucherID)
		if err != nil {
			if err == sql.ErrNoRows {
				return domain.Voucher{}, fmt.Errorf("pricing voucher %q not found", c.cfg.PricingVoucherCode)
			}
			return domain.Voucher{}, err
		}
	}

	startAt := winbackDate
	expiredAt := winbackDate.AddDate(0, 0, c.cfg.DurationDays.Value)
	requiresClaim := boolInt(c.cfg.RequiresClaim.Value)
	result, err := tx.ExecContext(ctx, `
INSERT INTO vouchers (
  code,
  name,
  description,
  type,
  amount,
  max_discount,
  min_purchase,
  distribution,
  requires_claim,
  max_claim_total,
  start_at,
  expired_at,
  usage_limit_total,
  usage_limit_per_user,
  is_active,
  claim_animation,
  created_at,
  updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, NOW(), NOW())`,
		code,
		c.voucherName(),
		stringPtrValue(c.cfg.Description),
		c.cfg.Type,
		c.cfg.Amount.Value,
		c.cfg.MaxDiscount.sqlValue(),
		c.cfg.MinPurchase.Value,
		c.cfg.Distribution,
		requiresClaim,
		c.cfg.MaxClaimTotal.sqlValue(),
		startAt,
		expiredAt,
		c.cfg.UsageLimitTotal.sqlValue(),
		c.cfg.UsageLimitPerUser.Value,
		stringPtrValue(c.cfg.ClaimAnimation),
	)
	if err != nil {
		return domain.Voucher{}, err
	}
	voucherID, err := result.LastInsertId()
	if err != nil {
		return domain.Voucher{}, err
	}

	_, err = tx.ExecContext(ctx, `
INSERT INTO voucher_pricing_aliases (
  voucher_id,
  pricing_voucher_id,
  alias_mode,
  created_at,
  updated_at
) VALUES (?, ?, 'pricing_only', NOW(), NOW())`, voucherID, pricingVoucherID)
	if err != nil {
		return domain.Voucher{}, err
	}

	if err = insertRule(ctx, tx, voucherID, "user", []string{user.ID}, "include"); err != nil {
		return domain.Voucher{}, err
	}
	if err = c.insertConfiguredRules(ctx, tx, voucherID, user.ID, itemIDs); err != nil {
		return domain.Voucher{}, err
	}
	if err = c.insertConfiguredBusinessRules(ctx, tx, voucherID, user.ID, itemIDs); err != nil {
		return domain.Voucher{}, err
	}
	if err = c.insertBasicInfoRules(ctx, tx, voucherID); err != nil {
		return domain.Voucher{}, err
	}
	if err = tx.Commit(); err != nil {
		return domain.Voucher{}, err
	}
	return domain.Voucher{ID: voucherID, Code: code}, nil
}

func (c *MySQLVoucherCreator) winbackVoucherCode(userID string, date time.Time) string {
	key := fmt.Sprintf("winback:%s:user:%s", date.Format("2006"), cleanCodePart(userID))
	secret := c.codeSecret
	if secret == "" {
		secret = "preview-only"
	}
	hash := hmac.New(sha256.New, []byte(secret))
	_, _ = hash.Write([]byte(key))
	code := strings.TrimRight(base32.StdEncoding.EncodeToString(hash.Sum(nil)), "=")[:16]
	if !strings.HasPrefix(code, "WB") {
		return "WB" + code[:14]
	}
	return code
}

func (c *MySQLVoucherCreator) wishlistBackInVoucherCode(userID string, date time.Time) string {
	year, week := date.ISOWeek()
	key := fmt.Sprintf("wishlist-back-in:%d-W%02d:user:%s", year, week, cleanCodePart(userID))
	secret := c.codeSecret
	if secret == "" {
		secret = "preview-only"
	}
	hash := hmac.New(sha256.New, []byte(secret))
	_, _ = hash.Write([]byte(key))
	code := strings.TrimRight(base32.StdEncoding.EncodeToString(hash.Sum(nil)), "=")[:13]
	return "WBI" + code
}

func (cfg BirthdayVoucherConfig) withDefaults() BirthdayVoucherConfig {
	if cfg.CodePrefix == "" {
		cfg.CodePrefix = "BIRTHDAY"
	}
	if cfg.CodeTemplate == "" {
		cfg.CodeTemplate = "{prefix}_{user_id}_{date}"
	}
	if cfg.Code != "" && cfg.CodePrefix == "BIRTHDAY" {
		cfg.CodePrefix = cfg.Code
	}
	if cfg.Name == "" {
		cfg.Name = "BIRTHDAY"
	}
	if cfg.Type == "" {
		cfg.Type = "discount_percent"
	}
	if !cfg.Amount.Valid {
		cfg.Amount = flexibleInt{Value: 8, Valid: true}
	}
	if cfg.Distribution == "" {
		cfg.Distribution = "manual"
	}
	if !cfg.RequiresClaim.Valid {
		cfg.RequiresClaim = flexibleBool{Value: true, Valid: true}
	}
	if !cfg.UsageLimitPerUser.Valid {
		cfg.UsageLimitPerUser = flexibleInt{Value: 1, Valid: true}
	}
	if !cfg.DurationDays.Valid {
		cfg.DurationDays = flexibleInt{Value: 14, Valid: true}
	}
	return cfg
}

func (c *MySQLVoucherCreator) voucherCode(userID string, date time.Time) string {
	key := c.idempotencyKey(userID, date)
	secret := c.codeSecret
	if secret == "" {
		secret = "preview-only"
	}
	hash := hmac.New(sha256.New, []byte(secret))
	_, _ = hash.Write([]byte(key))
	return strings.TrimRight(base32.StdEncoding.EncodeToString(hash.Sum(nil)), "=")[:16]
}

func (c *MySQLVoucherCreator) anniversaryVoucherCode(userID string, date time.Time) string {
	key := c.anniversaryIdempotencyKey(userID, date)
	secret := c.codeSecret
	if secret == "" {
		secret = "preview-only"
	}
	hash := hmac.New(sha256.New, []byte(secret))
	_, _ = hash.Write([]byte(key))
	code := strings.TrimRight(base32.StdEncoding.EncodeToString(hash.Sum(nil)), "=")[:16]
	// Prefix ANV if not present
	if !strings.HasPrefix(code, "ANV") {
		return "ANV" + code[:13]
	}
	return code
}

func (c *MySQLVoucherCreator) idempotencyKey(userID string, date time.Time) string {
	return fmt.Sprintf("birthday:%s:user:%s", date.Format("2006"), cleanCodePart(userID))
}

func (c *MySQLVoucherCreator) anniversaryIdempotencyKey(userID string, date time.Time) string {
	return fmt.Sprintf("anniversary:%s:user:%s", date.Format("2006"), cleanCodePart(userID))
}

func (c *MySQLVoucherCreator) voucherName() string {
	return c.cfg.Name
}

func (c *MySQLVoucherCreator) anniversaryVoucherName(user domain.User) string {
	name := strings.TrimSpace(c.cfg.Name)
	if name == "" {
		name = "ANNIVERSARY"
	}
	userName := strings.TrimSpace(user.Name)
	if userName == "" {
		return name
	}
	return name + " " + userName
}

func voucherByCode(ctx context.Context, tx *sql.Tx, code string) (domain.Voucher, bool, error) {
	var voucher domain.Voucher
	err := tx.QueryRowContext(ctx, `
SELECT id, code, created_at
FROM vouchers
WHERE code = ?
LIMIT 1`, code).Scan(&voucher.ID, &voucher.Code, &voucher.CreatedAt)
	if err == sql.ErrNoRows {
		return domain.Voucher{}, false, nil
	}
	if err != nil {
		return domain.Voucher{}, false, err
	}
	return voucher, true, nil
}

func insertRule(ctx context.Context, tx *sql.Tx, voucherID int64, ruleType string, values []string, operator string) error {
	value := ruleValue(values)
	_, err := tx.ExecContext(ctx, `
INSERT INTO voucher_rules (
  voucher_id,
  rule_type,
  rule_value,
  rule_operator,
  created_at,
  updated_at
) VALUES (?, ?, ?, ?, NOW(), NOW())`, voucherID, ruleType, value, operator)
	return err
}

func (c *MySQLVoucherCreator) insertBasicInfoRules(ctx context.Context, tx *sql.Tx, voucherID int64) error {
	if c.cfg.BasicInfo == nil || len(c.cfg.BasicInfo.ItemTypes) == 0 {
		return nil
	}
	val, err := json.Marshal(c.cfg.BasicInfo.ItemTypes)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO voucher_rules (voucher_id, rule_type, rule_value, rule_operator, created_at, updated_at)
VALUES (?, 'item_type', ?, 'include', NOW(), NOW())`, voucherID, string(val))
	return err
}

func (c *MySQLVoucherCreator) insertConfiguredRules(ctx context.Context, tx *sql.Tx, voucherID int64, userID string, itemIDs []string) error {
	return insertConfiguredRuleSet(ctx, tx, voucherID, c.cfg.Rules, userID, itemIDs)
}

func (c *MySQLVoucherCreator) insertConfiguredBusinessRules(ctx context.Context, tx *sql.Tx, voucherID int64, userID string, itemIDs []string) error {
	return insertConfiguredRuleSet(ctx, tx, voucherID, c.cfg.BusinessRules, userID, itemIDs)
}

func insertConfiguredRuleSet(ctx context.Context, tx *sql.Tx, voucherID int64, rules []VoucherRuleConfig, userID string, itemIDs []string) error {
	for _, rule := range rules {
		ruleType := strings.TrimSpace(rule.Type)
		if ruleType == "" || ruleType == "user" {
			continue
		}
		operator := strings.TrimSpace(rule.Operator)
		if operator == "" {
			operator = "include"
		}
		values := configuredRuleValues(rule.Value, userID, itemIDs)
		if len(values) == 0 {
			continue
		}
		if err := insertRule(ctx, tx, voucherID, ruleType, values, operator); err != nil {
			return err
		}
	}
	return nil
}

func configuredRuleValues(value string, userID string, itemIDs []string) []string {
	value = strings.TrimSpace(value)
	switch value {
	case "", "{{item_ids}}":
		return append([]string(nil), itemIDs...)
	case "{{user_id}}":
		return []string{userID}
	}
	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			values = append(values, item)
		}
	}
	return values
}

func ruleValues(values []string) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			result = append(result, parsed)
			continue
		}
		result = append(result, value)
	}
	return result
}

func ruleValue(values []string) string {
	if len(values) == 1 {
		return values[0]
	}
	payload, err := json.Marshal(ruleValues(values))
	if err != nil {
		return ""
	}
	return string(payload)
}

func cleanCodePart(value string) string {
	var b strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToUpper(r))
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func dayStart(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func stringPtrValue(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

type flexibleInt struct {
	Value int
	Valid bool
}

func (v *flexibleInt) UnmarshalJSON(payload []byte) error {
	if string(payload) == "null" {
		return nil
	}
	var number int
	if err := json.Unmarshal(payload, &number); err == nil {
		v.Value = number
		v.Valid = true
		return nil
	}
	var text string
	if err := json.Unmarshal(payload, &text); err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	parsed, err := strconv.Atoi(text)
	if err != nil {
		return err
	}
	v.Value = parsed
	v.Valid = true
	return nil
}

type flexibleNullInt struct {
	Value int
	Valid bool
}

func (v *flexibleNullInt) UnmarshalJSON(payload []byte) error {
	var parsed flexibleInt
	if err := parsed.UnmarshalJSON(payload); err != nil {
		return err
	}
	v.Value = parsed.Value
	v.Valid = parsed.Valid
	return nil
}

func (v flexibleNullInt) sqlValue() any {
	if !v.Valid {
		return nil
	}
	return v.Value
}

type flexibleNullInt64 struct {
	Value int64
	Valid bool
}

func (v *flexibleNullInt64) UnmarshalJSON(payload []byte) error {
	if string(payload) == "null" {
		return nil
	}
	var number int64
	if err := json.Unmarshal(payload, &number); err == nil {
		v.Value = number
		v.Valid = true
		return nil
	}
	var text string
	if err := json.Unmarshal(payload, &text); err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	parsed, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return err
	}
	v.Value = parsed
	v.Valid = true
	return nil
}

type flexibleBool struct {
	Value bool
	Valid bool
}

func (v *flexibleBool) UnmarshalJSON(payload []byte) error {
	if string(payload) == "null" {
		return nil
	}
	var boolean bool
	if err := json.Unmarshal(payload, &boolean); err == nil {
		v.Value = boolean
		v.Valid = true
		return nil
	}
	var number int
	if err := json.Unmarshal(payload, &number); err == nil {
		v.Value = number != 0
		v.Valid = true
		return nil
	}
	var text string
	if err := json.Unmarshal(payload, &text); err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	v.Value = text == "1" || strings.EqualFold(text, "true") || strings.EqualFold(text, "yes")
	v.Valid = true
	return nil
}
