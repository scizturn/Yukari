package repository

import (
	"context"
	"database/sql"
	"sort"
	"strings"
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
	names := []string{"birthday_users", "wishlist_items", "wishlist_items_winback", "wishlist_items_anniversary", "fyp_items", "popular_items", "user_converted", "anniversary_users", "historical_orders", "leftover_cart_users", "leftover_cart_items", "leftover_cart_reco", "discounted_wishlist_users", "discounted_wishlist_items", "discounted_wishlist_fill_pool", "discounted_wishlist_fill_series", "discounted_wishlist_fill_owned", "winback_users", "winback_fill_items", "wishlist_back_in_user_items", "wishlist_back_in_companion", "wishlist_back_in_reco", "wishlist_back_in_reco_category", "wishlist_back_in_reco_scores", "wishlist_back_in_reco_hydrate", "wishlist_back_in_forced_items", "po_ready_user_items", "po_ready_forced_items"}
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
		var birthday sql.NullTime
		if err := rows.Scan(&user.ID, &user.Name, &user.Email, &birthday, &active, &years); err != nil {
			return nil, nil, err
		}
		if birthday.Valid {
			user.Birthday = birthday.Time
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
		if err := rows.Scan(&item.Name, &item.SeriesName, &item.ImageURL, &item.URL, &item.OrderDate); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *MySQLStore) Wishlist(ctx context.Context, userID string) ([]domain.WishlistItem, error) {
	return s.wishlistRows(ctx, s.queries["wishlist_items"], userID)
}

// WishlistWinback returns up to 12 of the user's ready wishlist items (vs
// Wishlist which is LIMIT 1 for birthday). Winback fills its grid with the
// user's real wishlist first, so it needs more than one.
func (s *MySQLStore) WishlistWinback(ctx context.Context, userID string) ([]domain.WishlistItem, error) {
	return s.wishlistRows(ctx, s.queries["wishlist_items_winback"], userID)
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

// WinbackFillItems returns the most-popular READY items used to fill the winback
// wishlist grid up to 12.
func (s *MySQLStore) WinbackFillItems(ctx context.Context) ([]domain.FYPItem, error) {
	return s.fypRows(ctx, s.queries["winback_fill_items"])
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
	return s.fypRows(ctx, s.queries["leftover_cart_reco"], userID, userID, userID)
}

func (s *MySQLStore) DiscountedWishlistUsers(ctx context.Context, now time.Time) ([]domain.User, error) {
	rows, err := s.db.QueryContext(ctx, s.queries["discounted_wishlist_users"], now, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		var user domain.User
		var active bool
		if err := rows.Scan(&user.ID, &user.Name, &user.Email, &active); err != nil {
			return nil, err
		}
		user.IsActive = active
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *MySQLStore) DiscountedWishlistItems(ctx context.Context, userID string) ([]domain.DiscountedWishlistItem, error) {
	return s.discountedWishlistRows(ctx, s.queries["discounted_wishlist_items"], true, userID)
}

// discountedWishlistFillLimit is the size of the fill grid — the LIMIT the old per-user
// fill query carried.
const discountedWishlistFillLimit = 12

// discountedFillCandidate is a pool row plus the columns the per-user ranking needs but
// the email does not show.
type discountedFillCandidate struct {
	item      domain.DiscountedWishlistItem
	seriesID  int64
	viewCount int64
	updatedAt time.Time
}

// DiscountedWishlistFiller serves the fill grid for any user out of memory.
type DiscountedWishlistFiller struct {
	bySeries map[int64][]discountedFillCandidate
	series   map[string][]int64
	owned    map[string]map[string]struct{}
}

// DiscountedWishlistFillIndex loads the whole fill grid in three queries, once per run.
//
// The old shape ran one query per user. On 13 Jul 2026 that was 32,771 users — 32,771
// executions of a query whose candidate set barely differs between them. The candidates
// are in fact identical for everyone: every buyable discounted item. Only two things are
// per-user, and both are cheap set operations: which series the user cares about, and
// which of those items they already wishlist. So pull the pool once and do the rest in
// memory.
//
// Ranking is re-done per user (Fill) rather than baked into the pool order, because an
// item can be reached through more than one of a user's series.
func (s *MySQLStore) DiscountedWishlistFillIndex(ctx context.Context) (*DiscountedWishlistFiller, error) {
	pool, err := s.discountedWishlistFillPool(ctx)
	if err != nil {
		return nil, err
	}
	series, err := s.discountedWishlistFillSeries(ctx)
	if err != nil {
		return nil, err
	}
	owned, err := s.discountedWishlistFillOwned(ctx)
	if err != nil {
		return nil, err
	}

	bySeries := make(map[int64][]discountedFillCandidate, len(pool))
	for _, c := range pool {
		bySeries[c.seriesID] = append(bySeries[c.seriesID], c)
	}
	return &DiscountedWishlistFiller{bySeries: bySeries, series: series, owned: owned}, nil
}

// Fill returns the user's fill grid: buyable discounted items from the series they care
// about, minus the ones already on their wishlist, ranked and capped.
//
// The ranking mirrors the old SQL's `ORDER BY i.view_count DESC, i.updated_at DESC` and
// then adds item id as a final tiebreak. MySQL left ties unordered, so this is stricter
// than what it replaced, not different: a run is now reproducible.
func (f *DiscountedWishlistFiller) Fill(userID string) []domain.DiscountedWishlistItem {
	seriesIDs := f.series[userID]
	if len(seriesIDs) == 0 {
		return nil
	}
	owned := f.owned[userID]

	var cands []discountedFillCandidate
	for _, seriesID := range seriesIDs {
		for _, c := range f.bySeries[seriesID] {
			if _, ok := owned[c.item.ID]; ok {
				continue
			}
			cands = append(cands, c)
		}
	}
	sort.SliceStable(cands, func(a, b int) bool {
		if cands[a].viewCount != cands[b].viewCount {
			return cands[a].viewCount > cands[b].viewCount
		}
		if !cands[a].updatedAt.Equal(cands[b].updatedAt) {
			return cands[a].updatedAt.After(cands[b].updatedAt)
		}
		return cands[a].item.ID < cands[b].item.ID
	})
	if len(cands) > discountedWishlistFillLimit {
		cands = cands[:discountedWishlistFillLimit]
	}

	items := make([]domain.DiscountedWishlistItem, 0, len(cands))
	for _, c := range cands {
		items = append(items, c.item)
	}
	return items
}

func (s *MySQLStore) discountedWishlistFillPool(ctx context.Context) ([]discountedFillCandidate, error) {
	rows, err := s.db.QueryContext(ctx, s.queries["discounted_wishlist_fill_pool"])
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pool []discountedFillCandidate
	for rows.Next() {
		var c discountedFillCandidate
		var discountPrice sql.NullInt64
		var discountName sql.NullString
		var discountEnd sql.NullTime
		if err := rows.Scan(
			&c.item.ID, &c.item.Name, &c.item.CharacterName, &c.item.URL, &c.item.ImageURL,
			&c.item.OriginalPrice, &discountPrice, &discountName, &discountEnd, &c.item.Status,
			&c.item.Manufacturer, &c.item.SeriesName,
			&c.seriesID, &c.viewCount, &c.updatedAt,
		); err != nil {
			return nil, err
		}
		c.item.DiscountPrice = int(discountPrice.Int64)
		c.item.DiscountName = discountName.String
		c.item.DiscountEnd = timePtr(discountEnd)
		c.item.IsWishlisted = false
		pool = append(pool, c)
	}
	return pool, rows.Err()
}

func (s *MySQLStore) discountedWishlistFillSeries(ctx context.Context) (map[string][]int64, error) {
	rows, err := s.db.QueryContext(ctx, s.queries["discounted_wishlist_fill_series"])
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	series := make(map[string][]int64)
	for rows.Next() {
		var userID string
		var seriesID int64
		if err := rows.Scan(&userID, &seriesID); err != nil {
			return nil, err
		}
		series[userID] = append(series[userID], seriesID)
	}
	return series, rows.Err()
}

func (s *MySQLStore) discountedWishlistFillOwned(ctx context.Context) (map[string]map[string]struct{}, error) {
	rows, err := s.db.QueryContext(ctx, s.queries["discounted_wishlist_fill_owned"])
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	owned := make(map[string]map[string]struct{})
	for rows.Next() {
		var userID, itemID string
		if err := rows.Scan(&userID, &itemID); err != nil {
			return nil, err
		}
		if owned[userID] == nil {
			owned[userID] = make(map[string]struct{})
		}
		owned[userID][itemID] = struct{}{}
	}
	return owned, rows.Err()
}

func (s *MySQLStore) discountedWishlistRows(ctx context.Context, query string, isWishlisted bool, args ...any) ([]domain.DiscountedWishlistItem, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.DiscountedWishlistItem
	for rows.Next() {
		var item domain.DiscountedWishlistItem
		var discountPrice sql.NullInt64
		var discountName sql.NullString
		var discountEnd sql.NullTime
		if err := rows.Scan(&item.ID, &item.Name, &item.CharacterName, &item.URL, &item.ImageURL, &item.OriginalPrice, &discountPrice, &discountName, &discountEnd, &item.Status, &item.Manufacturer, &item.SeriesName); err != nil {
			return nil, err
		}
		item.DiscountPrice = int(discountPrice.Int64)
		item.DiscountName = discountName.String
		item.DiscountEnd = timePtr(discountEnd)
		item.IsWishlisted = isWishlisted
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *MySQLStore) PoReadyUserItems(ctx context.Context, startAt, endAt time.Time) ([]domain.PoReadyUserItem, error) {
	rows, err := s.db.QueryContext(ctx, s.queries["po_ready_user_items"], startAt, endAt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.PoReadyUserItem
	for rows.Next() {
		var row domain.PoReadyUserItem
		if err := rows.Scan(
			&row.User.ID, &row.User.Name, &row.User.Email, &row.User.IsActive,
			&row.Item.ID, &row.Item.Name, &row.Item.URL, &row.Item.ImageURL,
			&row.Item.Price, &row.Item.ReadyAt, &row.Item.DiscountPrice,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// PoReadyForcedItems is used by forcejob/previewjob to seed a test send: a user's
// currently-ready wishlist items, ignoring the conversion window and the dedup.
func (s *MySQLStore) PoReadyForcedItems(ctx context.Context, userID string) ([]domain.PoReadyItem, error) {
	rows, err := s.db.QueryContext(ctx, s.queries["po_ready_forced_items"], userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.PoReadyItem
	for rows.Next() {
		var item domain.PoReadyItem
		if err := rows.Scan(&item.ID, &item.Name, &item.URL, &item.ImageURL, &item.Price, &item.ReadyAt, &item.DiscountPrice); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *MySQLStore) WishlistBackInUserItems(ctx context.Context, startAt, endAt time.Time) ([]domain.WishlistBackInUserItem, error) {
	rows, err := s.db.QueryContext(ctx, s.queries["wishlist_back_in_user_items"], startAt, endAt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.WishlistBackInUserItem
	for rows.Next() {
		var row domain.WishlistBackInUserItem
		var gpRatio sql.NullFloat64
		if err := rows.Scan(
			&row.User.ID, &row.User.Name, &row.User.Email, &row.User.IsActive,
			&row.Item.ID, &row.Item.Name, &row.Item.URL, &row.Item.ImageURL, &row.Item.Price, &row.Item.Status,
			&row.Item.Manufacturer, &row.Item.SeriesName, &row.Item.CategoryName, &row.Item.RestockedAt,
			&row.Item.DiscountPrice, &row.Item.DownPayment, &gpRatio,
		); err != nil {
			return nil, err
		}
		if gpRatio.Valid {
			row.Item.GPRatio = &gpRatio.Float64
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *MySQLStore) WishlistBackInCompanion(ctx context.Context, userID string) (domain.WishlistBackInItem, error) {
	var item domain.WishlistBackInItem
	err := s.db.QueryRowContext(ctx, s.queries["wishlist_back_in_companion"], userID).Scan(
		&item.ID, &item.Name, &item.URL, &item.ImageURL, &item.Price, &item.Status,
		&item.Manufacturer, &item.SeriesName, &item.CategoryName, &item.RestockedAt,
	)
	if err == sql.ErrNoRows {
		return domain.WishlistBackInItem{}, nil
	}
	return item, err
}

// wishlistBackInRecoTarget is the size the cross-sell grid needs; it mirrors the
// reader's wishlistBackInRecoCount. The series query fills it on the fast path; only
// when the series comes up short does the slow category fallback run to top it up.
const wishlistBackInRecoTarget = 6

// WishlistBackInPopularityScores loads the 14-day "Most Popular" score per item
// once per run (wishlist_back_in_reco_scores.sql). The reader passes the map into
// every WishlistBackInRecommendations call so the series query does not re-aggregate
// user_item_actions per user.
func (s *MySQLStore) WishlistBackInPopularityScores(ctx context.Context) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx, s.queries["wishlist_back_in_reco_scores"])
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	scores := make(map[string]int64)
	for rows.Next() {
		var id string
		var score int64
		if err := rows.Scan(&id, &score); err != nil {
			return nil, err
		}
		scores[id] = score
	}
	return scores, rows.Err()
}

// wishlistBackInCandidate is a bare series candidate: just its id and the recency
// tiebreak. Full display columns are hydrated later for only the ranked winners.
type wishlistBackInCandidate struct {
	id        string
	updatedAt time.Time
}

func (s *MySQLStore) WishlistBackInRecommendations(ctx context.Context, userID, anchorItemID string, scores map[string]int64) ([]domain.WishlistBackInItem, error) {
	// Fast path: pull bare same-series candidate ids (cheap even for huge series),
	// rank them in Go by the prebuilt popularity map (highest score first, newest as
	// tiebreak — matching /search), then hydrate only the top few.
	cands, err := s.wishlistBackInSeriesCandidates(ctx, anchorItemID, userID)
	if err != nil {
		return nil, err
	}
	rankedIDs := rankWishlistBackInCandidates(cands, scores, wishlistBackInRecoTarget)
	items, err := s.wishlistBackInHydrate(ctx, rankedIDs)
	if err != nil {
		return nil, err
	}
	if len(items) >= wishlistBackInRecoTarget {
		return items, nil
	}

	// Series came up short (small or absent series). Top up from the same category.
	// That query scans the items table, so it only runs for the minority of anchors
	// that need it, and it hydrates + ranks by popularity itself. Dedupe by item id —
	// an item can match both series and category.
	catItems, err := s.wishlistBackInRecoQuery(ctx, "wishlist_back_in_reco_category", anchorItemID, userID)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(items))
	for _, it := range items {
		seen[it.ID] = struct{}{}
	}
	for _, it := range catItems {
		if len(items) >= wishlistBackInRecoTarget {
			break
		}
		if _, dup := seen[it.ID]; dup {
			continue
		}
		seen[it.ID] = struct{}{}
		items = append(items, it)
	}
	return items, nil
}

// rankWishlistBackInCandidates orders candidates by popularity score (desc), then by
// recency (desc) as the tiebreak, and returns the top `limit` item ids. This
// reproduces the old SQL `ORDER BY search_score DESC, updated_at DESC` now that
// scoring is done against the once-loaded map instead of a per-query aggregate.
func rankWishlistBackInCandidates(cands []wishlistBackInCandidate, scores map[string]int64, limit int) []string {
	sort.SliceStable(cands, func(a, b int) bool {
		sa, sb := scores[cands[a].id], scores[cands[b].id]
		if sa != sb {
			return sa > sb
		}
		return cands[a].updatedAt.After(cands[b].updatedAt)
	})
	if len(cands) > limit {
		cands = cands[:limit]
	}
	ids := make([]string, len(cands))
	for i, c := range cands {
		ids[i] = c.id
	}
	return ids
}

func (s *MySQLStore) wishlistBackInSeriesCandidates(ctx context.Context, anchorItemID, userID string) ([]wishlistBackInCandidate, error) {
	rows, err := s.db.QueryContext(ctx, s.queries["wishlist_back_in_reco"], anchorItemID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cands []wishlistBackInCandidate
	for rows.Next() {
		var c wishlistBackInCandidate
		if err := rows.Scan(&c.id, &c.updatedAt); err != nil {
			return nil, err
		}
		cands = append(cands, c)
	}
	return cands, rows.Err()
}

// wishlistBackInHydrate fetches full display columns for the ranked ids and returns
// them IN THAT ORDER (SQL IN does not preserve order, so we reindex).
func (s *MySQLStore) wishlistBackInHydrate(ctx context.Context, ids []string) ([]domain.WishlistBackInItem, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	// ReplaceAll (not Replace, 1): the token must not survive anywhere, or a stray
	// `/*IDS*/` would parse as an empty comment and break the IN list.
	query := strings.ReplaceAll(s.queries["wishlist_back_in_reco_hydrate"], "/*IDS*/", placeholders)
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byID := make(map[string]domain.WishlistBackInItem, len(ids))
	for rows.Next() {
		var item domain.WishlistBackInItem
		if err := rows.Scan(
			&item.ID, &item.Name, &item.URL, &item.ImageURL, &item.Price, &item.Status,
			&item.Manufacturer, &item.SeriesName, &item.CategoryName, &item.RestockedAt,
			&item.DiscountPrice, &item.DownPayment,
		); err != nil {
			return nil, err
		}
		byID[item.ID] = item
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	items := make([]domain.WishlistBackInItem, 0, len(ids))
	for _, id := range ids {
		if it, ok := byID[id]; ok {
			items = append(items, it)
		}
	}
	return items, nil
}

func (s *MySQLStore) wishlistBackInRecoQuery(ctx context.Context, name, anchorItemID, userID string) ([]domain.WishlistBackInItem, error) {
	rows, err := s.db.QueryContext(ctx, s.queries[name], anchorItemID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.WishlistBackInItem
	for rows.Next() {
		var item domain.WishlistBackInItem
		if err := rows.Scan(
			&item.ID, &item.Name, &item.URL, &item.ImageURL, &item.Price, &item.Status,
			&item.Manufacturer, &item.SeriesName, &item.CategoryName, &item.RestockedAt,
			&item.DiscountPrice, &item.DownPayment,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// WishlistBackInForcedItems is used by forcejob to seed a test send: a user's
// available wishlist items, bypassing the restock-window + dedup eligibility.
func (s *MySQLStore) WishlistBackInForcedItems(ctx context.Context, userID string) ([]domain.WishlistBackInItem, error) {
	rows, err := s.db.QueryContext(ctx, s.queries["wishlist_back_in_forced_items"], userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.WishlistBackInItem
	for rows.Next() {
		var item domain.WishlistBackInItem
		var gpRatio sql.NullFloat64
		if err := rows.Scan(
			&item.ID, &item.Name, &item.URL, &item.ImageURL, &item.Price, &item.Status,
			&item.Manufacturer, &item.SeriesName, &item.CategoryName, &item.RestockedAt,
			&item.DiscountPrice, &item.DownPayment, &gpRatio,
		); err != nil {
			return nil, err
		}
		if gpRatio.Valid {
			item.GPRatio = &gpRatio.Float64
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *MySQLStore) WinbackUsers(ctx context.Context, now time.Time) ([]domain.User, error) {
	rows, err := s.db.QueryContext(ctx, s.queries["winback_users"], now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		var user domain.User
		var active bool
		var createdAt sql.NullTime
		if err := rows.Scan(&user.ID, &user.Name, &user.Email, &createdAt, &active); err != nil {
			return nil, err
		}
		if createdAt.Valid {
			user.CreatedAt = createdAt.Time
		}
		user.IsActive = active
		users = append(users, user)
	}
	return users, rows.Err()
}
