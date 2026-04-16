package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"food-rescue/db"
	"food-rescue/model"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

func SearchProducts(c *gin.Context) {
	keyword := c.Query("keyword")
	sortBy := c.DefaultQuery("sort", "expiry")
	categoryID := c.Query("category_id")

	if keyword == "" && categoryID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请输入搜索关键词"})
		return
	}

	query := `
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
	`

	args := []interface{}{}
	argIndex := 1

	if keyword != "" {
		query += fmt.Sprintf(` AND (p.name ILIKE $%d OR p.description ILIKE $%d)`, argIndex, argIndex)
		args = append(args, "%"+keyword+"%")
		argIndex++
	}

	if categoryID != "" {
		query += fmt.Sprintf(` AND p.category_id = $%d`, argIndex)
		args = append(args, categoryID)
		argIndex++
	}

	switch sortBy {
	case "price_asc":
		query += " ORDER BY p.original_price ASC"
	case "price_desc":
		query += " ORDER BY p.original_price DESC"
	case "sales":
		query += " ORDER BY p.sold_count DESC"
	default:
		query += " ORDER BY p.expiry_date ASC"
	}

	query += " LIMIT 50"

	rows, err := db.PG.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "搜索失败"})
		return
	}
	defer rows.Close()

	products := parseSearchResults(rows)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": products})
}

func parseSearchResults(rows *sql.Rows) []ProductWithShop {
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
