package reader

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/kyou-id/yukari/internal/audit"
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

type AuditLogger interface {
	InsertQueued(ctx context.Context, email audit.QueuedEmail) error
	InsertSkipped(ctx context.Context, email audit.SkippedEmail) error
	HasBirthdayVoucherEmailInYear(ctx context.Context, userID string, year int) (bool, error)
}

type Reader struct {
	store    Store
	queue    Queue
	vouchers VoucherCreator
	audit    AuditLogger

	queueName string
	actionURL string
}

func New(store Store, queue Queue) Reader {
	return Reader{store: store, queue: queue}
}

func NewWithVoucherCreator(store Store, queue Queue, vouchers VoucherCreator) Reader {
	return Reader{store: store, queue: queue, vouchers: vouchers}
}

func NewWithVoucherCreatorAndAudit(store Store, queue Queue, vouchers VoucherCreator, auditLogger AuditLogger, queueName string, actionURL string) Reader {
	return Reader{store: store, queue: queue, vouchers: vouchers, audit: auditLogger, queueName: queueName, actionURL: actionURL}
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
			if err := r.insertSkipped(ctx, now, user, "converted_within_campaign_window"); err != nil {
				return enqueued, err
			}
			continue
		}
		alreadySentThisYear, err := r.hasBirthdayVoucherEmailInYear(ctx, user.ID, now.Year())
		if err != nil {
			return enqueued, err
		}
		if alreadySentThisYear {
			if err := r.insertSkipped(ctx, now, user, "birthday_voucher_already_sent_this_year"); err != nil {
				return enqueued, err
			}
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
				if err := r.insertSkipped(ctx, now, user, "already_sent_or_issued_within_period"); err != nil {
					return enqueued, err
				}
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
		if err := r.insertQueued(ctx, job); err != nil {
			return enqueued, err
		}
		if err := r.queue.Enqueue(ctx, job); err != nil {
			return enqueued, err
		}
		enqueued++
	}

	return enqueued, nil
}

func (r Reader) insertQueued(ctx context.Context, job domain.BirthdayJob) error {
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
		Metadata: map[string]any{
			"voucher_id":   job.VoucherID,
			"voucher_code": job.VoucherCode,
			"claim_url":    claimURL,
		},
	})
}

func (r Reader) insertSkipped(ctx context.Context, now time.Time, user domain.User, reason string) error {
	if r.audit == nil {
		return nil
	}
	return r.audit.InsertSkipped(ctx, audit.SkippedEmail{
		JobID:       fmt.Sprintf("skip-birthday-%s-user-%s", now.Format("2006"), user.ID),
		UserID:      user.ID,
		ToEmail:     user.Email,
		SkipReason:  reason,
		ReferenceID: user.ID,
	})
}

func (r Reader) hasBirthdayVoucherEmailInYear(ctx context.Context, userID string, year int) (bool, error) {
	if r.audit == nil {
		return false, nil
	}
	return r.audit.HasBirthdayVoucherEmailInYear(ctx, userID, year)
}

func actionURLWithClaim(baseURL string, voucherCode string) string {
	if baseURL == "" || voucherCode == "" {
		return baseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return baseURL
	}
	query := parsed.Query()
	query.Set("claim", voucherCode)
	parsed.RawQuery = query.Encode()
	return parsed.String()
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
