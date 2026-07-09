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
	"github.com/kyou-id/yukari/internal/reader"
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

	userID := env("YUKARI_FORCE_USER", "31877")
	outputPath := env("YUKARI_PREVIEW_JOB_PATH", fmt.Sprintf("/Users/sleepyreinze/Dev/anniversary-%s-job.json", userID))

	user, years, err := findUserWithYears(ctx, cfg.DatabaseDSN, userID)
	if err != nil {
		log.Fatal(err)
	}
	user.IsActive = true

	store, err := repository.OpenMySQLStore(cfg.DatabaseDSN, sqlfiles.NewLoader(cfg.SQLDir))
	if err != nil {
		log.Fatalf("open store: %v", err)
	}

	wishlist, err := store.WishlistAnniversary(ctx, user.ID)
	if err != nil {
		log.Fatalf("read wishlist: %v", err)
	}
	fyp, err := store.FYP(ctx, user.ID)
	if err != nil {
		log.Fatalf("read fyp: %v", err)
	}
	popular, err := store.Popular(ctx)
	if err != nil {
		log.Fatalf("read popular: %v", err)
	}
	wishlist = reader.FillWishlistToSix(wishlist, fyp, popular)

	orders, err := store.HistoricalOrders(ctx, user.ID)
	if err != nil {
		log.Fatalf("read historical orders: %v", err)
	}

	var historicalItem domain.HistoricalItem
	if len(orders) > 0 {
		historicalItem = orders[0]
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		historicalItem.DaysAgo = int(start.Sub(historicalItem.OrderDate).Hours() / 24)
	}

	popularFYP := make([]domain.FYPItem, 0, len(popular))
	for _, p := range popular {
		popularFYP = append(popularFYP, p)
	}

	job := domain.AnniversaryJob{
		ID:             fmt.Sprintf("preview-anniversary-%s-user-%s", now.Format("2006-01-02-150405"), user.ID),
		UserID:         user.ID,
		Date:           now,
		User:           user,
		Years:          years,
		VoucherCode:    previewVoucherCode(ctx, cfg.DatabaseDSN, user.ID, "ANV%", "", "ANVPREVIEW2026"),
		HistoricalItem: historicalItem,
		WishlistItems:  wishlist,
		PopularItems:   popularFYP,
		Attempt:        1,
	}

	payload, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(outputPath, payload, 0o600); err != nil {
		log.Fatal(err)
	}
	log.Printf("anniversary preview job written: path=%s user_id=%s years=%d wishlist=%d historical=%s",
		outputPath, user.ID, years, len(wishlist), historicalItem.Name)
}

func findUserWithYears(ctx context.Context, dsn string, userID string) (domain.User, int, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return domain.User{}, 0, err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return domain.User{}, 0, err
	}

	var user domain.User
	var birthday sql.NullTime
	var years int
	err = db.QueryRowContext(ctx, `
SELECT user_id, name, email, birthdate, email_verified_at IS NOT NULL, TIMESTAMPDIFF(YEAR, created_at, NOW())
FROM users
WHERE CAST(user_id AS CHAR) = ?
LIMIT 1`, userID).Scan(&user.ID, &user.Name, &user.Email, &birthday, &user.IsActive, &years)
	if err != nil {
		return domain.User{}, 0, fmt.Errorf("user %s not found: %w", userID, err)
	}
	if birthday.Valid {
		user.Birthday = birthday.Time
	}
	return user, years, nil
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

// previewVoucherCode returns the user's live, unused voucher for this campaign so
// the preview shows the code they would really get. Falls back to a stub when
// they have none. Read-only -- previews never mint.
func previewVoucherCode(ctx context.Context, dsn, userID, codeLike, codeNotLike, stub string) string {
	code, amount, found, err := repository.LiveVoucherCode(ctx, dsn, userID, codeLike, codeNotLike)
	if err != nil {
		log.Printf("voucher lookup failed (%v); using stub %s", err, stub)
		return stub
	}
	if !found {
		log.Printf("user %s has no live voucher matching %s; using stub %s", userID, codeLike, stub)
		return stub
	}
	log.Printf("using real voucher %s (%d%%) for user %s", code, amount, userID)
	return code
}
