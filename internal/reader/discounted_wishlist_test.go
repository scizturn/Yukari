package reader

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kyou-id/yukari/internal/audit"
	"github.com/kyou-id/yukari/internal/domain"
)

func TestDiscountedWishlistReaderBuildsAndEnqueuesJob(t *testing.T) {
	now := time.Date(2026, 6, 20, 0, 0, 0, 0, time.FixedZone("WIB", 7*60*60))
	store := &discountedWishlistStoreStub{
		users:      []domain.User{{ID: "123", Name: "Budi", Email: "budi@example.test", IsActive: true}},
		wishlisted: []domain.DiscountedWishlistItem{{ID: "wish-1", IsWishlisted: true}},
		fill:       []domain.DiscountedWishlistItem{{ID: "fill-1"}},
	}
	queue := &discountedWishlistQueueStub{}
	auditLog := &discountedWishlistAuditStub{}

	count, err := NewDiscountedWishlist(store, queue, auditLog, "discounted-jobs").Run(context.Background(), now)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if count != 1 || len(queue.jobs) != 1 {
		t.Fatalf("expected one job, count=%d jobs=%#v", count, queue.jobs)
	}
	job := queue.jobs[0]
	if job.ID != "discounted-wishlist-2026-06-20-user-123" || len(job.Items) != 2 || job.Attempt != 1 {
		t.Fatalf("unexpected job: %#v", job)
	}
	if len(auditLog.queued) != 1 || auditLog.queued[0].Metadata["wishlist_item_count"] != 1 || auditLog.queued[0].Metadata["fill_item_count"] != 1 {
		t.Fatalf("unexpected queued audit: %#v", auditLog.queued)
	}
	if len(auditLog.enqueueFailures) != 0 {
		t.Fatalf("unexpected enqueue failure audit: %#v", auditLog.enqueueFailures)
	}
}

func TestDiscountedWishlistReaderCompensatesAuditWhenEnqueueFails(t *testing.T) {
	now := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	store := &discountedWishlistStoreStub{
		users:      []domain.User{{ID: "123", Email: "budi@example.test", IsActive: true}},
		wishlisted: []domain.DiscountedWishlistItem{{ID: "wish-1", IsWishlisted: true}},
	}
	queue := &discountedWishlistQueueStub{err: errors.New("redis unavailable")}
	auditLog := &discountedWishlistAuditStub{}

	count, err := NewDiscountedWishlist(store, queue, auditLog, "discounted-jobs").Run(context.Background(), now)
	if err == nil || count != 0 {
		t.Fatalf("expected enqueue error and zero count, count=%d err=%v", count, err)
	}
	if len(auditLog.enqueueFailures) != 1 {
		t.Fatalf("expected compensated audit, got %#v", auditLog.enqueueFailures)
	}
	failure := auditLog.enqueueFailures[0]
	if failure.jobID != "discounted-wishlist-2026-06-20-user-123" || failure.attempt != 1 || failure.reason == "" {
		t.Fatalf("unexpected compensation: %#v", failure)
	}
}

func TestDiscountedWishlistReaderSkipsUserWithoutItems(t *testing.T) {
	now := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	store := &discountedWishlistStoreStub{users: []domain.User{{ID: "123", Email: "budi@example.test", IsActive: true}}}
	queue := &discountedWishlistQueueStub{}
	auditLog := &discountedWishlistAuditStub{}

	count, err := NewDiscountedWishlist(store, queue, auditLog, "discounted-jobs").Run(context.Background(), now)
	if err != nil || count != 0 || len(queue.jobs) != 0 {
		t.Fatalf("expected skip without enqueue, count=%d jobs=%#v err=%v", count, queue.jobs, err)
	}
	if len(auditLog.skipped) != 1 || auditLog.skipped[0].SkipReason != "no_discounted_wishlist_items" {
		t.Fatalf("unexpected skipped audit: %#v", auditLog.skipped)
	}
}

// The point of the fill index is that it is built once per run, not once per user. A
// regression here is invisible in the output — the emails come out identical — and only
// shows up as 32k extra queries against prod.
func TestDiscountedWishlistReaderBuildsTheFillIndexOnce(t *testing.T) {
	now := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	store := &discountedWishlistStoreStub{
		users: []domain.User{
			{ID: "1", Email: "a@example.test", IsActive: true},
			{ID: "2", Email: "b@example.test", IsActive: true},
			{ID: "3", Email: "c@example.test", IsActive: true},
		},
		wishlisted: []domain.DiscountedWishlistItem{{ID: "wish-1", IsWishlisted: true}},
		fill:       []domain.DiscountedWishlistItem{{ID: "fill-1"}},
	}

	count, err := NewDiscountedWishlist(store, &discountedWishlistQueueStub{}, &discountedWishlistAuditStub{}, "q").
		Run(context.Background(), now)
	if err != nil || count != 3 {
		t.Fatalf("expected three jobs, count=%d err=%v", count, err)
	}
	if store.fillIndexBuilds != 1 {
		t.Fatalf("fill index must be built once for the whole run, got %d builds for 3 users", store.fillIndexBuilds)
	}
}

type discountedWishlistStoreStub struct {
	users           []domain.User
	wishlisted      []domain.DiscountedWishlistItem
	fill            []domain.DiscountedWishlistItem
	fillIndexBuilds int
}

func (s *discountedWishlistStoreStub) DiscountedWishlistUsers(context.Context, time.Time) ([]domain.User, error) {
	return s.users, nil
}
func (s *discountedWishlistStoreStub) DiscountedWishlistItems(context.Context, string) ([]domain.DiscountedWishlistItem, error) {
	return s.wishlisted, nil
}
func (s *discountedWishlistStoreStub) DiscountedWishlistFillIndex(context.Context) (DiscountedWishlistFiller, error) {
	s.fillIndexBuilds++
	return discountedWishlistFillerStub{fill: s.fill}, nil
}

type discountedWishlistFillerStub struct {
	fill []domain.DiscountedWishlistItem
}

func (f discountedWishlistFillerStub) Fill(string) []domain.DiscountedWishlistItem {
	return f.fill
}

type discountedWishlistQueueStub struct {
	jobs []domain.DiscountedWishlistJob
	err  error
}

func (q *discountedWishlistQueueStub) EnqueueDiscountedWishlistTo(_ context.Context, _ string, job domain.DiscountedWishlistJob) error {
	if q.err != nil {
		return q.err
	}
	q.jobs = append(q.jobs, job)
	return nil
}

type enqueueFailureRecord struct {
	jobID   string
	attempt int
	reason  string
}

type discountedWishlistAuditStub struct {
	queued          []audit.QueuedEmail
	skipped         []audit.SkippedEmail
	enqueueFailures []enqueueFailureRecord
}

func (a *discountedWishlistAuditStub) InsertQueued(_ context.Context, email audit.QueuedEmail) error {
	a.queued = append(a.queued, email)
	return nil
}
func (a *discountedWishlistAuditStub) InsertSkipped(_ context.Context, email audit.SkippedEmail) error {
	a.skipped = append(a.skipped, email)
	return nil
}
func (a *discountedWishlistAuditStub) MarkEnqueueFailed(_ context.Context, jobID string, attempt int, reason string) error {
	a.enqueueFailures = append(a.enqueueFailures, enqueueFailureRecord{jobID: jobID, attempt: attempt, reason: reason})
	return nil
}
