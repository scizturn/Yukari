package reader

import (
	"context"
	"fmt"
	"time"

	"github.com/kyou-id/yukari/internal/domain"
)

type Store interface {
	BirthdayUsers(ctx context.Context, monthDay string) ([]domain.User, error)
	Wishlist(ctx context.Context, userID string) ([]domain.WishlistItem, error)
	FYP(ctx context.Context, userID string) ([]domain.FYPItem, error)
	Popular(ctx context.Context) ([]domain.FYPItem, error)
	HasConverted(ctx context.Context, userID string, from time.Time, to time.Time) (bool, error)
}

type Queue interface {
	Enqueue(ctx context.Context, job domain.BirthdayJob) error
}

type VoucherCreator interface {
	CreateBirthdayVoucher(ctx context.Context, user domain.User, birthdayDate time.Time, itemIDs []string) (domain.Voucher, error)
}

type Reader struct {
	store    Store
	queue    Queue
	vouchers VoucherCreator
}

func New(store Store, queue Queue) Reader {
	return Reader{store: store, queue: queue}
}

func NewWithVoucherCreator(store Store, queue Queue, vouchers VoucherCreator) Reader {
	return Reader{store: store, queue: queue, vouchers: vouchers}
}

func (r Reader) Run(ctx context.Context, now time.Time) (int, error) {
	users, err := r.store.BirthdayUsers(ctx, now.Format("01-02"))
	if err != nil {
		return 0, err
	}

	popular, err := r.store.Popular(ctx)
	if err != nil {
		return 0, err
	}

	enqueued := 0
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	for _, user := range users {
		converted, err := r.store.HasConverted(ctx, user.ID, start, start.Add(14*24*time.Hour))
		if err != nil {
			return enqueued, err
		}
		if converted {
			continue
		}

		wishlist, err := r.store.Wishlist(ctx, user.ID)
		if err != nil {
			return enqueued, err
		}
		fyp, err := r.store.FYP(ctx, user.ID)
		if err != nil {
			return enqueued, err
		}

		var voucher domain.Voucher
		if r.vouchers != nil {
			voucher, err = r.vouchers.CreateBirthdayVoucher(ctx, user, now, voucherItemIDs(wishlist, fyp))
			if err != nil {
				return enqueued, err
			}
			if voucher.Existed {
				continue
			}
		}

		job := domain.BirthdayJob{
			ID:            fmt.Sprintf("birthday-%s-user-%s", now.Format("2006-01-02"), user.ID),
			UserID:        user.ID,
			Date:          now,
			User:          user,
			VoucherCode:   voucher.Code,
			VoucherID:     voucher.ID,
			WishlistItems: wishlist,
			FYPItems:      fyp,
			PopularItems:  popular,
			Attempt:       1,
		}
		if err := r.queue.Enqueue(ctx, job); err != nil {
			return enqueued, err
		}
		enqueued++
	}

	return enqueued, nil
}

func voucherItemIDs(wishlist []domain.WishlistItem, fyp []domain.FYPItem) []string {
	seen := make(map[string]struct{}, len(wishlist)+len(fyp))
	itemIDs := make([]string, 0, len(wishlist)+len(fyp))
	add := func(id string) {
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		itemIDs = append(itemIDs, id)
	}
	for _, item := range wishlist {
		add(item.ID)
	}
	for _, item := range fyp {
		add(item.ID)
	}
	return itemIDs
}
