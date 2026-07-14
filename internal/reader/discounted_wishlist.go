package reader

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kyou-id/yukari/internal/audit"
	"github.com/kyou-id/yukari/internal/domain"
)

type DiscountedWishlistStore interface {
	DiscountedWishlistUsers(ctx context.Context, now time.Time) ([]domain.User, error)
	DiscountedWishlistItems(ctx context.Context, userID string) ([]domain.DiscountedWishlistItem, error)
	DiscountedWishlistFillIndex(ctx context.Context) (DiscountedWishlistFiller, error)
}

// DiscountedWishlistFiller serves the fill grid for a user without touching the DB. It is
// built once per run; the fill candidates are the same for everyone, so asking the DB per
// user recomputed one answer 32,771 times on the 13 Jul 2026 run.
type DiscountedWishlistFiller interface {
	Fill(userID string) []domain.DiscountedWishlistItem
}

type DiscountedWishlistQueue interface {
	EnqueueDiscountedWishlistTo(ctx context.Context, queueName string, job domain.DiscountedWishlistJob) error
}

type DiscountedWishlistAuditLogger interface {
	InsertQueued(ctx context.Context, email audit.QueuedEmail) error
	InsertSkipped(ctx context.Context, email audit.SkippedEmail) error
	MarkEnqueueFailed(ctx context.Context, jobID string, attempt int, reason string) error
}

type DiscountedWishlistReader struct {
	store     DiscountedWishlistStore
	queue     DiscountedWishlistQueue
	audit     DiscountedWishlistAuditLogger
	queueName string
}

func NewDiscountedWishlist(store DiscountedWishlistStore, queue DiscountedWishlistQueue, auditLogger DiscountedWishlistAuditLogger, queueName string) DiscountedWishlistReader {
	return DiscountedWishlistReader{store: store, queue: queue, audit: auditLogger, queueName: queueName}
}

func (r DiscountedWishlistReader) Run(ctx context.Context, now time.Time) (int, error) {
	users, err := r.store.DiscountedWishlistUsers(ctx, now)
	if err != nil {
		return 0, err
	}

	// Built once, before the loop: every user's fill grid is drawn from the same pool.
	filler, err := r.store.DiscountedWishlistFillIndex(ctx)
	if err != nil {
		return 0, err
	}

	enqueued := 0
	for _, user := range users {
		wishlisted, err := r.store.DiscountedWishlistItems(ctx, user.ID)
		if err != nil {
			return enqueued, err
		}
		if len(wishlisted) == 0 {
			if err := r.insertSkipped(ctx, now, user, "no_discounted_wishlist_items"); err != nil {
				return enqueued, err
			}
			continue
		}

		fill := filler.Fill(user.ID)

		items := append(wishlisted, fill...)

		job := domain.DiscountedWishlistJob{
			ID:      fmt.Sprintf("discounted-wishlist-%s-user-%s", now.Format("2006-01-02"), user.ID),
			UserID:  user.ID,
			Date:    now,
			User:    user,
			Items:   items,
			Attempt: 1,
		}
		if err := r.insertQueued(ctx, job, len(wishlisted), len(fill)); err != nil {
			return enqueued, err
		}
		if err := r.queue.EnqueueDiscountedWishlistTo(ctx, r.queueName, job); err != nil {
			markErr := r.markEnqueueFailed(ctx, job, err)
			return enqueued, errors.Join(err, markErr)
		}
		enqueued++
	}

	return enqueued, nil
}

func (r DiscountedWishlistReader) markEnqueueFailed(ctx context.Context, job domain.DiscountedWishlistJob, enqueueErr error) error {
	if r.audit == nil {
		return nil
	}
	return r.audit.MarkEnqueueFailed(ctx, job.ID, job.Attempt, "redis enqueue failed: "+enqueueErr.Error())
}

func (r DiscountedWishlistReader) insertQueued(ctx context.Context, job domain.DiscountedWishlistJob, wishlistCount, fillCount int) error {
	if r.audit == nil {
		return nil
	}
	return r.audit.InsertQueued(ctx, audit.QueuedEmail{
		JobID:       job.ID,
		QueueName:   r.queueName,
		Attempt:     job.Attempt,
		UserID:      job.UserID,
		ToEmail:     job.User.Email,
		ReferenceID: job.UserID,
		Feature:     audit.FeatureDiscountedWishlist,
		Metadata: map[string]any{
			"wishlist_item_count": wishlistCount,
			"fill_item_count":     fillCount,
		},
	})
}

func (r DiscountedWishlistReader) insertSkipped(ctx context.Context, now time.Time, user domain.User, reason string) error {
	if r.audit == nil {
		return nil
	}
	return r.audit.InsertSkipped(ctx, audit.SkippedEmail{
		JobID:       fmt.Sprintf("skip-discounted-wishlist-%s-user-%s", now.Format("2006-01-02"), user.ID),
		UserID:      user.ID,
		ToEmail:     user.Email,
		SkipReason:  reason,
		ReferenceID: user.ID,
		Feature:     audit.FeatureDiscountedWishlist,
	})
}
