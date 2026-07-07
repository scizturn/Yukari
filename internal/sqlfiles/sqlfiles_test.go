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
	query, err := NewLoader("../../data/sql").Read("discounted_wishlist_fill")
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
	for _, name := range []string{"discounted_wishlist_users", "discounted_wishlist_items", "discounted_wishlist_fill"} {
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
		"u.email_verified_at IS NOT NULL",
		"edl.feature = 'wishlist_back_in'",
		"JSON_CONTAINS",
		"INTERVAL 90 DAY",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("expected wishlist back in query to contain %q", want)
		}
	}
}
