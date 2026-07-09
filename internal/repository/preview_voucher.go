package repository

import (
	"context"
	"database/sql"
)

// LiveVoucherCode returns the code of a live, unused voucher scoped to userID
// whose code matches codeLike (and, when codeNotLike is non-empty, does not match
// it). It reports found=false when the user has none.
//
// Read-only: the preview commands use it so a rendered email shows the code the
// user would really receive, without minting anything in production.
//
// codeNotLike exists because the campaign prefixes overlap. A winback code is
// "WB" + base32, so `code LIKE 'WB%'` also matches every wishlist-back-in code
// ("WBI8-"/"WBI6-" + base32) — and the legacy "WBI" + base32 ones. Callers
// looking for winback must exclude 'WBI%' explicitly.
func LiveVoucherCode(ctx context.Context, dsn, userID, codeLike, codeNotLike string) (code string, amount int, found bool, err error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return "", 0, false, err
	}
	defer db.Close()

	query := `
SELECT v.code, v.amount
FROM vouchers v
JOIN voucher_rules ur ON ur.voucher_id = v.id AND ur.rule_type = 'user' AND ur.rule_value = ?
WHERE v.code LIKE ?
  AND v.is_active = 1
  AND (v.expired_at IS NULL OR v.expired_at > NOW())
  AND NOT EXISTS (
    SELECT 1 FROM voucher_claims vc
    WHERE vc.voucher_id = v.id AND vc.used_at IS NOT NULL
  )`
	args := []any{userID, codeLike}
	if codeNotLike != "" {
		query += "\n  AND v.code NOT LIKE ?"
		args = append(args, codeNotLike)
	}
	query += "\nORDER BY v.expired_at DESC\nLIMIT 1"

	err = db.QueryRowContext(ctx, query, args...).Scan(&code, &amount)
	if err == sql.ErrNoRows {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, err
	}
	return code, amount, true, nil
}
