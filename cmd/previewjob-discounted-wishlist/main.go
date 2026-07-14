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
	userID := env("YUKARI_FORCE_USER", "147044")
	outputPath := env("YUKARI_PREVIEW_JOB_PATH", "/Users/sleepyreinze/Dev/Email-Api/Makoto/templates/preview/discounted-wishlist-job.json")

	user, err := findUserByID(ctx, cfg.DatabaseDSN, userID)
	if err != nil {
		log.Fatal(err)
	}
	user.IsActive = true

	store, err := repository.OpenMySQLStore(cfg.DatabaseDSN, sqlfiles.NewLoader(cfg.SQLDir))
	if err != nil {
		log.Fatalf("open store: %v", err)
	}

	wishlisted, err := store.DiscountedWishlistItems(ctx, user.ID)
	if err != nil {
		log.Fatalf("read discounted wishlist items: %v", err)
	}

	filler, err := store.DiscountedWishlistFillIndex(ctx)
	if err != nil {
		log.Fatalf("read discounted wishlist fill items: %v", err)
	}
	fill := filler.Fill(user.ID)

	items := append(wishlisted, fill...)
	job := domain.DiscountedWishlistJob{
		ID:      fmt.Sprintf("preview-discounted-wishlist-%s-user-%s", now.Format("2006-01-02-150405"), user.ID),
		UserID:  user.ID,
		Date:    now,
		User:    user,
		Items:   items,
		Attempt: 1,
	}

	payload, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(outputPath, payload, 0o600); err != nil {
		log.Fatal(err)
	}
	log.Printf("preview job written: path=%s user_id=%s wishlisted=%d fill=%d", outputPath, user.ID, len(wishlisted), len(fill))
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
