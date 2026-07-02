package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/kyou-id/yukari/internal/config"
	"github.com/kyou-id/yukari/internal/domain"
	"github.com/kyou-id/yukari/internal/reader"
	"github.com/kyou-id/yukari/internal/repository"
	"github.com/kyou-id/yukari/internal/sqlfiles"
)

// winbackReadyTarget and winbackPastLimit mirror the reader's constants so the
// preview job matches what `yukari winback` would actually enqueue.
const (
	winbackReadyTarget = 12
	winbackPastLimit   = 3
)

func main() {
	ctx := context.Background()
	cfg := config.Load()
	if strings.TrimSpace(cfg.DatabaseDSN) == "" {
		log.Fatal("DATABASE_DSN is required (set OLD_DATABASE_* in .env)")
	}

	location, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		log.Fatalf("load timezone: %v", err)
	}
	now := time.Now().In(location)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)

	userID := env("YUKARI_FORCE_USER", "31877")
	outputPath := env("YUKARI_PREVIEW_JOB_PATH", "/Users/sleepyreinze/Dev/Email-Api/Makoto/templates/preview/winback-job.json")

	user, err := findUserByID(ctx, cfg.DatabaseDSN, userID)
	if err != nil {
		log.Fatalf("find user %s: %v", userID, err)
	}
	user.IsActive = true

	store, err := repository.OpenMySQLStore(cfg.DatabaseDSN, sqlfiles.NewLoader(cfg.SQLDir))
	if err != nil {
		log.Fatalf("open store: %v", err)
	}

	wishlist, err := store.WishlistWinback(ctx, user.ID)
	if err != nil {
		log.Fatalf("read wishlist: %v", err)
	}
	fillItems, err := store.WinbackFillItems(ctx)
	if err != nil {
		log.Fatalf("read winback fill items: %v", err)
	}
	wishlist = reader.FillWishlistReadyTo(wishlist, winbackReadyTarget, fillItems)

	popular, err := store.Popular(ctx)
	if err != nil {
		log.Fatalf("read popular: %v", err)
	}

	orders, err := store.HistoricalOrders(ctx, user.ID)
	if err != nil {
		log.Fatalf("read historical orders: %v", err)
	}
	historicalItems := recentHistoricalItems(orders, start, winbackPastLimit)
	var historicalItem domain.HistoricalItem
	if len(historicalItems) > 0 {
		historicalItem = historicalItems[len(historicalItems)-1] // most recent = last (list is oldest → latest)
	}

	job := domain.WinbackJob{
		ID:              "preview-winback-" + now.Format("2006-01-02-150405") + "-user-" + user.ID,
		UserID:          user.ID,
		Date:            now,
		User:            user,
		VoucherCode:     "KANGEN" + user.ID,
		WishlistItems:   wishlist,
		HistoricalItem:  historicalItem,
		HistoricalItems: historicalItems,
		PopularItems:    popular,
		Attempt:         1,
	}

	payload, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(outputPath, payload, 0o600); err != nil {
		log.Fatal(err)
	}
	log.Printf("preview job written: path=%s user_id=%s wishlist=%d historical=%d popular=%d",
		outputPath, user.ID, len(wishlist), len(historicalItems), len(popular))
}

// recentHistoricalItems mirrors reader.recentHistoricalItems: input orders are
// oldest-first (historical_orders.sql orders by created_at ASC); take the last
// limit (the most recent) and keep them oldest → latest.
func recentHistoricalItems(orders []domain.HistoricalItem, start time.Time, limit int) []domain.HistoricalItem {
	if len(orders) == 0 || limit <= 0 {
		return nil
	}
	from := len(orders) - limit
	if from < 0 {
		from = 0
	}
	recent := orders[from:]
	items := make([]domain.HistoricalItem, 0, len(recent))
	for i := 0; i < len(recent); i++ {
		item := recent[i]
		item.DaysAgo = int(start.Sub(item.OrderDate).Hours() / 24)
		items = append(items, item)
	}
	return items
}

func findUserByID(ctx context.Context, dsn string, userID string) (domain.User, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return domain.User{}, err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return domain.User{}, err
	}

	var user domain.User
	var birthday sql.NullTime
	err = db.QueryRowContext(ctx, `
SELECT user_id, name, email, birthdate, is_confirmed
FROM users
WHERE CAST(user_id AS CHAR) = ?
LIMIT 1`, userID).Scan(&user.ID, &user.Name, &user.Email, &birthday, &user.IsActive)
	if err != nil {
		return domain.User{}, err
	}
	if birthday.Valid {
		user.Birthday = birthday.Time
	}
	return user, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
