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

func runLeftoverCart() {
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

	cartItems, err := store.CartItems(ctx, user.ID)
	if err != nil {
		log.Fatalf("read cart items: %v", err)
	}
	if len(cartItems) == 0 {
		log.Printf("warning: no available cart items for user %s; forcing with empty cart", user.ID)
	}

	reco, err := store.LeftoverCartReco(ctx, user.ID)
	if err != nil {
		log.Fatalf("read reco items: %v", err)
	}
	if len(reco) < 3 {
		popular, err := store.Popular(ctx)
		if err != nil {
			log.Fatalf("read popular: %v", err)
		}
		reco = reader.FillRecoFromPopular(reco, popular, 3)
	}

	job := domain.LeftoverCartJob{
		ID:        fmt.Sprintf("force-leftover-cart-%s-user-%s", now.Format("2006-01-02-150405"), user.ID),
		UserID:    user.ID,
		Date:      now,
		User:      user,
		CartItems: cartItems,
		RecoItems: reco,
		Attempt:   1,
	}

	redisQueue := queue.NewRedisQueue(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.QueueName)
	defer func() {
		if err := redisQueue.Close(); err != nil {
			log.Printf("redis close failed: %v", err)
		}
	}()

	if err := redisQueue.EnqueueLeftoverCartTo(ctx, cfg.LeftoverCartQueueName, job); err != nil {
		log.Fatalf("enqueue force job: %v", err)
	}

	log.Printf("forced leftover cart job enqueued: queue=%s job_id=%s user_id=%s name=%q email=%s cart_items=%d reco_items=%d",
		cfg.LeftoverCartQueueName,
		job.ID,
		user.ID,
		user.Name,
		maskEmail(user.Email),
		len(cartItems),
		len(reco),
	)
}
