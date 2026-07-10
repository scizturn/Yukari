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
	// Keep the interface nil when there is no creator. Assigning a nil *T to an
	// interface makes it non-nil, and the reader's `r.vouchers != nil` guard would
	// then call through into a nil receiver.
	var voucherCreator reader.WishlistBackInVoucherCreator
	if vouchers != nil {
		voucherCreator = vouchers
		defer vouchers.Close()
	}
	auditLogger, err := buildAuditLogger(cfg)
	if err != nil {
		log.Fatalf("build audit logger: %v", err)
	}
	if auditLogger != nil {
		defer auditLogger.Close()
	}

	wbiReader := reader.NewWishlistBackIn(wbiStore, redisQueue, voucherCreator, auditLogger, cfg.WishlistBackInQueueName, cfg.ActionURL)
	if days := cfg.WishlistBackInWindowDays; days > 0 {
		wbiReader.Window = time.Duration(days) * 24 * time.Hour
		log.Printf("wishlist back in detection window: %d day(s)", days)
	}

	count, err := wbiReader.Run(ctx, now)
	if err != nil {
		log.Fatalf("wishlist back in reader failed: %v", err)
	}
	log.Printf("yukari enqueued %d wishlist back in email job(s)", count)
}
