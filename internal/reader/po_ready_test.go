package reader

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kyou-id/yukari/internal/audit"
	"github.com/kyou-id/yukari/internal/domain"
)

func TestPoReadyReaderBuildsAndEnqueuesJobPerOrder(t *testing.T) {
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.FixedZone("WIB", 7*60*60))
	store := &poReadyStoreStub{
		orders: []domain.PoReadyOrder{{
			User:        domain.User{ID: "123", Name: "Budi", Email: "budi@example.test", IsActive: true},
			OrderID:     "555",
			Remaining:   250000,
			DownPayment: 100000,
			ETA:         "Juli 2026",
		}},
		items: []domain.PoReadyItem{{ID: "item-1", Name: "Figure", Price: 350000, Quantity: 1}},
	}
	queue := &poReadyQueueStub{}
	auditLog := &poReadyAuditStub{}

	count, err := NewPoReady(store, queue, auditLog, "po-ready-jobs").Run(context.Background(), now)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if count != 1 || len(queue.jobs) != 1 {
		t.Fatalf("expected one job, count=%d jobs=%#v", count, queue.jobs)
	}
	job := queue.jobs[0]
	if job.ID != "po-ready-2026-07-01-order-555" || job.OrderID != "555" || job.Remaining != 250000 || job.Attempt != 1 {
		t.Fatalf("unexpected job: %#v", job)
	}
	if len(auditLog.queued) != 1 || auditLog.queued[0].ReferenceID != "555" || auditLog.queued[0].Feature != audit.FeaturePoReady {
		t.Fatalf("unexpected queued audit: %#v", auditLog.queued)
	}
	if auditLog.queued[0].Metadata["item_count"] != 1 || auditLog.queued[0].Metadata["remaining"] != 250000 {
		t.Fatalf("unexpected queued metadata: %#v", auditLog.queued[0].Metadata)
	}
}

func TestPoReadyReaderSkipsOrderWithoutReadyItems(t *testing.T) {
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	store := &poReadyStoreStub{
		orders: []domain.PoReadyOrder{{User: domain.User{ID: "123", Email: "budi@example.test", IsActive: true}, OrderID: "555", Remaining: 100000}},
		items:  nil,
	}
	queue := &poReadyQueueStub{}
	auditLog := &poReadyAuditStub{}

	count, err := NewPoReady(store, queue, auditLog, "po-ready-jobs").Run(context.Background(), now)
	if err != nil || count != 0 || len(queue.jobs) != 0 {
		t.Fatalf("expected no enqueue for empty order, count=%d jobs=%#v err=%v", count, queue.jobs, err)
	}
	if len(auditLog.queued) != 0 {
		t.Fatalf("expected no audit row for empty order, got %#v", auditLog.queued)
	}
}

func TestPoReadyReaderCompensatesAuditWhenEnqueueFails(t *testing.T) {
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	store := &poReadyStoreStub{
		orders: []domain.PoReadyOrder{{User: domain.User{ID: "123", Email: "budi@example.test", IsActive: true}, OrderID: "555", Remaining: 100000}},
		items:  []domain.PoReadyItem{{ID: "item-1"}},
	}
	queue := &poReadyQueueStub{err: errors.New("redis unavailable")}
	auditLog := &poReadyAuditStub{}

	count, err := NewPoReady(store, queue, auditLog, "po-ready-jobs").Run(context.Background(), now)
	if err == nil || count != 0 {
		t.Fatalf("expected enqueue error and zero count, count=%d err=%v", count, err)
	}
	if len(auditLog.enqueueFailures) != 1 {
		t.Fatalf("expected compensated audit, got %#v", auditLog.enqueueFailures)
	}
	if auditLog.enqueueFailures[0].jobID != "po-ready-2026-07-01-order-555" {
		t.Fatalf("unexpected compensation: %#v", auditLog.enqueueFailures[0])
	}
}

type poReadyStoreStub struct {
	orders []domain.PoReadyOrder
	items  []domain.PoReadyItem
}

func (s *poReadyStoreStub) PoReadyOrders(context.Context) ([]domain.PoReadyOrder, error) {
	return s.orders, nil
}
func (s *poReadyStoreStub) PoReadyItems(context.Context, string) ([]domain.PoReadyItem, error) {
	return s.items, nil
}

type poReadyQueueStub struct {
	jobs []domain.PoReadyJob
	err  error
}

func (q *poReadyQueueStub) EnqueuePoReadyTo(_ context.Context, _ string, job domain.PoReadyJob) error {
	if q.err != nil {
		return q.err
	}
	q.jobs = append(q.jobs, job)
	return nil
}

type poReadyAuditStub struct {
	queued          []audit.QueuedEmail
	enqueueFailures []enqueueFailureRecord
}

func (a *poReadyAuditStub) InsertQueued(_ context.Context, email audit.QueuedEmail) error {
	a.queued = append(a.queued, email)
	return nil
}
func (a *poReadyAuditStub) MarkEnqueueFailed(_ context.Context, jobID string, attempt int, reason string) error {
	a.enqueueFailures = append(a.enqueueFailures, enqueueFailureRecord{jobID: jobID, attempt: attempt, reason: reason})
	return nil
}
