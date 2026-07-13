package main

import (
	"context"
	"fmt"
	"log"

	"github.com/kyou-id/yukari/internal/config"
	"github.com/kyou-id/yukari/internal/queue"
	"github.com/kyou-id/yukari/internal/reader"
	"github.com/kyou-id/yukari/internal/runreport"
)

func runBirthday(ctx context.Context, run *runreport.Run) error {
	cfg := config.Load()
	now := run.StartedAt
	run.QueueName = cfg.QueueName

	store, err := buildStore(cfg, now)
	if err != nil {
		return fmt.Errorf("build store: %w", err)
	}

	redisQueue := queue.NewRedisQueue(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.QueueName)
	defer func() {
		if err := redisQueue.Close(); err != nil {
			log.Printf("redis close failed: %v", err)
		}
	}()

	vouchers, err := buildVoucherCreator(cfg)
	if err != nil {
		return fmt.Errorf("build voucher creator: %w", err)
	}
	// Keep the interface nil when there is no creator. A nil *T assigned to an
	// interface makes it non-nil, and the reader's `r.vouchers != nil` guard would
	// then call straight into a nil receiver.
	var voucherCreator reader.VoucherCreator
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

	count, err := reader.NewWithVoucherCreatorAndAudit(store, redisQueue, voucherCreator, run.Audit(auditLogger), cfg.QueueName, cfg.ActionURL).Run(ctx, now)
	run.Queued = count
	return err
}
