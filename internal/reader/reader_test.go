package reader

import (
	"context"
	"testing"
	"time"

	"github.com/kyou-id/yukari/internal/domain"
)

func TestReaderBuildsFullBirthdayJob(t *testing.T) {
	now := time.Date(2026, 5, 21, 7, 0, 0, 0, time.UTC)
	store := &fakeStore{
		users: []domain.User{{
			ID:       "123",
			Name:     "Garvin",
			Email:    "garvin@example.test",
			Birthday: now,
			IsActive: true,
		}},
		wishlist: []domain.WishlistItem{{ID: "wish-1", Name: "Figure"}},
		fyp:      []domain.FYPItem{{ID: "fyp-1", Name: "Chara", Kind: "character"}},
		popular:  []domain.FYPItem{{ID: "popular-1", Name: "Popular", Kind: "series"}},
	}
	queue := &fakeQueue{}

	count, err := New(store, queue).Run(context.Background(), now)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one job, got %d", count)
	}
	job := queue.jobs[0]
	if job.ID != "birthday-2026-05-21-user-123" || job.User.Email != "garvin@example.test" {
		t.Fatalf("unexpected job: %#v", job)
	}
	if len(job.WishlistItems) != 1 || len(job.FYPItems) != 1 || len(job.PopularItems) != 1 {
		t.Fatalf("expected full personalization payload, got %#v", job)
	}
}

func TestReaderCreatesVoucherBeforeEnqueue(t *testing.T) {
	now := time.Date(2026, 5, 21, 7, 0, 0, 0, time.UTC)
	store := &fakeStore{
		users: []domain.User{{
			ID:       "123",
			Name:     "Garvin",
			Email:    "garvin@example.test",
			Birthday: now,
			IsActive: true,
		}},
		wishlist: []domain.WishlistItem{{ID: "1001", Name: "Figure"}},
		fyp:      []domain.FYPItem{{ID: "1002", Name: "Chara"}},
		popular:  []domain.FYPItem{{ID: "popular-1", Name: "Popular"}},
	}
	queue := &fakeQueue{}
	vouchers := &fakeVoucherCreator{
		voucher: domain.Voucher{ID: 54, Code: "BIRTHDAY_123_20260521"},
	}

	count, err := NewWithVoucherCreator(store, queue, vouchers).Run(context.Background(), now)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one job, got %d", count)
	}
	if vouchers.userID != "123" {
		t.Fatalf("expected voucher for user 123, got %q", vouchers.userID)
	}
	if len(vouchers.itemIDs) != 2 || vouchers.itemIDs[0] != "1001" || vouchers.itemIDs[1] != "1002" {
		t.Fatalf("expected wishlist and fyp item ids, got %#v", vouchers.itemIDs)
	}
	if queue.jobs[0].VoucherCode != "BIRTHDAY_123_20260521" || queue.jobs[0].VoucherID != 54 {
		t.Fatalf("expected voucher fields in job, got %#v", queue.jobs[0])
	}
}

func TestReaderSkipsExistingYearlyVoucher(t *testing.T) {
	now := time.Date(2026, 5, 21, 7, 0, 0, 0, time.UTC)
	store := &fakeStore{
		users: []domain.User{{
			ID:       "123",
			Name:     "Garvin",
			Email:    "garvin@example.test",
			Birthday: now,
			IsActive: true,
		}},
	}
	queue := &fakeQueue{}
	vouchers := &fakeVoucherCreator{
		voucher: domain.Voucher{ID: 54, Code: "RANDOMCODE123456", Existed: true},
	}

	count, err := NewWithVoucherCreator(store, queue, vouchers).Run(context.Background(), now)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no enqueued jobs, got %d", count)
	}
	if len(queue.jobs) != 0 {
		t.Fatalf("expected queue to stay empty, got %#v", queue.jobs)
	}
}

type fakeStore struct {
	users     []domain.User
	wishlist  []domain.WishlistItem
	fyp       []domain.FYPItem
	popular   []domain.FYPItem
	converted bool
}

type fakeVoucherCreator struct {
	voucher domain.Voucher
	userID  string
	itemIDs []string
}

func (c *fakeVoucherCreator) CreateBirthdayVoucher(_ context.Context, user domain.User, _ time.Time, itemIDs []string) (domain.Voucher, error) {
	c.userID = user.ID
	c.itemIDs = append([]string(nil), itemIDs...)
	return c.voucher, nil
}

func (s *fakeStore) BirthdayUsers(context.Context, string) ([]domain.User, error) {
	return s.users, nil
}

func (s *fakeStore) Wishlist(context.Context, string) ([]domain.WishlistItem, error) {
	return s.wishlist, nil
}

func (s *fakeStore) FYP(context.Context, string) ([]domain.FYPItem, error) {
	return s.fyp, nil
}

func (s *fakeStore) Popular(context.Context) ([]domain.FYPItem, error) {
	return s.popular, nil
}

func (s *fakeStore) HasConverted(context.Context, string, time.Time, time.Time) (bool, error) {
	return s.converted, nil
}

type fakeQueue struct {
	jobs []domain.BirthdayJob
}

func (q *fakeQueue) Enqueue(_ context.Context, job domain.BirthdayJob) error {
	q.jobs = append(q.jobs, job)
	return nil
}
