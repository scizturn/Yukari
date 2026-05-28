package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/kyou-id/yukari/internal/config"
	"github.com/kyou-id/yukari/internal/queue"
	"github.com/kyou-id/yukari/internal/reader"
	"github.com/kyou-id/yukari/internal/repository"
	"github.com/kyou-id/yukari/internal/sqlfiles"
)

func main() {
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

	count, err := reader.NewWithVoucherCreator(store, redisQueue, voucherCreator).Run(ctx, now)
	if err != nil {
		log.Fatalf("reader failed: %v", err)
	}
	log.Printf("yukari enqueued %d birthday email job(s)", count)
}

func buildStore(cfg config.Config, now time.Time) (reader.Store, error) {
	if strings.TrimSpace(cfg.DatabaseDSN) == "" {
		log.Print("DATABASE_DSN is empty; using stub repository")
		return repository.NewStubStore(now), nil
	}
	return repository.OpenMySQLStore(cfg.DatabaseDSN, sqlfiles.NewLoader(cfg.SQLDir))
}

func buildVoucherCreator(cfg config.Config) (*repository.MySQLVoucherCreator, error) {
	voucherCfg, err := repository.LoadBirthdayVoucherConfig(cfg.VoucherConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("voucher config %s not found; Yukari will enqueue jobs without creating vouchers", cfg.VoucherConfigPath)
			return nil, nil
		}
		return nil, err
	}
	if !voucherCfg.PricingVoucherID.Valid && strings.TrimSpace(voucherCfg.PricingVoucherCode) == "" {
		log.Printf("voucher config %s has no pricing_voucher_id or pricing_voucher_code; Yukari will enqueue jobs without creating vouchers", cfg.VoucherConfigPath)
		return nil, nil
	}
	if strings.TrimSpace(cfg.DatabaseDSN) == "" {
		log.Print("OLD_DATABASE_* is empty; Yukari will enqueue jobs without creating vouchers")
		return nil, nil
	}
	return repository.OpenMySQLVoucherCreator(cfg.DatabaseDSN, voucherCfg, cfg.VoucherCodeSecret)
}
