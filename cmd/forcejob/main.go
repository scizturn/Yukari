package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/kyou-id/yukari/internal/config"
	"github.com/kyou-id/yukari/internal/domain"
	"github.com/kyou-id/yukari/internal/queue"
	"github.com/kyou-id/yukari/internal/repository"
	"github.com/kyou-id/yukari/internal/sqlfiles"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()
	if strings.TrimSpace(cfg.DatabaseDSN) == "" {
		log.Fatal("DATABASE_DSN is required")
	}

	location, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		log.Fatalf("load timezone: %v", err)
	}
	now := time.Now().In(location)
	needle := env("YUKARI_FORCE_USER", "abimanyu")

	user, originalActive, err := findUser(ctx, cfg.DatabaseDSN, needle)
	if err != nil {
		log.Fatal(err)
	}
	user.IsActive = true

	store, err := repository.OpenMySQLStore(cfg.DatabaseDSN, sqlfiles.NewLoader(cfg.SQLDir))
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	wishlist, err := store.Wishlist(ctx, user.ID)
	if err != nil {
		log.Fatalf("read wishlist: %v", err)
	}
	fyp, err := store.FYP(ctx, user.ID)
	if err != nil {
		log.Fatalf("read fyp: %v", err)
	}
	popular, err := store.Popular(ctx)
	if err != nil {
		log.Fatalf("read popular: %v", err)
	}

	job := domain.BirthdayJob{
		ID:            fmt.Sprintf("force-birthday-%s-user-%s", now.Format("2006-01-02-150405"), user.ID),
		UserID:        user.ID,
		Date:          now,
		User:          user,
		WishlistItems: wishlist,
		FYPItems:      fyp,
		PopularItems:  popular,
		Attempt:       1,
	}

	redisQueue := queue.NewRedisQueue(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.QueueName)
	defer func() {
		if err := redisQueue.Close(); err != nil {
			log.Printf("redis close failed: %v", err)
		}
	}()
	if err := redisQueue.Enqueue(ctx, job); err != nil {
		log.Fatalf("enqueue force job: %v", err)
	}

	log.Printf("forced birthday job enqueued: queue=%s job_id=%s user_id=%s name=%q email=%s original_active=%t forced_active=%t wishlist=%d fyp=%d popular=%d",
		cfg.QueueName,
		job.ID,
		user.ID,
		user.Name,
		maskEmail(user.Email),
		originalActive,
		user.IsActive,
		len(wishlist),
		len(fyp),
		len(popular),
	)
}

func findUser(ctx context.Context, dsn string, needle string) (domain.User, bool, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return domain.User{}, false, err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return domain.User{}, false, err
	}

	columns, err := userSearchColumns(ctx, db)
	if err != nil {
		return domain.User{}, false, err
	}
	if len(columns) == 0 {
		return domain.User{}, false, fmt.Errorf("no searchable user columns found")
	}

	conditions := make([]string, 0, len(columns))
	args := make([]any, 0, len(columns)+1)
	for _, column := range columns {
		if column == "user_id" {
			conditions = append(conditions, "CAST(user_id AS CHAR) = ?")
			args = append(args, needle)
			continue
		}
		conditions = append(conditions, "LOWER("+column+") LIKE CONCAT('%', LOWER(?), '%')")
		args = append(args, needle)
	}
	args = append(args, needle)

	query := `
SELECT user_id, name, email, birthdate, is_confirmed
FROM users
WHERE ` + strings.Join(conditions, " OR ") + `
ORDER BY CASE WHEN LOWER(name) = LOWER(?) THEN 0 ELSE 1 END, user_id
LIMIT 1`

	var user domain.User
	var active bool
	var birthday sql.NullTime
	err = db.QueryRowContext(ctx, query, args...).Scan(&user.ID, &user.Name, &user.Email, &birthday, &active)
	if err != nil {
		return domain.User{}, false, err
	}
	if birthday.Valid {
		user.Birthday = birthday.Time
	}
	user.IsActive = active
	return user, active, nil
}

func userSearchColumns(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, "SHOW COLUMNS FROM users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	allowed := map[string]bool{
		"user_id":      true,
		"name":         true,
		"email":        true,
		"username":     true,
		"user_name":    true,
		"slug":         true,
		"nickname":     true,
		"display_name": true,
	}
	var columns []string
	for rows.Next() {
		var field, columnType, null, key, extra string
		var defaultValue sql.NullString
		if err := rows.Scan(&field, &columnType, &null, &key, &defaultValue, &extra); err != nil {
			return nil, err
		}
		if allowed[field] {
			columns = append(columns, field)
		}
	}
	return columns, rows.Err()
}

func env(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func maskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" {
		return email
	}
	local := parts[0]
	if len(local) == 1 {
		return local[:1] + "***@" + parts[1]
	}
	return local[:1] + "***" + local[len(local)-1:] + "@" + parts[1]
}
