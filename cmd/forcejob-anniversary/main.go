package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/kyou-id/yukari/internal/config"
	"github.com/kyou-id/yukari/internal/domain"
	"github.com/kyou-id/yukari/internal/queue"
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

	userID := env("YUKARI_FORCE_USER", "")
	if userID == "" {
		log.Fatal("YUKARI_FORCE_USER is required")
	}

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
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		historicalItem = orders[0]
		historicalItem.DaysAgo = int(start.Sub(historicalItem.OrderDate).Hours() / 24)
	}

	voucherCfg, err := repository.LoadBirthdayVoucherConfig(cfg.AnniversaryVoucherConfigPath)
	if err != nil {
		log.Fatalf("load anniversary voucher config: %v", err)
	}
	voucherCreator, err := repository.OpenMySQLVoucherCreator(cfg.DatabaseDSN, voucherCfg, cfg.VoucherCodeSecret)
	if err != nil {
		log.Fatalf("open voucher creator: %v", err)
	}
	defer func() {
		if err := voucherCreator.Close(); err != nil {
			log.Printf("voucher db close failed: %v", err)
		}
	}()

	itemIDs := wishlistItemIDs(wishlist)
	voucher, err := voucherCreator.CreateAnniversaryVoucher(ctx, user, now, itemIDs)
	if err != nil {
		log.Fatalf("create anniversary voucher: %v", err)
	}
	if voucher.Existed {
		log.Printf("voucher already exists for this user/year: voucher_id=%d voucher_code=%s; using existing voucher", voucher.ID, voucher.Code)
	}

	popularFYP := make([]domain.FYPItem, 0, len(popular))
	for _, p := range popular {
		popularFYP = append(popularFYP, p)
	}

	job := domain.AnniversaryJob{
		ID:             fmt.Sprintf("force-anniversary-%s-user-%s", now.Format("2006-01-02-150405"), user.ID),
		UserID:         user.ID,
		Date:           now,
		User:           user,
		Years:          years,
		VoucherCode:    voucher.Code,
		VoucherID:      voucher.ID,
		HistoricalItem: historicalItem,
		WishlistItems:  wishlist,
		PopularItems:   popularFYP,
		Attempt:        1,
	}

	redisQueue := queue.NewRedisQueue(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.QueueName)
	defer func() {
		if err := redisQueue.Close(); err != nil {
			log.Printf("redis close failed: %v", err)
		}
	}()

	if err := redisQueue.EnqueueAnniversaryTo(ctx, cfg.AnniversaryQueueName, job); err != nil {
		log.Fatalf("enqueue anniversary job: %v", err)
	}

	log.Printf("forced anniversary job enqueued: queue=%s job_id=%s user_id=%s years=%d voucher_id=%d voucher_code=%s name=%q email=%s wishlist=%d historical=%s",
		cfg.AnniversaryQueueName,
		job.ID,
		user.ID,
		years,
		voucher.ID,
		voucher.Code,
		user.Name,
		maskEmail(user.Email),
		len(wishlist),
		historicalItem.Name,
	)
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

func wishlistItemIDs(wishlist []domain.WishlistItem) []string {
	ids := make([]string, 0, len(wishlist))
	for _, item := range wishlist {
		if item.ID != "" {
			ids = append(ids, item.ID)
		}
	}
	return ids
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func maskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" {
		return email
	}
	local := parts[0]
	if len(local) == 1 {
		return local[:1] + "***@" + parts[1]
	}
	return local[:1] + "***" + local[len(local)-1:] + "@" + parts[1]
}
