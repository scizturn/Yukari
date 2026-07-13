package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/kyou-id/yukari/internal/config"
	"github.com/kyou-id/yukari/internal/queue"
	"github.com/kyou-id/yukari/internal/reader"
	"github.com/kyou-id/yukari/internal/runreport"
)

func runWishlistBackIn(ctx context.Context, run *runreport.Run) error {
	cfg := config.Load()
	now := run.StartedAt
	run.QueueName = cfg.WishlistBackInQueueName
	// See runPoReady: the reader's own Friday guard still decides, this just skips
	// opening connections a stand-down run never uses.
	if !reader.WishlistBackInRunsOn(now) {
		run.Note = fmt.Sprintf("wishlist-back-in reader only runs on Friday (today is %s)", now.Weekday())
		return nil
	}

	store, err := buildStore(cfg, now)
	if err != nil {
		return fmt.Errorf("build store: %w", err)
	}
	wbiStore, ok := store.(reader.WishlistBackInStore)
	if !ok {
		return errors.New("store does not support wishlist back in queries")
	}

	redisQueue := queue.NewRedisQueue(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.QueueName)
	defer redisQueue.Close()
	vouchers, err := buildWishlistBackInVoucherCreator(cfg)
	if err != nil {
		return fmt.Errorf("build wishlist back in voucher creator: %w", err)
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
		return fmt.Errorf("build audit logger: %w", err)
	}
	if auditLogger != nil {
		defer auditLogger.Close()
	}

	wbiReader := reader.NewWishlistBackIn(wbiStore, redisQueue, voucherCreator, run.Audit(auditLogger), cfg.WishlistBackInQueueName, cfg.ActionURL)
	if days := cfg.WishlistBackInWindowDays; days > 0 {
		wbiReader.Window = time.Duration(days) * 24 * time.Hour
		log.Printf("wishlist back in detection window: %d day(s)", days)
	}

	count, err := wbiReader.Run(ctx, now)
	run.Queued = count
	return err
}
