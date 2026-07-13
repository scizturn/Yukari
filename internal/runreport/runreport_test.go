package runreport

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kyou-id/yukari/internal/audit"
)

func testRun(t *testing.T) *Run {
	t.Helper()
	started := time.Date(2026, 7, 13, 9, 2, 11, 0, time.FixedZone("WIB", 7*3600))
	run := New("po-ready", "PO Ready", started)
	run.QueueName = "po_ready_email_jobs"
	return run
}

func finish(run *Run, after time.Duration) time.Time {
	return run.StartedAt.Add(after)
}

// The readers are handed the recorder in place of *audit.Logger, and Yukari runs
// without a DB in local dev — so a nil logger must still count, not panic.
func TestAuditRecorderCountsSkipsWithoutDatabase(t *testing.T) {
	run := testRun(t)
	recorder := run.Audit(nil)
	ctx := context.Background()

	for _, reason := range []string{"already_sent", "already_sent", "empty_email"} {
		if err := recorder.InsertSkipped(ctx, audit.SkippedEmail{SkipReason: reason}); err != nil {
			t.Fatalf("insert skipped: %v", err)
		}
	}
	if err := recorder.MarkEnqueueFailed(ctx, "job-1", 1, "redis down"); err != nil {
		t.Fatalf("mark enqueue failed: %v", err)
	}
	if err := recorder.InsertQueued(ctx, audit.QueuedEmail{JobID: "job-2"}); err != nil {
		t.Fatalf("insert queued: %v", err)
	}

	if got := run.SkippedTotal(); got != 3 {
		t.Fatalf("expected 3 skips, got %d", got)
	}
	if run.enqueueFailed != 1 {
		t.Fatalf("expected 1 enqueue failure, got %d", run.enqueueFailed)
	}
}

func TestMessageReportsQueuedAndSkipReasons(t *testing.T) {
	run := testRun(t)
	run.Queued = 42
	recorder := run.Audit(nil)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_ = recorder.InsertSkipped(ctx, audit.SkippedEmail{SkipReason: "already_sent_or_issued_within_period"})
	}
	for i := 0; i < 2; i++ {
		_ = recorder.InsertSkipped(ctx, audit.SkippedEmail{SkipReason: "no_available_cart_items"})
	}

	message := run.Message(finish(run, 3400*time.Millisecond), nil)

	for _, want := range []string{
		"✅ [Yukari · PO Ready] Cron Run OK",
		"Run: 2026-07-13 09:02:11 WIB (3.4s)",
		"Queued: 42 job(s) → po_ready_email_jobs",
		"Skipped: 7",
		"• already_sent_or_issued_within_period: 5",
		"• no_available_cart_items: 2",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected message to contain %q, got:\n%s", want, message)
		}
	}

	// Most frequent reason first, so the message reads as a ranking.
	if strings.Index(message, "already_sent_or_issued_within_period") > strings.Index(message, "no_available_cart_items") {
		t.Fatalf("expected reasons ordered by count, got:\n%s", message)
	}
}

func TestMessageReportsFailureWithJobsQueuedSoFar(t *testing.T) {
	run := testRun(t)
	run.Queued = 12

	message := run.Message(finish(run, time.Second), errors.New("dial tcp: connection refused"))

	for _, want := range []string{
		"❌ [Yukari · PO Ready] Cron Run FAILED",
		"Queued before failure: 12 job(s) → po_ready_email_jobs",
		"Error: dial tcp: connection refused",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected message to contain %q, got:\n%s", want, message)
		}
	}
}

// A weekday-gated reader that stands down and a reader that found nobody both
// queue zero jobs. The report has to tell them apart, or a cron scheduled on the
// wrong day looks exactly like a quiet week.
func TestMessageSeparatesWeekdayStandDownFromEmptyRun(t *testing.T) {
	standDown := testRun(t)
	standDown.Note = "po-ready reader only runs on Saturday (today is Monday)"

	message := standDown.Message(finish(standDown, time.Second), nil)
	if !strings.Contains(message, "⏭️ [Yukari · PO Ready] Cron Run Skipped") {
		t.Fatalf("expected a skipped header, got:\n%s", message)
	}
	if !strings.Contains(message, "Reason: po-ready reader only runs on Saturday (today is Monday)") {
		t.Fatalf("expected the stand-down reason, got:\n%s", message)
	}
	if strings.Contains(message, "Queued") {
		t.Fatalf("a stand-down run should not report a queue count, got:\n%s", message)
	}

	empty := testRun(t)
	message = empty.Message(finish(empty, time.Second), nil)
	if !strings.Contains(message, "✅ [Yukari · PO Ready] Cron Run OK") {
		t.Fatalf("expected an OK header, got:\n%s", message)
	}
	if !strings.Contains(message, "No eligible user today.") {
		t.Fatalf("expected the empty-run note, got:\n%s", message)
	}
}

// A failing run still reports the error even when a weekday note is set — the
// note explains a quiet run, it must not swallow a loud one.
func TestMessageFailureWinsOverNote(t *testing.T) {
	run := testRun(t)
	run.Note = "po-ready reader only runs on Saturday (today is Monday)"

	message := run.Message(finish(run, time.Second), errors.New("boom"))

	if !strings.Contains(message, "Cron Run FAILED") || !strings.Contains(message, "Error: boom") {
		t.Fatalf("expected the failure to be reported, got:\n%s", message)
	}
}

func TestMessageCapsReasonLines(t *testing.T) {
	run := testRun(t)
	recorder := run.Audit(nil)
	ctx := context.Background()
	for i := 0; i < maxReasonLines+3; i++ {
		_ = recorder.InsertSkipped(ctx, audit.SkippedEmail{SkipReason: string(rune('a' + i))})
	}

	message := run.Message(finish(run, time.Second), nil)

	if got := strings.Count(message, "• "); got != maxReasonLines+1 {
		t.Fatalf("expected %d reason lines plus the overflow line, got %d:\n%s", maxReasonLines, got, message)
	}
	if !strings.Contains(message, "…and 3 more reason(s)") {
		t.Fatalf("expected the overflow line, got:\n%s", message)
	}
}
