package handler

import (
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"time"

	"food-rescue/db"
	"food-rescue/model"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

type NearbyShop struct {
	MerchantID    int     `json:"merchant_id"`
	ShopName      string  `json:"shop_name"`
	ShopLogo      string  `json:"shop_logo"`
	Description   string  `json:"description"`
	Address       string  `json:"address"`
	District      string  `json:"district"`
	BusinessHours string  `json:"business_hours"`
	Rating        float64 `json:"rating"`
	RatingCount   int     `json:"rating_count"`
	IsVerified    bool    `json:"is_verified"`
	ProductCount  int     `json:"product_count"`
	Distance      float64 `json:"distance"`
}

type ProductWithShop struct {
	model.Product
	ShopName   string `json:"shop_name"`
	ShopLogo   string `json:"shop_logo"`
	IsVerified bool   `json:"is_verified"`
}

func GetNearbyShops(c *gin.Context) {
	rows, err := db.PG.Query(`
		SELECT m.id, m.shop_name, m.shop_logo, m.description, m.address, m.district,
		       m.business_hours, m.rating, m.rating_count, m.is_verified,
		       (SELECT COUNT(*) FROM products p WHERE p.merchant_id = m.id AND p.status = 'active') as product_count
		FROM merchants m
		WHERE m.status = 'active'
		ORDER BY m.rating DESC, m.rating_count DESC
		LIMIT 20
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "获取店铺失败"})
		return
	}
	defer rows.Close()

	shops := []NearbyShop{}
	for rows.Next() {
		var shop struct {
			ID            int
			ShopName      sql.NullString
			ShopLogo      sql.NullString
			Description   sql.NullString
			Address       sql.NullString
			District      sql.NullString
			BusinessHours sql.NullString
			Rating        float64
			RatingCount   int
			IsVerified    bool
			ProductCount  int
		}
		rows.Scan(&shop.ID, &shop.ShopName, &shop.ShopLogo, &shop.Description,
			&shop.Address, &shop.District, &shop.BusinessHours,
			&shop.Rating, &shop.RatingCount, &shop.IsVerified, &shop.ProductCount)

		shops = append(shops, NearbyShop{
			MerchantID:    shop.ID,
			ShopName:      shop.ShopName.String,
			ShopLogo:      shop.ShopLogo.String,
			Description:   shop.Description.String,
			Address:       shop.Address.String,
			District:      shop.District.String,
			BusinessHours: shop.BusinessHours.String,
			Rating:        shop.Rating,
			RatingCount:   shop.RatingCount,
			IsVerified:    shop.IsVerified,
			ProductCount:  shop.ProductCount,
		})
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": shops})
}

func GetExpiringProducts(c *gin.Context) {
	limit := c.DefaultQuery("limit", "20")
	limitNum, _ := strconv.Atoi(limit)

	rows, err := db.PG.Query(`
		SELECT p.id, p.merchant_id, p.category_id, p.name, p.description, p.images,
		       p.original_price, p.production_date, p.expiry_date, p.storage_condition,
		       p.stock, p.sold_count, p.price_rule_type, p.price_rules,
		       p.is_blind_box, p.blind_box_min_value, p.blind_box_max_value,
		       p.status, p.created_at,
		       m.shop_name, m.shop_logo, m.is_verified
		FROM products p
		JOIN merchants m ON p.merchant_id = m.id
		WHERE p.status = 'active' AND m.status = 'active'
		  AND p.expiry_date >= CURRENT_DATE
		  AND p.stock > 0
		ORDER BY p.expiry_date ASC, p.created_at DESC
		LIMIT $1
	`, limitNum)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "获取商品失败"})
		return
	}
	defer rows.Close()

	products := parseProductsWithShop(rows)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": products})
}

func GetFlashSaleProducts(c *gin.Context) {
	rows, err := db.PG.Query(`
		SELECT p.id, p.merchant_id, p.category_id, p.name, p.description, p.images,
		       p.original_price, p.production_date, p.expiry_date, p.storage_condition,
		       p.stock, p.sold_count, p.price_rule_type, p.price_rules,
		       p.is_blind_box, p.blind_box_min_value, p.blind_box_max_value,
		       p.status, p.created_at,
		       m.shop_name, m.shop_logo, m.is_verified
		FROM products p
		JOIN merchants m ON p.merchant_id = m.id
		WHERE p.status = 'active' AND m.status = 'active'
		  AND p.price_rule_type = 'time_based'
		  AND p.expiry_date <= CURRENT_DATE + INTERVAL '3 days'
		  AND p.expiry_date >= CURRENT_DATE
		  AND p.stock > 0
		ORDER BY p.expiry_date ASC
		LIMIT 20
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "获取商品失败"})
		return
	}
	defer rows.Close()

	products := parseProductsWithShop(rows)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": products})
}

func GetProductsByCategory(c *gin.Context) {
	categoryID := c.Param("id")

	rows, err := db.PG.Query(`
		SELECT p.id, p.merchant_id, p.category_id, p.name, p.description, p.images,
		       p.original_price, p.production_date, p.expiry_date, p.storage_condition,
		       p.stock, p.sold_count, p.price_rule_type, p.price_rules,
		       p.is_blind_box, p.blind_box_min_value, p.blind_box_max_value,
		       p.status, p.created_at,
		       m.shop_name, m.shop_logo, m.is_verified
		FROM products p
		JOIN merchants m ON p.merchant_id = m.id
		WHERE p.status = 'active' AND m.status = 'active'
		  AND p.category_id = $1
		  AND p.expiry_date >= CURRENT_DATE
		  AND p.stock > 0
		ORDER BY p.expiry_date ASC
		LIMIT 50
	`, categoryID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "获取商品失败"})
		return
	}
	defer rows.Close()

	products := parseProductsWithShop(rows)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": products})
}

func GetShopProducts(c *gin.Context) {
	shopID := c.Param("id")

	rows, err := db.PG.Query(`
		SELECT p.id, p.merchant_id, p.category_id, p.name, p.description, p.images,
		       p.original_price, p.production_date, p.expiry_date, p.storage_condition,
		       p.stock, p.sold_count, p.price_rule_type, p.price_rules,
		       p.is_blind_box, p.blind_box_min_value, p.blind_box_max_value,
		       p.status, p.created_at,
		       m.shop_name, m.shop_logo, m.is_verified
		FROM products p
		JOIN merchants m ON p.merchant_id = m.id
		WHERE p.merchant_id = $1 AND p.status = 'active'
		  AND p.expiry_date >= CURRENT_DATE
		  AND p.stock > 0
		ORDER BY p.expiry_date ASC
	`, shopID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "获取商品失败"})
		return
	}
	defer rows.Close()

	products := parseProductsWithShop(rows)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": products})
}

func GetProductDetail(c *gin.Context) {
	productID := c.Param("id")

	var p struct {
		ID               int
		MerchantID       int
		CategoryID       sql.NullInt64
		Name             string
		Description      sql.NullString
		Images           pq.StringArray
		OriginalPrice    float64
		ProductionDate   sql.NullTime
		ExpiryDate       time.Time
		StorageCondition sql.NullString
		Stock            int
		SoldCount        int
		PriceRuleType    sql.NullString
		PriceRules       sql.NullString
		IsBlindBox       bool
		BlindBoxMinValue sql.NullFloat64
		BlindBoxMaxValue sql.NullFloat64
		Status           string
		CreatedAt        time.Time
		ShopName         sql.NullString
		ShopLogo         sql.NullString
		IsVerified       bool
		ShopAddress      sql.NullString
		ShopPhone        sql.NullString
		BusinessHours    sql.NullString
	}

	err := db.PG.QueryRow(`
		SELECT p.id, p.merchant_id, p.category_id, p.name, p.description, p.images,
		       p.original_price, p.production_date, p.expiry_date, p.storage_condition,
		       p.stock, p.sold_count, p.price_rule_type, p.price_rules,
		       p.is_blind_box, p.blind_box_min_value, p.blind_box_max_value,
		       p.status, p.created_at,
		       m.shop_name, m.shop_logo, m.is_verified, m.address, m.contact_phone, m.business_hours
		FROM products p
		JOIN merchants m ON p.merchant_id = m.id
		WHERE p.id = $1
	`, productID).Scan(
		&p.ID, &p.MerchantID, &p.CategoryID, &p.Name, &p.Description, &p.Images,
		&p.OriginalPrice, &p.ProductionDate, &p.ExpiryDate, &p.StorageCondition,
		&p.Stock, &p.SoldCount, &p.PriceRuleType, &p.PriceRules,
		&p.IsBlindBox, &p.BlindBoxMinValue, &p.BlindBoxMaxValue,
		&p.Status, &p.CreatedAt,
		&p.ShopName, &p.ShopLogo, &p.IsVerified, &p.ShopAddress, &p.ShopPhone, &p.BusinessHours,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "商品不存在"})
		return
	}

	now := time.Now()
	daysLeft := int(math.Ceil(p.ExpiryDate.Sub(now).Hours() / 24))
	if daysLeft < 0 {
		daysLeft = 0
	}

	currentPrice, discount := calculatePrice(p.OriginalPrice, p.PriceRuleType.String, p.PriceRules.String, p.ExpiryDate)

	prodDate := ""
	if p.ProductionDate.Valid {
		prodDate = p.ProductionDate.Time.Format("2006-01-02")
	}

	var priceRules []model.PriceRule
	if p.PriceRules.Valid && p.PriceRules.String != "" {
		json.Unmarshal([]byte(p.PriceRules.String), &priceRules)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":                 p.ID,
			"merchant_id":        p.MerchantID,
			"category_id":        p.CategoryID.Int64,
			"name":               p.Name,
			"description":        p.Description.String,
			"images":             p.Images,
			"original_price":     p.OriginalPrice,
			"current_price":      currentPrice,
			"discount":           discount,
			"production_date":    prodDate,
			"expiry_date":        p.ExpiryDate.Format("2006-01-02"),
			"days_left":          daysLeft,
			"storage_condition":  p.StorageCondition.String,
			"stock":              p.Stock,
			"sold_count":         p.SoldCount,
			"price_rule_type":    p.PriceRuleType.String,
			"price_rules":        priceRules,
			"is_blind_box":       p.IsBlindBox,
			"blind_box_min_value": p.BlindBoxMinValue.Float64,
			"blind_box_max_value": p.BlindBoxMaxValue.Float64,
			"status":             p.Status,
			"shop_name":          p.ShopName.String,
			"shop_logo":          p.ShopLogo.String,
			"is_verified":        p.IsVerified,
			"shop_address":       p.ShopAddress.String,
			"shop_phone":         p.ShopPhone.String,
			"business_hours":     p.BusinessHours.String,
		},
	})
}

func GetHomeStats(c *gin.Context) {
	var stats struct {
		FlashSaleCount  int `json:"flash_sale_count"`
		ExpiringCount   int `json:"expiring_count"`
		ShopCount       int `json:"shop_count"`
		TotalSavedToday float64 `json:"total_saved_today"`
	}

	db.PG.QueryRow(`
		SELECT COUNT(*) FROM products 
		WHERE status = 'active' AND price_rule_type = 'time_based' 
		AND expiry_date <= CURRENT_DATE + INTERVAL '3 days' AND expiry_date >= CURRENT_DATE AND stock > 0
	`).Scan(&stats.FlashSaleCount)

	db.PG.QueryRow(`
		SELECT COUNT(*) FROM products 
		WHERE status = 'active' AND expiry_date <= CURRENT_DATE + INTERVAL '1 day' AND expiry_date >= CURRENT_DATE AND stock > 0
	`).Scan(&stats.ExpiringCount)

	db.PG.QueryRow(`SELECT COUNT(*) FROM merchants WHERE status = 'active'`).Scan(&stats.ShopCount)

	db.PG.QueryRow(`
		SELECT COALESCE(SUM(discount_amount), 0) FROM orders 
		WHERE status IN ('paid', 'completed') AND DATE(pay_time) = CURRENT_DATE
	`).Scan(&stats.TotalSavedToday)

	c.JSON(http.StatusOK, gin.H{"success": true, "data": stats})
}

// GetHomeData 首页聚合接口 - 一次返回所有首页数据
func GetHomeData(c *gin.Context) {
	type homeResult struct {
		Stats      interface{} `json:"stats"`
		Products   interface{} `json:"products"`
		Shops      interface{} `json:"shops"`
		Categories interface{} `json:"categories"`
	}

	var result homeResult

	// 1. 统计数据
	var stats struct {
		FlashSaleCount  int     `json:"flash_sale_count"`
		ExpiringCount   int     `json:"expiring_count"`
		ShopCount       int     `json:"shop_count"`
		TotalSavedToday float64 `json:"total_saved_today"`
	}
	db.PG.QueryRow(`SELECT COUNT(*) FROM products WHERE status = 'active' AND price_rule_type = 'time_based' AND expiry_date <= CURRENT_DATE + INTERVAL '3 days' AND expiry_date >= CURRENT_DATE AND stock > 0`).Scan(&stats.FlashSaleCount)
	db.PG.QueryRow(`SELECT COUNT(*) FROM products WHERE status = 'active' AND expiry_date <= CURRENT_DATE + INTERVAL '1 day' AND expiry_date >= CURRENT_DATE AND stock > 0`).Scan(&stats.ExpiringCount)
	db.PG.QueryRow(`SELECT COUNT(*) FROM merchants WHERE status = 'active'`).Scan(&stats.ShopCount)
	db.PG.QueryRow(`SELECT COALESCE(SUM(discount_amount), 0) FROM orders WHERE status IN ('paid', 'completed') AND DATE(pay_time) = CURRENT_DATE`).Scan(&stats.TotalSavedToday)
	result.Stats = stats

	// 2. 即将到期商品（前10个）
	prodRows, err := db.PG.Query(`
		SELECT p.id, p.merchant_id, p.category_id, p.name, p.description, p.images,
		       p.original_price, p.production_date, p.expiry_date, p.storage_condition,
		       p.stock, p.sold_count, p.price_rule_type, p.price_rules,
		       p.is_blind_box, p.blind_box_min_value, p.blind_box_max_value,
		       p.status, p.created_at, m.shop_name, m.shop_logo, m.is_verified
		FROM products p JOIN merchants m ON p.merchant_id = m.id
		WHERE p.status = 'active' AND m.status = 'active' AND p.expiry_date >= CURRENT_DATE AND p.stock > 0
		ORDER BY p.expiry_date ASC, p.created_at DESC LIMIT 10
	`)
	if err == nil {
		defer prodRows.Close()
		result.Products = parseProductsWithShop(prodRows)
	} else {
		result.Products = []ProductWithShop{}
	}

	// 3. 附近商家（前10个，用LEFT JOIN避免子查询）
	shopRows, err := db.PG.Query(`
		SELECT m.id, m.shop_name, m.shop_logo, m.description, m.address, m.district,
		       m.business_hours, m.rating, m.rating_count, m.is_verified,
		       COUNT(p.id) as product_count
		FROM merchants m
		LEFT JOIN products p ON p.merchant_id = m.id AND p.status = 'active'
		WHERE m.status = 'active'
		GROUP BY m.id
		ORDER BY m.rating DESC, m.rating_count DESC LIMIT 10
	`)
	if err == nil {
		defer shopRows.Close()
		shops := []NearbyShop{}
		for shopRows.Next() {
			var s struct {
				ID int; ShopName, ShopLogo, Description, Address, District, BusinessHours sql.NullString
				Rating float64; RatingCount int; IsVerified bool; ProductCount int
			}
			shopRows.Scan(&s.ID, &s.ShopName, &s.ShopLogo, &s.Description, &s.Address, &s.District,
				&s.BusinessHours, &s.Rating, &s.RatingCount, &s.IsVerified, &s.ProductCount)
			shops = append(shops, NearbyShop{MerchantID: s.ID, ShopName: s.ShopName.String, ShopLogo: s.ShopLogo.String,
				Description: s.Description.String, Address: s.Address.String, District: s.District.String,
				BusinessHours: s.BusinessHours.String, Rating: s.Rating, RatingCount: s.RatingCount,
				IsVerified: s.IsVerified, ProductCount: s.ProductCount})
		}
		result.Shops = shops
	} else {
		result.Shops = []NearbyShop{}
	}

	// 4. 分类（去重）
	catRows, err := db.PG.Query(`
		SELECT DISTINCT ON (name) id, name, icon, is_active
		FROM categories WHERE is_active = true ORDER BY name, sort_order, id
	`)
	if err == nil {
		defer catRows.Close()
		var cats []model.Category
		for catRows.Next() {
			var cat model.Category
			catRows.Scan(&cat.ID, &cat.Name, &cat.Icon, &cat.IsActive)
			cats = append(cats, cat)
		}
		result.Categories = cats
	} else {
		result.Categories = []model.Category{}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func parseProductsWithShop(rows *sql.Rows) []ProductWithShop {
	products := []ProductWithShop{}
	now := time.Now()

	for rows.Next() {
		var p struct {
			ID               int
			MerchantID       int
			CategoryID       sql.NullInt64
			Name             string
			Description      sql.NullString
			Images           pq.StringArray
			OriginalPrice    float64
			ProductionDate   sql.NullTime
			ExpiryDate       time.Time
			StorageCondition sql.NullString
			Stock            int
			SoldCount        int
			PriceRuleType    sql.NullString
			PriceRules       sql.NullString
			IsBlindBox       bool
			BlindBoxMinValue sql.NullFloat64
			BlindBoxMaxValue sql.NullFloat64
			Status           string
			CreatedAt        time.Time
			ShopName         sql.NullString
			ShopLogo         sql.NullString
			IsVerified       bool
		}

		err := rows.Scan(
			&p.ID, &p.MerchantID, &p.CategoryID, &p.Name, &p.Description, &p.Images,
			&p.OriginalPrice, &p.ProductionDate, &p.ExpiryDate, &p.StorageCondition,
			&p.Stock, &p.SoldCount, &p.PriceRuleType, &p.PriceRules,
			&p.IsBlindBox, &p.BlindBoxMinValue, &p.BlindBoxMaxValue,
			&p.Status, &p.CreatedAt,
			&p.ShopName, &p.ShopLogo, &p.IsVerified,
		)
		if err != nil {
			continue
		}

		daysLeft := int(math.Ceil(p.ExpiryDate.Sub(now).Hours() / 24))
		if daysLeft < 0 {
			daysLeft = 0
		}

		currentPrice, discount := calculatePrice(p.OriginalPrice, p.PriceRuleType.String, p.PriceRules.String, p.ExpiryDate)

		prodDate := ""
		if p.ProductionDate.Valid {
			prodDate = p.ProductionDate.Time.Format("2006-01-02")
		}

		var priceRules []model.PriceRule
		if p.PriceRules.Valid && p.PriceRules.String != "" {
			json.Unmarshal([]byte(p.PriceRules.String), &priceRules)
		}

		products = append(products, ProductWithShop{
			Product: model.Product{
				ID:               p.ID,
				MerchantID:       p.MerchantID,
				CategoryID:       int(p.CategoryID.Int64),
				Name:             p.Name,
				Description:      p.Description.String,
				Images:           p.Images,
				OriginalPrice:    p.OriginalPrice,
				CurrentPrice:     currentPrice,
				Discount:         discount,
				ProductionDate:   prodDate,
				ExpiryDate:       p.ExpiryDate.Format("2006-01-02"),
				DaysLeft:         daysLeft,
				StorageCondition: p.StorageCondition.String,
				Stock:            p.Stock,
				SoldCount:        p.SoldCount,
				PriceRuleType:    p.PriceRuleType.String,
				PriceRules:       priceRules,
				IsBlindBox:       p.IsBlindBox,
				BlindBoxMinValue: p.BlindBoxMinValue.Float64,
				BlindBoxMaxValue: p.BlindBoxMaxValue.Float64,
				Status:           p.Status,
				CreatedAt:        p.CreatedAt.Format("2006-01-02 15:04:05"),
			},
			ShopName:   p.ShopName.String,
			ShopLogo:   p.ShopLogo.String,
			IsVerified: p.IsVerified,
		})
	}

	return products
}

func calculatePrice(originalPrice float64, ruleType string, rulesJSON string, expiryDate time.Time) (float64, float64) {
	if ruleType != model.PriceRuleTimeBased || rulesJSON == "" {
		return originalPrice, 100
	}

	var rules []model.PriceRule
	if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
		return originalPrice, 100
	}

	hoursLeft := int(math.Ceil(time.Until(expiryDate).Hours()))
	if hoursLeft < 0 {
		hoursLeft = 0
	}

	discount := 100.0
	for _, rule := range rules {
		if hoursLeft <= rule.HoursLeft {
			discount = rule.Discount
		}
	}

	currentPrice := originalPrice * discount / 100
	return math.Round(currentPrice*100) / 100, discount
}
