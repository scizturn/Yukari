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

const maxItems = 5

func main() {
	ctx := context.Background()
	cfg := config.Load()
	location, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		log.Fatal(err)
	}
	now := time.Now().In(location)
	cutoff := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	if now.Weekday() != time.Friday {
		days := (int(now.Weekday()) - int(time.Friday) + 7) % 7
		cutoff = cutoff.AddDate(0, 0, -days)
	}
	startAt := cutoff.AddDate(0, 0, -7)

	store, err := repository.OpenMySQLStore(cfg.DatabaseDSN, sqlfiles.NewLoader(cfg.SQLDir))
	if err != nil {
		log.Fatal(err)
	}

	forced := strings.TrimSpace(os.Getenv("YUKARI_FORCE_USER"))

	rows, err := store.WishlistBackInUserItems(ctx, startAt, cutoff)
	if err != nil {
		log.Fatalf("read wishlist back in user items: %v", err)
	}

	var user domain.User
	var items []domain.WishlistBackInItem
	inWindow := false
	if forced == "" {
		if len(rows) == 0 {
			log.Fatal("no wishlist back in items in window; set YUKARI_FORCE_USER to preview a specific user")
		}
		user, inWindow = rows[0].User, true
	} else {
		for _, row := range rows {
			if row.User.ID == forced {
				user, inWindow = row.User, true
				break
			}
		}
	}

	if inWindow {
		for _, row := range rows {
			if row.User.ID == user.ID && len(items) < maxItems {
				items = append(items, row.Item)
			}
		}
	} else {
		// Forced user has no in-window restock; fall back to their available
		// wishlist items (same source as forcejob) so any user can be previewed.
		user, err = lookupUserByID(ctx, cfg.DatabaseDSN, forced)
		if err != nil {
			log.Fatalf("lookup forced user %s: %v", forced, err)
		}
		items, err = store.WishlistBackInForcedItems(ctx, user.ID)
		if err != nil {
			log.Fatalf("read forced wishlist items: %v", err)
		}
		if len(items) == 0 {
			log.Fatalf("user %s has no available wishlist items to preview", user.ID)
		}
		if len(items) > maxItems {
			items = items[:maxItems]
		}
	}

	companion, err := store.WishlistBackInCompanion(ctx, user.ID)
	if err != nil {
		log.Fatal(err)
	}
	var recos []domain.WishlistBackInItem
	if companion.ID != "" {
		scores, err := store.WishlistBackInPopularityScores(ctx)
		if err != nil {
			log.Fatal(err)
		}
		recos, err = store.WishlistBackInRecommendations(ctx, user.ID, companion.ID, scores)
		if err != nil {
			log.Fatal(err)
		}
		if len(recos) < 6 { // need a full 6; else hide the section
			companion, recos = domain.WishlistBackInItem{}, nil
		}
	}
	// Pick the tier the cron would pick, then show the user's real voucher for that
	// tier if they hold one. Makoto hides the coupon block when the percent is 0,
	// so a preview without the tier would render an email with no voucher at all.
	//
	// Read-only: a preview never mints. A user with no live voucher of this tier
	// gets a stub code, which looks identical in the rendered email.
	tier := reader.WishlistBackInTier(items)
	voucherCode := ""
	if tier > 0 {
		prefix := fmt.Sprintf("WBI%d-", tier)
		stub := prefix + "PREVIEW14D"
		code, amount, found, err := repository.LiveVoucherCode(ctx, cfg.DatabaseDSN, user.ID, prefix+"%", "")
		switch {
		case err != nil:
			log.Printf("voucher lookup failed (%v); using stub %s", err, stub)
			voucherCode = stub
		case !found:
			log.Printf("user %s holds no live %d%% wishlist-back-in voucher; using stub %s", user.ID, tier, stub)
			voucherCode = stub
		default:
			log.Printf("using real voucher %s (%d%%) for user %s", code, amount, user.ID)
			voucherCode = code
		}
	}
	job := domain.WishlistBackInJob{
		ID:     "preview-wishlist-back-in-" + cutoff.Format("2006-01-02") + "-user-" + user.ID,
		UserID: user.ID, Date: now, User: user, VoucherCode: voucherCode,
		VoucherDiscountPercent: tier,
		Items:                  items, CompanionItem: companion, RecoItems: recos, Attempt: 1,
	}
	payload, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	path := env("YUKARI_PREVIEW_JOB_PATH", "/Users/sleepyreinze/Dev/Email-Api/Makoto/templates/preview/wishlist-back-in-job.json")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		log.Fatal(err)
	}
	log.Printf("preview job written: path=%s user_id=%s in_window=%t items=%d reco=%d", path, user.ID, inWindow, len(items), len(recos))
}

// lookupUserByID fetches a user's identity by exact user_id (for previewing a
// specific user who has no restock in the detection window).
func lookupUserByID(ctx context.Context, dsn, id string) (domain.User, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return domain.User{}, err
	}
	defer db.Close()
	var user domain.User
	err = db.QueryRowContext(ctx,
		`SELECT CAST(user_id AS CHAR), name, email FROM users WHERE CAST(user_id AS CHAR) = ? LIMIT 1`, id,
	).Scan(&user.ID, &user.Name, &user.Email)
	user.IsActive = true
	return user, err
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
