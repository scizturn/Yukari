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

func runWishlistBackIn() {
	ctx := context.Background()
	cfg := config.Load()
	if strings.TrimSpace(cfg.DatabaseDSN) == "" {
		log.Fatal("DATABASE_DSN is required")
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

	// Bypass the restock-window + dedup eligibility: any available wishlist item.
	items, err := store.WishlistBackInForcedItems(ctx, user.ID)
	if err != nil {
		log.Fatalf("read wishlist back in items: %v", err)
	}
	if len(items) == 0 {
		log.Fatalf("user %s has no available wishlist items to force; pick a user with in-stock items in their wishlist", user.ID)
	}

	// Companion + recommendations use the same gating as the reader (need a full 6).
	companion, err := store.WishlistBackInCompanion(ctx, user.ID)
	if err != nil {
		log.Fatalf("read companion: %v", err)
	}
	var recos []domain.WishlistBackInItem
	if companion.ID != "" {
		recos, err = store.WishlistBackInRecommendations(ctx, user.ID, companion.ID)
		if err != nil {
			log.Fatalf("read recommendations: %v", err)
		}
		if len(recos) < 6 {
			companion, recos = domain.WishlistBackInItem{}, nil
		}
	}

	itemIDs := make([]string, len(items))
	for i, it := range items {
		itemIDs[i] = it.ID
	}

	voucherCfg, err := repository.LoadBirthdayVoucherConfig(cfg.WishlistBackInVoucherConfigPath)
	if err != nil {
		log.Fatalf("load voucher config: %v", err)
	}
	voucherCreator, err := repository.OpenMySQLVoucherCreator(cfg.DatabaseDSN, voucherCfg, cfg.VoucherCodeSecret)
	if err != nil {
		log.Fatalf("open voucher creator: %v", err)
	}
	defer func() {
		if err := voucherCreator.Close(); err != nil {
			log.Printf("voucher db close failed: %v", err)
		}
	}()
	// Existed=true just means the user's live voucher was reused (anti-spam) — fine.
	voucher, err := voucherCreator.CreateWishlistBackInVoucher(ctx, user, now, itemIDs)
	if err != nil {
		log.Fatalf("create voucher: %v", err)
	}

	job := domain.WishlistBackInJob{
		ID:            fmt.Sprintf("force-wishlist-back-in-%s-user-%s", now.Format("2006-01-02-150405"), user.ID),
		UserID:        user.ID,
		Date:          now,
		User:          user,
		VoucherCode:   voucher.Code,
		VoucherID:     voucher.ID,
		Items:         items,
		CompanionItem: companion,
		RecoItems:     recos,
		Attempt:       1,
	}

	redisQueue := queue.NewRedisQueue(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.QueueName)
	defer func() {
		if err := redisQueue.Close(); err != nil {
			log.Printf("redis close failed: %v", err)
		}
	}()
	if err := redisQueue.EnqueueWishlistBackInTo(ctx, cfg.WishlistBackInQueueName, job); err != nil {
		log.Fatalf("enqueue force job: %v", err)
	}

	log.Printf("forced wishlist back in job enqueued: queue=%s job_id=%s user_id=%s name=%q email=%s items=%d reco=%d voucher_id=%d voucher_code=%s reused=%t",
		cfg.WishlistBackInQueueName, job.ID, user.ID, user.Name, maskEmail(user.Email),
		len(items), len(recos), voucher.ID, voucher.Code, voucher.Existed,
	)
}
