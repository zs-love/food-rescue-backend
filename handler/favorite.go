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

func AddFavorite(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的用户ID"})
		return
	}

	var req struct {
		ProductID int `json:"product_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误"})
		return
	}

	_, err = db.PG.Exec(`
		INSERT INTO user_favorites (user_id, product_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id, product_id) DO NOTHING
	`, userID, req.ProductID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "收藏失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "收藏成功"})
}

func RemoveFavorite(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的用户ID"})
		return
	}

	productID, err := strconv.Atoi(c.Param("product_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的商品ID"})
		return
	}

	_, err = db.PG.Exec(`DELETE FROM user_favorites WHERE user_id = $1 AND product_id = $2`, userID, productID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "取消收藏失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已取消收藏"})
}

func GetFavorites(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的用户ID"})
		return
	}

	rows, err := db.PG.Query(`
		SELECT p.id, p.merchant_id, p.category_id, p.name, p.description, p.images,
		       p.original_price, p.production_date, p.expiry_date, p.storage_condition,
		       p.stock, p.sold_count, p.price_rule_type, p.price_rules,
		       p.is_blind_box, p.blind_box_min_value, p.blind_box_max_value,
		       p.status, p.created_at,
		       m.shop_name, m.shop_logo, m.is_verified
		FROM user_favorites f
		JOIN products p ON f.product_id = p.id
		JOIN merchants m ON p.merchant_id = m.id
		WHERE f.user_id = $1
		ORDER BY f.created_at DESC
	`, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "获取收藏失败"})
		return
	}
	defer rows.Close()

	products := parseFavoriteProducts(rows)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": products})
}

func CheckFavorite(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的用户ID"})
		return
	}

	productID, err := strconv.Atoi(c.Param("product_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的商品ID"})
		return
	}

	var exists bool
	db.PG.QueryRow(`SELECT EXISTS(SELECT 1 FROM user_favorites WHERE user_id = $1 AND product_id = $2)`, userID, productID).Scan(&exists)

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"is_favorite": exists}})
}

func parseFavoriteProducts(rows *sql.Rows) []ProductWithShop {
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
