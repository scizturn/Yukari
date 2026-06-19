package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"os"
	"strings"
	"time"

	"github.com/kyou-id/yukari/internal/config"
	"github.com/kyou-id/yukari/internal/domain"
	"github.com/kyou-id/yukari/internal/repository"
	"github.com/kyou-id/yukari/internal/sqlfiles"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()
	location, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		log.Fatal(err)
	}
	now := time.Now().In(location)
	cutoff := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	if now.Weekday() != time.Friday {
		days := (int(now.Weekday()) - int(time.Friday) + 7) % 7
		cutoff = cutoff.AddDate(0, 0, -days)
	}
	startAt, err := time.ParseInLocation("2006-01-02", cfg.WishlistBackInStartAt, location)
	if err != nil {
		log.Fatal(err)
	}
	store, err := repository.OpenMySQLStore(cfg.DatabaseDSN, sqlfiles.NewLoader(cfg.SQLDir))
	if err != nil {
		log.Fatal(err)
	}
	var item domain.WishlistBackInItem
	forced := strings.TrimSpace(os.Getenv("YUKARI_FORCE_USER"))
	if forced != "" {
		item, err = store.WishlistBackInPreviewItem(ctx, forced, startAt, cutoff)
		if errors.Is(err, sql.ErrNoRows) {
			previewStart := cutoff.AddDate(-1, 0, 0)
			log.Printf("no current-window item for forced user %s; searching historical restocks since %s for preview only", forced, previewStart.Format("2006-01-02"))
			item, err = store.WishlistBackInPreviewItem(ctx, forced, previewStart, cutoff)
		}
		if err != nil {
			log.Fatalf("no eligible wishlist back in item for forced user %s: %v", forced, err)
		}
	} else {
		items, readErr := store.WishlistBackInItems(ctx, startAt, cutoff)
		if readErr != nil || len(items) == 0 {
			log.Fatalf("read wishlist back in items: count=%d err=%v", len(items), readErr)
		}
		item = items[0]
	}
	users, err := store.WishlistBackInUsers(ctx, item.ID)
	if err != nil || len(users) == 0 {
		log.Fatalf("read wishlist users: count=%d err=%v", len(users), err)
	}
	user := users[0]
	if forced != "" {
		found := false
		for _, candidate := range users {
			if candidate.ID == forced {
				user = candidate
				found = true
				break
			}
		}
		if !found {
			log.Fatalf("forced user %s did not wishlist item %s", forced, item.ID)
		}
	}
	companion, err := store.WishlistBackInCompanion(ctx, user.ID, item.ID)
	if err != nil {
		log.Fatal(err)
	}
	job := domain.WishlistBackInJob{
		ID:     "preview-wishlist-back-in-" + cutoff.Format("2006-01-02") + "-user-" + user.ID,
		UserID: user.ID, Date: now, User: user, VoucherCode: "WBI-PREVIEW-14D",
		Item: item, CompanionItem: companion, Attempt: 1,
	}
	payload, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	path := env("YUKARI_PREVIEW_JOB_PATH", "/Users/sleepyreinze/Dev/Email-Api/Makoto/templates/preview/wishlist-back-in-job.json")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		log.Fatal(err)
	}
	log.Printf("preview job written: path=%s user_id=%s item_id=%s", path, user.ID, item.ID)
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
