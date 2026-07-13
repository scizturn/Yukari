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

func runPoReady(ctx context.Context, run *runreport.Run) error {
	cfg := config.Load()
	now := run.StartedAt
	run.QueueName = cfg.PoReadyQueueName
	// The reader stands down on its own (that guard is the authority on the
	// cadence); returning here just spares the run a MySQL and a Redis connection
	// it would never use.
	if !reader.PoReadyRunsOn(now) {
		run.Note = fmt.Sprintf("po-ready reader only runs on Saturday (today is %s)", now.Weekday())
		return nil
	}

	store, err := buildStore(cfg, now)
	if err != nil {
		return fmt.Errorf("build store: %w", err)
	}

	prStore, ok := store.(reader.PoReadyStore)
	if !ok {
		return errors.New("store does not support po ready queries")
	}

	redisQueue := queue.NewRedisQueue(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.QueueName)
	defer func() {
		if err := redisQueue.Close(); err != nil {
			log.Printf("redis close failed: %v", err)
		}
	}()

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

	count, err := reader.NewPoReady(prStore, redisQueue, run.Audit(auditLogger), cfg.PoReadyQueueName).Run(ctx, now)
	run.Queued = count
	return err
}
