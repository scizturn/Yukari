package repository

import "testing"

// Both wishlist-back-in tiers must keep declaring a 14-day life, because Makoto's
// coupon block prints "Berlaku 14 Hari" as a literal (templates/wishlist_back_in/
// wishlist_back_in*.html). Change duration_days and that copy silently lies.
//
// The tiers' amounts are likewise mirrored in prod's two head vouchers (10352 =
// 8%, 10747 = 6%), which supply the struck-through /search price. hanayo takes
// the checkout discount from the child's amount and never reads the head, so the
// two must agree or /search quotes a discount checkout does not honour.
func TestWishlistBackInVoucherConfigs(t *testing.T) {
	for _, tc := range []struct {
		path             string
		wantAmount       int
		wantPricingID    int64
		wantDurationDays int
	}{
		{"../../data/vouchers/wishlist_back_in.json", 8, 10352, 14},
		{"../../data/vouchers/wishlist_back_in_low.json", 6, 10747, 14},
	} {
		t.Run(tc.path, func(t *testing.T) {
			cfg, err := LoadBirthdayVoucherConfig(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Amount.Value != tc.wantAmount {
				t.Errorf("amount = %d, want %d", cfg.Amount.Value, tc.wantAmount)
			}
			if cfg.DurationDays.Value != tc.wantDurationDays {
				t.Errorf("duration_days = %d, want %d (Makoto's coupon prints this)", cfg.DurationDays.Value, tc.wantDurationDays)
			}
			if !cfg.PricingVoucherID.Valid || cfg.PricingVoucherID.Value != tc.wantPricingID {
				t.Errorf("pricing_voucher_id = %v, want %d", cfg.PricingVoucherID.Value, tc.wantPricingID)
			}
		})
	}
}
