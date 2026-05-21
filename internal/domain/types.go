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
	ID    string `json:"id"`
	Name  string `json:"name"`
	URL   string `json:"url"`
	Price int    `json:"price"`
}

type FYPItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	SeriesID string `json:"series_id"`
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
