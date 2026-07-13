// Package runreport turns one Yukari cron invocation into a single Discord
// message: what was queued, what was skipped and why, and whether the run died.
//
// The counts are collected by wrapping the audit logger rather than by changing
// every reader's Run signature. The wrapper sees exactly the rows that landed in
// email_delivery_logs, which is the same spine Makoto reads later — so the report
// cannot claim a job that was never recorded.
package runreport

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kyou-id/yukari/internal/audit"
)

// maxReasonLines caps how many distinct skip reasons the message lists. A run
// that skips thousands of users still only has a handful of reasons, so this is
// a guard against a pathological campaign, not a routine truncation.
const maxReasonLines = 10

// Run accumulates everything one campaign invocation wants to report. A campaign
// sets Title/QueueName and Queued; the audit wrapper fills in the skip counts.
type Run struct {
	Campaign  string // subcommand, e.g. "po-ready"
	Title     string // display name, e.g. "PO Ready"
	QueueName string
	StartedAt time.Time

	// Queued is the reader's own count of jobs pushed to Redis.
	Queued int

	// Note explains a run that did nothing by design — the weekday guards. Set it
	// and the message says "skipped" instead of "OK with 0 jobs", which otherwise
	// look identical from outside and hide a cron scheduled on the wrong day.
	Note string

	skipped       map[string]int
	enqueueFailed int
}

func New(campaign, title string, startedAt time.Time) *Run {
	return &Run{
		Campaign:  campaign,
		Title:     title,
		StartedAt: startedAt,
		skipped:   map[string]int{},
	}
}

// Audit wraps the real audit logger so every InsertSkipped/MarkEnqueueFailed the
// readers make is counted on the way through. logger may be nil — audit.Logger's
// methods are nil-safe, which is how the readers already run without a DB.
func (r *Run) Audit(logger *audit.Logger) *AuditRecorder {
	return &AuditRecorder{Logger: logger, run: r}
}

// AuditRecorder satisfies every reader's *AuditLogger interface: the embedded
// *audit.Logger supplies the read methods (HasBirthdayVoucherEmailInYear and
// friends), and the three write methods below are intercepted for counting.
type AuditRecorder struct {
	*audit.Logger
	run *Run
}

func (a *AuditRecorder) InsertSkipped(ctx context.Context, email audit.SkippedEmail) error {
	if err := a.Logger.InsertSkipped(ctx, email); err != nil {
		return err
	}
	reason := strings.TrimSpace(email.SkipReason)
	if reason == "" {
		reason = "unknown"
	}
	a.run.skipped[reason]++
	return nil
}

func (a *AuditRecorder) MarkEnqueueFailed(ctx context.Context, jobID string, attempt int, reason string) error {
	if err := a.Logger.MarkEnqueueFailed(ctx, jobID, attempt, reason); err != nil {
		return err
	}
	a.run.enqueueFailed++
	return nil
}

// SkippedTotal is the number of users audited as skipped during the run.
func (r *Run) SkippedTotal() int {
	total := 0
	for _, count := range r.skipped {
		total += count
	}
	return total
}

// Message renders the Discord post for a finished run. err is the reader's error,
// or nil.
func (r *Run) Message(finishedAt time.Time, err error) string {
	var b strings.Builder

	switch {
	case err != nil:
		fmt.Fprintf(&b, "❌ [Yukari · %s] Cron Run FAILED\n", r.Title)
	case r.Note != "":
		fmt.Fprintf(&b, "⏭️ [Yukari · %s] Cron Run Skipped\n", r.Title)
	default:
		fmt.Fprintf(&b, "✅ [Yukari · %s] Cron Run OK\n", r.Title)
	}

	elapsed := finishedAt.Sub(r.StartedAt).Round(100 * time.Millisecond)
	fmt.Fprintf(&b, "Run: %s (%s)\n", r.StartedAt.Format("2006-01-02 15:04:05 MST"), elapsed)

	if r.Note != "" && err == nil {
		fmt.Fprintf(&b, "Reason: %s", r.Note)
		return b.String()
	}

	label := "Queued"
	if err != nil {
		label = "Queued before failure"
	}
	fmt.Fprintf(&b, "%s: %d job(s) → %s\n", label, r.Queued, r.QueueName)

	if total := r.SkippedTotal(); total > 0 {
		fmt.Fprintf(&b, "Skipped: %d\n", total)
		for _, line := range r.reasonLines() {
			fmt.Fprintf(&b, "  • %s\n", line)
		}
	}

	if r.enqueueFailed > 0 {
		fmt.Fprintf(&b, "Enqueue failed: %d (audit row marked failed)\n", r.enqueueFailed)
	}

	if err != nil {
		fmt.Fprintf(&b, "Error: %v\n", err)
	} else if r.Queued == 0 && r.SkippedTotal() == 0 {
		b.WriteString("No eligible user today.\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

// reasonLines renders the skip reasons most-frequent first, ties broken by name
// so the message is stable between runs with the same shape.
func (r *Run) reasonLines() []string {
	reasons := make([]string, 0, len(r.skipped))
	for reason := range r.skipped {
		reasons = append(reasons, reason)
	}
	sort.Slice(reasons, func(i, j int) bool {
		if r.skipped[reasons[i]] != r.skipped[reasons[j]] {
			return r.skipped[reasons[i]] > r.skipped[reasons[j]]
		}
		return reasons[i] < reasons[j]
	})

	lines := make([]string, 0, len(reasons))
	for i, reason := range reasons {
		if i == maxReasonLines {
			lines = append(lines, fmt.Sprintf("…and %d more reason(s)", len(reasons)-maxReasonLines))
			break
		}
		lines = append(lines, fmt.Sprintf("%s: %d", reason, r.skipped[reason]))
	}
	return lines
}
