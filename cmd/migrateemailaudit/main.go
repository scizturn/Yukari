package main

import (
	"context"
	"database/sql"
	"log"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/kyou-id/yukari/internal/config"
)

const createEmailDeliveryLogsTable = `
CREATE TABLE IF NOT EXISTS email_delivery_logs (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,

  feature VARCHAR(100) NOT NULL,
  reference_type VARCHAR(100) NULL,
  reference_id VARCHAR(191) NULL,

  job_id VARCHAR(191) NOT NULL,
  queue_name VARCHAR(191) NULL,
  attempt INT UNSIGNED NOT NULL DEFAULT 1,

  user_id BIGINT UNSIGNED NULL,
  to_email VARCHAR(255) NOT NULL,

  template_id VARCHAR(191) NULL,
  subject VARCHAR(255) NULL,
  action_url TEXT NULL,
  metadata JSON NULL,

  provider VARCHAR(100) NOT NULL,
  provider_message_id VARCHAR(191) NULL,
  provider_status_code INT NULL,
  provider_response TEXT NULL,

  status ENUM('queued', 'sending', 'sent', 'failed', 'dead_letter', 'skipped') NOT NULL DEFAULT 'queued',
  failure_reason TEXT NULL,
  skip_reason VARCHAR(255) NULL,

  queued_at DATETIME NULL,
  sending_at DATETIME NULL,
  sent_at DATETIME NULL,
  failed_at DATETIME NULL,
  skipped_at DATETIME NULL,
  created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

  UNIQUE KEY uniq_email_delivery_job_attempt (job_id, attempt),
  KEY idx_email_delivery_feature_reference (feature, reference_type, reference_id),
  KEY idx_email_delivery_user_id (user_id),
  KEY idx_email_delivery_to_email (to_email),
  KEY idx_email_delivery_status (status),
  KEY idx_email_delivery_sent_at (sent_at),
  KEY idx_email_delivery_provider_message_id (provider_message_id)
)`

func main() {
	ctx := context.Background()
	cfg := config.Load()
	if strings.TrimSpace(cfg.DatabaseDSN) == "" {
		log.Fatal("OLD_DATABASE_* is required")
	}

	db, err := sql.Open("mysql", cfg.DatabaseDSN)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("ping db: %v", err)
	}
	if _, err := db.ExecContext(ctx, createEmailDeliveryLogsTable); err != nil {
		log.Fatalf("apply email_delivery_logs migration: %v", err)
	}

	log.Print("email_delivery_logs migration applied")
}
