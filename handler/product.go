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

type CreateProductRequest struct {
	CategoryID       int               `json:"category_id"`
	Name             string            `json:"name" binding:"required"`
	Description      string            `json:"description"`
	Images           []string          `json:"images"`
	OriginalPrice    float64           `json:"original_price" binding:"required"`
	ProductionDate   string            `json:"production_date"`
	ExpiryDate       string            `json:"expiry_date" binding:"required"`
	StorageCondition string            `json:"storage_condition"`
	Stock            int               `json:"stock"`
	PriceRuleType    string            `json:"price_rule_type"`
	PriceRules       []model.PriceRule `json:"price_rules"`
	IsBlindBox       bool              `json:"is_blind_box"`
	BlindBoxMinValue float64           `json:"blind_box_min_value"`
	BlindBoxMaxValue float64           `json:"blind_box_max_value"`
}

func GetCategories(c *gin.Context) {
	rows, err := db.PG.Query(`
		SELECT DISTINCT ON (name) id, name, icon, is_active FROM categories 
		WHERE is_active = true ORDER BY name, sort_order, id
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "获取分类失败"})
		return
	}
	defer rows.Close()

	var categories []model.Category
	for rows.Next() {
		var cat model.Category
		rows.Scan(&cat.ID, &cat.Name, &cat.Icon, &cat.IsActive)
		categories = append(categories, cat)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": categories})
}

func GetMerchantProducts(c *gin.Context) {
	merchantID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的商家ID"})
		return
	}

	status := c.DefaultQuery("status", "")
	categoryID := c.DefaultQuery("category_id", "")

	query := `
		SELECT p.id, p.merchant_id, p.category_id, p.name, p.description, p.images,
		       p.original_price, p.production_date, p.expiry_date, p.storage_condition,
		       p.stock, p.sold_count, p.price_rule_type, p.price_rules,
		       p.is_blind_box, p.blind_box_min_value, p.blind_box_max_value,
		       p.status, p.created_at
		FROM products p
		WHERE p.merchant_id = $1
	`
	args := []interface{}{merchantID}
	argIndex := 2

	if status != "" {
		query += " AND p.status = $" + strconv.Itoa(argIndex)
		args = append(args, status)
		argIndex++
	}
	if categoryID != "" {
		query += " AND p.category_id = $" + strconv.Itoa(argIndex)
		args = append(args, categoryID)
	}

	query += " ORDER BY p.created_at DESC"

	rows, err := db.PG.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "获取商品失败"})
		return
	}
	defer rows.Close()

	products := []model.Product{}
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
		}

		err := rows.Scan(
			&p.ID, &p.MerchantID, &p.CategoryID, &p.Name, &p.Description, &p.Images,
			&p.OriginalPrice, &p.ProductionDate, &p.ExpiryDate, &p.StorageCondition,
			&p.Stock, &p.SoldCount, &p.PriceRuleType, &p.PriceRules,
			&p.IsBlindBox, &p.BlindBoxMinValue, &p.BlindBoxMaxValue,
			&p.Status, &p.CreatedAt,
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

		products = append(products, model.Product{
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
		})
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": products})
}

func CreateProduct(c *gin.Context) {
	merchantID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的商家ID"})
		return
	}

	var req CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误: " + err.Error()})
		return
	}

	priceRulesJSON, _ := json.Marshal(req.PriceRules)
	if req.PriceRuleType == "" {
		req.PriceRuleType = model.PriceRuleFixed
	}

	var productID int
	err = db.PG.QueryRow(`
		INSERT INTO products (
			merchant_id, category_id, name, description, images,
			original_price, production_date, expiry_date, storage_condition,
			stock, price_rule_type, price_rules,
			is_blind_box, blind_box_min_value, blind_box_max_value
		) VALUES ($1, NULLIF($2, 0), $3, $4, $5, $6, NULLIF($7, '')::DATE, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING id
	`, merchantID, req.CategoryID, req.Name, req.Description, pq.Array(req.Images),
		req.OriginalPrice, req.ProductionDate, req.ExpiryDate, req.StorageCondition,
		req.Stock, req.PriceRuleType, string(priceRulesJSON),
		req.IsBlindBox, req.BlindBoxMinValue, req.BlindBoxMaxValue,
	).Scan(&productID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "创建商品失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "创建成功", "data": gin.H{"product_id": productID}})
}

func UpdateProduct(c *gin.Context) {
	productID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的商品ID"})
		return
	}

	var req struct {
		CategoryID       int               `json:"category_id"`
		Name             string            `json:"name"`
		Description      string            `json:"description"`
		Images           []string          `json:"images"`
		OriginalPrice    float64           `json:"original_price"`
		ProductionDate   string            `json:"production_date"`
		ExpiryDate       string            `json:"expiry_date"`
		StorageCondition string            `json:"storage_condition"`
		Stock            int               `json:"stock"`
		PriceRuleType    string            `json:"price_rule_type"`
		PriceRules       []model.PriceRule `json:"price_rules"`
		IsBlindBox       bool              `json:"is_blind_box"`
		BlindBoxMinValue float64           `json:"blind_box_min_value"`
		BlindBoxMaxValue float64           `json:"blind_box_max_value"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误: " + err.Error()})
		return
	}

	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "商品名称不能为空"})
		return
	}

	if req.ExpiryDate == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "保质期不能为空"})
		return
	}

	priceRulesJSON, _ := json.Marshal(req.PriceRules)

	_, err = db.PG.Exec(`
		UPDATE products SET
			category_id = NULLIF($1, 0),
			name = $2,
			description = $3,
			images = $4,
			original_price = $5,
			production_date = NULLIF($6, '')::DATE,
			expiry_date = $7,
			storage_condition = $8,
			stock = $9,
			price_rule_type = $10,
			price_rules = $11,
			is_blind_box = $12,
			blind_box_min_value = $13,
			blind_box_max_value = $14,
			updated_at = $15
		WHERE id = $16
	`, req.CategoryID, req.Name, req.Description, pq.Array(req.Images),
		req.OriginalPrice, req.ProductionDate, req.ExpiryDate, req.StorageCondition,
		req.Stock, req.PriceRuleType, string(priceRulesJSON),
		req.IsBlindBox, req.BlindBoxMinValue, req.BlindBoxMaxValue,
		time.Now(), productID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "更新失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "更新成功"})
}

func DeleteProduct(c *gin.Context) {
	productID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的商品ID"})
		return
	}

	_, err = db.PG.Exec("UPDATE products SET status = $1 WHERE id = $2", model.ProductStatusInactive, productID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "删除失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "删除成功"})
}

func UpdateProductStatus(c *gin.Context) {
	productID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的商品ID"})
		return
	}

	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误"})
		return
	}

	_, err = db.PG.Exec("UPDATE products SET status = $1, updated_at = $2 WHERE id = $3",
		req.Status, time.Now(), productID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "状态更新成功"})
}

func GetProductStats(c *gin.Context) {
	merchantID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的商家ID"})
		return
	}

	var stats struct {
		Active   int `json:"active"`
		Expiring int `json:"expiring"`
		Inactive int `json:"inactive"`
		Total    int `json:"total"`
	}

	db.PG.QueryRow(`SELECT COUNT(*) FROM products WHERE merchant_id = $1 AND status = 'active'`, merchantID).Scan(&stats.Active)
	db.PG.QueryRow(`SELECT COUNT(*) FROM products WHERE merchant_id = $1 AND status = 'active' AND expiry_date <= CURRENT_DATE + INTERVAL '3 days'`, merchantID).Scan(&stats.Expiring)
	db.PG.QueryRow(`SELECT COUNT(*) FROM products WHERE merchant_id = $1 AND status = 'inactive'`, merchantID).Scan(&stats.Inactive)
	db.PG.QueryRow(`SELECT COUNT(*) FROM products WHERE merchant_id = $1`, merchantID).Scan(&stats.Total)

	c.JSON(http.StatusOK, gin.H{"success": true, "data": stats})
}

