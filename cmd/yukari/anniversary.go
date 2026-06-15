package main

import (
	"context"
	"log"
	"time"

	"github.com/kyou-id/yukari/internal/config"
	"github.com/kyou-id/yukari/internal/domain"
	"github.com/kyou-id/yukari/internal/queue"
	"github.com/kyou-id/yukari/internal/reader"
)

type anniversaryQueueAdapter struct {
	queue     *queue.RedisQueue
	queueName string
}

func (a anniversaryQueueAdapter) EnqueueAnniversary(ctx context.Context, job domain.AnniversaryJob) error {
	return a.queue.EnqueueAnniversaryTo(ctx, a.queueName, job)
}

func runAnniversary() {
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

	anniversaryStore, ok := store.(reader.AnniversaryStore)
	if !ok {
		log.Fatal("store does not support anniversary queries")
	}

	redisQueue := queue.NewRedisQueue(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.QueueName)
	defer func() {
		if err := redisQueue.Close(); err != nil {
			log.Printf("redis close failed: %v", err)
		}
	}()

	anniversaryVoucherCreator, err := buildAnniversaryVoucherCreator(cfg)
	if err != nil {
		log.Fatalf("build anniversary voucher creator: %v", err)
	}
	if anniversaryVoucherCreator != nil {
		defer func() {
			if err := anniversaryVoucherCreator.Close(); err != nil {
				log.Printf("anniversary voucher db close failed: %v", err)
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

	annivQueue := anniversaryQueueAdapter{queue: redisQueue, queueName: cfg.AnniversaryQueueName}
	count, err := reader.NewAnniversary(anniversaryStore, annivQueue, anniversaryVoucherCreator, auditLogger, cfg.AnniversaryQueueName, cfg.ActionURL).Run(ctx, now)
	if err != nil {
		log.Fatalf("anniversary reader failed: %v", err)
	}
	log.Printf("yukari enqueued %d anniversary email job(s)", count)
}
