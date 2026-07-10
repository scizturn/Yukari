package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/kyou-id/yukari/internal/config"
	"github.com/kyou-id/yukari/internal/domain"
	"github.com/kyou-id/yukari/internal/repository"
	"github.com/kyou-id/yukari/internal/sqlfiles"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()
	if strings.TrimSpace(cfg.DatabaseDSN) == "" {
		log.Fatal("OLD_DATABASE_* env vars are required")
	}

	location, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		log.Fatalf("load timezone: %v", err)
	}
	now := time.Now().In(location)
	userID := env("YUKARI_FORCE_USER", "")
	outputPath := env("YUKARI_PREVIEW_JOB_PATH", "/Users/sleepyreinze/Dev/Email-Api/Makoto/templates/preview/po-ready-job.json")

	store, err := repository.OpenMySQLStore(cfg.DatabaseDSN, sqlfiles.NewLoader(cfg.SQLDir))
	if err != nil {
		log.Fatalf("open store: %v", err)
	}

	job, err := buildPreviewJob(ctx, store, cfg.DatabaseDSN, now, userID)
	if err != nil {
		log.Fatal(err)
	}

	payload, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(outputPath, payload, 0o600); err != nil {
		log.Fatal(err)
	}
	log.Printf("preview job written: path=%s user_id=%s items=%d", outputPath, job.UserID, len(job.Items))
}

// previewWindow mirrors the reader's detection window so a preview with no forced
// user reflects what the cron would actually pick up.
const previewWindow = 7 * 24 * time.Hour

// previewMaxItems mirrors the reader's per-user cap.
const previewMaxItems = 5

// buildPreviewJob previews the given user, or — with no YUKARI_FORCE_USER — the
// first user the live eligibility query returns.
func buildPreviewJob(ctx context.Context, store *repository.MySQLStore, dsn string, now time.Time, userID string) (domain.PoReadyJob, error) {
	job := domain.PoReadyJob{Date: now, Attempt: 1}

	if strings.TrimSpace(userID) == "" {
		rows, err := store.PoReadyUserItems(ctx, now.Add(-previewWindow), now)
		if err != nil {
			return job, err
		}
		if len(rows) == 0 {
			return job, fmt.Errorf("no eligible po-ready users found; set YUKARI_FORCE_USER to preview a specific user")
		}
		job.User = rows[0].User
		for _, row := range rows {
			if row.User.ID != job.User.ID || len(job.Items) >= previewMaxItems {
				break
			}
			job.Items = append(job.Items, row.Item)
		}
	} else {
		user, err := lookupUserByID(ctx, dsn, userID)
		if err != nil {
			return job, err
		}
		items, err := store.PoReadyForcedItems(ctx, userID)
		if err != nil {
			return job, err
		}
		if len(items) == 0 {
			return job, fmt.Errorf("user %s has no ready wishlist items to preview", userID)
		}
		job.User, job.Items = user, items
	}

	job.UserID = job.User.ID
	job.ID = fmt.Sprintf("preview-po-ready-%s-user-%s", now.Format("2006-01-02-150405"), job.UserID)
	return job, nil
}

// lookupUserByID fetches a user's identity by exact user_id (for previewing a
// specific user who has no conversion in the detection window).
func lookupUserByID(ctx context.Context, dsn, id string) (domain.User, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return domain.User{}, err
	}
	defer db.Close()
	var user domain.User
	err = db.QueryRowContext(ctx,
		`SELECT CAST(user_id AS CHAR), name, email FROM users WHERE CAST(user_id AS CHAR) = ? LIMIT 1`, id,
	).Scan(&user.ID, &user.Name, &user.Email)
	user.IsActive = true
	return user, err
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
