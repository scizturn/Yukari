package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/kyou-id/yukari/internal/config"
	"github.com/kyou-id/yukari/internal/domain"
	"github.com/kyou-id/yukari/internal/repository"
	"github.com/kyou-id/yukari/internal/sqlfiles"
)

func main() {
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
	orderID := env("YUKARI_FORCE_ORDER", "")
	outputPath := env("YUKARI_PREVIEW_JOB_PATH", "/Users/sleepyreinze/Dev/Email-Api/Makoto/templates/preview/po-ready-job.json")

	store, err := repository.OpenMySQLStore(cfg.DatabaseDSN, sqlfiles.NewLoader(cfg.SQLDir))
	if err != nil {
		log.Fatalf("open store: %v", err)
	}

	// With no explicit order, take the first eligible order from the live query.
	order, err := resolvePreviewOrder(ctx, store, cfg.DatabaseDSN, orderID)
	if err != nil {
		log.Fatal(err)
	}

	items, err := store.PoReadyItems(ctx, order.OrderID)
	if err != nil {
		log.Fatalf("read po ready items: %v", err)
	}

	job := domain.PoReadyJob{
		ID:          fmt.Sprintf("preview-po-ready-%s-order-%s", now.Format("2006-01-02-150405"), order.OrderID),
		OrderID:     order.OrderID,
		UserID:      order.User.ID,
		Date:        now,
		User:        order.User,
		Items:       items,
		Remaining:   order.Remaining,
		DownPayment: order.DownPayment,
		ETA:         order.ETA,
		Attempt:     1,
	}

	payload, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(outputPath, payload, 0o600); err != nil {
		log.Fatal(err)
	}
	log.Printf("preview job written: path=%s order_id=%s user_id=%s items=%d remaining=%d", outputPath, order.OrderID, order.User.ID, len(items), order.Remaining)
}

func resolvePreviewOrder(ctx context.Context, store *repository.MySQLStore, dsn, orderID string) (domain.PoReadyOrder, error) {
	if strings.TrimSpace(orderID) == "" {
		orders, err := store.PoReadyOrders(ctx)
		if err != nil {
			return domain.PoReadyOrder{}, err
		}
		if len(orders) == 0 {
			return domain.PoReadyOrder{}, fmt.Errorf("no eligible po-ready orders found; set YUKARI_FORCE_ORDER to preview a specific order")
		}
		return orders[0], nil
	}
	return findPreviewOrder(ctx, dsn, orderID)
}

func findPreviewOrder(ctx context.Context, dsn, orderID string) (domain.PoReadyOrder, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return domain.PoReadyOrder{}, err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return domain.PoReadyOrder{}, err
	}

	var order domain.PoReadyOrder
	var active bool
	err = db.QueryRowContext(ctx, `
SELECT
  CAST(o.order_id AS CHAR),
  CAST(o.user_id AS CHAR),
  u.name,
  u.email,
  u.email_verified_at IS NOT NULL,
  o.remaining,
  COALESCE((SELECT SUM(oi.down_payment) FROM order_items oi WHERE oi.order_id = o.order_id), 0),
  COALESCE(o.eta, '')
FROM orders o
JOIN users u ON u.user_id = o.user_id
WHERE CAST(o.order_id AS CHAR) = ?
LIMIT 1`, orderID).Scan(&order.OrderID, &order.User.ID, &order.User.Name, &order.User.Email, &active, &order.Remaining, &order.DownPayment, &order.ETA)
	if err != nil {
		return domain.PoReadyOrder{}, fmt.Errorf("order %s not found: %w", orderID, err)
	}
	order.User.IsActive = active
	return order, nil
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
