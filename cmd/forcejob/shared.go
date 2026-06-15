package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/kyou-id/yukari/internal/domain"
)

// findUserByNeedle searches by user_id, name, email, or username (fuzzy).
// Used by birthday which accepts a name/id needle from env.
func findUserByNeedle(ctx context.Context, dsn string, needle string) (domain.User, bool, error) {
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

// findUserByID looks up a user by exact user_id.
// Used by anniversary and leftover-cart which require YUKARI_FORCE_USER to be a numeric ID.
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
SELECT user_id, name, email, birthdate, email_verified_at IS NOT NULL
FROM users
WHERE CAST(user_id AS CHAR) = ?
LIMIT 1`, userID).Scan(&user.ID, &user.Name, &user.Email, &birthday, &user.IsActive)
	if err != nil {
		return domain.User{}, fmt.Errorf("user %s not found: %w", userID, err)
	}
	if birthday.Valid {
		user.Birthday = birthday.Time
	}
	return user, nil
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

func env(key, fallback string) string {
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
