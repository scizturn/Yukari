package reader

import (
	"context"
	"fmt"
	"time"

	"github.com/kyou-id/yukari/internal/audit"
	"github.com/kyou-id/yukari/internal/domain"
)

type WishlistBackInStore interface {
	WishlistBackInItems(ctx context.Context, startAt, endAt time.Time) ([]domain.WishlistBackInItem, error)
	WishlistBackInUsers(ctx context.Context, itemID string) ([]domain.User, error)
	WishlistBackInCompanion(ctx context.Context, userID, itemID string) (domain.WishlistBackInItem, error)
}

type WishlistBackInQueue interface {
	EnqueueWishlistBackInTo(ctx context.Context, queueName string, job domain.WishlistBackInJob) error
}

type WishlistBackInVoucherCreator interface {
	CreateWishlistBackInVoucher(ctx context.Context, user domain.User, campaignDate time.Time, itemIDs []string) (domain.Voucher, error)
}

type WishlistBackInAuditLogger interface {
	InsertQueued(ctx context.Context, email audit.QueuedEmail) error
}

type WishlistBackInReader struct {
	store     WishlistBackInStore
	queue     WishlistBackInQueue
	vouchers  WishlistBackInVoucherCreator
	audit     WishlistBackInAuditLogger
	queueName string
	actionURL string
	startAt   time.Time
}

func NewWishlistBackIn(store WishlistBackInStore, queue WishlistBackInQueue, vouchers WishlistBackInVoucherCreator, auditLogger WishlistBackInAuditLogger, queueName, actionURL string, startAt time.Time) WishlistBackInReader {
	return WishlistBackInReader{store: store, queue: queue, vouchers: vouchers, audit: auditLogger, queueName: queueName, actionURL: actionURL, startAt: startAt}
}

func (r WishlistBackInReader) Run(ctx context.Context, now time.Time) (int, error) {
	if now.Weekday() != time.Friday {
		return 0, nil
	}

	cutoff := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	items, err := r.store.WishlistBackInItems(ctx, r.startAt, cutoff)
	if err != nil {
		return 0, err
	}

	enqueued := 0
	for _, item := range items {
		users, err := r.store.WishlistBackInUsers(ctx, item.ID)
		if err != nil {
			return enqueued, err
		}
		for _, user := range users {
			companion, err := r.store.WishlistBackInCompanion(ctx, user.ID, item.ID)
			if err != nil {
				return enqueued, err
			}

			var voucher domain.Voucher
			if r.vouchers != nil {
				voucher, err = r.vouchers.CreateWishlistBackInVoucher(ctx, user, cutoff, []string{item.ID})
				if err != nil {
					return enqueued, err
				}
			}

			job := domain.WishlistBackInJob{
				ID:            fmt.Sprintf("wishlist-back-in-%s-item-%s-user-%s", cutoff.Format("2006-01-02"), item.ID, user.ID),
				UserID:        user.ID,
				Date:          now,
				User:          user,
				VoucherCode:   voucher.Code,
				VoucherID:     voucher.ID,
				Item:          item,
				CompanionItem: companion,
				Attempt:       1,
			}
			if err := r.insertQueued(ctx, job); err != nil {
				return enqueued, err
			}
			if err := r.queue.EnqueueWishlistBackInTo(ctx, r.queueName, job); err != nil {
				return enqueued, err
			}
			enqueued++
		}
	}
	return enqueued, nil
}

func (r WishlistBackInReader) insertQueued(ctx context.Context, job domain.WishlistBackInJob) error {
	if r.audit == nil {
		return nil
	}
	claimURL := actionURLWithClaim(r.actionURL, job.VoucherCode)
	return r.audit.InsertQueued(ctx, audit.QueuedEmail{
		JobID:       job.ID,
		QueueName:   r.queueName,
		Attempt:     job.Attempt,
		UserID:      job.UserID,
		ToEmail:     job.User.Email,
		ActionURL:   claimURL,
		ReferenceID: job.Item.ID,
		Feature:     audit.FeatureWishlistBackIn,
		Metadata: map[string]any{
			"item_id":       job.Item.ID,
			"item_name":     job.Item.Name,
			"restocked_at":  job.Item.RestockedAt,
			"popular_score": job.Item.PopularScore,
			"companion_id":  job.CompanionItem.ID,
			"voucher_id":    job.VoucherID,
			"voucher_code":  job.VoucherCode,
			"claim_url":     claimURL,
		},
	})
}
