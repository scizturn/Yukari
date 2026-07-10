package reader

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kyou-id/yukari/internal/audit"
	"github.com/kyou-id/yukari/internal/domain"
)

// poReadyMaxItems caps how many readied items one user's email lists. Overflow is
// not lost: the per-(user, item) dedup suppresses only the items that were sent,
// so the rest surface on a later run until the window ages them out.
const poReadyMaxItems = 5

// poReadyWindow is how far back a PO->ready conversion still counts. The reader
// runs weekly, so 7 days tiles exactly: each run sees the week just gone.
//
// That means po-ready has NO carry-over, unlike wishlist-back-in. With a weekly
// cron the window length *is* the carry-over depth — an item that loses the
// 5-item cap this Saturday has its conversion row fall outside next Saturday's
// window, and nothing else remembers that it was ever eligible. This is a
// deliberate trade: measured against prod, only 75 of 3,929 users (1.9%) have
// more than 5 conversions in a week, and they still get their 5 newest. In
// exchange the news is never more than 7 days stale. Widen this and carry-over
// comes back (21 days = 3 Saturdays), at the cost of announcing 3-week-old news.
//
// A skipped run therefore loses that week permanently — Yukari has no catch-up.
const poReadyWindow = 7 * 24 * time.Hour

type PoReadyStore interface {
	PoReadyUserItems(ctx context.Context, startAt, endAt time.Time) ([]domain.PoReadyUserItem, error)
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
	// Weekly, on Saturday. The window tiles exactly one week (see poReadyWindow),
	// so a daily cron would re-scan the same six days and a missed weekday would
	// shift the tiling — the guard makes the cadence a property of the code rather
	// than of whatever is typed into Coolify. `now` is already in WIB (see
	// cmd/yukari/po_ready.go), matching wishlist-back-in's Friday guard.
	//
	// Which campaign wins an item that trips both triggers is NOT decided here.
	// It is decided by the event gate in the two SQL files: a conversion row sends
	// the item to po-ready, a 0->>0 restock row sends it to wishlist-back-in, and
	// wishlist-back-in explicitly stands down on items po-ready is about to claim.
	// Do not reintroduce a dependency on which cron fires first.
	if now.Weekday() != time.Saturday {
		return 0, nil
	}

	rows, err := r.store.PoReadyUserItems(ctx, now.Add(-poReadyWindow), now)
	if err != nil {
		return 0, err
	}

	// rows arrive grouped by user, newest conversion first (see the SQL ORDER BY).
	// Walk them into one job per user, capping each user's list to the 5 most
	// recently readied items.
	enqueued := 0
	i := 0
	for i < len(rows) {
		user := rows[i].User
		var items []domain.PoReadyItem
		for i < len(rows) && rows[i].User.ID == user.ID {
			if len(items) < poReadyMaxItems {
				items = append(items, rows[i].Item)
			}
			i++
		}
		if len(items) == 0 {
			continue
		}

		itemIDs := make([]string, len(items))
		for j, item := range items {
			itemIDs[j] = item.ID
		}

		job := domain.PoReadyJob{
			ID:      fmt.Sprintf("po-ready-%s-user-%s", now.Format("2006-01-02"), user.ID),
			UserID:  user.ID,
			Date:    now,
			User:    user,
			Items:   items,
			Attempt: 1,
		}
		if err := r.insertQueued(ctx, job, itemIDs); err != nil {
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

func (r PoReadyReader) insertQueued(ctx context.Context, job domain.PoReadyJob, itemIDs []string) error {
	if r.audit == nil {
		return nil
	}
	// item_ids feeds the per-(user, item) dedup shared with wishlist-back-in.
	return r.audit.InsertQueued(ctx, audit.QueuedEmail{
		JobID:       job.ID,
		QueueName:   r.queueName,
		Attempt:     job.Attempt,
		UserID:      job.UserID,
		ToEmail:     job.User.Email,
		ReferenceID: job.UserID,
		Feature:     audit.FeaturePoReady,
		Metadata: map[string]any{
			"item_ids":   itemIDs,
			"item_count": len(job.Items),
		},
	})
}
