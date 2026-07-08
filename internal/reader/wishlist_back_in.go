package reader

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/kyou-id/yukari/internal/audit"
	"github.com/kyou-id/yukari/internal/domain"
)

// wishlistBackInMaxItems caps how many restocked items one user's email lists.
const wishlistBackInMaxItems = 5

// wishlistBackInWindow is how far back a restock still counts as a pending
// carry-over item. It is wider than one week (21 days ≈ 3 Fridays) so items that
// overflowed a user's 5-item cap in an earlier week remain candidates and fire on
// a later Friday — the queue behaviour ("continue to the next item id that has
// not yet fired"). The per-(user,item) 90-day dedup drains the queue: an item
// fires once, then is suppressed, letting the next un-fired item through. 21 days
// keeps the weekly query fast (~1.5s) and the first-run blast bounded; an
// un-fired item that never wins a slot ages out after 3 weeks. Widen it if longer
// carry-over is wanted (cost grows: 30d ≈ 7s, 90d ≈ 17s).
const wishlistBackInWindow = 21 * 24 * time.Hour

// wishlistBackInRecoCount is the exact number of cross-sell recommendations the
// "Gas, nemenin yang udah kamu beli" section needs; fewer -> the section is hidden.
const wishlistBackInRecoCount = 6

// Voucher tiers, by gross-profit floor. These mirror data/vouchers/
// wishlist_back_in.json (8%) and wishlist_back_in_low.json (6%), and the two head
// vouchers in prod. Change all four together or /search will quote a discount
// checkout does not honour.
const (
	wishlistBackInHighPercent = 8
	wishlistBackInHighMinGP   = 35.0
	wishlistBackInLowPercent  = 6
	wishlistBackInLowMinGP    = 25.0
)

// WishlistBackInTier picks the single discount tier for one user's email, or 0 for
// "mint no voucher". Exported so forcejob mints the same tier the cron would.
//
// A user's email lists up to 5 items with different GP ratios, but a voucher has
// exactly one `amount` and hanayo evaluates rules per cart item. So the tier is
// driven by the LOWEST-GP item that still clears the 25% floor: that keeps every
// item in the email covered, at the cost of billing a GP-40 item at 6% when it
// shares an email with a GP-30 one.
//
// Items below 25% GP (and items whose GP is unknown, which hanayo refuses to
// discount at all) are ignored here. They still appear in the email as restock
// news; the voucher's gp_ratio_min simply never matches them at checkout.
func WishlistBackInTier(items []domain.WishlistBackInItem) int {
	minGP := math.Inf(1)
	for _, item := range items {
		if item.GPRatio == nil || *item.GPRatio < wishlistBackInLowMinGP {
			continue
		}
		if *item.GPRatio < minGP {
			minGP = *item.GPRatio
		}
	}
	switch {
	case math.IsInf(minGP, 1):
		return 0
	case minGP >= wishlistBackInHighMinGP:
		return wishlistBackInHighPercent
	default:
		return wishlistBackInLowPercent
	}
}

type WishlistBackInStore interface {
	WishlistBackInUserItems(ctx context.Context, startAt, endAt time.Time) ([]domain.WishlistBackInUserItem, error)
	WishlistBackInCompanion(ctx context.Context, userID string) (domain.WishlistBackInItem, error)
	WishlistBackInRecommendations(ctx context.Context, userID, anchorItemID string) ([]domain.WishlistBackInItem, error)
}

type WishlistBackInQueue interface {
	EnqueueWishlistBackInTo(ctx context.Context, queueName string, job domain.WishlistBackInJob) error
}

type WishlistBackInVoucherCreator interface {
	CreateWishlistBackInVoucher(ctx context.Context, user domain.User, campaignDate time.Time, itemIDs []string, discountPercent int) (domain.Voucher, error)
}

type WishlistBackInAuditLogger interface {
	InsertQueued(ctx context.Context, email audit.QueuedEmail) error
	MarkEnqueueFailed(ctx context.Context, jobID string, attempt int, reason string) error
}

type WishlistBackInReader struct {
	store     WishlistBackInStore
	queue     WishlistBackInQueue
	vouchers  WishlistBackInVoucherCreator
	audit     WishlistBackInAuditLogger
	queueName string
	actionURL string
}

func NewWishlistBackIn(store WishlistBackInStore, queue WishlistBackInQueue, vouchers WishlistBackInVoucherCreator, auditLogger WishlistBackInAuditLogger, queueName, actionURL string) WishlistBackInReader {
	return WishlistBackInReader{store: store, queue: queue, vouchers: vouchers, audit: auditLogger, queueName: queueName, actionURL: actionURL}
}

func (r WishlistBackInReader) Run(ctx context.Context, now time.Time) (int, error) {
	if now.Weekday() != time.Friday {
		return 0, nil
	}

	cutoff := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startAt := cutoff.Add(-wishlistBackInWindow)

	rows, err := r.store.WishlistBackInUserItems(ctx, startAt, cutoff)
	if err != nil {
		return 0, err
	}

	// rows arrive grouped by user, newest restock first (see the SQL ORDER BY).
	// Walk them into one job per user, capping each user's list to the 5 most
	// recently restocked items.
	enqueued := 0
	i := 0
	for i < len(rows) {
		user := rows[i].User
		var items []domain.WishlistBackInItem
		for i < len(rows) && rows[i].User.ID == user.ID {
			if len(items) < wishlistBackInMaxItems {
				items = append(items, rows[i].Item)
			}
			i++
		}
		if len(items) == 0 {
			continue
		}

		// Anchor = the user's most recent completed (received) purchase that has a
		// series/category. It names the "Gas, nemenin..." section and seeds the
		// recommendations. Recommendations = 6 most-popular items in that
		// series/category; the section only renders with a full 6.
		companion, err := r.store.WishlistBackInCompanion(ctx, user.ID)
		if err != nil {
			return enqueued, err
		}
		var recos []domain.WishlistBackInItem
		if companion.ID != "" {
			recos, err = r.store.WishlistBackInRecommendations(ctx, user.ID, companion.ID)
			if err != nil {
				return enqueued, err
			}
			if len(recos) < wishlistBackInRecoCount {
				companion, recos = domain.WishlistBackInItem{}, nil
			}
		}

		itemIDs := make([]string, len(items))
		for j, item := range items {
			itemIDs[j] = item.ID
		}

		// tier == 0 -> no item clears the 25% GP floor, so no voucher is minted.
		// The email still goes out; it just renders without the coupon block.
		tier := WishlistBackInTier(items)
		var voucher domain.Voucher
		if r.vouchers != nil && tier > 0 {
			voucher, err = r.vouchers.CreateWishlistBackInVoucher(ctx, user, cutoff, itemIDs, tier)
			if err != nil {
				return enqueued, err
			}
		}
		if voucher.Code == "" {
			tier = 0
		}

		job := domain.WishlistBackInJob{
			ID:                     fmt.Sprintf("wishlist-back-in-%s-user-%s", cutoff.Format("2006-01-02"), user.ID),
			UserID:                 user.ID,
			Date:                   now,
			User:                   user,
			VoucherCode:            voucher.Code,
			VoucherID:              voucher.ID,
			VoucherDiscountPercent: tier,
			Items:                  items,
			CompanionItem:          companion,
			RecoItems:              recos,
			Attempt:                1,
		}
		if err := r.insertQueued(ctx, job, itemIDs); err != nil {
			return enqueued, err
		}
		if err := r.queue.EnqueueWishlistBackInTo(ctx, r.queueName, job); err != nil {
			markErr := r.markEnqueueFailed(ctx, job, err)
			return enqueued, errors.Join(err, markErr)
		}
		enqueued++
	}
	return enqueued, nil
}

func (r WishlistBackInReader) markEnqueueFailed(ctx context.Context, job domain.WishlistBackInJob, enqueueErr error) error {
	if r.audit == nil {
		return nil
	}
	return r.audit.MarkEnqueueFailed(ctx, job.ID, job.Attempt, "redis enqueue failed: "+enqueueErr.Error())
}

func (r WishlistBackInReader) insertQueued(ctx context.Context, job domain.WishlistBackInJob, itemIDs []string) error {
	if r.audit == nil {
		return nil
	}
	claimURL := actionURLWithClaim(r.actionURL, job.VoucherCode)
	// item_ids feeds the per-(user, item) dedup in wishlist_back_in_user_items.sql.
	return r.audit.InsertQueued(ctx, audit.QueuedEmail{
		JobID:       job.ID,
		QueueName:   r.queueName,
		Attempt:     job.Attempt,
		UserID:      job.UserID,
		ToEmail:     job.User.Email,
		ActionURL:   claimURL,
		ReferenceID: job.UserID,
		Feature:     audit.FeatureWishlistBackIn,
		Metadata: map[string]any{
			"item_ids":                 itemIDs,
			"item_count":               len(job.Items),
			"companion_id":             job.CompanionItem.ID,
			"voucher_id":               job.VoucherID,
			"voucher_code":             job.VoucherCode,
			"voucher_discount_percent": job.VoucherDiscountPercent,
			"claim_url":                claimURL,
		},
	})
}
