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
	VoucherCode   string         `json:"voucher_code,omitempty"`
	VoucherID     int64          `json:"voucher_id,omitempty"`
	WishlistItems []WishlistItem `json:"wishlist_items"`
	FYPItems      []FYPItem      `json:"fyp_items"`
	PopularItems  []FYPItem      `json:"popular_items"`
	Attempt       int            `json:"attempt"`
}

type HistoricalItem struct {
	Name      string    `json:"name"`
	ImageURL  string    `json:"image_url"`
	OrderDate time.Time `json:"order_date"`
	DaysAgo   int       `json:"days_ago"`
}

type AnniversaryJob struct {
	ID             string         `json:"job_id"`
	UserID         string         `json:"user_id"`
	Date           time.Time      `json:"anniversary_date"`
	User           User           `json:"user"`
	Years          int            `json:"years"`
	VoucherCode    string         `json:"voucher_code,omitempty"`
	VoucherID      int64          `json:"voucher_id,omitempty"`
	HistoricalItem HistoricalItem `json:"historical_item"`
	WishlistItems  []WishlistItem `json:"wishlist_items"`
	PopularItems   []FYPItem      `json:"popular_items"`
	Attempt        int            `json:"attempt"`
}

type Voucher struct {
	ID        int64
	Code      string
	Existed   bool
	CreatedAt time.Time
}
