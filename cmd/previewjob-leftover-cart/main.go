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
	"github.com/kyou-id/yukari/internal/reader"
	"github.com/kyou-id/yukari/internal/repository"
	"github.com/kyou-id/yukari/internal/sqlfiles"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()
	if strings.TrimSpace(cfg.DatabaseDSN) == "" {
		log.Fatal("OLD_DATABASE_DSN is required")
	}

	location, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		log.Fatalf("load timezone: %v", err)
	}
	now := time.Now().In(location)
	userID := env("YUKARI_FORCE_USER", "147044")
	outputPath := env("YUKARI_PREVIEW_JOB_PATH", "/Users/sleepyreinze/Dev/Email-Api/Makoto/templates/preview/leftover-cart-job.json")

	user, err := findUserByID(ctx, cfg.DatabaseDSN, userID)
	if err != nil {
		log.Fatal(err)
	}
	user.IsActive = true

	store, err := repository.OpenMySQLStore(cfg.DatabaseDSN, sqlfiles.NewLoader(cfg.SQLDir))
	if err != nil {
		log.Fatalf("open store: %v", err)
	}

	cartItems, err := store.CartItems(ctx, user.ID)
	if err != nil {
		log.Fatalf("read cart items: %v", err)
	}

	orders, err := store.HistoricalOrders(ctx, user.ID)
	if err != nil {
		log.Fatalf("read historical orders: %v", err)
	}
	var historicalItem domain.HistoricalItem
	if len(orders) > 0 {
		historicalItem = orders[len(orders)-1]
	}
	var reco []domain.FYPItem
	if historicalItem.Name != "" {
		reco, err = store.LeftoverCartReco(ctx, user.ID)
		if err != nil {
			log.Fatalf("read reco items: %v", err)
		}
		if len(reco) < 3 {
			popular, err := store.Popular(ctx)
			if err != nil {
				log.Fatalf("read popular: %v", err)
			}
			reco = reader.FillRecoFromPopular(reco, popular, 3)
		}
	}

	job := domain.LeftoverCartJob{
		ID:             fmt.Sprintf("preview-leftover-cart-%s-user-%s", now.Format("2006-01-02-150405"), user.ID),
		UserID:         user.ID,
		Date:           now,
		User:           user,
		HistoricalItem: historicalItem,
		CartItems:      cartItems,
		RecoItems:      reco,
		Attempt:        1,
	}

	payload, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(outputPath, payload, 0o600); err != nil {
		log.Fatal(err)
	}
	log.Printf("preview job written: path=%s user_id=%s cart_items=%d historical_item=%q reco_items=%d", outputPath, user.ID, len(cartItems), historicalItem.Name, len(reco))
}

func findUserByID(ctx context.Context, dsn string, userID string) (domain.User, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return domain.User{}, err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return domain.User{}, err
	}

	var user domain.User
	var birthday sql.NullTime
	err = db.QueryRowContext(ctx, `
SELECT user_id, name, email, birthdate, is_confirmed
FROM users
WHERE CAST(user_id AS CHAR) = ?
LIMIT 1`, userID).Scan(&user.ID, &user.Name, &user.Email, &birthday, &user.IsActive)
	if err != nil {
		return domain.User{}, err
	}
	if birthday.Valid {
		user.Birthday = birthday.Time
	}
	return user, nil
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
