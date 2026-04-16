package handler

import (
	"database/sql"
	"math"
	"net/http"
	"strconv"
	"time"

	"food-rescue/db"

	"github.com/gin-gonic/gin"
)

type CartItem struct {
	ID           int     `json:"id"`
	ProductID    int     `json:"product_id"`
	Quantity     int     `json:"quantity"`
	Name         string  `json:"name"`
	Image        string  `json:"image"`
	OriginalPrice float64 `json:"original_price"`
	CurrentPrice float64 `json:"current_price"`
	Discount     float64 `json:"discount"`
	ExpiryDate   string  `json:"expiry_date"`
	DaysLeft     int     `json:"days_left"`
	Stock        int     `json:"stock"`
	Status       string  `json:"status"`
	ShopName     string  `json:"shop_name"`
	MerchantID   int     `json:"merchant_id"`
}

func GetCart(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的用户ID"})
		return
	}

	rows, err := db.PG.Query(`
		SELECT ci.id, ci.product_id, ci.quantity,
		       p.name, p.images[1], p.original_price, p.expiry_date, p.stock, p.status,
		       p.price_rule_type, p.price_rules,
		       m.shop_name, m.id as merchant_id
		FROM cart_items ci
		JOIN products p ON ci.product_id = p.id
		JOIN merchants m ON p.merchant_id = m.id
		WHERE ci.user_id = $1
		ORDER BY ci.created_at DESC
	`, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "获取购物车失败"})
		return
	}
	defer rows.Close()

	items := []CartItem{}
	now := time.Now()

	for rows.Next() {
		var item struct {
			ID            int
			ProductID     int
			Quantity      int
			Name          string
			Image         sql.NullString
			OriginalPrice float64
			ExpiryDate    time.Time
			Stock         int
			Status        string
			PriceRuleType sql.NullString
			PriceRules    sql.NullString
			ShopName      sql.NullString
			MerchantID    int
		}

		rows.Scan(&item.ID, &item.ProductID, &item.Quantity,
			&item.Name, &item.Image, &item.OriginalPrice, &item.ExpiryDate, &item.Stock, &item.Status,
			&item.PriceRuleType, &item.PriceRules,
			&item.ShopName, &item.MerchantID)

		daysLeft := int(math.Ceil(item.ExpiryDate.Sub(now).Hours() / 24))
		if daysLeft < 0 {
			daysLeft = 0
		}

		currentPrice, discount := calculatePrice(item.OriginalPrice, item.PriceRuleType.String, item.PriceRules.String, item.ExpiryDate)

		items = append(items, CartItem{
			ID:            item.ID,
			ProductID:     item.ProductID,
			Quantity:      item.Quantity,
			Name:          item.Name,
			Image:         item.Image.String,
			OriginalPrice: item.OriginalPrice,
			CurrentPrice:  currentPrice,
			Discount:      discount,
			ExpiryDate:    item.ExpiryDate.Format("2006-01-02"),
			DaysLeft:      daysLeft,
			Stock:         item.Stock,
			Status:        item.Status,
			ShopName:      item.ShopName.String,
			MerchantID:    item.MerchantID,
		})
	}

	var totalPrice float64
	var totalCount int
	for _, item := range items {
		if item.Status == "active" && item.Stock > 0 {
			totalPrice += item.CurrentPrice * float64(item.Quantity)
			totalCount += item.Quantity
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items":       items,
			"total_price": math.Round(totalPrice*100) / 100,
			"total_count": totalCount,
		},
	})
}

func AddToCart(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的用户ID"})
		return
	}

	var req struct {
		ProductID int `json:"product_id" binding:"required"`
		Quantity  int `json:"quantity"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误"})
		return
	}

	if req.Quantity <= 0 {
		req.Quantity = 1
	}

	var stock int
	err = db.PG.QueryRow("SELECT stock FROM products WHERE id = $1 AND status = 'active'", req.ProductID).Scan(&stock)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "商品不存在或已下架"})
		return
	}

	_, err = db.PG.Exec(`
		INSERT INTO cart_items (user_id, product_id, quantity)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, product_id) 
		DO UPDATE SET quantity = cart_items.quantity + $3, updated_at = CURRENT_TIMESTAMP
	`, userID, req.ProductID, req.Quantity)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "添加失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已加入购物车"})
}

func UpdateCartItem(c *gin.Context) {
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

	var req struct {
		Quantity int `json:"quantity" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误"})
		return
	}

	if req.Quantity <= 0 {
		db.PG.Exec("DELETE FROM cart_items WHERE user_id = $1 AND product_id = $2", userID, productID)
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已移除"})
		return
	}

	_, err = db.PG.Exec(`
		UPDATE cart_items SET quantity = $1, updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $2 AND product_id = $3
	`, req.Quantity, userID, productID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已更新"})
}

func RemoveFromCart(c *gin.Context) {
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

	_, err = db.PG.Exec("DELETE FROM cart_items WHERE user_id = $1 AND product_id = $2", userID, productID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "移除失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已移除"})
}

func ClearCart(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的用户ID"})
		return
	}

	_, err = db.PG.Exec("DELETE FROM cart_items WHERE user_id = $1", userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "清空失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已清空"})
}

func GetCartCount(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的用户ID"})
		return
	}

	var count int
	db.PG.QueryRow("SELECT COALESCE(SUM(quantity), 0) FROM cart_items WHERE user_id = $1", userID).Scan(&count)

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"count": count}})
}
