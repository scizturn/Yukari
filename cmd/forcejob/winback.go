package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/kyou-id/yukari/internal/config"
	"github.com/kyou-id/yukari/internal/domain"
	"github.com/kyou-id/yukari/internal/queue"
	"github.com/kyou-id/yukari/internal/reader"
	"github.com/kyou-id/yukari/internal/repository"
	"github.com/kyou-id/yukari/internal/sqlfiles"
)

func runWinback() {
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

	user, err := findUserByID(ctx, cfg.DatabaseDSN, userID)
	if err != nil {
		log.Fatal(err)
	}
	user.IsActive = true

	store, err := repository.OpenMySQLStore(cfg.DatabaseDSN, sqlfiles.NewLoader(cfg.SQLDir))
	if err != nil {
		log.Fatalf("open store: %v", err)
	}

	wishlist, err := store.Wishlist(ctx, user.ID)
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
		historicalItem = orders[len(orders)-1]
		historicalItem.DaysAgo = int(start.Sub(historicalItem.OrderDate).Hours() / 24)
	}

	voucherCfg, err := repository.LoadBirthdayVoucherConfig(cfg.WinbackVoucherConfigPath)
	if err != nil {
		log.Fatalf("load winback voucher config: %v", err)
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

	itemIDs := winbackWishlistItemIDs(wishlist)
	voucher, err := voucherCreator.CreateWinbackVoucher(ctx, user, now, itemIDs)
	if err != nil {
		log.Fatalf("create winback voucher: %v", err)
	}
	if voucher.Existed {
		log.Printf("winback voucher already exists for this user/year: voucher_id=%d voucher_code=%s; using existing voucher", voucher.ID, voucher.Code)
	}

	job := domain.WinbackJob{
		ID:             fmt.Sprintf("force-winback-%s-user-%s", now.Format("2006-01-02-150405"), user.ID),
		UserID:         user.ID,
		Date:           now,
		User:           user,
		VoucherCode:    voucher.Code,
		VoucherID:      voucher.ID,
		WishlistItems:  wishlist,
		HistoricalItem: historicalItem,
		PopularItems:   popular,
		Attempt:        1,
	}

	redisQueue := queue.NewRedisQueue(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.QueueName)
	defer func() {
		if err := redisQueue.Close(); err != nil {
			log.Printf("redis close failed: %v", err)
		}
	}()

	if err := redisQueue.EnqueueWinbackTo(ctx, cfg.WinbackQueueName, job); err != nil {
		log.Fatalf("enqueue force job: %v", err)
	}

	log.Printf("forced winback job enqueued: queue=%s job_id=%s user_id=%s voucher_id=%d voucher_code=%s name=%q email=%s wishlist=%d historical=%s",
		cfg.WinbackQueueName,
		job.ID,
		user.ID,
		voucher.ID,
		voucher.Code,
		user.Name,
		maskEmail(user.Email),
		len(wishlist),
		historicalItem.Name,
	)
}

func winbackWishlistItemIDs(wishlist []domain.WishlistItem) []string {
	ids := make([]string, 0, len(wishlist))
	for _, item := range wishlist {
		if item.ID != "" {
			ids = append(ids, item.ID)
		}
	}
	return ids
}
