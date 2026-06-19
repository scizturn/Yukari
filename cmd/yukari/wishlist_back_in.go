package main

import (
	"context"
	"log"
	"time"

	"github.com/kyou-id/yukari/internal/config"
	"github.com/kyou-id/yukari/internal/queue"
	"github.com/kyou-id/yukari/internal/reader"
)

func runWishlistBackIn() {
	ctx := context.Background()
	cfg := config.Load()
	location, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		log.Fatalf("load timezone: %v", err)
	}
	now := time.Now().In(location)
	startAt, err := time.ParseInLocation("2006-01-02", cfg.WishlistBackInStartAt, location)
	if err != nil {
		log.Fatalf("parse YUKARI_WISHLIST_BACK_IN_START_AT: %v", err)
	}

	store, err := buildStore(cfg, now)
	if err != nil {
		log.Fatalf("build store: %v", err)
	}
	wbiStore, ok := store.(reader.WishlistBackInStore)
	if !ok {
		log.Fatal("store does not support wishlist back in queries")
	}

	redisQueue := queue.NewRedisQueue(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.QueueName)
	defer redisQueue.Close()
	vouchers, err := buildWishlistBackInVoucherCreator(cfg)
	if err != nil {
		log.Fatalf("build wishlist back in voucher creator: %v", err)
	}
	if vouchers != nil {
		defer vouchers.Close()
	}
	auditLogger, err := buildAuditLogger(cfg)
	if err != nil {
		log.Fatalf("build audit logger: %v", err)
	}
	if auditLogger != nil {
		defer auditLogger.Close()
	}

	count, err := reader.NewWishlistBackIn(wbiStore, redisQueue, vouchers, auditLogger, cfg.WishlistBackInQueueName, cfg.ActionURL, startAt).Run(ctx, now)
	if err != nil {
		log.Fatalf("wishlist back in reader failed: %v", err)
	}
	log.Printf("yukari enqueued %d wishlist back in email job(s)", count)
}
