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
	"github.com/kyou-id/yukari/internal/repository"
	"github.com/kyou-id/yukari/internal/sqlfiles"
)

func runPoReady() {
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

	items, err := store.PoReadyForcedItems(ctx, user.ID)
	if err != nil {
		log.Fatalf("read po ready items: %v", err)
	}
	if len(items) == 0 {
		log.Fatalf("user %s has no ready wishlist items; nothing to force", user.ID)
	}

	job := domain.PoReadyJob{
		ID:      fmt.Sprintf("force-po-ready-%s-user-%s", now.Format("2006-01-02-150405"), user.ID),
		UserID:  user.ID,
		Date:    now,
		User:    user,
		Items:   items,
		Attempt: 1,
	}

	redisQueue := queue.NewRedisQueue(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.QueueName)
	defer func() {
		if err := redisQueue.Close(); err != nil {
			log.Printf("redis close failed: %v", err)
		}
	}()

	if err := redisQueue.EnqueuePoReadyTo(ctx, cfg.PoReadyQueueName, job); err != nil {
		log.Fatalf("enqueue force job: %v", err)
	}

	log.Printf("forced po ready job enqueued: queue=%s job_id=%s user_id=%s name=%q email=%s items=%d",
		cfg.PoReadyQueueName,
		job.ID,
		user.ID,
		user.Name,
		maskEmail(user.Email),
		len(items),
	)
}
