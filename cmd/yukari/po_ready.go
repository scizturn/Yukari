package main

import (
	"context"
	"log"
	"time"

	"github.com/kyou-id/yukari/internal/config"
	"github.com/kyou-id/yukari/internal/queue"
	"github.com/kyou-id/yukari/internal/reader"
)

func runPoReady() {
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

	prStore, ok := store.(reader.PoReadyStore)
	if !ok {
		log.Fatal("store does not support po ready queries")
	}

	redisQueue := queue.NewRedisQueue(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.QueueName)
	defer func() {
		if err := redisQueue.Close(); err != nil {
			log.Printf("redis close failed: %v", err)
		}
	}()

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

	count, err := reader.NewPoReady(prStore, redisQueue, auditLogger, cfg.PoReadyQueueName).Run(ctx, now)
	if err != nil {
		log.Fatalf("po ready reader failed: %v", err)
	}
	log.Printf("yukari enqueued %d po ready email job(s)", count)
}
