package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"food-rescue/db"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

type UpdateShopRequest struct {
	ShopName        string   `json:"shop_name"`
	Description     string   `json:"description"`
	ShopLogo        string   `json:"shop_logo"`
	ShopImages      []string `json:"shop_images"`
	Province        string   `json:"province"`
	City            string   `json:"city"`
	District        string   `json:"district"`
	Address         string   `json:"address"`
	Latitude        float64  `json:"latitude"`
	Longitude       float64  `json:"longitude"`
	BusinessHours   string   `json:"business_hours"`
	PickupStartTime string   `json:"pickup_start_time"`
	PickupEndTime   string   `json:"pickup_end_time"`
	ContactPhone    string   `json:"contact_phone"`
}

type ShopInfo struct {
	MerchantID        int      `json:"merchant_id"`
	Phone             string   `json:"phone"`
	ShopName          string   `json:"shop_name"`
	ShopLogo          string   `json:"shop_logo"`
	ShopImages        []string `json:"shop_images"`
	Description       string   `json:"description"`
	IsVerified        bool     `json:"is_verified"`
	Province          string   `json:"province"`
	City              string   `json:"city"`
	District          string   `json:"district"`
	Address           string   `json:"address"`
	Latitude          float64  `json:"latitude"`
	Longitude         float64  `json:"longitude"`
	BusinessHours     string   `json:"business_hours"`
	PickupStartTime   string   `json:"pickup_start_time"`
	PickupEndTime     string   `json:"pickup_end_time"`
	ContactPhone      string   `json:"contact_phone"`
	TotalSales        float64  `json:"total_sales"`
	TotalSavedLoss    float64  `json:"total_saved_loss"`
	TotalProductsSold int      `json:"total_products_sold"`
	Rating            float64  `json:"rating"`
	RatingCount       int      `json:"rating_count"`
	Status            string   `json:"status"`
}

func GetShopInfo(c *gin.Context) {
	merchantID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的商家ID"})
		return
	}

	var shop struct {
		ID                int
		Phone             string
		ShopName          sql.NullString
		ShopLogo          sql.NullString
		ShopImages        pq.StringArray
		Description       sql.NullString
		IsVerified        bool
		Province          sql.NullString
		City              sql.NullString
		District          sql.NullString
		Address           sql.NullString
		Latitude          sql.NullFloat64
		Longitude         sql.NullFloat64
		BusinessHours     sql.NullString
		PickupStartTime   sql.NullTime
		PickupEndTime     sql.NullTime
		ContactPhone      sql.NullString
		TotalSales        float64
		TotalSavedLoss    float64
		TotalProductsSold int
		Rating            float64
		RatingCount       int
		Status            string
	}

	err = db.PG.QueryRow(`
		SELECT id, phone, shop_name, shop_logo, shop_images, description, is_verified,
		       province, city, district, address, latitude, longitude,
		       business_hours, pickup_start_time, pickup_end_time, contact_phone,
		       total_sales, total_saved_loss, total_products_sold, rating, rating_count, status
		FROM merchants WHERE id = $1
	`, merchantID).Scan(
		&shop.ID, &shop.Phone, &shop.ShopName, &shop.ShopLogo, &shop.ShopImages,
		&shop.Description, &shop.IsVerified, &shop.Province, &shop.City, &shop.District,
		&shop.Address, &shop.Latitude, &shop.Longitude, &shop.BusinessHours,
		&shop.PickupStartTime, &shop.PickupEndTime, &shop.ContactPhone,
		&shop.TotalSales, &shop.TotalSavedLoss, &shop.TotalProductsSold,
		&shop.Rating, &shop.RatingCount, &shop.Status,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "商家不存在"})
		return
	}

	pickupStart := ""
	pickupEnd := ""
	if shop.PickupStartTime.Valid {
		pickupStart = shop.PickupStartTime.Time.Format("15:04")
	}
	if shop.PickupEndTime.Valid {
		pickupEnd = shop.PickupEndTime.Time.Format("15:04")
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": ShopInfo{
			MerchantID:        shop.ID,
			Phone:             shop.Phone,
			ShopName:          shop.ShopName.String,
			ShopLogo:          shop.ShopLogo.String,
			ShopImages:        shop.ShopImages,
			Description:       shop.Description.String,
			IsVerified:        shop.IsVerified,
			Province:          shop.Province.String,
			City:              shop.City.String,
			District:          shop.District.String,
			Address:           shop.Address.String,
			Latitude:          shop.Latitude.Float64,
			Longitude:         shop.Longitude.Float64,
			BusinessHours:     shop.BusinessHours.String,
			PickupStartTime:   pickupStart,
			PickupEndTime:     pickupEnd,
			ContactPhone:      shop.ContactPhone.String,
			TotalSales:        shop.TotalSales,
			TotalSavedLoss:    shop.TotalSavedLoss,
			TotalProductsSold: shop.TotalProductsSold,
			Rating:            shop.Rating,
			RatingCount:       shop.RatingCount,
			Status:            shop.Status,
		},
	})
}

func UpdateShopInfo(c *gin.Context) {
	merchantID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的商家ID"})
		return
	}

	var req UpdateShopRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误"})
		return
	}

	var pickupStart, pickupEnd interface{}
	if req.PickupStartTime != "" {
		pickupStart = req.PickupStartTime
	}
	if req.PickupEndTime != "" {
		pickupEnd = req.PickupEndTime
	}

	_, err = db.PG.Exec(`
		UPDATE merchants SET
			shop_name = COALESCE(NULLIF($1, ''), shop_name),
			description = $2,
			shop_logo = COALESCE(NULLIF($3, ''), shop_logo),
			shop_images = $4,
			province = $5,
			city = $6,
			district = $7,
			address = $8,
			latitude = NULLIF($9, 0),
			longitude = NULLIF($10, 0),
			business_hours = $11,
			pickup_start_time = $12,
			pickup_end_time = $13,
			contact_phone = $14,
			updated_at = $15
		WHERE id = $16
	`, req.ShopName, req.Description, req.ShopLogo, pq.Array(req.ShopImages),
		req.Province, req.City, req.District, req.Address,
		req.Latitude, req.Longitude, req.BusinessHours,
		pickupStart, pickupEnd, req.ContactPhone, time.Now(), merchantID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "更新成功"})
}
