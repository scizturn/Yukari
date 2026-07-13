package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/kyou-id/yukari/internal/config"
	"github.com/kyou-id/yukari/internal/queue"
	"github.com/kyou-id/yukari/internal/reader"
	"github.com/kyou-id/yukari/internal/runreport"
)

func runWinback(ctx context.Context, run *runreport.Run) error {
	cfg := config.Load()
	now := run.StartedAt
	run.QueueName = cfg.WinbackQueueName

	store, err := buildStore(cfg, now)
	if err != nil {
		return fmt.Errorf("build store: %w", err)
	}

	wbStore, ok := store.(reader.WinbackStore)
	if !ok {
		return errors.New("store does not support winback queries")
	}

	redisQueue := queue.NewRedisQueue(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.QueueName)
	defer func() {
		if err := redisQueue.Close(); err != nil {
			log.Printf("redis close failed: %v", err)
		}
	}()

	vouchers, err := buildWinbackVoucherCreator(cfg)
	if err != nil {
		return fmt.Errorf("build winback voucher creator: %w", err)
	}
	// A nil *T in an interface is not nil — see runBirthday.
	var voucherCreator reader.WinbackVoucherCreator
	if vouchers != nil {
		voucherCreator = vouchers
		defer func() {
			if err := vouchers.Close(); err != nil {
				log.Printf("voucher db close failed: %v", err)
			}
		}()
	}

	auditLogger, err := buildAuditLogger(cfg)
	if err != nil {
		return fmt.Errorf("build audit logger: %w", err)
	}
	if auditLogger != nil {
		defer func() {
			if err := auditLogger.Close(); err != nil {
				log.Printf("audit db close failed: %v", err)
			}
		}()
	}

	count, err := reader.NewWinback(wbStore, redisQueue, voucherCreator, run.Audit(auditLogger), cfg.WinbackQueueName, cfg.ActionURL).Run(ctx, now)
	run.Queued = count
	return err
}
