package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/kyou-id/yukari/internal/config"
	"github.com/kyou-id/yukari/internal/domain"
	"github.com/kyou-id/yukari/internal/queue"
	"github.com/kyou-id/yukari/internal/reader"
	"github.com/kyou-id/yukari/internal/runreport"
)

type anniversaryQueueAdapter struct {
	queue     *queue.RedisQueue
	queueName string
}

func (a anniversaryQueueAdapter) EnqueueAnniversary(ctx context.Context, job domain.AnniversaryJob) error {
	return a.queue.EnqueueAnniversaryTo(ctx, a.queueName, job)
}

func runAnniversary(ctx context.Context, run *runreport.Run) error {
	cfg := config.Load()
	now := run.StartedAt
	run.QueueName = cfg.AnniversaryQueueName

	store, err := buildStore(cfg, now)
	if err != nil {
		return fmt.Errorf("build store: %w", err)
	}

	anniversaryStore, ok := store.(reader.AnniversaryStore)
	if !ok {
		return errors.New("store does not support anniversary queries")
	}

	redisQueue := queue.NewRedisQueue(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.QueueName)
	defer func() {
		if err := redisQueue.Close(); err != nil {
			log.Printf("redis close failed: %v", err)
		}
	}()

	vouchers, err := buildAnniversaryVoucherCreator(cfg)
	if err != nil {
		return fmt.Errorf("build anniversary voucher creator: %w", err)
	}
	// A nil *T in an interface is not nil — see runBirthday.
	var anniversaryVoucherCreator reader.AnniversaryVoucherCreator
	if vouchers != nil {
		anniversaryVoucherCreator = vouchers
		defer func() {
			if err := vouchers.Close(); err != nil {
				log.Printf("anniversary voucher db close failed: %v", err)
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

	annivQueue := anniversaryQueueAdapter{queue: redisQueue, queueName: cfg.AnniversaryQueueName}
	count, err := reader.NewAnniversary(anniversaryStore, annivQueue, anniversaryVoucherCreator, run.Audit(auditLogger), cfg.AnniversaryQueueName, cfg.ActionURL).Run(ctx, now)
	run.Queued = count
	return err
}
