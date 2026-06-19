package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	FeatureBirthdayVoucher    = "birthday_voucher"
	FeatureAnniversaryVoucher = "anniversary_voucher"
	FeatureLeftoverCart       = "leftover_cart"
	FeatureDiscountedWishlist = "discounted_wishlist"
	FeatureWinback            = "winback"
	FeatureWishlistBackIn     = "wishlist_back_in"
	ProviderKirimEmail        = "kirim.email"
)

type Logger struct {
	db *sql.DB
}

type QueuedEmail struct {
	JobID       string
	QueueName   string
	Attempt     int
	UserID      string
	ToEmail     string
	TemplateID  string
	Subject     string
	ActionURL   string
	Metadata    map[string]any
	ReferenceID string
	Feature     string
}

type SkippedEmail struct {
	JobID       string
	UserID      string
	ToEmail     string
	SkipReason  string
	ReferenceID string
	Feature     string
}

func Open(dsn string) (*Logger, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Logger{db: db}, nil
}

func (l *Logger) Close() error {
	if l == nil || l.db == nil {
		return nil
	}
	return l.db.Close()
}

func (l *Logger) InsertQueued(ctx context.Context, email QueuedEmail) error {
	if l == nil {
		return nil
	}
	attempt := email.Attempt
	if attempt <= 0 {
		attempt = 1
	}
	metadata, err := json.Marshal(email.Metadata)
	if err != nil {
		return err
	}
	_, err = l.db.ExecContext(ctx, `
INSERT INTO email_delivery_logs (
  feature,
  reference_type,
  reference_id,
  job_id,
  queue_name,
  attempt,
  user_id,
  to_email,
  template_id,
  subject,
  action_url,
  metadata,
  provider,
  status,
  queued_at
) VALUES (?, 'user', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'queued', NOW())
ON DUPLICATE KEY UPDATE
  queue_name = VALUES(queue_name),
  to_email = VALUES(to_email),
  template_id = VALUES(template_id),
  subject = VALUES(subject),
  action_url = VALUES(action_url),
  metadata = VALUES(metadata),
  status = 'queued',
  queued_at = COALESCE(queued_at, NOW()),
  updated_at = NOW()`,
		featureValue(email.Feature),
		referenceID(email.ReferenceID, email.UserID),
		email.JobID,
		email.QueueName,
		attempt,
		userIDValue(email.UserID),
		email.ToEmail,
		email.TemplateID,
		email.Subject,
		email.ActionURL,
		string(metadata),
		ProviderKirimEmail,
	)
	return err
}

func (l *Logger) InsertSkipped(ctx context.Context, email SkippedEmail) error {
	if l == nil {
		return nil
	}
	_, err := l.db.ExecContext(ctx, `
INSERT INTO email_delivery_logs (
  feature,
  reference_type,
  reference_id,
  job_id,
  attempt,
  user_id,
  to_email,
  provider,
  status,
  skip_reason,
  skipped_at
) VALUES (?, 'user', ?, ?, 1, ?, ?, ?, 'skipped', ?, NOW())
ON DUPLICATE KEY UPDATE
  status = 'skipped',
  skip_reason = VALUES(skip_reason),
  skipped_at = COALESCE(skipped_at, NOW()),
  updated_at = NOW()`,
		featureValue(email.Feature),
		referenceID(email.ReferenceID, email.UserID),
		email.JobID,
		userIDValue(email.UserID),
		email.ToEmail,
		ProviderKirimEmail,
		email.SkipReason,
	)
	return err
}

func (l *Logger) HasBirthdayVoucherEmailInYear(ctx context.Context, userID string, year int) (bool, error) {
	if l == nil {
		return false, nil
	}
	start := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(1, 0, 0)

	var exists bool
	err := l.db.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM email_delivery_logs
  WHERE feature = ?
    AND reference_type = 'user'
    AND reference_id = ?
    AND created_at >= ?
    AND created_at < ?
  LIMIT 1
)`,
		FeatureBirthdayVoucher,
		userID,
		start,
		end,
	).Scan(&exists)
	return exists, err
}

func (l *Logger) CountPreviousAnniversaryEmails(ctx context.Context, userID string, currentYear int) (int, error) {
	if l == nil {
		return 0, nil
	}
	end := time.Date(currentYear, 1, 1, 0, 0, 0, 0, time.UTC)

	var count int
	err := l.db.QueryRowContext(ctx, `
SELECT COUNT(DISTINCT YEAR(created_at))
FROM email_delivery_logs
WHERE feature = ?
  AND reference_type = 'user'
  AND reference_id = ?
  AND created_at < ?`,
		FeatureAnniversaryVoucher,
		userID,
		end,
	).Scan(&count)
	return count, err
}

func (l *Logger) HasAnniversaryVoucherEmailInYear(ctx context.Context, userID string, year int) (bool, error) {
	if l == nil {
		return false, nil
	}
	start := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(1, 0, 0)

	var exists bool
	err := l.db.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM email_delivery_logs
  WHERE feature = ?
    AND reference_type = 'user'
    AND reference_id = ?
    AND created_at >= ?
    AND created_at < ?
  LIMIT 1
)`,
		FeatureAnniversaryVoucher,
		userID,
		start,
		end,
	).Scan(&exists)
	return exists, err
}

func (l *Logger) HasLeftoverCartEmailSinceActivity(ctx context.Context, userID string, since time.Time) (bool, error) {
	if l == nil {
		return false, nil
	}
	var exists bool
	err := l.db.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM email_delivery_logs
  WHERE feature = ?
    AND reference_type = 'user'
    AND reference_id = ?
    AND status IN ('sent', 'queued', 'sending')
    AND created_at >= ?
  LIMIT 1
)`,
		FeatureLeftoverCart,
		userID,
		since,
	).Scan(&exists)
	return exists, err
}

func referenceID(referenceID string, userID string) string {
	if referenceID != "" {
		return referenceID
	}
	return userID
}

func featureValue(feature string) string {
	if feature == "" {
		return FeatureBirthdayVoucher
	}
	return feature
}

func userIDValue(userID string) any {
	if userID == "" {
		return nil
	}
	parsed, err := strconv.ParseUint(userID, 10, 64)
	if err != nil {
		return nil
	}
	return parsed
}
