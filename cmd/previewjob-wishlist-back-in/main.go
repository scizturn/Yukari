package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"
	"time"

	"github.com/kyou-id/yukari/internal/config"
	"github.com/kyou-id/yukari/internal/domain"
	"github.com/kyou-id/yukari/internal/repository"
	"github.com/kyou-id/yukari/internal/sqlfiles"
)

const maxItems = 5

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
	startAt := cutoff.AddDate(0, 0, -7)

	store, err := repository.OpenMySQLStore(cfg.DatabaseDSN, sqlfiles.NewLoader(cfg.SQLDir))
	if err != nil {
		log.Fatal(err)
	}
	rows, err := store.WishlistBackInUserItems(ctx, startAt, cutoff)
	if err != nil || len(rows) == 0 {
		log.Fatalf("read wishlist back in user items: count=%d err=%v", len(rows), err)
	}

	forced := strings.TrimSpace(os.Getenv("YUKARI_FORCE_USER"))
	user := rows[0].User
	if forced != "" {
		found := false
		for _, row := range rows {
			if row.User.ID == forced {
				user, found = row.User, true
				break
			}
		}
		if !found {
			log.Fatalf("forced user %s has no wishlist back in item in window", forced)
		}
	}

	var items []domain.WishlistBackInItem
	for _, row := range rows {
		if row.User.ID == user.ID && len(items) < maxItems {
			items = append(items, row.Item)
		}
	}
	companion, err := store.WishlistBackInCompanion(ctx, user.ID, items[0].ID)
	if err != nil {
		log.Fatal(err)
	}
	var recos []domain.WishlistBackInItem
	if companion.ID != "" {
		recos, err = store.WishlistBackInRecommendations(ctx, user.ID, companion.ID)
		if err != nil {
			log.Fatal(err)
		}
		if len(recos) < 6 { // need a full 6; else hide the section
			companion, recos = domain.WishlistBackInItem{}, nil
		}
	}
	job := domain.WishlistBackInJob{
		ID:     "preview-wishlist-back-in-" + cutoff.Format("2006-01-02") + "-user-" + user.ID,
		UserID: user.ID, Date: now, User: user, VoucherCode: "WBI-PREVIEW-14D",
		Items: items, CompanionItem: companion, RecoItems: recos, Attempt: 1,
	}
	payload, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	path := env("YUKARI_PREVIEW_JOB_PATH", "/Users/sleepyreinze/Dev/Email-Api/Makoto/templates/preview/wishlist-back-in-job.json")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		log.Fatal(err)
	}
	log.Printf("preview job written: path=%s user_id=%s items=%d", path, user.ID, len(items))
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
