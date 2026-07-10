package reader

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/kyou-id/yukari/internal/domain"
)

func TestWishlistBackInRunsOnlyOnFriday(t *testing.T) {
	store := &fakeWishlistBackInStore{}
	count, err := NewWishlistBackIn(store, &fakeWishlistBackInQueue{}, nil, nil, "queue", "").Run(context.Background(), time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 || store.userItemsCalled {
		t.Fatalf("expected no work outside Friday, count=%d called=%t", count, store.userItemsCalled)
	}
}

func TestWishlistBackInBuildsOneJobPerUserWithTheirItems(t *testing.T) {
	now := time.Date(2026, 6, 19, 9, 0, 0, 0, time.UTC)
	userA := domain.User{ID: "1", Name: "A", Email: "a@example.test", IsActive: true}
	userB := domain.User{ID: "2", Name: "B", Email: "b@example.test", IsActive: true}
	store := &fakeWishlistBackInStore{
		// Rows arrive grouped by user, newest restock first (as the SQL orders).
		rows: []domain.WishlistBackInUserItem{
			{User: userA, Item: domain.WishlistBackInItem{ID: "101", Name: "Newest", RestockedAt: now.Add(-time.Hour), GPRatio: gp(50)}},
			{User: userA, Item: domain.WishlistBackInItem{ID: "102", Name: "Older", RestockedAt: now.Add(-48 * time.Hour), GPRatio: gp(40)}},
			{User: userB, Item: domain.WishlistBackInItem{ID: "103", Name: "Solo", RestockedAt: now.Add(-2 * time.Hour), GPRatio: gp(30)}},
		},
		companion: domain.WishlistBackInItem{ID: "202", Name: "Purchased Pair"},
		recos:     sixRecos(),
	}
	queue := &fakeWishlistBackInQueue{}
	vouchers := &fakeWishlistBackInVoucherCreator{}

	count, err := NewWishlistBackIn(store, queue, vouchers, nil, "wishlist_back_in_email_jobs", "https://kyou.id/user/my-voucher").Run(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 || len(queue.jobs) != 2 {
		t.Fatalf("expected two jobs, count=%d jobs=%d", count, len(queue.jobs))
	}
	// Jobs come back in whatever order the worker pool finished, so index by user.
	byUser := map[string]domain.WishlistBackInJob{}
	for _, job := range queue.jobs {
		byUser[job.UserID] = job
	}

	first, ok := byUser["1"]
	if !ok {
		t.Fatal("expected a job for userA")
	}
	if len(first.Items) != 2 || first.Items[0].ID != "101" || first.Items[1].ID != "102" {
		t.Fatalf("unexpected userA job items: %#v", first)
	}
	if first.CompanionItem.ID != "202" {
		t.Fatalf("expected companion on hero item, got %#v", first.CompanionItem)
	}
	if len(first.RecoItems) != 6 {
		t.Fatalf("expected 6 reco items when a full set is available, got %d", len(first.RecoItems))
	}
	// userA's items are GP 50 and 40 -> lowest clears 35 -> 8%. userB's lone item
	// is GP 30 -> 6%. The percent on the job must match the voucher that was minted.
	if first.VoucherDiscountPercent != 8 || first.VoucherCode != "WBI8-1" {
		t.Fatalf("expected 8%% tier for userA, got %d%% code=%s", first.VoucherDiscountPercent, first.VoucherCode)
	}
	second, ok := byUser["2"]
	if !ok {
		t.Fatal("expected a job for userB")
	}
	if len(second.Items) != 1 || second.VoucherDiscountPercent != 6 || second.VoucherCode != "WBI6-2" {
		t.Fatalf("expected userB's lone item at 6%%, got %#v", second)
	}
	ids := append([]string(nil), store.companionUserIDs...)
	sort.Strings(ids)
	if len(ids) != 2 || ids[0] != "1" || ids[1] != "2" {
		t.Fatalf("companion should key off each user (not the wishlist item), got %v", store.companionUserIDs)
	}
}

func TestWishlistBackInCapsItemsAtTen(t *testing.T) {
	now := time.Date(2026, 6, 19, 9, 0, 0, 0, time.UTC)
	user := domain.User{ID: "9", Name: "Packrat", Email: "p@example.test", IsActive: true}
	var rows []domain.WishlistBackInUserItem
	// More than the cap: the tail is dropped, not carried over to a later Friday.
	for i := 0; i < wishlistBackInMaxItems+3; i++ {
		rows = append(rows, domain.WishlistBackInUserItem{User: user, Item: domain.WishlistBackInItem{ID: string(rune('a' + i))}})
	}
	store := &fakeWishlistBackInStore{rows: rows}
	queue := &fakeWishlistBackInQueue{}

	count, err := NewWishlistBackIn(store, queue, nil, nil, "q", "").Run(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || len(queue.jobs[0].Items) != wishlistBackInMaxItems {
		t.Fatalf("expected one job capped at %d items, got count=%d items=%d", wishlistBackInMaxItems, count, len(queue.jobs[0].Items))
	}
}

func TestWishlistBackInHidesRecoSectionWhenFewerThanSix(t *testing.T) {
	now := time.Date(2026, 6, 19, 9, 0, 0, 0, time.UTC)
	user := domain.User{ID: "5", Name: "Solo", Email: "s@example.test", IsActive: true}
	store := &fakeWishlistBackInStore{
		rows:      []domain.WishlistBackInUserItem{{User: user, Item: domain.WishlistBackInItem{ID: "101"}}},
		companion: domain.WishlistBackInItem{ID: "202", Name: "Purchased Pair"},
		recos:     sixRecos()[:5], // only 5 available -> section must be hidden
	}
	queue := &fakeWishlistBackInQueue{}

	if _, err := NewWishlistBackIn(store, queue, nil, nil, "q", "").Run(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	job := queue.jobs[0]
	if job.CompanionItem.ID != "" || len(job.RecoItems) != 0 {
		t.Fatalf("expected reco section hidden with <6 recos, got companion=%q recos=%d", job.CompanionItem.ID, len(job.RecoItems))
	}
}

type fakeWishlistBackInStore struct {
	rows             []domain.WishlistBackInUserItem
	companion        domain.WishlistBackInItem
	recos            []domain.WishlistBackInItem
	userItemsCalled  bool
	companionUserIDs []string
	gotStartAt       time.Time
	gotEndAt         time.Time
	companionErr     error
}

func (s *fakeWishlistBackInStore) WishlistBackInUserItems(_ context.Context, startAt, endAt time.Time) ([]domain.WishlistBackInUserItem, error) {
	s.userItemsCalled = true
	s.gotStartAt = startAt
	s.gotEndAt = endAt
	return s.rows, nil
}

func (s *fakeWishlistBackInStore) WishlistBackInCompanion(_ context.Context, userID string) (domain.WishlistBackInItem, error) {
	s.companionUserIDs = append(s.companionUserIDs, userID)
	if s.companionErr != nil {
		return domain.WishlistBackInItem{}, s.companionErr
	}
	return s.companion, nil
}

func (s *fakeWishlistBackInStore) WishlistBackInPopularityScores(context.Context) (map[string]int64, error) {
	return map[string]int64{}, nil
}

func (s *fakeWishlistBackInStore) WishlistBackInRecommendations(context.Context, string, string, map[string]int64) ([]domain.WishlistBackInItem, error) {
	return s.recos, nil
}

func sixRecos() []domain.WishlistBackInItem {
	var out []domain.WishlistBackInItem
	for i := 0; i < 6; i++ {
		out = append(out, domain.WishlistBackInItem{ID: "reco" + string(rune('1'+i))})
	}
	return out
}

type fakeWishlistBackInQueue struct{ jobs []domain.WishlistBackInJob }

func (q *fakeWishlistBackInQueue) EnqueueWishlistBackInTo(_ context.Context, _ string, job domain.WishlistBackInJob) error {
	q.jobs = append(q.jobs, job)
	return nil
}

func gp(percent float64) *float64 { return &percent }

// items builds a candidate list carrying only the field the tier rule reads.
func gpItems(ratios ...*float64) []domain.WishlistBackInItem {
	items := make([]domain.WishlistBackInItem, len(ratios))
	for i, ratio := range ratios {
		items[i] = domain.WishlistBackInItem{ID: fmt.Sprint(i), GPRatio: ratio}
	}
	return items
}

func TestWishlistBackInTier(t *testing.T) {
	for _, tc := range []struct {
		name  string
		items []domain.WishlistBackInItem
		want  int
	}{
		{"all above the high floor", gpItems(gp(50), gp(40), gp(35)), 8},
		{"exactly at the high floor", gpItems(gp(35)), 8},
		{"a hair under the high floor drops the whole email", gpItems(gp(90), gp(34.9)), 6},
		{"straddling both tiers bills at the lower one", gpItems(gp(40), gp(30)), 6},
		{"exactly at the low floor", gpItems(gp(25)), 6},
		{"below the low floor earns nothing", gpItems(gp(24.9)), 0},
		// Sub-floor items are restock news, not discountable stock: they must not
		// drag the tier down, because the voucher's gp_ratio_min already excludes
		// them at checkout.
		{"sub-floor items are ignored, not tier-setting", gpItems(gp(50), gp(10)), 8},
		{"sub-floor items alongside a low-tier item", gpItems(gp(30), gp(10)), 6},
		// hanayo refuses to apply any gp_ratio rule when cogs is unknown, so an
		// unknown-GP item can neither earn nor lower a tier.
		{"unknown GP is ignored, not tier-setting", gpItems(gp(50), nil), 8},
		{"all unknown GP means no voucher", gpItems(nil, nil), 0},
		{"no items", nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := WishlistBackInTier(tc.items); got != tc.want {
				t.Fatalf("WishlistBackInTier() = %d, want %d", got, tc.want)
			}
		})
	}
}

// A user whose items all sit below the 25% GP floor still gets the restock email,
// just without a coupon block — and no voucher row is burned on them.
func TestWishlistBackInEnqueuesWithoutVoucherBelowGPFloor(t *testing.T) {
	now := time.Date(2026, 6, 19, 9, 0, 0, 0, time.UTC)
	user := domain.User{ID: "7", Name: "Thin Margin", Email: "t@example.test", IsActive: true}
	store := &fakeWishlistBackInStore{
		rows: []domain.WishlistBackInUserItem{
			{User: user, Item: domain.WishlistBackInItem{ID: "301", RestockedAt: now.Add(-time.Hour), GPRatio: gp(20)}},
		},
	}
	queue := &fakeWishlistBackInQueue{}
	vouchers := &fakeWishlistBackInVoucherCreator{}

	count, err := NewWishlistBackIn(store, queue, vouchers, nil, "wishlist_back_in_email_jobs", "https://kyou.id/user/my-voucher").Run(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || len(queue.jobs) != 1 {
		t.Fatalf("expected the email to still go out, count=%d jobs=%d", count, len(queue.jobs))
	}
	if len(vouchers.tiers) != 0 {
		t.Fatalf("no voucher should have been minted, got tiers %v", vouchers.tiers)
	}
	job := queue.jobs[0]
	if job.VoucherCode != "" || job.VoucherID != 0 || job.VoucherDiscountPercent != 0 {
		t.Fatalf("expected a voucher-less job, got %#v", job)
	}
}

type fakeWishlistBackInVoucherCreator struct {
	tiers []int
}

func (f *fakeWishlistBackInVoucherCreator) CreateWishlistBackInVoucher(_ context.Context, user domain.User, _ time.Time, _ []string, discountPercent int) (domain.Voucher, error) {
	f.tiers = append(f.tiers, discountPercent)
	return domain.Voucher{ID: 1, Code: fmt.Sprintf("WBI%d-%s", discountPercent, user.ID)}, nil
}

func TestWishlistBackInWindowDefaultsToOneWeek(t *testing.T) {
	store := &fakeWishlistBackInStore{}
	friday := time.Date(2026, 7, 10, 16, 0, 0, 0, time.UTC)

	if _, err := NewWishlistBackIn(store, &fakeWishlistBackInQueue{}, nil, nil, "queue", "").Run(context.Background(), friday); err != nil {
		t.Fatal(err)
	}

	wantEnd := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	if !store.gotEndAt.Equal(wantEnd) {
		t.Fatalf("window ends at %s, want midnight %s", store.gotEndAt, wantEnd)
	}
	// One week exactly: the Friday cron tiles it, so nothing is announced twice
	// and nothing older than seven days is announced as news.
	if got := wantEnd.Sub(store.gotStartAt); got != 7*24*time.Hour {
		t.Fatalf("default window is %s, want 7 days", got)
	}
}

func TestWishlistBackInWindowOverrideWidensTheSweep(t *testing.T) {
	store := &fakeWishlistBackInStore{}
	friday := time.Date(2026, 7, 10, 16, 0, 0, 0, time.UTC)

	r := NewWishlistBackIn(store, &fakeWishlistBackInQueue{}, nil, nil, "queue", "")
	r.Window = 21 * 24 * time.Hour
	if _, err := r.Run(context.Background(), friday); err != nil {
		t.Fatal(err)
	}

	if got := store.gotEndAt.Sub(store.gotStartAt); got != 21*24*time.Hour {
		t.Fatalf("window is %s, want the 21-day override", got)
	}
	if want := time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC); !store.gotStartAt.Equal(want) {
		t.Fatalf("window starts %s, want %s", store.gotStartAt, want)
	}
}

func TestWishlistBackInStillSendsWhenCompanionLookupFails(t *testing.T) {
	now := time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)
	userA := domain.User{ID: "1", Name: "A", Email: "a@example.test", IsActive: true}
	userB := domain.User{ID: "2", Name: "B", Email: "b@example.test", IsActive: true}
	store := &fakeWishlistBackInStore{
		rows: []domain.WishlistBackInUserItem{
			{User: userA, Item: domain.WishlistBackInItem{ID: "101", Name: "One"}},
			{User: userB, Item: domain.WishlistBackInItem{ID: "102", Name: "Two"}},
		},
		// A single user's bad row (e.g. a NULL order_items.item_price) must not
		// abort the campaign for everyone queued behind them.
		companionErr: errors.New("sql: Scan error on column index 4, name \"price\""),
	}
	queue := &fakeWishlistBackInQueue{}

	count, err := NewWishlistBackIn(store, queue, nil, nil, "q", "").Run(context.Background(), now)
	if err != nil {
		t.Fatalf("companion failure must not fail the run: %v", err)
	}
	if count != 2 || len(queue.jobs) != 2 {
		t.Fatalf("expected both users enqueued, count=%d jobs=%d", count, len(queue.jobs))
	}
	for _, job := range queue.jobs {
		if job.CompanionItem.ID != "" || len(job.RecoItems) != 0 {
			t.Fatalf("expected cross-sell dropped, got companion=%q recos=%d", job.CompanionItem.ID, len(job.RecoItems))
		}
	}
}

