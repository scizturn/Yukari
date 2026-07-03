package reader

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kyou-id/yukari/internal/audit"
	"github.com/kyou-id/yukari/internal/domain"
)

type PoReadyStore interface {
	PoReadyOrders(ctx context.Context) ([]domain.PoReadyOrder, error)
	PoReadyItems(ctx context.Context, orderID string) ([]domain.PoReadyItem, error)
}

type PoReadyQueue interface {
	EnqueuePoReadyTo(ctx context.Context, queueName string, job domain.PoReadyJob) error
}

type PoReadyAuditLogger interface {
	InsertQueued(ctx context.Context, email audit.QueuedEmail) error
	MarkEnqueueFailed(ctx context.Context, jobID string, attempt int, reason string) error
}

type PoReadyReader struct {
	store     PoReadyStore
	queue     PoReadyQueue
	audit     PoReadyAuditLogger
	queueName string
}

func NewPoReady(store PoReadyStore, queue PoReadyQueue, auditLogger PoReadyAuditLogger, queueName string) PoReadyReader {
	return PoReadyReader{store: store, queue: queue, audit: auditLogger, queueName: queueName}
}

func (r PoReadyReader) Run(ctx context.Context, now time.Time) (int, error) {
	orders, err := r.store.PoReadyOrders(ctx)
	if err != nil {
		return 0, err
	}

	enqueued := 0
	for _, order := range orders {
		items, err := r.store.PoReadyItems(ctx, order.OrderID)
		if err != nil {
			return enqueued, err
		}
		// State-based query already guarantees ready items exist, but guard so a
		// racing settlement between the two queries never enqueues an empty email.
		if len(items) == 0 {
			continue
		}

		job := domain.PoReadyJob{
			ID:          fmt.Sprintf("po-ready-%s-order-%s", now.Format("2006-01-02"), order.OrderID),
			OrderID:     order.OrderID,
			UserID:      order.User.ID,
			Date:        now,
			User:        order.User,
			Items:       items,
			Remaining:   order.Remaining,
			DownPayment: order.DownPayment,
			ETA:         order.ETA,
			Attempt:     1,
		}
		if err := r.insertQueued(ctx, job); err != nil {
			return enqueued, err
		}
		if err := r.queue.EnqueuePoReadyTo(ctx, r.queueName, job); err != nil {
			markErr := r.markEnqueueFailed(ctx, job, err)
			return enqueued, errors.Join(err, markErr)
		}
		enqueued++
	}

	return enqueued, nil
}

func (r PoReadyReader) markEnqueueFailed(ctx context.Context, job domain.PoReadyJob, enqueueErr error) error {
	if r.audit == nil {
		return nil
	}
	return r.audit.MarkEnqueueFailed(ctx, job.ID, job.Attempt, "redis enqueue failed: "+enqueueErr.Error())
}

func (r PoReadyReader) insertQueued(ctx context.Context, job domain.PoReadyJob) error {
	if r.audit == nil {
		return nil
	}
	return r.audit.InsertQueued(ctx, audit.QueuedEmail{
		JobID:       job.ID,
		QueueName:   r.queueName,
		Attempt:     job.Attempt,
		UserID:      job.UserID,
		ToEmail:     job.User.Email,
		ReferenceID: job.OrderID,
		Feature:     audit.FeaturePoReady,
		Metadata: map[string]any{
			"order_id":     job.OrderID,
			"item_count":   len(job.Items),
			"remaining":    job.Remaining,
			"down_payment": job.DownPayment,
		},
	})
}
