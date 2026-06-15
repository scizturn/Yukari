package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/kyou-id/yukari/internal/config"
	"github.com/kyou-id/yukari/internal/domain"
	"github.com/kyou-id/yukari/internal/queue"
	"github.com/kyou-id/yukari/internal/repository"
	"github.com/kyou-id/yukari/internal/sqlfiles"
)

func runBirthday() {
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
	needle := env("YUKARI_FORCE_USER", "abimanyu")

	user, originalActive, err := findUserByNeedle(ctx, cfg.DatabaseDSN, needle)
	if err != nil {
		log.Fatal(err)
	}
	user.IsActive = true

	store, err := repository.OpenMySQLStore(cfg.DatabaseDSN, sqlfiles.NewLoader(cfg.SQLDir))
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	wishlist, err := store.Wishlist(ctx, user.ID)
	if err != nil {
		log.Fatalf("read wishlist: %v", err)
	}
	fyp, err := store.FYP(ctx, user.ID)
	if err != nil {
		log.Fatalf("read fyp: %v", err)
	}
	popular, err := store.Popular(ctx)
	if err != nil {
		log.Fatalf("read popular: %v", err)
	}

	voucherCfg, err := repository.LoadBirthdayVoucherConfig(cfg.VoucherConfigPath)
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
	voucher, err := voucherCreator.CreateBirthdayVoucher(ctx, user, now, birthdayVoucherItemIDs(wishlist, fyp))
	if err != nil {
		log.Fatalf("create voucher: %v", err)
	}
	if voucher.Existed {
		log.Fatalf("voucher already exists for this user/year: voucher_id=%d voucher_code=%s; refusing to enqueue duplicate email", voucher.ID, voucher.Code)
	}

	job := domain.BirthdayJob{
		ID:            fmt.Sprintf("force-birthday-%s-user-%s", now.Format("2006-01-02-150405"), user.ID),
		UserID:        user.ID,
		Date:          now,
		User:          user,
		VoucherCode:   voucher.Code,
		VoucherID:     voucher.ID,
		WishlistItems: wishlist,
		FYPItems:      fyp,
		PopularItems:  popular,
		Attempt:       1,
	}

	redisQueue := queue.NewRedisQueue(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.QueueName)
	defer func() {
		if err := redisQueue.Close(); err != nil {
			log.Printf("redis close failed: %v", err)
		}
	}()
	if err := redisQueue.Enqueue(ctx, job); err != nil {
		log.Fatalf("enqueue force job: %v", err)
	}

	log.Printf("forced birthday job enqueued: queue=%s job_id=%s user_id=%s voucher_id=%d voucher_code=%s name=%q email=%s original_active=%t forced_active=%t wishlist=%d fyp=%d popular=%d",
		cfg.QueueName,
		job.ID,
		user.ID,
		voucher.ID,
		voucher.Code,
		user.Name,
		maskEmail(user.Email),
		originalActive,
		user.IsActive,
		len(wishlist),
		len(fyp),
		len(popular),
	)
}

func birthdayActionURL(baseURL string, voucherCode string) string {
	if baseURL == "" || voucherCode == "" {
		return baseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return baseURL
	}
	query := parsed.Query()
	query.Set("claim", voucherCode)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func birthdayVoucherItemIDs(wishlist []domain.WishlistItem, fyp []domain.FYPItem) []string {
	seen := make(map[string]struct{}, len(wishlist)+len(fyp))
	itemIDs := make([]string, 0, len(wishlist)+len(fyp))
	add := func(id string) {
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		itemIDs = append(itemIDs, id)
	}
	for _, item := range wishlist {
		add(item.ID)
	}
	for _, item := range fyp {
		add(item.ID)
	}
	return itemIDs
}
