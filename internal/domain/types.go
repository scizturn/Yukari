package domain

import "time"

type User struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	Birthday time.Time `json:"birthday"`
	IsActive bool      `json:"is_active"`
}

type WishlistItem struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	URL          string     `json:"url"`
	ImageURL     string     `json:"image_url"`
	Price        int        `json:"price"`
	Status       string     `json:"status"`
	Manufacturer string     `json:"manufacturer"`
	SeriesName   string     `json:"series_name"`
	PODeadline   *time.Time `json:"po_deadline,omitempty"`
	POReleaseAt  *time.Time `json:"po_release_at,omitempty"`
}

type FYPItem struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Kind         string     `json:"kind"`
	SeriesID     string     `json:"series_id"`
	ImageURL     string     `json:"image_url"`
	Price        int        `json:"price"`
	Status       string     `json:"status"`
	Manufacturer string     `json:"manufacturer"`
	SeriesName   string     `json:"series_name"`
	PODeadline   *time.Time `json:"po_deadline,omitempty"`
	POReleaseAt  *time.Time `json:"po_release_at,omitempty"`
}

type BirthdayJob struct {
	ID            string         `json:"job_id"`
	UserID        string         `json:"user_id"`
	Date          time.Time      `json:"birthday_date"`
	User          User           `json:"user"`
	WishlistItems []WishlistItem `json:"wishlist_items"`
	FYPItems      []FYPItem      `json:"fyp_items"`
	PopularItems  []FYPItem      `json:"popular_items"`
	Attempt       int            `json:"attempt"`
}
