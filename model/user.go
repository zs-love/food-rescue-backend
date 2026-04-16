package model

import "time"

type User struct {
	ID               int       `json:"id"`
	Phone            string    `json:"phone"`
	Password         string    `json:"-"`
	Nickname         string    `json:"nickname"`
	Avatar           string    `json:"avatar"`
	TotalSavedWeight float64   `json:"total_saved_weight"`
	TotalSavedMoney  float64   `json:"total_saved_money"`
	TotalOrders      int       `json:"total_orders"`
	RankLevel        string    `json:"rank_level"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type Merchant struct {
	ID                int       `json:"id"`
	Phone             string    `json:"phone"`
	Password          string    `json:"-"`
	ShopName          string    `json:"shop_name"`
	ShopLogo          string    `json:"shop_logo"`
	Description       string    `json:"description"`
	LicenseNo         string    `json:"license_no"`
	LicenseImage      string    `json:"license_image"`
	FoodPermitNo      string    `json:"food_permit_no"`
	FoodPermitImage   string    `json:"food_permit_image"`
	IsVerified        bool      `json:"is_verified"`
	Province          string    `json:"province"`
	City              string    `json:"city"`
	District          string    `json:"district"`
	Address           string    `json:"address"`
	Latitude          float64   `json:"latitude"`
	Longitude         float64   `json:"longitude"`
	BusinessHours     string    `json:"business_hours"`
	PickupStartTime   string    `json:"pickup_start_time"`
	PickupEndTime     string    `json:"pickup_end_time"`
	ContactPhone      string    `json:"contact_phone"`
	TotalSales        float64   `json:"total_sales"`
	TotalSavedLoss    float64   `json:"total_saved_loss"`
	TotalProductsSold int       `json:"total_products_sold"`
	Rating            float64   `json:"rating"`
	RatingCount       int       `json:"rating_count"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// 段位等级
const (
	RankNewcomer  = "newcomer"   // 食物新手 0-10kg
	RankExpert    = "expert"     // 环保达人 10-50kg
	RankGuardian  = "guardian"   // 星球卫士 50-200kg
	RankProtector = "protector"  // 地球守护者 200kg+
)

// 商家状态
const (
	MerchantStatusPending  = "pending"  // 待审核
	MerchantStatusActive   = "active"   // 正常营业
	MerchantStatusInactive = "inactive" // 暂停营业
	MerchantStatusBanned   = "banned"   // 已封禁
)

// 用户状态
const (
	UserStatusActive = "active"
	UserStatusBanned = "banned"
)
