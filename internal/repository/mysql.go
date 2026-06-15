package repository

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/kyou-id/yukari/internal/domain"
	"github.com/kyou-id/yukari/internal/sqlfiles"
)

type MySQLStore struct {
	db      *sql.DB
	queries map[string]string
}

func OpenMySQLStore(dsn string, loader sqlfiles.Loader) (*MySQLStore, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return NewMySQLStore(db, loader)
}

func NewMySQLStore(db *sql.DB, loader sqlfiles.Loader) (*MySQLStore, error) {
	names := []string{"birthday_users", "wishlist_items", "wishlist_items_anniversary", "fyp_items", "popular_items", "user_converted", "anniversary_users", "historical_orders", "leftover_cart_users", "leftover_cart_items", "leftover_cart_reco"}
	queries := make(map[string]string, len(names))
	for _, name := range names {
		query, err := loader.Read(name)
		if err != nil {
			return nil, err
		}
		queries[name] = query
	}
	return &MySQLStore{db: db, queries: queries}, nil
}

func (s *MySQLStore) BirthdayUsers(ctx context.Context, monthDay string) ([]domain.User, error) {
	rows, err := s.db.QueryContext(ctx, s.queries["birthday_users"], monthDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		var user domain.User
		var active bool
		if err := rows.Scan(&user.ID, &user.Name, &user.Email, &user.Birthday, &active); err != nil {
			return nil, err
		}
		user.IsActive = active
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *MySQLStore) AnniversaryUsers(ctx context.Context, monthDay string) ([]domain.User, map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, s.queries["anniversary_users"], monthDay)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var users []domain.User
	yearsMap := make(map[string]int)
	for rows.Next() {
		var user domain.User
		var active bool
		var years int
		if err := rows.Scan(&user.ID, &user.Name, &user.Email, &user.Birthday, &active, &years); err != nil {
			return nil, nil, err
		}
		user.IsActive = active
		users = append(users, user)
		yearsMap[user.ID] = years
	}
	return users, yearsMap, rows.Err()
}

func (s *MySQLStore) HistoricalOrders(ctx context.Context, userID string) ([]domain.HistoricalItem, error) {
	rows, err := s.db.QueryContext(ctx, s.queries["historical_orders"], userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.HistoricalItem
	for rows.Next() {
		var item domain.HistoricalItem
		if err := rows.Scan(&item.Name, &item.ImageURL, &item.OrderDate); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *MySQLStore) Wishlist(ctx context.Context, userID string) ([]domain.WishlistItem, error) {
	return s.wishlistRows(ctx, s.queries["wishlist_items"], userID)
}

func (s *MySQLStore) WishlistAnniversary(ctx context.Context, userID string) ([]domain.WishlistItem, error) {
	return s.wishlistRows(ctx, s.queries["wishlist_items_anniversary"], userID)
}

func (s *MySQLStore) wishlistRows(ctx context.Context, query string, userID string) ([]domain.WishlistItem, error) {
	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.WishlistItem
	for rows.Next() {
		var item domain.WishlistItem
		var poDeadline sql.NullTime
		var poReleaseAt sql.NullTime
		if err := rows.Scan(&item.ID, &item.Name, &item.URL, &item.ImageURL, &item.Price, &item.Status, &item.Manufacturer, &item.SeriesName, &poDeadline, &poReleaseAt); err != nil {
			return nil, err
		}
		item.PODeadline = timePtr(poDeadline)
		item.POReleaseAt = timePtr(poReleaseAt)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *MySQLStore) FYP(ctx context.Context, userID string) ([]domain.FYPItem, error) {
	return s.fypRows(ctx, s.queries["fyp_items"], userID)
}

func (s *MySQLStore) Popular(ctx context.Context) ([]domain.FYPItem, error) {
	return s.fypRows(ctx, s.queries["popular_items"])
}

func (s *MySQLStore) HasConverted(ctx context.Context, userID string, from time.Time, to time.Time) (bool, error) {
	var converted bool
	err := s.db.QueryRowContext(ctx, s.queries["user_converted"], userID, from, to).Scan(&converted)
	return converted, err
}

func (s *MySQLStore) fypRows(ctx context.Context, query string, args ...any) ([]domain.FYPItem, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.FYPItem
	for rows.Next() {
		var item domain.FYPItem
		var poDeadline sql.NullTime
		var poReleaseAt sql.NullTime
		if err := rows.Scan(&item.ID, &item.Name, &item.Kind, &item.SeriesID, &item.ImageURL, &item.Price, &item.Status, &item.Manufacturer, &item.SeriesName, &poDeadline, &poReleaseAt); err != nil {
			return nil, err
		}
		item.PODeadline = timePtr(poDeadline)
		item.POReleaseAt = timePtr(poReleaseAt)
		items = append(items, item)
	}
	return items, rows.Err()
}

func timePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

func (s *MySQLStore) LeftoverCartUsers(ctx context.Context, now time.Time) ([]domain.User, error) {
	rows, err := s.db.QueryContext(ctx, s.queries["leftover_cart_users"], now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		var user domain.User
		var active bool
		var birthday sql.NullTime
		if err := rows.Scan(&user.ID, &user.Name, &user.Email, &birthday, &active); err != nil {
			return nil, err
		}
		user.Birthday = birthday.Time
		user.IsActive = active
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *MySQLStore) CartItems(ctx context.Context, userID string) ([]domain.WishlistItem, error) {
	return s.wishlistRows(ctx, s.queries["leftover_cart_items"], userID)
}

func (s *MySQLStore) LeftoverCartReco(ctx context.Context, userID string) ([]domain.FYPItem, error) {
	return s.fypRows(ctx, s.queries["leftover_cart_reco"], userID, userID)
}
