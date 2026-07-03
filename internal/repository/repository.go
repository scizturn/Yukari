package repository

import (
	"context"
	"time"

	"github.com/kyou-id/yukari/internal/domain"
)

type StubStore struct {
	users []domain.User
}

func NewStubStore(now time.Time) *StubStore {
	return &StubStore{
		users: []domain.User{{
			ID:        "1",
			Name:      "Ruby",
			Email:     "ruby@example.test",
			Birthday:  time.Date(1999, now.Month(), now.Day(), 0, 0, 0, 0, now.Location()),
			CreatedAt: time.Date(2019, 8, 15, 0, 0, 0, 0, now.Location()),
			IsActive:  true,
		}},
	}
}

func (s *StubStore) BirthdayUsers(_ context.Context, monthDay string) ([]domain.User, error) {
	users := make([]domain.User, 0, len(s.users))
	for _, user := range s.users {
		if user.Birthday.Format("01-02") == monthDay {
			users = append(users, user)
		}
	}
	return users, nil
}

func (s *StubStore) Wishlist(context.Context, string) ([]domain.WishlistItem, error) {
	return []domain.WishlistItem{{ID: "wish-1", Name: "Birthday Pick", URL: "https://kyou.id/items/1/", Price: 0}}, nil
}

func (s *StubStore) FYP(context.Context, string) ([]domain.FYPItem, error) {
	return nil, nil
}

func (s *StubStore) Popular(context.Context) ([]domain.FYPItem, error) {
	return []domain.FYPItem{{ID: "popular-1", Name: "Popular Series", Kind: "series", SeriesID: "popular-1"}}, nil
}

func (s *StubStore) HasConverted(context.Context, string, time.Time, time.Time) (bool, error) {
	return false, nil
}
