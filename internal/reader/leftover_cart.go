package reader

import (
	"context"
	"fmt"
	"time"

	"github.com/kyou-id/yukari/internal/audit"
	"github.com/kyou-id/yukari/internal/domain"
)

type LeftoverCartStore interface {
	LeftoverCartUsers(ctx context.Context, now time.Time) ([]domain.User, error)
	CartItems(ctx context.Context, userID string) ([]domain.WishlistItem, error)
	LeftoverCartReco(ctx context.Context, userID string) ([]domain.FYPItem, error)
	Popular(ctx context.Context) ([]domain.FYPItem, error)
}

type LeftoverCartQueue interface {
	EnqueueLeftoverCartTo(ctx context.Context, queueName string, job domain.LeftoverCartJob) error
}

type LeftoverCartAuditLogger interface {
	InsertQueued(ctx context.Context, email audit.QueuedEmail) error
	InsertSkipped(ctx context.Context, email audit.SkippedEmail) error
}

type LeftoverCartReader struct {
	store     LeftoverCartStore
	queue     LeftoverCartQueue
	audit     LeftoverCartAuditLogger
	queueName string
}

func NewLeftoverCart(store LeftoverCartStore, queue LeftoverCartQueue, auditLogger LeftoverCartAuditLogger, queueName string) LeftoverCartReader {
	return LeftoverCartReader{store: store, queue: queue, audit: auditLogger, queueName: queueName}
}

func (r LeftoverCartReader) Run(ctx context.Context, now time.Time) (int, error) {
	users, err := r.store.LeftoverCartUsers(ctx, now)
	if err != nil {
		return 0, err
	}

	popular, err := r.store.Popular(ctx)
	if err != nil {
		return 0, err
	}

	enqueued := 0
	for _, user := range users {
		cartItems, err := r.store.CartItems(ctx, user.ID)
		if err != nil {
			return enqueued, err
		}
		if len(cartItems) == 0 {
			if err := r.insertSkipped(ctx, now, user, "no_available_cart_items"); err != nil {
				return enqueued, err
			}
			continue
		}

		reco, err := r.store.LeftoverCartReco(ctx, user.ID)
		if err != nil {
			return enqueued, err
		}
		// fallback: fill reco with popular items if no series match found
		if len(reco) < 3 {
			reco = FillRecoFromPopular(reco, popular, 3)
		}

		job := domain.LeftoverCartJob{
			ID:        fmt.Sprintf("leftover-cart-%s-user-%s", now.Format("2006-01-02"), user.ID),
			UserID:    user.ID,
			Date:      now,
			User:      user,
			CartItems: cartItems,
			RecoItems: reco,
			Attempt:   1,
		}
		if err := r.insertQueued(ctx, job); err != nil {
			return enqueued, err
		}
		if err := r.queue.EnqueueLeftoverCartTo(ctx, r.queueName, job); err != nil {
			return enqueued, err
		}
		enqueued++
	}

	return enqueued, nil
}

func FillRecoFromPopular(reco []domain.FYPItem, popular []domain.FYPItem, limit int) []domain.FYPItem {
	seen := make(map[string]bool, len(reco))
	for _, r := range reco {
		if r.ID != "" {
			seen[r.ID] = true
		}
	}
	for _, p := range popular {
		if len(reco) >= limit {
			break
		}
		if p.ID != "" && seen[p.ID] {
			continue
		}
		if p.ID != "" {
			seen[p.ID] = true
		}
		reco = append(reco, p)
	}
	return reco
}

func (r LeftoverCartReader) insertQueued(ctx context.Context, job domain.LeftoverCartJob) error {
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
		Feature:     audit.FeatureLeftoverCart,
		Metadata: map[string]any{
			"cart_item_count": len(job.CartItems),
			"reco_item_count": len(job.RecoItems),
		},
	})
}

func (r LeftoverCartReader) insertSkipped(ctx context.Context, now time.Time, user domain.User, reason string) error {
	if r.audit == nil {
		return nil
	}
	return r.audit.InsertSkipped(ctx, audit.SkippedEmail{
		JobID:       fmt.Sprintf("skip-leftover-cart-%s-user-%s", now.Format("2006-01-02"), user.ID),
		UserID:      user.ID,
		ToEmail:     user.Email,
		SkipReason:  reason,
		ReferenceID: user.ID,
		Feature:     audit.FeatureLeftoverCart,
	})
}
