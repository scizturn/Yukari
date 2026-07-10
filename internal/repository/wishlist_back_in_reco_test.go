package repository

import (
	"strings"
	"testing"
	"time"
)

func cand(id string, updated time.Time) wishlistBackInCandidate {
	return wishlistBackInCandidate{id: id, updatedAt: updated}
}

func TestRankWishlistBackInCandidates(t *testing.T) {
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	// B and C tie on score; C is newer so it wins the tiebreak. D has no score entry
	// (treated as 0) even though it is the newest, so it ranks last.
	cands := []wishlistBackInCandidate{
		cand("A", newer),
		cand("B", older),
		cand("C", newer),
		cand("D", time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)),
	}
	scores := map[string]int64{"A": 10, "B": 50, "C": 50}

	got := rankWishlistBackInCandidates(cands, scores, 3)

	want := []string{"C", "B", "A"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestRankWishlistBackInCandidatesReturnsAllWhenUnderLimit(t *testing.T) {
	cands := []wishlistBackInCandidate{cand("A", time.Time{}), cand("B", time.Time{})}
	got := rankWishlistBackInCandidates(cands, map[string]int64{"A": 1, "B": 2}, 6)
	if strings.Join(got, ",") != "B,A" {
		t.Fatalf("expected [B A], got %v", got)
	}
}
