package repository

import (
	"testing"
	"time"

	"github.com/kyou-id/yukari/internal/domain"
)

// The fill ranking used to be MySQL's job (ORDER BY view_count DESC, updated_at DESC,
// LIMIT 12). Moving it into Go is the whole point of the pooled index, and it is also the
// one place a mistake changes the emails without failing anything — so pin the rules.

func candidate(id string, seriesID int64, viewCount int64, updated time.Time) discountedFillCandidate {
	return discountedFillCandidate{
		item:      domain.DiscountedWishlistItem{ID: id},
		seriesID:  seriesID,
		viewCount: viewCount,
		updatedAt: updated,
	}
}

func fillerWith(cands []discountedFillCandidate, series map[string][]int64, owned map[string]map[string]struct{}) *DiscountedWishlistFiller {
	bySeries := make(map[int64][]discountedFillCandidate)
	for _, c := range cands {
		bySeries[c.seriesID] = append(bySeries[c.seriesID], c)
	}
	return &DiscountedWishlistFiller{bySeries: bySeries, series: series, owned: owned}
}

func ids(items []domain.DiscountedWishlistItem) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.ID
	}
	return out
}

func TestFillRanksByViewCountThenRecency(t *testing.T) {
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	f := fillerWith(
		[]discountedFillCandidate{
			candidate("low", 10, 5, recent),
			candidate("high", 10, 99, old),
			candidate("mid-old", 10, 50, old),
			candidate("mid-new", 10, 50, recent),
		},
		map[string][]int64{"u1": {10}},
		nil,
	)

	got := ids(f.Fill("u1"))
	want := []string{"high", "mid-new", "mid-old", "low"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestFillExcludesItemsTheUserAlreadyWishlists(t *testing.T) {
	now := time.Now()
	f := fillerWith(
		[]discountedFillCandidate{
			candidate("owned", 10, 99, now),
			candidate("fresh", 10, 1, now),
		},
		map[string][]int64{"u1": {10}},
		map[string]map[string]struct{}{"u1": {"owned": {}}},
	)

	got := ids(f.Fill("u1"))
	if len(got) != 1 || got[0] != "fresh" {
		t.Fatalf("expected the wishlisted item to be excluded, got %v", got)
	}
}

func TestFillDrawsFromEverySeriesTheUserCaresAbout(t *testing.T) {
	now := time.Now()
	f := fillerWith(
		[]discountedFillCandidate{
			candidate("vocaloid", 10, 50, now),
			candidate("rezero", 20, 99, now),
			candidate("unrelated", 30, 999, now),
		},
		map[string][]int64{"u1": {10, 20}},
		nil,
	)

	got := ids(f.Fill("u1"))
	// Ranked across both series, and the series the user does not care about stays out —
	// even though it is the most viewed item in the pool.
	want := []string{"rezero", "vocaloid"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestFillCapsTheGrid(t *testing.T) {
	now := time.Now()
	var cands []discountedFillCandidate
	for i := 0; i < discountedWishlistFillLimit+8; i++ {
		cands = append(cands, candidate(string(rune('a'+i)), 10, int64(i), now))
	}

	f := fillerWith(cands, map[string][]int64{"u1": {10}}, nil)
	if got := f.Fill("u1"); len(got) != discountedWishlistFillLimit {
		t.Fatalf("expected the grid capped at %d, got %d", discountedWishlistFillLimit, len(got))
	}
}

func TestFillIsEmptyForAUserWithNoDiscountedSeries(t *testing.T) {
	f := fillerWith(
		[]discountedFillCandidate{candidate("x", 10, 1, time.Now())},
		map[string][]int64{},
		nil,
	)
	if got := f.Fill("nobody"); got != nil {
		t.Fatalf("expected no fill for an unknown user, got %v", ids(got))
	}
}

// MySQL left ties in (view_count, updated_at) unordered, so two runs could disagree. The
// Go ranking breaks the tie on item id: same input, same email, every time.
func TestFillBreaksTiesDeterministically(t *testing.T) {
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	build := func() *DiscountedWishlistFiller {
		return fillerWith(
			[]discountedFillCandidate{
				candidate("b", 10, 7, now),
				candidate("c", 10, 7, now),
				candidate("a", 10, 7, now),
			},
			map[string][]int64{"u1": {10}},
			nil,
		)
	}

	want := []string{"a", "b", "c"}
	for run := 0; run < 5; run++ {
		got := ids(build().Fill("u1"))
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("run %d: expected %v, got %v", run, want, got)
			}
		}
	}
}
