package main

import (
	"context"
	"log"
	"time"

	"github.com/kyou-id/yukari/internal/config"
	"github.com/kyou-id/yukari/internal/queue"
	"github.com/kyou-id/yukari/internal/reader"
)

func runBirthday() {
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

	redisQueue := queue.NewRedisQueue(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.QueueName)
	defer func() {
		if err := redisQueue.Close(); err != nil {
			log.Printf("redis close failed: %v", err)
		}
	}()

	voucherCreator, err := buildVoucherCreator(cfg)
	if err != nil {
		log.Fatalf("build voucher creator: %v", err)
	}
	if voucherCreator != nil {
		defer func() {
			if err := voucherCreator.Close(); err != nil {
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

	count, err := reader.NewWithVoucherCreatorAndAudit(store, redisQueue, voucherCreator, auditLogger, cfg.QueueName, cfg.ActionURL).Run(ctx, now)
	if err != nil {
		log.Fatalf("birthday reader failed: %v", err)
	}
	log.Printf("yukari enqueued %d birthday email job(s)", count)
}
