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
	basicInfoJSON, err := json.Marshal(c.cfg.BasicInfo)
	if err != nil {
		return domain.Voucher{}, err
	}
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
  basic_info,
  created_at,
  updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, NOW(), NOW())`,
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
		string(basicInfoJSON),
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
	basicInfoJSON, err := json.Marshal(c.cfg.BasicInfo)
	if err != nil {
		return domain.Voucher{}, err
	}
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
  basic_info,
  created_at,
  updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, NOW(), NOW())`,
		code,
		"ANNIVERSARY",
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
		string(basicInfoJSON),
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
