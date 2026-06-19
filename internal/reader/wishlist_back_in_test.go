package reader

import (
	"context"
	"testing"
	"time"

	"github.com/kyou-id/yukari/internal/domain"
)

func TestWishlistBackInRunsOnlyOnFriday(t *testing.T) {
	store := &fakeWishlistBackInStore{}
	count, err := NewWishlistBackIn(store, &fakeWishlistBackInQueue{}, nil, nil, "queue", "", time.Time{}).Run(context.Background(), time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 || store.itemsCalled {
		t.Fatalf("expected no work outside Friday, count=%d called=%t", count, store.itemsCalled)
	}
}

func TestWishlistBackInBuildsOneJobPerWishlistUser(t *testing.T) {
	now := time.Date(2026, 6, 19, 9, 0, 0, 0, time.UTC)
	item := domain.WishlistBackInItem{ID: "101", Name: "Restocked Figure", PopularScore: 8, RestockedAt: now.Add(-time.Hour)}
	store := &fakeWishlistBackInStore{
		items: []domain.WishlistBackInItem{item},
		users: []domain.User{
			{ID: "1", Name: "A", Email: "a@example.test", IsActive: true},
			{ID: "2", Name: "B", Email: "b@example.test", IsActive: true},
		},
		companion: domain.WishlistBackInItem{ID: "202", Name: "Purchased Pair"},
	}
	queue := &fakeWishlistBackInQueue{}
	vouchers := &fakeWishlistBackInVoucherCreator{}

	count, err := NewWishlistBackIn(store, queue, vouchers, nil, "wishlist_back_in_email_jobs", "https://kyou.id/user/my-voucher", now.AddDate(0, 0, -7)).Run(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 || len(queue.jobs) != 2 {
		t.Fatalf("expected two jobs, count=%d jobs=%d", count, len(queue.jobs))
	}
	if queue.jobs[0].Item.ID != "101" || queue.jobs[0].CompanionItem.ID != "202" {
		t.Fatalf("unexpected personalization: %#v", queue.jobs[0])
	}
	if queue.jobs[0].VoucherCode == "" {
		t.Fatal("expected voucher in job")
	}
}

type fakeWishlistBackInStore struct {
	items       []domain.WishlistBackInItem
	users       []domain.User
	companion   domain.WishlistBackInItem
	itemsCalled bool
}

func (s *fakeWishlistBackInStore) WishlistBackInItems(context.Context, time.Time, time.Time) ([]domain.WishlistBackInItem, error) {
	s.itemsCalled = true
	return s.items, nil
}

func (s *fakeWishlistBackInStore) WishlistBackInUsers(context.Context, string) ([]domain.User, error) {
	return s.users, nil
}

func (s *fakeWishlistBackInStore) WishlistBackInCompanion(context.Context, string, string) (domain.WishlistBackInItem, error) {
	return s.companion, nil
}

type fakeWishlistBackInQueue struct{ jobs []domain.WishlistBackInJob }

func (q *fakeWishlistBackInQueue) EnqueueWishlistBackInTo(_ context.Context, _ string, job domain.WishlistBackInJob) error {
	q.jobs = append(q.jobs, job)
	return nil
}

type fakeWishlistBackInVoucherCreator struct{}

func (fakeWishlistBackInVoucherCreator) CreateWishlistBackInVoucher(_ context.Context, user domain.User, _ time.Time, _ []string) (domain.Voucher, error) {
	return domain.Voucher{ID: 1, Code: "WBI-" + user.ID}, nil
}
