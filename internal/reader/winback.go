package reader

import (
	"context"
	"fmt"
	"time"

	"github.com/kyou-id/yukari/internal/audit"
	"github.com/kyou-id/yukari/internal/domain"
)

type WinbackStore interface {
	WinbackUsers(ctx context.Context, now time.Time) ([]domain.User, error)
	Wishlist(ctx context.Context, userID string) ([]domain.WishlistItem, error)
	HistoricalOrders(ctx context.Context, userID string) ([]domain.HistoricalItem, error)
	Popular(ctx context.Context) ([]domain.FYPItem, error)
	FYP(ctx context.Context, userID string) ([]domain.FYPItem, error)
}

type WinbackQueue interface {
	EnqueueWinbackTo(ctx context.Context, queueName string, job domain.WinbackJob) error
}

type WinbackVoucherCreator interface {
	CreateWinbackVoucher(ctx context.Context, user domain.User, winbackDate time.Time, itemIDs []string) (domain.Voucher, error)
}

type WinbackAuditLogger interface {
	InsertQueued(ctx context.Context, email audit.QueuedEmail) error
	InsertSkipped(ctx context.Context, email audit.SkippedEmail) error
}

type WinbackReader struct {
	store     WinbackStore
	queue     WinbackQueue
	vouchers  WinbackVoucherCreator
	audit     WinbackAuditLogger
	queueName string
	actionURL string
}

func NewWinback(store WinbackStore, queue WinbackQueue, vouchers WinbackVoucherCreator, auditLogger WinbackAuditLogger, queueName string, actionURL string) WinbackReader {
	return WinbackReader{store: store, queue: queue, vouchers: vouchers, audit: auditLogger, queueName: queueName, actionURL: actionURL}
}

func (r WinbackReader) Run(ctx context.Context, now time.Time) (int, error) {
	users, err := r.store.WinbackUsers(ctx, now)
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
		wishlist, err := r.store.Wishlist(ctx, user.ID)
		if err != nil {
			return enqueued, err
		}
		fyp, err := r.store.FYP(ctx, user.ID)
		if err != nil {
			return enqueued, err
		}
		wishlist = FillWishlistToSix(wishlist, fyp, popular)

		var voucher domain.Voucher
		if r.vouchers != nil {
			voucher, err = r.vouchers.CreateWinbackVoucher(ctx, user, now, voucherItemIDs(wishlist, nil))
			if err != nil {
				return enqueued, err
			}
			if voucher.Existed {
				if err := r.insertSkipped(ctx, now, user, "winback_voucher_already_issued_this_year"); err != nil {
					return enqueued, err
				}
				continue
			}
		}

		orders, err := r.store.HistoricalOrders(ctx, user.ID)
		if err != nil {
			return enqueued, err
		}
		var historicalItem domain.HistoricalItem
		if len(orders) > 0 {
			historicalItem = orders[len(orders)-1]
			historicalItem.DaysAgo = int(start.Sub(historicalItem.OrderDate).Hours() / 24)
		}

		job := domain.WinbackJob{
			ID:             fmt.Sprintf("winback-%s-user-%s", now.Format("2006-01-02"), user.ID),
			UserID:         user.ID,
			Date:           now,
			User:           user,
			VoucherCode:    voucher.Code,
			VoucherID:      voucher.ID,
			WishlistItems:  wishlist,
			HistoricalItem: historicalItem,
			PopularItems:   popular,
			Attempt:        1,
		}
		if err := r.insertQueued(ctx, job); err != nil {
			return enqueued, err
		}
		if err := r.queue.EnqueueWinbackTo(ctx, r.queueName, job); err != nil {
			return enqueued, err
		}
		enqueued++
	}

	return enqueued, nil
}

func (r WinbackReader) insertQueued(ctx context.Context, job domain.WinbackJob) error {
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
		ReferenceID: job.UserID,
		Feature:     audit.FeatureWinback,
		Metadata: map[string]any{
			"voucher_id":       job.VoucherID,
			"voucher_code":     job.VoucherCode,
			"claim_url":        claimURL,
			"historical_item":  job.HistoricalItem.Name,
		},
	})
}

func (r WinbackReader) insertSkipped(ctx context.Context, now time.Time, user domain.User, reason string) error {
	if r.audit == nil {
		return nil
	}
	return r.audit.InsertSkipped(ctx, audit.SkippedEmail{
		JobID:       fmt.Sprintf("skip-winback-%s-user-%s", now.Format("2006"), user.ID),
		UserID:      user.ID,
		ToEmail:     user.Email,
		SkipReason:  reason,
		ReferenceID: user.ID,
		Feature:     audit.FeatureWinback,
	})
}
