package reader

import (
	"context"
	"fmt"
	"time"

	"github.com/kyou-id/yukari/internal/audit"
	"github.com/kyou-id/yukari/internal/domain"
)

type AnniversaryStore interface {
	Store
	AnniversaryUsers(ctx context.Context, monthDay string) ([]domain.User, map[string]int, error)
	HistoricalOrders(ctx context.Context, userID string) ([]domain.HistoricalItem, error)
	WishlistAnniversary(ctx context.Context, userID string) ([]domain.WishlistItem, error)
}

type AnniversaryQueue interface {
	EnqueueAnniversary(ctx context.Context, job domain.AnniversaryJob) error
}

type AnniversaryVoucherCreator interface {
	CreateAnniversaryVoucher(ctx context.Context, user domain.User, anniversaryDate time.Time, itemIDs []string) (domain.Voucher, error)
}

type AnniversaryAuditLogger interface {
	AuditLogger
	HasAnniversaryVoucherEmailInYear(ctx context.Context, userID string, year int) (bool, error)
	CountPreviousAnniversaryEmails(ctx context.Context, userID string, currentYear int) (int, error)
}

type AnniversaryReader struct {
	store    AnniversaryStore
	queue    AnniversaryQueue
	vouchers AnniversaryVoucherCreator
	audit    AnniversaryAuditLogger

	queueName string
	actionURL string
}

func NewAnniversary(store AnniversaryStore, queue AnniversaryQueue, vouchers AnniversaryVoucherCreator, auditLogger AnniversaryAuditLogger, queueName string, actionURL string) AnniversaryReader {
	return AnniversaryReader{store: store, queue: queue, vouchers: vouchers, audit: auditLogger, queueName: queueName, actionURL: actionURL}
}

func (r AnniversaryReader) Run(ctx context.Context, now time.Time) (int, error) {
	users, yearsMap, err := r.store.AnniversaryUsers(ctx, now.Format("01-02"))
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
		years := yearsMap[user.ID]

		// Ensure we don't send to users who converted within the campaign window if that logic applies to Anniversary
		// Assuming we don't apply HasConverted to anniversary emails because the user wants "Retention up & Sales"
		// The requirements specifically mention the fallback mechanism for nostalgia.

		alreadySentThisYear, err := r.hasAnniversaryVoucherEmailInYear(ctx, user.ID, now.Year())
		if err != nil {
			return enqueued, err
		}
		if alreadySentThisYear {
			if err := r.insertSkipped(ctx, now, user, "anniversary_voucher_already_sent_this_year"); err != nil {
				return enqueued, err
			}
			continue
		}

		wishlist, err := r.store.WishlistAnniversary(ctx, user.ID)
		if err != nil {
			return enqueued, err
		}
		if len(wishlist) < 6 {
			fyp, err := r.store.FYP(ctx, user.ID)
			if err != nil {
				return enqueued, err
			}
			wishlist = FillWishlistToSix(wishlist, fyp, popular)
		}

		var voucher domain.Voucher
		if r.vouchers != nil {
			voucher, err = r.vouchers.CreateAnniversaryVoucher(ctx, user, now, voucherItemIDs(wishlist, nil))
			if err != nil {
				return enqueued, err
			}
			if voucher.Existed {
				if err := r.insertSkipped(ctx, now, user, "already_sent_or_issued_within_period"); err != nil {
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
			emailsSentBefore, err := r.countPreviousAnniversaryEmails(ctx, user.ID, now.Year())
			if err != nil {
				return enqueued, err
			}
			index := emailsSentBefore
			if index >= len(orders) {
				index = len(orders) - 1
			}
			historicalItem = orders[index]
			historicalItem.DaysAgo = int(start.Sub(historicalItem.OrderDate).Hours() / 24)
		}

		job := domain.AnniversaryJob{
			ID:             fmt.Sprintf("anniversary-%s-user-%s", now.Format("2006-01-02"), user.ID),
			UserID:         user.ID,
			Date:           now,
			User:           user,
			Years:          years,
			VoucherCode:    voucher.Code,
			VoucherID:      voucher.ID,
			HistoricalItem: historicalItem,
			WishlistItems:  wishlist,
			PopularItems:   popular,
			Attempt:        1,
		}
		if err := r.insertQueued(ctx, job); err != nil {
			return enqueued, err
		}
		if err := r.queue.EnqueueAnniversary(ctx, job); err != nil {
			return enqueued, err
		}
		enqueued++
	}

	return enqueued, nil
}

func (r AnniversaryReader) insertQueued(ctx context.Context, job domain.AnniversaryJob) error {
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
		Feature:     audit.FeatureAnniversaryVoucher,
		Metadata: map[string]any{
			"voucher_id":   job.VoucherID,
			"voucher_code": job.VoucherCode,
			"claim_url":    claimURL,
			"years":        job.Years,
		},
	})
}

func (r AnniversaryReader) insertSkipped(ctx context.Context, now time.Time, user domain.User, reason string) error {
	if r.audit == nil {
		return nil
	}
	return r.audit.InsertSkipped(ctx, audit.SkippedEmail{
		JobID:       fmt.Sprintf("skip-anniversary-%s-user-%s", now.Format("2006"), user.ID),
		UserID:      user.ID,
		ToEmail:     user.Email,
		SkipReason:  reason,
		ReferenceID: user.ID,
		Feature:     audit.FeatureAnniversaryVoucher,
	})
}

func FillWishlistToSix(wishlist []domain.WishlistItem, fyp []domain.FYPItem, popular []domain.FYPItem) []domain.WishlistItem {
	seen := make(map[string]bool, len(wishlist))
	for _, w := range wishlist {
		if w.ID != "" {
			seen[w.ID] = true
		}
	}
	for _, src := range [][]domain.FYPItem{fyp, popular} {
		for _, p := range src {
			if len(wishlist) >= 6 {
				return wishlist
			}
			if p.ID != "" && seen[p.ID] {
				continue
			}
			if p.ID != "" {
				seen[p.ID] = true
			}
			wishlist = append(wishlist, fypToWishlist(p))
		}
	}
	return wishlist
}

func fypToWishlist(p domain.FYPItem) domain.WishlistItem {
	return domain.WishlistItem{
		ID:          p.ID,
		Name:        p.Name,
		URL:         "https://kyou.id/items/" + p.ID + "/",
		ImageURL:    p.ImageURL,
		Price:       p.Price,
		Status:      p.Status,
		Manufacturer: p.Manufacturer,
		SeriesName:  p.SeriesName,
		PODeadline:  p.PODeadline,
		POReleaseAt: p.POReleaseAt,
	}
}

func (r AnniversaryReader) hasAnniversaryVoucherEmailInYear(ctx context.Context, userID string, year int) (bool, error) {
	if r.audit == nil {
		return false, nil
	}
	return r.audit.HasAnniversaryVoucherEmailInYear(ctx, userID, year)
}

func (r AnniversaryReader) countPreviousAnniversaryEmails(ctx context.Context, userID string, currentYear int) (int, error) {
	if r.audit == nil {
		return 0, nil
	}
	return r.audit.CountPreviousAnniversaryEmails(ctx, userID, currentYear)
}
