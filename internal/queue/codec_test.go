package queue

import (
	"strings"
	"testing"
	"time"

	"github.com/kyou-id/yukari/internal/domain"
)

func TestEncodeBirthdayJobIncludesItemCommerceFields(t *testing.T) {
	deadline := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)
	releaseDate := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	job := domain.BirthdayJob{
		ID:     "birthday-2026-05-21-user-123",
		UserID: "123",
		Date:   time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC),
		WishlistItems: []domain.WishlistItem{{
			ID:           "wish-1",
			Name:         "Ready Figure",
			URL:          "https://kyou.id/items/1/",
			ImageURL:     "https://kyoucdn.id/items/ready.jpg.webp",
			Price:        850000,
			Status:       "ready",
			Manufacturer: "Vocaloid",
			SeriesName:   "Zenless Zone Zero",
			PODeadline:   &deadline,
			POReleaseAt:  &releaseDate,
		}},
		FYPItems: []domain.FYPItem{{
			ID:           "fyp-1",
			Name:         "Chara Pick",
			Kind:         "character",
			SeriesID:     "series-1",
			ImageURL:     "https://kyoucdn.id/items/fyp.jpg.webp",
			Price:        150000,
			Status:       "PO",
			Manufacturer: "Good Smile Company",
			SeriesName:   "Honkai: Star Rail",
		}},
		Attempt: 1,
	}

	payload, err := EncodeBirthdayJob(job)
	if err != nil {
		t.Fatalf("expected encode success, got %v", err)
	}

	for _, want := range []string{
		`"price":850000`,
		`"status":"ready"`,
		`"manufacturer":"Vocaloid"`,
		`"series_name":"Zenless Zone Zero"`,
		`"po_deadline":"2026-06-12T00:00:00Z"`,
		`"po_release_at":"2026-07-01T00:00:00Z"`,
		`"price":150000`,
		`"status":"PO"`,
		`"manufacturer":"Good Smile Company"`,
		`"series_name":"Honkai: Star Rail"`,
	} {
		if !strings.Contains(payload, want) {
			t.Fatalf("expected payload to contain %q, got %s", want, payload)
		}
	}
}
