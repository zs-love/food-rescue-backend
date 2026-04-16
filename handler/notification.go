package handler

import (
	"database/sql"
	"food-rescue/db"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// 通知类型
const (
	NotifyTypePriceDrop  = "price_drop"    // 降价提醒
	NotifyTypeFlashSale  = "flash_sale"    // 闪购提醒
	NotifyTypePickup     = "pickup_remind" // 取货提醒
	NotifyTypeOrderStatus = "order_status" // 订单状态
	NotifyTypeDonation   = "donation"      // 公益通知
	NotifyTypeSystem     = "system"        // 系统通知
)

// 获取用户通知列表
func GetNotifications(c *gin.Context) {
	userID := c.Param("user_id")
	limit := c.DefaultQuery("limit", "20")
	offset := c.DefaultQuery("offset", "0")
	limitNum, _ := strconv.Atoi(limit)
	offsetNum, _ := strconv.Atoi(offset)

	rows, err := db.PG.Query(`
		SELECT id, type, title, content, related_id, related_type, is_read, created_at
		FROM notifications
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limitNum, offsetNum)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []gin.H{}})
		return
	}
	defer rows.Close()

	var notifications []gin.H
	for rows.Next() {
		var id int
		var nType, title, content string
		var relatedID sql.NullInt64
		var relatedType sql.NullString
		var isRead bool
		var createdAt time.Time

		rows.Scan(&id, &nType, &title, &content, &relatedID, &relatedType, &isRead, &createdAt)

		notifications = append(notifications, gin.H{
			"id":           id,
			"type":         nType,
			"title":        title,
			"content":      content,
			"related_id":   relatedID.Int64,
			"related_type": relatedType.String,
			"is_read":      isRead,
			"created_at":   createdAt,
			"time_ago":     formatTimeAgo(createdAt),
		})
	}

	// 获取未读数量
	var unreadCount int
	db.PG.QueryRow(`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND is_read = false`, userID).Scan(&unreadCount)

	if notifications == nil {
		notifications = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"notifications": notifications,
			"unread_count":  unreadCount,
		},
	})
}

// 获取未读通知数量
func GetUnreadCount(c *gin.Context) {
	userID := c.Param("user_id")

	var count int
	db.PG.QueryRow(`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND is_read = false`, userID).Scan(&count)

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"count": count}})
}

// 标记通知为已读
func MarkNotificationRead(c *gin.Context) {
	userID := c.Param("user_id")
	notificationID := c.Param("notification_id")

	if notificationID == "all" {
		// 标记所有为已读
		db.PG.Exec(`UPDATE notifications SET is_read = true WHERE user_id = $1`, userID)
	} else {
		db.PG.Exec(`UPDATE notifications SET is_read = true WHERE id = $1 AND user_id = $2`, notificationID, userID)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已标记为已读"})
}

// 获取通知设置
func GetNotificationSettings(c *gin.Context) {
	userID := c.Param("user_id")

	var settings struct {
		PriceDropEnabled   bool `json:"price_drop_enabled"`
		FlashSaleEnabled   bool `json:"flash_sale_enabled"`
		PickupRemindEnabled bool `json:"pickup_remind_enabled"`
		OrderStatusEnabled bool `json:"order_status_enabled"`
		DonationEnabled    bool `json:"donation_enabled"`
	}

	err := db.PG.QueryRow(`
		SELECT price_drop_enabled, flash_sale_enabled, pickup_remind_enabled, 
		       order_status_enabled, donation_enabled
		FROM notification_settings WHERE user_id = $1
	`, userID).Scan(&settings.PriceDropEnabled, &settings.FlashSaleEnabled,
		&settings.PickupRemindEnabled, &settings.OrderStatusEnabled, &settings.DonationEnabled)

	if err != nil {
		// 默认全部开启
		settings = struct {
			PriceDropEnabled   bool `json:"price_drop_enabled"`
			FlashSaleEnabled   bool `json:"flash_sale_enabled"`
			PickupRemindEnabled bool `json:"pickup_remind_enabled"`
			OrderStatusEnabled bool `json:"order_status_enabled"`
			DonationEnabled    bool `json:"donation_enabled"`
		}{true, true, true, true, true}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": settings})
}

// 更新通知设置
func UpdateNotificationSettings(c *gin.Context) {
	userID := c.Param("user_id")

	var req struct {
		PriceDropEnabled   *bool `json:"price_drop_enabled"`
		FlashSaleEnabled   *bool `json:"flash_sale_enabled"`
		PickupRemindEnabled *bool `json:"pickup_remind_enabled"`
		OrderStatusEnabled *bool `json:"order_status_enabled"`
		DonationEnabled    *bool `json:"donation_enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误"})
		return
	}

	// 使用 UPSERT
	_, err := db.PG.Exec(`
		INSERT INTO notification_settings (user_id, price_drop_enabled, flash_sale_enabled, 
		                                   pickup_remind_enabled, order_status_enabled, donation_enabled)
		VALUES ($1, COALESCE($2, true), COALESCE($3, true), COALESCE($4, true), COALESCE($5, true), COALESCE($6, true))
		ON CONFLICT (user_id) DO UPDATE SET
			price_drop_enabled = COALESCE($2, notification_settings.price_drop_enabled),
			flash_sale_enabled = COALESCE($3, notification_settings.flash_sale_enabled),
			pickup_remind_enabled = COALESCE($4, notification_settings.pickup_remind_enabled),
			order_status_enabled = COALESCE($5, notification_settings.order_status_enabled),
			donation_enabled = COALESCE($6, notification_settings.donation_enabled),
			updated_at = NOW()
	`, userID, req.PriceDropEnabled, req.FlashSaleEnabled, req.PickupRemindEnabled,
		req.OrderStatusEnabled, req.DonationEnabled)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "设置已更新"})
}

// 关注商品（降价提醒）
func WatchProduct(c *gin.Context) {
	userID := c.Param("user_id")
	var req struct {
		ProductID   int      `json:"product_id"`
		TargetPrice *float64 `json:"target_price"` // 可选，目标价格
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误"})
		return
	}

	// 获取当前价格作为参考
	var currentPrice float64
	db.PG.QueryRow(`
		SELECT CASE 
			WHEN price_rule_type = 'time_based' THEN
				original_price * COALESCE(
					(SELECT (rule->>'discount')::numeric / 100
					FROM jsonb_array_elements(price_rules) AS rule
					WHERE (rule->>'hours_left')::int >= EXTRACT(EPOCH FROM (expiry_date - NOW())) / 3600
					ORDER BY (rule->>'hours_left')::int ASC
					LIMIT 1), 1)
			ELSE original_price
		END FROM products WHERE id = $1
	`, req.ProductID).Scan(&currentPrice)

	targetPrice := currentPrice * 0.8 // 默认降价20%时提醒
	if req.TargetPrice != nil {
		targetPrice = *req.TargetPrice
	}

	_, err := db.PG.Exec(`
		INSERT INTO product_watches (user_id, product_id, target_price)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, product_id) DO UPDATE SET target_price = $3
	`, userID, req.ProductID, targetPrice)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "关注失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "已开启降价提醒",
		"data": gin.H{
			"target_price": targetPrice,
		},
	})
}

// 取消关注商品
func UnwatchProduct(c *gin.Context) {
	userID := c.Param("user_id")
	productID := c.Param("product_id")

	db.PG.Exec(`DELETE FROM product_watches WHERE user_id = $1 AND product_id = $2`, userID, productID)

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已取消降价提醒"})
}

// 检查是否关注商品
func CheckProductWatch(c *gin.Context) {
	userID := c.Param("user_id")
	productID := c.Param("product_id")

	var targetPrice sql.NullFloat64
	err := db.PG.QueryRow(`
		SELECT target_price FROM product_watches WHERE user_id = $1 AND product_id = $2
	`, userID, productID).Scan(&targetPrice)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"is_watching":  err == nil,
			"target_price": targetPrice.Float64,
		},
	})
}

// 获取闪购活动
func GetFlashSales(c *gin.Context) {
	rows, err := db.PG.Query(`
		SELECT id, title, description, start_time, end_time, status
		FROM flash_sales
		WHERE end_time > NOW() - INTERVAL '1 hour'
		ORDER BY start_time ASC
		LIMIT 5
	`)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []gin.H{}})
		return
	}
	defer rows.Close()

	var sales []gin.H
	for rows.Next() {
		var id int
		var title, description, status string
		var startTime, endTime time.Time

		rows.Scan(&id, &title, &description, &startTime, &endTime, &status)

		// 计算状态
		now := time.Now()
		if now.Before(startTime) {
			status = "upcoming"
		} else if now.After(endTime) {
			status = "ended"
		} else {
			status = "active"
		}

		sales = append(sales, gin.H{
			"id":                id,
			"title":             title,
			"description":       description,
			"start_time":        startTime.Format("15:04"),
			"end_time":          endTime.Format("15:04"),
			"status":            status,
			"remaining_seconds": int(time.Until(startTime).Seconds()),
		})
	}

	if sales == nil {
		sales = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": sales})
}

// ========== 内部函数：发送通知 ==========

// 发送通知给用户
func SendNotification(userID int, notifyType, title, content string, relatedID int, relatedType string) error {
	_, err := db.PG.Exec(`
		INSERT INTO notifications (user_id, type, title, content, related_id, related_type)
		VALUES ($1, $2, $3, $4, NULLIF($5, 0), NULLIF($6, ''))
	`, userID, notifyType, title, content, relatedID, relatedType)
	return err
}

// 发送降价提醒（检查关注的商品）
func CheckAndSendPriceDropNotifications(productID int, newPrice float64) {
	rows, _ := db.PG.Query(`
		SELECT pw.user_id, pw.target_price, p.name
		FROM product_watches pw
		JOIN products p ON pw.product_id = p.id
		WHERE pw.product_id = $1 
		  AND pw.target_price >= $2
		  AND (pw.notified_price IS NULL OR pw.notified_price > $2)
	`, productID, newPrice)
	defer rows.Close()

	for rows.Next() {
		var userID int
		var targetPrice float64
		var productName string
		rows.Scan(&userID, &targetPrice, &productName)

		// 发送通知
		SendNotification(userID, NotifyTypePriceDrop,
			"降价啦！",
			productName+" 已降至 ¥"+strconv.FormatFloat(newPrice, 'f', 2, 64),
			productID, "product")

		// 更新已通知价格
		db.PG.Exec(`UPDATE product_watches SET notified_price = $1 WHERE user_id = $2 AND product_id = $3`,
			newPrice, userID, productID)
	}
}

// 发送取货提醒
func SendPickupReminder(userID int, orderID int, orderNo string, shopName string) {
	SendNotification(userID, NotifyTypePickup,
		"别忘了取货哦！",
		"您在 "+shopName+" 的订单待取货，取货码："+orderNo,
		orderID, "order")
}

// 发送订单状态通知
func SendOrderStatusNotification(userID int, orderID int, status string, shopName string) {
	var title, content string
	switch status {
	case "paid":
		title = "支付成功"
		content = "您在 " + shopName + " 的订单已支付，请尽快取货"
	case "completed":
		title = "取货成功"
		content = "感谢您拯救食物！期待下次光临"
	case "cancelled":
		title = "订单已取消"
		content = "您的订单已取消"
	}

	if title != "" {
		SendNotification(userID, NotifyTypeOrderStatus, title, content, orderID, "order")
	}
}
