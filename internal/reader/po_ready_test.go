package reader

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/kyou-id/yukari/internal/audit"
	"github.com/kyou-id/yukari/internal/domain"
)

func poReadyRow(userID, itemID string) domain.PoReadyUserItem {
	return domain.PoReadyUserItem{
		User: domain.User{ID: userID, Name: "Budi Santoso", Email: userID + "@example.test", IsActive: true},
		Item: domain.PoReadyItem{ID: itemID, Name: "Figure " + itemID, Price: 350000},
	}
}

func TestPoReadyReaderBuildsOneJobPerUser(t *testing.T) {
	now := time.Date(2026, 7, 11, 9, 0, 0, 0, time.FixedZone("WIB", 7*60*60))
	store := &poReadyStoreStub{rows: []domain.PoReadyUserItem{
		poReadyRow("123", "item-1"),
		poReadyRow("123", "item-2"),
		poReadyRow("456", "item-9"),
	}}
	queue := &poReadyQueueStub{}
	auditLog := &poReadyAuditStub{}

	count, err := NewPoReady(store, queue, auditLog, "po-ready-jobs").Run(context.Background(), now)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if count != 2 || len(queue.jobs) != 2 {
		t.Fatalf("expected two jobs, count=%d jobs=%#v", count, queue.jobs)
	}

	first := queue.jobs[0]
	if first.ID != "po-ready-2026-07-11-user-123" || first.UserID != "123" || first.Attempt != 1 {
		t.Fatalf("unexpected first job: %#v", first)
	}
	if len(first.Items) != 2 || first.Items[0].ID != "item-1" || first.Items[1].ID != "item-2" {
		t.Fatalf("expected both of user 123's items in order, got %#v", first.Items)
	}
	if queue.jobs[1].UserID != "456" || len(queue.jobs[1].Items) != 1 {
		t.Fatalf("unexpected second job: %#v", queue.jobs[1])
	}

	if len(auditLog.queued) != 2 {
		t.Fatalf("expected one audit row per user, got %#v", auditLog.queued)
	}
	row := auditLog.queued[0]
	if row.ReferenceID != "123" || row.Feature != audit.FeaturePoReady {
		t.Fatalf("audit row must reference the user: %#v", row)
	}
	// item_ids is what the cross-feature dedup in SQL matches on; without it the
	// same user is re-emailed about the same item on the next run.
	if got := row.Metadata["item_ids"]; !reflect.DeepEqual(got, []string{"item-1", "item-2"}) {
		t.Fatalf("unexpected item_ids metadata: %#v", got)
	}
	if row.Metadata["item_count"] != 2 {
		t.Fatalf("unexpected item_count metadata: %#v", row.Metadata)
	}
}

func TestPoReadyReaderCapsItemsPerUser(t *testing.T) {
	now := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	rows := []domain.PoReadyUserItem{}
	for _, id := range []string{"i1", "i2", "i3", "i4", "i5", "i6", "i7"} {
		rows = append(rows, poReadyRow("123", id))
	}
	store := &poReadyStoreStub{rows: rows}
	queue := &poReadyQueueStub{}
	auditLog := &poReadyAuditStub{}

	if _, err := NewPoReady(store, queue, auditLog, "po-ready-jobs").Run(context.Background(), now); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(queue.jobs) != 1 {
		t.Fatalf("expected a single job, got %#v", queue.jobs)
	}
	if got := len(queue.jobs[0].Items); got != poReadyMaxItems {
		t.Fatalf("expected items capped at %d, got %d", poReadyMaxItems, got)
	}
	// Only the sent items may be recorded — recording the overflow would suppress
	// items the user was never told about.
	if got := auditLog.queued[0].Metadata["item_ids"]; !reflect.DeepEqual(got, []string{"i1", "i2", "i3", "i4", "i5"}) {
		t.Fatalf("overflow items must not be marked as sent: %#v", got)
	}
}

// Wishlist-back-in runs Friday and carries a voucher; po-ready runs Saturday and
// does not. They share one dedup, so whichever runs first claims the ~212 users
// they overlap on. A po-ready run on any other day would silently take those
// vouchers away, so it must not query at all.
func TestPoReadyReaderRunsOnlyOnSaturday(t *testing.T) {
	wib := time.FixedZone("WIB", 7*60*60)
	for _, day := range []int{5, 6, 7, 8, 9, 10, 12} { // 11 July 2026 is the Saturday
		now := time.Date(2026, 7, day, 9, 0, 0, 0, wib)
		store := &poReadyStoreStub{rows: []domain.PoReadyUserItem{poReadyRow("123", "item-1")}}
		queue := &poReadyQueueStub{}
		auditLog := &poReadyAuditStub{}

		count, err := NewPoReady(store, queue, auditLog, "q").Run(context.Background(), now)
		if err != nil || count != 0 {
			t.Fatalf("%s: expected a no-op, count=%d err=%v", now.Weekday(), count, err)
		}
		if !store.endAt.IsZero() {
			t.Fatalf("%s: reader must not even query the store", now.Weekday())
		}
		if len(queue.jobs) != 0 || len(auditLog.queued) != 0 {
			t.Fatalf("%s: reader must not enqueue or claim any item", now.Weekday())
		}
	}
}

// wishlist_back_in_user_items.sql hardcodes `INTERVAL 7 DAY` to stand down on the
// items this campaign is about to claim. If this constant moves and that literal
// does not, items converted inside the gap are rejected by back-in and missed by
// po-ready — announced by nobody, with no error anywhere.
func TestPoReadyWindowMatchesTheBackInGate(t *testing.T) {
	if poReadyWindow != 7*24*time.Hour {
		t.Fatalf("poReadyWindow is %v; update the INTERVAL 7 DAY gate in wishlist_back_in_user_items.sql to match, then fix this test", poReadyWindow)
	}
}

func TestPoReadyReaderQueriesTheConfiguredWindow(t *testing.T) {
	now := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	store := &poReadyStoreStub{}

	if _, err := NewPoReady(store, &poReadyQueueStub{}, &poReadyAuditStub{}, "q").Run(context.Background(), now); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !store.endAt.Equal(now) {
		t.Fatalf("expected endAt=%v, got %v", now, store.endAt)
	}
	if want := now.Add(-poReadyWindow); !store.startAt.Equal(want) {
		t.Fatalf("expected startAt=%v, got %v", want, store.startAt)
	}
}

func TestPoReadyReaderEnqueuesNothingWhenNoConversions(t *testing.T) {
	now := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	queue := &poReadyQueueStub{}
	auditLog := &poReadyAuditStub{}

	count, err := NewPoReady(&poReadyStoreStub{}, queue, auditLog, "po-ready-jobs").Run(context.Background(), now)
	if err != nil || count != 0 || len(queue.jobs) != 0 {
		t.Fatalf("expected no enqueue, count=%d jobs=%#v err=%v", count, queue.jobs, err)
	}
	if len(auditLog.queued) != 0 {
		t.Fatalf("expected no audit row, got %#v", auditLog.queued)
	}
}

func TestPoReadyReaderCompensatesAuditWhenEnqueueFails(t *testing.T) {
	now := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	store := &poReadyStoreStub{rows: []domain.PoReadyUserItem{poReadyRow("123", "item-1")}}
	queue := &poReadyQueueStub{err: errors.New("redis unavailable")}
	auditLog := &poReadyAuditStub{}

	count, err := NewPoReady(store, queue, auditLog, "po-ready-jobs").Run(context.Background(), now)
	if err == nil || count != 0 {
		t.Fatalf("expected enqueue error and zero count, count=%d err=%v", count, err)
	}
	if len(auditLog.enqueueFailures) != 1 {
		t.Fatalf("expected compensated audit, got %#v", auditLog.enqueueFailures)
	}
	if auditLog.enqueueFailures[0].jobID != "po-ready-2026-07-11-user-123" {
		t.Fatalf("unexpected compensation: %#v", auditLog.enqueueFailures[0])
	}
}

type poReadyStoreStub struct {
	rows           []domain.PoReadyUserItem
	startAt, endAt time.Time
}

func (s *poReadyStoreStub) PoReadyUserItems(_ context.Context, startAt, endAt time.Time) ([]domain.PoReadyUserItem, error) {
	s.startAt, s.endAt = startAt, endAt
	return s.rows, nil
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
