package main

import (
	"context"
	"log"
	"time"

	"github.com/kyou-id/yukari/internal/config"
	"github.com/kyou-id/yukari/internal/queue"
	"github.com/kyou-id/yukari/internal/reader"
)

func runWinback() {
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

	wbStore, ok := store.(reader.WinbackStore)
	if !ok {
		log.Fatal("store does not support winback queries")
	}

	redisQueue := queue.NewRedisQueue(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.QueueName)
	defer func() {
		if err := redisQueue.Close(); err != nil {
			log.Printf("redis close failed: %v", err)
		}
	}()

	vouchers, err := buildWinbackVoucherCreator(cfg)
	if err != nil {
		log.Fatalf("build winback voucher creator: %v", err)
	}
	if vouchers != nil {
		defer func() {
			if err := vouchers.Close(); err != nil {
				log.Printf("voucher db close failed: %v", err)
			}
		}()
	}

	auditLogger, err := buildAuditLogger(cfg)
	if err != nil {
		log.Fatalf("build audit logger: %v", err)
	}
	if auditLogger != nil {
		defer func() {
			if err := auditLogger.Close(); err != nil {
				log.Printf("audit db close failed: %v", err)
			}
		}()
	}

	count, err := reader.NewWinback(wbStore, redisQueue, vouchers, auditLogger, cfg.WinbackQueueName, cfg.ActionURL).Run(ctx, now)
	if err != nil {
		log.Fatalf("winback reader failed: %v", err)
	}
	log.Printf("yukari enqueued %d winback email job(s)", count)
}
