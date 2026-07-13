package sqlfiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoaderReadsNamedSQLFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "birthday_users.sql"), []byte("SELECT 1;"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := NewLoader(dir).Read("birthday_users")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "SELECT 1;" {
		t.Fatalf("expected SQL text, got %q", got)
	}
}

func TestLoaderRejectsUnsafeNames(t *testing.T) {
	_, err := NewLoader(t.TempDir()).Read("../secret")
	if err == nil {
		t.Fatal("expected unsafe name error")
	}
}

func TestBirthdayItemQueriesIncludeImageURLs(t *testing.T) {
	loader := NewLoader("../../data/sql")
	for _, name := range []string{"wishlist_items", "fyp_items", "popular_items"} {
		query, err := loader.Read(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, want := range []string{"https://kyoucdn.id/", "images", "image_url"} {
			if !strings.Contains(query, want) {
				t.Fatalf("expected %s query to contain %q, got %q", name, want, query)
			}
		}
	}
}

func TestBirthdayItemQueriesFilterOrderableNonAdultItems(t *testing.T) {
	loader := NewLoader("../../data/sql")
	for _, name := range []string{"wishlist_items", "fyp_items", "popular_items"} {
		query, err := loader.Read(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, want := range []string{
			"i.is_available = 1",
			"i.stock > 0",
			"COALESCE(i.isAdult, 0) = 0",
			"(ip.po_deadline IS NULL OR ip.po_deadline >= CURRENT_DATE)",
		} {
			if !strings.Contains(query, want) {
				t.Fatalf("expected %s query to contain %q, got %q", name, want, query)
			}
		}
	}
}

func TestBirthdayItemQueriesShapeWishlistAndRecommendations(t *testing.T) {
	loader := NewLoader("../../data/sql")

	wishlistQuery, err := loader.Read("wishlist_items")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"i.name AS name",
		"CONCAT('https://kyou.id/items/', i.item_id, '/') AS url",
		"ip.price",
		"i.status",
		"m.name AS manufacturer",
		"s.name AS series_name",
		"ip.po_deadline",
		"ip.po_release_date",
		"ORDER BY i.view_count DESC",
		"LIMIT 1",
	} {
		if !strings.Contains(wishlistQuery, want) {
			t.Fatalf("expected wishlist query to contain %q, got %q", want, wishlistQuery)
		}
	}

	fypQuery, err := loader.Read("fyp_items")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"i.name AS name",
		"ip.price",
		"i.status",
		"m.name AS manufacturer",
		"s.name AS series_name",
		"series_rank <= 3",
		"item_rank = 1",
		"LIMIT 3",
	} {
		if !strings.Contains(fypQuery, want) {
			t.Fatalf("expected fyp query to contain %q, got %q", want, fypQuery)
		}
	}
	for _, unwanted := range []string{"ranked.kind = 'character'", "ranked.kind = 'series'"} {
		if strings.Contains(fypQuery, unwanted) {
			t.Fatalf("expected fyp query not to contain %q, got %q", unwanted, fypQuery)
		}
	}

	popularQuery, err := loader.Read("popular_items")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"ip.price",
		"i.status",
		"m.name AS manufacturer",
		"LIMIT 3",
	} {
		if !strings.Contains(popularQuery, want) {
			t.Fatalf("expected popular query to contain %q, got %q", want, popularQuery)
		}
	}
}

func TestDiscountedWishlistFillOnlyIncludesActiveDiscounts(t *testing.T) {
	query, err := NewLoader("../../data/sql").Read("discounted_wishlist_fill_candidates")
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"i.discount_name IS NOT NULL AND i.discount_name != ''",
		"i.discount_end_date >= CURRENT_DATE",
		"i.discount_price > 0",
		"i.discount_price < ip.price",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("expected discounted wishlist fill query to contain %q, got %q", want, query)
		}
	}
}

func TestDiscountedWishlistQueriesRequireSendableDiscounts(t *testing.T) {
	loader := NewLoader("../../data/sql")
	for _, name := range []string{"discounted_wishlist_users", "discounted_wishlist_items", "discounted_wishlist_fill_candidates"} {
		query, err := loader.Read(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{
			"i.status = 'ready'",
			"i.stock > 0",
			"i.is_available = 1",
			"COALESCE(i.isAdult, 0) = 0",
			"i.discount_price > 0",
			"i.discount_price < ip.price",
		} {
			if !strings.Contains(compactSQL(query), compactSQL(want)) {
				t.Fatalf("expected %s query to contain %q, got %q", name, want, query)
			}
		}
	}
}

// The whole point of splitting the fill into candidate + hydrate is that the candidate
// scan carries neither the per-row image subquery nor the display joins. Reintroduce
// either and the scan pays them for every item in the user's discounted series again,
// which is the cost the split exists to remove.
func TestDiscountedWishlistCandidateQueryStaysLightweight(t *testing.T) {
	query, err := NewLoader("../../data/sql").Read("discounted_wishlist_fill_candidates")
	if err != nil {
		t.Fatal(err)
	}

	// Check the statement, not the comments — the header explains the omissions by name.
	stmt := stripSQLComments(query)
	for _, unwanted := range []string{"FROM images", "manufactures", "LEFT JOIN series"} {
		if strings.Contains(stmt, unwanted) {
			t.Fatalf("candidate query must not contain %q — it ranks bare ids only, got %q", unwanted, stmt)
		}
	}
	if !strings.Contains(compactSQL(stmt), "LIMIT 12") {
		t.Fatalf("expected candidate query to cap the grid at LIMIT 12, got %q", stmt)
	}
}

// The users query was flattened into staged subqueries so the cooldown NOT EXISTS runs
// once per user instead of once per (user, item) pair. DiscountedWishlistUsers scans its
// four columns positionally and passes exactly two params (now, now) — both survive the
// rewrite only by accident unless pinned here.
func TestDiscountedWishlistUsersKeepsItsScanContract(t *testing.T) {
	query, err := NewLoader("../../data/sql").Read("discounted_wishlist_users")
	if err != nil {
		t.Fatal(err)
	}
	stmt := stripSQLComments(query)

	if got, want := selectList(t, stmt), "CAST(u.user_id AS CHAR) AS user_id, u.name, u.email, TRUE AS is_active"; got != want {
		t.Fatalf("users query must project the scanned columns in order:\n got:  %s\n want: %s", got, want)
	}
	if got := strings.Count(stmt, "?"); got != 2 {
		t.Fatalf("users query takes (now, now) — expected 2 placeholders, got %d", got)
	}
	// A single top-level SELECT DISTINCT is the shape the rewrite removed: it sorts wide
	// rows per (user, item) and re-runs the cooldown check for each.
	if strings.HasPrefix(strings.TrimSpace(compactSQL(stmt)), "SELECT DISTINCT") {
		t.Fatalf("users query must not dedupe wide rows at the top level, got %q", stmt)
	}
}

// stripSQLComments drops `--` line comments so an assertion about a query reads the
// statement rather than the prose above it.
func stripSQLComments(query string) string {
	var kept []string
	for _, line := range strings.Split(query, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// discountedWishlistRows scans both queries into the same struct, so the hydrate query
// must project the same columns in the same order as the wishlisted-items query. A
// column added to one and not the other is a scan error at run time, not compile time.
func TestDiscountedWishlistHydrateMatchesItemsColumns(t *testing.T) {
	loader := NewLoader("../../data/sql")
	items, err := loader.Read("discounted_wishlist_items")
	if err != nil {
		t.Fatal(err)
	}
	hydrate, err := loader.Read("discounted_wishlist_hydrate")
	if err != nil {
		t.Fatal(err)
	}

	if selectList(t, items) != selectList(t, hydrate) {
		t.Fatalf("hydrate SELECT list must match discounted_wishlist_items:\n items:   %s\n hydrate: %s",
			selectList(t, items), selectList(t, hydrate))
	}
	if !strings.Contains(hydrate, "/*IDS*/") {
		t.Fatalf("expected hydrate query to carry the /*IDS*/ IN-list token, got %q", hydrate)
	}
}

// selectList returns the compacted text between SELECT and the first FROM.
func selectList(t *testing.T, query string) string {
	t.Helper()
	compact := compactSQL(stripSQLComments(query))
	start := strings.Index(compact, "SELECT ")
	end := strings.Index(compact, " FROM ")
	if start < 0 || end < 0 || end < start {
		t.Fatalf("could not find a SELECT ... FROM in %q", query)
	}
	return compact[start+len("SELECT ") : end]
}

func compactSQL(query string) string {
	return strings.Join(strings.Fields(query), " ")
}

func TestWishlistBackInQueriesEnforceCampaignRules(t *testing.T) {
	query, err := NewLoader("../../data/sql").Read("wishlist_back_in_user_items")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"sl.is_restock = 1",
		"sl.type = 'increase'",
		"sl.description = 'Increased via Insert Stock (Adjusment)'",
		"'$.before_all_stock'",
		"'$.after_all_stock'",
		"i.status IN ('ready', 'PO')",
		"ip.po_deadline IS NULL OR ip.po_deadline >= CURRENT_DATE",
		"i.stock > 0",
		"u.email IS NOT NULL AND u.email <> ''",
		"edl.feature IN ('wishlist_back_in', 'po_ready')",
		"JSON_CONTAINS",
		"INTERVAL 90 DAY",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("expected wishlist back in query to contain %q", want)
		}
	}
}

// order_items.item_price is NULL on ~50k historical rows. Scanning it bare into an
// int killed a whole Friday run with "converting NULL to int is unsupported", so
// every scanned column of this query must survive a NULL.
func TestWishlistBackInCompanionSurvivesNullPrices(t *testing.T) {
	query, err := NewLoader("../../data/sql").Read("wishlist_back_in_companion")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(query, "oi.item_price  ") || !strings.Contains(query, "COALESCE(oi.item_price") {
		t.Fatal("expected companion query to COALESCE oi.item_price, which is NULL on ~50k rows")
	}
}

// The reco fast path is series-index-driven and does NOT aggregate popularity
// (that is loaded once via the scores file and applied in Go); the fallback scans
// the category and still ranks in SQL. Both keep the buyability filters/exclusions.
func TestWishlistBackInRecoIsSeriesDrivenWithCategoryFallback(t *testing.T) {
	loader := NewLoader("../../data/sql")
	series, err := loader.Read("wishlist_back_in_reco")
	if err != nil {
		t.Fatal(err)
	}
	// Lightweight: series-index-driven, id + recency tiebreak only. No image lookup,
	// no display joins, no aggregate, no LIMIT — the reader ranks against the loaded
	// popularity map and hydrates only the winners.
	for _, want := range []string{
		"JOIN item_products ip ON ip.series_id = target.series_id",
		"target.series_id > 0",
		"AS raw_updated_at",
	} {
		if !strings.Contains(series, want) {
			t.Fatalf("series candidate query must contain %q", want)
		}
	}
	for _, forbidden := range []string{"INTERVAL 14 DAY", "ip.category_id = target.category_id", "kyoucdn.id", "LIMIT"} {
		if strings.Contains(series, forbidden) {
			t.Fatalf("series candidate query must not contain %q (it is bare ids; images/popularity/trim come later)", forbidden)
		}
	}

	// Hydrate returns the full display columns for the ranked ids via an injected
	// IN list.
	hydrate, err := loader.Read("wishlist_back_in_reco_hydrate")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"/*IDS*/", "image_url", "discount_price"} {
		if !strings.Contains(hydrate, want) {
			t.Fatalf("hydrate query must contain %q", want)
		}
	}

	category, err := loader.Read("wishlist_back_in_reco_category")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"target.category_id IS NOT NULL",
		"ip.category_id = target.category_id",
		"INTERVAL 14 DAY", // the rare fallback still ranks by popularity in SQL
		"LIMIT 6",
	} {
		if !strings.Contains(category, want) {
			t.Fatalf("category fallback must contain %q", want)
		}
	}

	// Buyability + exclusion invariants shared by both reco files.
	for _, q := range []string{series, category} {
		for _, want := range []string{"i.status = 'ready'", "i.is_available = 1", "i.stock > 0", "NOT EXISTS"} {
			if !strings.Contains(q, want) {
				t.Fatalf("reco query missing %q", want)
			}
		}
	}

	// The once-per-run popularity source: same 14-day weighted score as the fallback.
	scores, err := loader.Read("wishlist_back_in_reco_scores")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"INTERVAL 14 DAY",
		"WHEN 'view' THEN 1 WHEN 'wishlist' THEN 3 WHEN 'cart' THEN 5 WHEN 'bought' THEN 10",
		"GROUP BY uia.item_id",
	} {
		if !strings.Contains(scores, want) {
			t.Fatalf("reco scores query missing %q", want)
		}
	}
}

// The two campaigns are partitioned by event type. A conversion sends the item to
// po-ready; a 0->>0 restock keeps it here. The gate below is what makes that
// deterministic — without it the winner is whichever cron happens to run first.
func TestBackInStandsDownOnItemsPoReadyWillClaim(t *testing.T) {
	backIn, err := NewLoader("../../data/sql").Read("wishlist_back_in_user_items")
	if err != nil {
		t.Fatal(err)
	}
	poReady, err := NewLoader("../../data/sql").Read("po_ready_user_items")
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"i.status = 'ready'",
		"'convert po by excel'",
		"'convert po manual'",
		"'reconvert PO to ready'",
		// Must mirror po-ready's own window, not "has ever been converted", or an
		// item converted long ago and restocked today is rejected here while
		// sitting outside po-ready's reach — announced by nobody.
		"conv.created_at >= DATE_SUB(NOW(), INTERVAL 7 DAY)",
	} {
		if !strings.Contains(backIn, want) {
			t.Fatalf("expected wishlist back in gate to contain %q", want)
		}
	}

	// The gate's window is only correct while po-ready's own window agrees with it.
	// po_ready.go's poReadyWindow is the source of truth; this pins the SQL side.
	if !strings.Contains(poReady, "last 7 days") {
		t.Fatal("po ready window changed: update the 7-day gate in wishlist_back_in_user_items.sql to match")
	}
}

func TestPoReadyQueryEnforcesCampaignRules(t *testing.T) {
	query, err := NewLoader("../../data/sql").Read("po_ready_user_items")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		// The three admin paths that convert PO stock to ready. `reconvert PO to
		// ready` alone would duplicate wishlist-back-in for 98% of its items.
		"'convert po by excel'",
		"'convert po manual'",
		"'reconvert PO to ready'",
		// The conversion events carry no stock data, so availability must come
		// from current item state or the email links to an out-of-stock item.
		"i.status = 'ready'",
		"i.stock > 0",
		"i.is_available = 1",
		"u.email IS NOT NULL AND u.email <> ''",
		// Dedup spans both item-news features, or one item earns two emails.
		"edl.feature IN ('po_ready', 'wishlist_back_in')",
		"JSON_CONTAINS",
		"INTERVAL 90 DAY",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("expected po ready query to contain %q", want)
		}
	}
	// One item logs one conversion row per branch; without the rollup a user's
	// 5-item cap would be spent on duplicates of the same item.
	if !strings.Contains(query, "MAX(sl.created_at)") || !strings.Contains(query, "GROUP BY") {
		t.Fatal("expected po ready query to collapse per-branch conversion rows")
	}
}
