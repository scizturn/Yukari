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

type fakeStore struct {
	users     []domain.User
	wishlist  []domain.WishlistItem
	fyp       []domain.FYPItem
	popular   []domain.FYPItem
	converted bool
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
