package repository

import (
	"strings"
	"testing"
	"time"
)

func TestVoucherCodeIsDeterministicHMAC(t *testing.T) {
	creator := MySQLVoucherCreator{
		cfg:        BirthdayVoucherConfig{}.withDefaults(),
		codeSecret: "test-secret",
	}
	date := time.Date(2026, 5, 27, 12, 51, 30, 0, time.UTC)

	first := creator.voucherCode("147044", date)
	second := creator.voucherCode("147044", date)

	if first != second {
		t.Fatalf("expected deterministic voucher code, got %q and %q", first, second)
	}
	if len(first) != 16 {
		t.Fatalf("expected 16 character code, got %q", first)
	}
	if strings.Contains(first, "147044") || strings.Contains(first, "20260527") {
		t.Fatalf("expected random-looking code without user/date leakage, got %q", first)
	}
}

func TestVoucherCodeChangesByUserYearOrSecret(t *testing.T) {
	date := time.Date(2026, 5, 27, 12, 51, 30, 0, time.UTC)
	creator := MySQLVoucherCreator{
		cfg:        BirthdayVoucherConfig{}.withDefaults(),
		codeSecret: "test-secret",
	}

	base := creator.voucherCode("147044", date)
	otherUser := creator.voucherCode("147045", date)
	otherBirthdayDateSameYear := creator.voucherCode("147044", date.AddDate(0, 0, 1))
	otherYear := creator.voucherCode("147044", date.AddDate(1, 0, 0))
	otherSecretCreator := MySQLVoucherCreator{
		cfg:        BirthdayVoucherConfig{}.withDefaults(),
		codeSecret: "other-secret",
	}
	otherSecret := otherSecretCreator.voucherCode("147044", date)

	if base != otherBirthdayDateSameYear {
		t.Fatalf("expected same-year birthday date change to reuse code, got base=%q next_day=%q", base, otherBirthdayDateSameYear)
	}
	if base == otherUser || base == otherYear || base == otherSecret {
		t.Fatalf("expected code to change by user/year/secret, got base=%q user=%q year=%q secret=%q", base, otherUser, otherYear, otherSecret)
	}
}

func TestEmptyConfiguredRulesMeansNoItemRestriction(t *testing.T) {
	cfg := BirthdayVoucherConfig{Rules: nil}.withDefaults()

	if len(cfg.Rules) != 0 {
		t.Fatalf("expected empty configured rules")
	}

	values := configuredRuleValues("", "147044", []string{"185135"})
	if len(values) != 1 || values[0] != "185135" {
		t.Fatalf("expected placeholder helper to still resolve item ids, got %#v", values)
	}
}

func TestRuleValueStoresSingleValueAsScalar(t *testing.T) {
	if got := ruleValue([]string{"90"}); got != "90" {
		t.Fatalf("expected scalar rule value, got %q", got)
	}
}

func TestRuleValueStoresMultipleValuesAsJSONArray(t *testing.T) {
	if got := ruleValue([]string{"147044", "147045"}); got != "[147044,147045]" {
		t.Fatalf("expected JSON array rule value, got %q", got)
	}
}
