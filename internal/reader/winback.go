package reader

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kyou-id/yukari/internal/audit"
	"github.com/kyou-id/yukari/internal/domain"
)

// FillWishlistReadyTo pads the wishlist up to target using ready fill items
// (most-popular ready), keeping the user's own wishlist first, de-duplicating by
// ID and skipping anything not ready.
func FillWishlistReadyTo(wishlist []domain.WishlistItem, target int, fill []domain.FYPItem) []domain.WishlistItem {
	seen := make(map[string]bool, len(wishlist))
	for i := range wishlist {
		wishlist[i].IsWishlisted = true // the user's own wishlist items get the orange border
		if wishlist[i].ID != "" {
			seen[wishlist[i].ID] = true
		}
	}
	for _, p := range fill {
		if len(wishlist) >= target {
			break
		}
		if p.ID != "" && seen[p.ID] {
			continue
		}
		if !strings.EqualFold(p.Status, "ready") {
			continue
		}
		if p.ID != "" {
			seen[p.ID] = true
		}
		wishlist = append(wishlist, fypToWishlist(p))
	}
	return wishlist
}

type WinbackStore interface {
	WinbackUsers(ctx context.Context, now time.Time) ([]domain.User, error)
	WishlistWinback(ctx context.Context, userID string) ([]domain.WishlistItem, error)
	HistoricalOrders(ctx context.Context, userID string) ([]domain.HistoricalItem, error)
	Popular(ctx context.Context) ([]domain.FYPItem, error)
	FYP(ctx context.Context, userID string) ([]domain.FYPItem, error)
	WinbackFillItems(ctx context.Context) ([]domain.FYPItem, error)
}

// winbackReadyTarget is how many ready items the winback wishlist grid shows:
// the user's own wishlist first, then most-popular ready items to fill.
const winbackReadyTarget = 12

// winbackPastLimit caps how many past orders the "past collection" list shows.
const winbackPastLimit = 3

// recentHistoricalItems returns up to limit of the most-recent orders, ordered
// oldest → latest, with DaysAgo computed relative to start. Input orders are
// expected oldest-first (historical_orders.sql orders by created_at ASC), so we
// select the last `limit` (the most recent) and keep them in chronological
// order — the past-collection list relives the timeline forward.
func recentHistoricalItems(orders []domain.HistoricalItem, start time.Time, limit int) []domain.HistoricalItem {
	if len(orders) == 0 || limit <= 0 {
		return nil
	}
	from := len(orders) - limit
	if from < 0 {
		from = 0
	}
	recent := orders[from:]
	items := make([]domain.HistoricalItem, 0, len(recent))
	for i := 0; i < len(recent); i++ { // oldest → latest (input is ASC by created_at)
		item := recent[i]
		item.DaysAgo = int(start.Sub(item.OrderDate).Hours() / 24)
		items = append(items, item)
	}
	return items
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

	fillItems, err := r.store.WinbackFillItems(ctx)
	if err != nil {
		return 0, err
	}

	enqueued := 0
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	for _, user := range users {
		wishlist, err := r.store.WishlistWinback(ctx, user.ID)
		if err != nil {
			return enqueued, err
		}
		wishlist = FillWishlistReadyTo(wishlist, winbackReadyTarget, fillItems)

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
		// orders arrive oldest-first (SQL ORDER BY created_at ASC); take the last
		// winbackPastLimit (the most recent) and keep them oldest → latest so the
		// list relives the timeline forward. HistoricalItem stays the single
		// most-recent order for audit — now the LAST element of the list.
		historicalItems := recentHistoricalItems(orders, start, winbackPastLimit)
		var historicalItem domain.HistoricalItem
		if len(historicalItems) > 0 {
			historicalItem = historicalItems[len(historicalItems)-1]
		}

		job := domain.WinbackJob{
			ID:              fmt.Sprintf("winback-%s-user-%s", now.Format("2006-01-02"), user.ID),
			UserID:          user.ID,
			Date:            now,
			User:            user,
			VoucherCode:     voucher.Code,
			VoucherID:       voucher.ID,
			WishlistItems:   wishlist,
			HistoricalItem:  historicalItem,
			HistoricalItems: historicalItems,
			PopularItems:    popular,
			Attempt:         1,
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
			"voucher_id":      job.VoucherID,
			"voucher_code":    job.VoucherCode,
			"claim_url":       claimURL,
			"historical_item": job.HistoricalItem.Name,
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
