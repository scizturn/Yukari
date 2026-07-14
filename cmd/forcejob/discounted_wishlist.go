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

func runDiscountedWishlist() {
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

	needle := env("YUKARI_FORCE_USER", "")
	if needle == "" {
		log.Fatal("YUKARI_FORCE_USER is required (user_id, email, name, or username)")
	}

	user, _, err := findUserByNeedle(ctx, cfg.DatabaseDSN, needle)
	if err != nil {
		log.Fatal(err)
	}
	user.IsActive = true

	store, err := repository.OpenMySQLStore(cfg.DatabaseDSN, sqlfiles.NewLoader(cfg.SQLDir))
	if err != nil {
		log.Fatalf("open store: %v", err)
	}

	wishlisted, err := store.DiscountedWishlistItems(ctx, user.ID)
	if err != nil {
		log.Fatalf("read discounted wishlist items: %v", err)
	}
	if len(wishlisted) == 0 {
		log.Printf("warning: no discounted wishlist items for user %s; forcing with empty wishlisted items", user.ID)
	}

	filler, err := store.DiscountedWishlistFillIndex(ctx)
	if err != nil {
		log.Fatalf("read discounted wishlist fill items: %v", err)
	}
	fill := filler.Fill(user.ID)

	items := append(wishlisted, fill...)

	job := domain.DiscountedWishlistJob{
		ID:      fmt.Sprintf("force-discounted-wishlist-%s-user-%s", now.Format("2006-01-02-150405"), user.ID),
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

	if err := redisQueue.EnqueueDiscountedWishlistTo(ctx, cfg.DiscountedWishlistQueueName, job); err != nil {
		log.Fatalf("enqueue force job: %v", err)
	}

	log.Printf("forced discounted wishlist job enqueued: queue=%s job_id=%s user_id=%s name=%q email=%s wishlisted=%d fill=%d",
		cfg.DiscountedWishlistQueueName,
		job.ID,
		user.ID,
		user.Name,
		maskEmail(user.Email),
		len(wishlisted),
		len(fill),
	)
}
