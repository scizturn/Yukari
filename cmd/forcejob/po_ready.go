package main

import (
	"context"
	"database/sql"
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

	order, err := findPoReadyOrderForUser(ctx, cfg.DatabaseDSN, user.ID)
	if err != nil {
		log.Fatalf("find po ready order: %v", err)
	}

	store, err := repository.OpenMySQLStore(cfg.DatabaseDSN, sqlfiles.NewLoader(cfg.SQLDir))
	if err != nil {
		log.Fatalf("open store: %v", err)
	}

	items, err := store.PoReadyItems(ctx, order.OrderID)
	if err != nil {
		log.Fatalf("read po ready items: %v", err)
	}
	if len(items) == 0 {
		log.Printf("warning: no ready PO items for order %s; forcing with empty item list", order.OrderID)
	}

	job := domain.PoReadyJob{
		ID:          fmt.Sprintf("force-po-ready-%s-order-%s", now.Format("2006-01-02-150405"), order.OrderID),
		OrderID:     order.OrderID,
		UserID:      user.ID,
		Date:        now,
		User:        user,
		Items:       items,
		Remaining:   order.Remaining,
		DownPayment: order.DownPayment,
		ETA:         order.ETA,
		Attempt:     1,
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

	log.Printf("forced po ready job enqueued: queue=%s job_id=%s order_id=%s user_id=%s name=%q email=%s items=%d remaining=%d",
		cfg.PoReadyQueueName,
		job.ID,
		job.OrderID,
		user.ID,
		user.Name,
		maskEmail(user.Email),
		len(items),
		order.Remaining,
	)
}

// findPoReadyOrderForUser picks the user's highest-balance DP-paid order that has
// at least one arrived-but-unpaid PO item, bypassing the dedup gate.
func findPoReadyOrderForUser(ctx context.Context, dsn string, userID string) (domain.PoReadyOrder, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return domain.PoReadyOrder{}, err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return domain.PoReadyOrder{}, err
	}

	var order domain.PoReadyOrder
	err = db.QueryRowContext(ctx, `
SELECT
  CAST(o.order_id AS CHAR),
  o.remaining,
  COALESCE(SUM(oi.down_payment), 0),
  COALESCE(o.eta, '')
FROM orders o
JOIN order_items oi ON oi.order_id = o.order_id
JOIN items i        ON i.item_id   = oi.item_id
WHERE o.user_id      = ?
  AND o.status       = 'dp paid'
  AND o.remaining    > 0
  AND oi.status      = 'po'
  AND i.status       = 'ready'
  AND oi.cancelled_at IS NULL
  AND oi.refund_status = 'none'
GROUP BY o.order_id, o.remaining, o.eta
ORDER BY o.remaining DESC
LIMIT 1`, userID).Scan(&order.OrderID, &order.Remaining, &order.DownPayment, &order.ETA)
	if err != nil {
		return domain.PoReadyOrder{}, fmt.Errorf("no eligible po-ready order for user %s: %w", userID, err)
	}
	order.User.ID = userID
	return order, nil
}
