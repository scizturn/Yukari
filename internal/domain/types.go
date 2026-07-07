package domain

import "time"

type User struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	Birthday time.Time `json:"birthday"`
	// CreatedAt is when the user's Kyou account was created. Winback templates
	// render "sejak <year>" from it; zero value falls back to Kyou's founding year.
	CreatedAt time.Time `json:"created_at"`
	IsActive  bool      `json:"is_active"`
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
	IsWishlisted bool       `json:"is_wishlisted,omitempty"`
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
	Name       string    `json:"name"`
	SeriesName string    `json:"series_name,omitempty"`
	ImageURL   string    `json:"image_url"`
	URL        string    `json:"url,omitempty"`
	OrderDate  time.Time `json:"order_date"`
	DaysAgo    int       `json:"days_ago"`
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
	CharacterName string     `json:"character_name"`
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
	// HistoricalItem is the single most-recent order, kept for audit metadata
	// and backward compatibility with jobs enqueued before HistoricalItems.
	HistoricalItem HistoricalItem `json:"historical_item"`
	// HistoricalItems is the user's most-recent orders (up to 3, newest first)
	// rendered as the "past collection" list.
	HistoricalItems []HistoricalItem `json:"historical_items,omitempty"`
	PopularItems    []FYPItem        `json:"popular_items"`
	Attempt         int              `json:"attempt"`
}

type DiscountedWishlistJob struct {
	ID      string                   `json:"job_id"`
	UserID  string                   `json:"user_id"`
	Date    time.Time                `json:"date"`
	User    User                     `json:"user"`
	Items   []DiscountedWishlistItem `json:"items"`
	Attempt int                      `json:"attempt"`
}

type PoReadyItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	ImageURL string `json:"image_url"`
	Price    int    `json:"price"`
	Quantity int    `json:"quantity"`
}

// PoReadyJob is a per-order pelunasan reminder: a PO the user paid a DP on has
// arrived (item.status='ready') while the order still owes a balance.
type PoReadyJob struct {
	ID          string        `json:"job_id"`
	OrderID     string        `json:"order_id"`
	UserID      string        `json:"user_id"`
	Date        time.Time     `json:"date"`
	User        User          `json:"user"`
	Items       []PoReadyItem `json:"items"`
	Remaining   int           `json:"remaining"`
	DownPayment int           `json:"down_payment"`
	ETA         string        `json:"eta"`
	Attempt     int           `json:"attempt"`
}

// PoReadyOrder is the per-order header Yukari reads before pulling the order's
// ready items. It is not part of the wire contract (Makoto never sees it).
type PoReadyOrder struct {
	User        User
	OrderID     string
	Remaining   int
	DownPayment int
	ETA         string
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
	RestockedAt  time.Time `json:"restocked_at"`
	// DiscountPrice is the active discounted price (0 if not discounted). Price
	// stays the original; the template struck-through Price when this is set.
	DiscountPrice int `json:"discount_price,omitempty"`
	// DownPayment is the PO down payment (0 for ready items or no DP). When set,
	// the template shows "DP IDR <dp> / <Price>".
	DownPayment int `json:"down_payment,omitempty"`
}

// WishlistBackInJob is user-centric: one email per user listing up to 5 of the
// user's own wishlisted items that came back in stock this window (newest restock
// first). CompanionItem cross-sells against the hero (first) item.
type WishlistBackInJob struct {
	ID            string               `json:"job_id"`
	UserID        string               `json:"user_id"`
	Date          time.Time            `json:"date"`
	User          User                 `json:"user"`
	VoucherCode   string               `json:"voucher_code,omitempty"`
	VoucherID     int64                `json:"voucher_id,omitempty"`
	Items         []WishlistBackInItem `json:"items"`
	// CompanionItem is the item the user already bought that anchors the
	// "Gas, nemenin yang udah kamu beli" section (shown as the header reference).
	CompanionItem WishlistBackInItem `json:"companion_item"`
	// RecoItems are the 6 most-popular Kyou items in the companion's
	// series/category (cross-sell to drive sales). The section renders only when
	// there is an anchor and a full 6 recommendations.
	RecoItems []WishlistBackInItem `json:"reco_items,omitempty"`
	Attempt   int                  `json:"attempt"`
}

// WishlistBackInUserItem is one (user, restocked wishlist item) row from the
// reader query. It is not part of the wire contract (Makoto never sees it);
// the reader groups these by user into a WishlistBackInJob.
type WishlistBackInUserItem struct {
	User User
	Item WishlistBackInItem
}
