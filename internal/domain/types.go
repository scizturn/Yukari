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

type LeftoverCartJob struct {
	ID             string         `json:"job_id"`
	UserID         string         `json:"user_id"`
	Date           time.Time      `json:"date"`
	User           User           `json:"user"`
	HistoricalItem HistoricalItem `json:"historical_item"`
	CartItems      []WishlistItem `json:"cart_items"`
	RecoItems      []FYPItem      `json:"reco_items"`
	Attempt        int            `json:"attempt"`
}

type DiscountedWishlistItem struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	URL           string     `json:"url"`
	ImageURL      string     `json:"image_url"`
	OriginalPrice int        `json:"original_price"`
	DiscountPrice int        `json:"discount_price"`
	DiscountName  string     `json:"discount_name"`
	DiscountEnd   *time.Time `json:"discount_end,omitempty"`
	Status        string     `json:"status"`
	Manufacturer  string     `json:"manufacturer"`
	SeriesName    string     `json:"series_name"`
	IsWishlisted  bool       `json:"is_wishlisted"`
}

type WinbackJob struct {
	ID             string         `json:"job_id"`
	UserID         string         `json:"user_id"`
	Date           time.Time      `json:"date"`
	User           User           `json:"user"`
	VoucherCode    string         `json:"voucher_code,omitempty"`
	VoucherID      int64          `json:"voucher_id,omitempty"`
	WishlistItems  []WishlistItem `json:"wishlist_items"`
	HistoricalItem HistoricalItem `json:"historical_item"`
	PopularItems   []FYPItem      `json:"popular_items"`
	Attempt        int            `json:"attempt"`
}

type DiscountedWishlistJob struct {
	ID      string                   `json:"job_id"`
	UserID  string                   `json:"user_id"`
	Date    time.Time                `json:"date"`
	User    User                     `json:"user"`
	Items   []DiscountedWishlistItem `json:"items"`
	Attempt int                      `json:"attempt"`
}

type WishlistBackInItem struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	URL          string    `json:"url"`
	ImageURL     string    `json:"image_url"`
	Price        int       `json:"price"`
	Status       string    `json:"status"`
	Manufacturer string    `json:"manufacturer"`
	SeriesName   string    `json:"series_name"`
	CategoryName string    `json:"category_name"`
	PopularScore int       `json:"popular_score"`
	RestockedAt  time.Time `json:"restocked_at"`
}

type WishlistBackInJob struct {
	ID            string             `json:"job_id"`
	UserID        string             `json:"user_id"`
	Date          time.Time          `json:"date"`
	User          User               `json:"user"`
	VoucherCode   string             `json:"voucher_code,omitempty"`
	VoucherID     int64              `json:"voucher_id,omitempty"`
	Item          WishlistBackInItem `json:"item"`
	CompanionItem WishlistBackInItem `json:"companion_item"`
	Attempt       int                `json:"attempt"`
}
