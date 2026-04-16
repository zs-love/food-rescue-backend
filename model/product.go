package model

const (
	ProductStatusActive   = "active"
	ProductStatusInactive = "inactive"
	ProductStatusSoldOut  = "sold_out"
	ProductStatusExpired  = "expired"

	PriceRuleFixed    = "fixed"
	PriceRuleTimeBased = "time_based"
)

type PriceRule struct {
	Discount  float64 `json:"discount"`
	HoursLeft int     `json:"hours_left"`
}

type Product struct {
	ID          int     `json:"id"`
	MerchantID  int     `json:"merchant_id"`
	CategoryID  int     `json:"category_id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Images      []string `json:"images"`

	OriginalPrice float64 `json:"original_price"`
	CurrentPrice  float64 `json:"current_price"`
	Discount      float64 `json:"discount"`

	ProductionDate   string `json:"production_date"`
	ExpiryDate       string `json:"expiry_date"`
	DaysLeft         int    `json:"days_left"`
	StorageCondition string `json:"storage_condition"`

	Stock     int `json:"stock"`
	SoldCount int `json:"sold_count"`

	PriceRuleType string      `json:"price_rule_type"`
	PriceRules    []PriceRule `json:"price_rules"`

	IsBlindBox       bool    `json:"is_blind_box"`
	BlindBoxMinValue float64 `json:"blind_box_min_value"`
	BlindBoxMaxValue float64 `json:"blind_box_max_value"`

	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type Category struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Icon     string `json:"icon"`
	IsActive bool   `json:"is_active"`
}
