package handler

import (
	"database/sql"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"food-rescue/db"

	"github.com/gin-gonic/gin"
)

// 生成订单号
func generateOrderNo() string {
	return fmt.Sprintf("%s%06d", time.Now().Format("20060102150405"), rand.Intn(1000000))
}

// 生成取货码
func generatePickupCode() string {
	return fmt.Sprintf("%06d", rand.Intn(1000000))
}

// 订单商品项
type OrderItemRequest struct {
	ProductID int `json:"product_id"`
	Quantity  int `json:"quantity"`
}

// 配送地址
type DeliveryAddress struct {
	Name     string `json:"name"`
	Phone    string `json:"phone"`
	Province string `json:"province"`
	City     string `json:"city"`
	District string `json:"district"`
	Detail   string `json:"detail"`
}

// 创建订单请求
type CreateOrderRequest struct {
	Items        []OrderItemRequest `json:"items"`
	DeliveryType string             `json:"delivery_type"` // pickup 或 delivery
	Address      *DeliveryAddress   `json:"address"`       // 配送地址（delivery模式必填）
	Remark       string             `json:"remark"`
}

// 创建订单（从商品详情直接购买或从购物车结算）
func CreateOrder(c *gin.Context) {
	userID := c.Param("user_id")
	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误"})
		return
	}

	if len(req.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请选择商品"})
		return
	}

	// 默认到店自取
	if req.DeliveryType == "" {
		req.DeliveryType = "pickup"
	}

	// 配送模式必须有地址
	if req.DeliveryType == "delivery" && (req.Address == nil || req.Address.Name == "" || req.Address.Phone == "" || req.Address.Detail == "") {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请填写配送地址"})
		return
	}

	// 开启事务
	tx, err := db.PG.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "系统错误"})
		return
	}
	defer tx.Rollback()

	// 按商家分组创建订单
	merchantOrders := make(map[int][]OrderItemRequest)
	for _, item := range req.Items {
		var merchantID int
		err := tx.QueryRow(`SELECT merchant_id FROM products WHERE id = $1 AND status = 'active'`, item.ProductID).Scan(&merchantID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "商品不存在或已下架"})
			return
		}
		merchantOrders[merchantID] = append(merchantOrders[merchantID], item)
	}

	var orderIDs []int
	var orderNos []string

	for merchantID, items := range merchantOrders {
		orderNo := generateOrderNo()
		pickupCode := ""
		if req.DeliveryType == "pickup" {
			pickupCode = generatePickupCode()
		}
		var totalAmount, payAmount float64
		var deliveryFee float64 = 0
		if req.DeliveryType == "delivery" {
			deliveryFee = 5.0 // 配送费
		}

		// 创建订单
		var orderID int
		expireTime := time.Now().Add(30 * time.Minute) // 30分钟后过期

		var insertErr error
		if req.DeliveryType == "delivery" && req.Address != nil {
			insertErr = tx.QueryRow(`
				INSERT INTO orders (order_no, user_id, merchant_id, total_amount, pay_amount, delivery_type, 
					address_name, address_phone, address_province, address_city, address_district, address_detail,
					pickup_code, expire_time, remark)
				VALUES ($1, $2, $3, 0, 0, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
				RETURNING id
			`, orderNo, userID, merchantID, req.DeliveryType,
				req.Address.Name, req.Address.Phone, req.Address.Province, req.Address.City, req.Address.District, req.Address.Detail,
				pickupCode, expireTime, req.Remark).Scan(&orderID)
		} else {
			insertErr = tx.QueryRow(`
				INSERT INTO orders (order_no, user_id, merchant_id, total_amount, pay_amount, delivery_type, pickup_code, expire_time, remark)
				VALUES ($1, $2, $3, 0, 0, $4, $5, $6, $7)
				RETURNING id
			`, orderNo, userID, merchantID, req.DeliveryType, pickupCode, expireTime, req.Remark).Scan(&orderID)
		}

		if insertErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "创建订单失败"})
			return
		}

		// 添加订单商品
		for _, item := range items {
			var productName, productImage sql.NullString
			var originalPrice, currentPrice float64
			var stock int

			err := tx.QueryRow(`
				SELECT name, images[1], original_price, 
					CASE 
						WHEN price_rule_type = 'time_based' THEN
							original_price * COALESCE(
								(SELECT (rule->>'discount')::numeric / 100
								FROM jsonb_array_elements(price_rules) AS rule
								WHERE (rule->>'hours_left')::int >= EXTRACT(EPOCH FROM (expiry_date - NOW())) / 3600
								ORDER BY (rule->>'hours_left')::int ASC
								LIMIT 1),
								1
							)
						ELSE original_price
					END as current_price,
					stock
				FROM products WHERE id = $1 AND status = 'active'
			`, item.ProductID).Scan(&productName, &productImage, &originalPrice, &currentPrice, &stock)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "商品信息获取失败"})
				return
			}

			if stock < item.Quantity {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": fmt.Sprintf("%s 库存不足", productName.String)})
				return
			}

			subtotal := currentPrice * float64(item.Quantity)
			totalAmount += originalPrice * float64(item.Quantity)
			payAmount += subtotal

			// 插入订单商品
			_, err = tx.Exec(`
				INSERT INTO order_items (order_id, product_id, product_name, product_image, original_price, current_price, quantity, subtotal)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			`, orderID, item.ProductID, productName.String, productImage.String, originalPrice, currentPrice, item.Quantity, subtotal)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "添加订单商品失败"})
				return
			}

			// 扣减库存
			_, err = tx.Exec(`UPDATE products SET stock = stock - $1 WHERE id = $2`, item.Quantity, item.ProductID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "扣减库存失败"})
				return
			}
		}

		// 更新订单金额
		discountAmount := totalAmount - payAmount
		finalPayAmount := payAmount + deliveryFee
		_, err = tx.Exec(`
			UPDATE orders SET total_amount = $1, discount_amount = $2, delivery_fee = $3, pay_amount = $4 WHERE id = $5
		`, totalAmount, discountAmount, deliveryFee, finalPayAmount, orderID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "更新订单金额失败"})
			return
		}

		orderIDs = append(orderIDs, orderID)
		orderNos = append(orderNos, orderNo)
	}

	// 清除购物车中已下单的商品
	for _, item := range req.Items {
		tx.Exec(`DELETE FROM cart_items WHERE user_id = $1 AND product_id = $2`, userID, item.ProductID)
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "提交订单失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "订单创建成功",
		"data": gin.H{
			"order_ids": orderIDs,
			"order_nos": orderNos,
		},
	})
}

// 模拟支付（实际项目中对接支付平台）
func PayOrder(c *gin.Context) {
	orderID := c.Param("order_id")

	// 检查订单状态
	var status string
	var expireTime time.Time
	var userID, merchantID int
	err := db.PG.QueryRow(`SELECT status, expire_time, user_id, merchant_id FROM orders WHERE id = $1`, orderID).Scan(&status, &expireTime, &userID, &merchantID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "订单不存在"})
		return
	}

	if status != "pending" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "订单状态不正确"})
		return
	}

	if time.Now().After(expireTime) {
		// 订单已过期，恢复库存
		db.PG.Exec(`UPDATE orders SET status = 'expired' WHERE id = $1`, orderID)
		restoreStock(orderID)
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "订单已过期"})
		return
	}

	// 检查是否是盲盒订单
	var isBlindBox bool
	var productID int
	var price float64
	blindBoxErr := db.PG.QueryRow(`
		SELECT p.is_blind_box, oi.product_id, oi.current_price
		FROM order_items oi
		JOIN products p ON oi.product_id = p.id
		WHERE oi.order_id = $1
		LIMIT 1
	`, orderID).Scan(&isBlindBox, &productID, &price)
	if blindBoxErr != nil {
		// 查询失败时默认不是盲盒
		isBlindBox = false
	}

	// 开始事务
	tx, err := db.PG.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "系统错误"})
		return
	}
	defer tx.Rollback()

	// 更新订单状态为已支付
	_, err = tx.Exec(`
		UPDATE orders SET status = 'paid', pay_time = NOW(), updated_at = NOW() WHERE id = $1
	`, orderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "支付失败"})
		return
	}

	// 如果是盲盒订单，创建盲盒购买记录（库存已在CreateOrder中扣减，这里只更新销量和创建记录）
	var purchaseID int
	if isBlindBox {
		// 更新盲盒销量（库存已在CreateOrder扣减，不再重复扣减）
		tx.Exec(`
			UPDATE products SET sold_count = sold_count + 1 
			WHERE id = $1
		`, productID)

		// 创建盲盒购买记录
		err = tx.QueryRow(`
			INSERT INTO blind_box_purchases (user_id, product_id, merchant_id, price, status)
			VALUES ($1, $2, $3, $4, 'unopened')
			RETURNING id
		`, userID, productID, merchantID, price).Scan(&purchaseID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "创建盲盒记录失败: " + err.Error()})
			return
		}
	} else {
		// 普通订单：更新商品销量
		tx.Exec(`
			UPDATE products p SET sold_count = sold_count + oi.quantity
			FROM order_items oi WHERE oi.order_id = $1 AND oi.product_id = p.id
		`, orderID)
	}

	// 更新用户订单数
	tx.Exec(`
		UPDATE users SET total_orders = total_orders + 1 WHERE id = $1
	`, userID)

	if err = tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "支付失败"})
		return
	}

	// 更新挑战任务进度（支付成功后立即更新）
	var discountAmount float64
	var totalQuantity int
	db.PG.QueryRow(`SELECT discount_amount FROM orders WHERE id = $1`, orderID).Scan(&discountAmount)
	db.PG.QueryRow(`SELECT COALESCE(SUM(quantity), 0) FROM order_items WHERE order_id = $1`, orderID).Scan(&totalQuantity)
	// 按每件商品约0.3kg估算拯救食物重量
	estimatedWeight := float64(totalQuantity) * 0.3
	if estimatedWeight < 0.1 {
		estimatedWeight = 0.1
	}
	UpdateTaskProgress(userID, discountAmount, estimatedWeight)

	response := gin.H{"success": true, "message": "支付成功"}
	if isBlindBox && purchaseID > 0 {
		response["data"] = gin.H{
			"is_blind_box": true,
			"purchase_id":  purchaseID,
		}
	}

	c.JSON(http.StatusOK, response)
}

// 取消订单
func CancelOrder(c *gin.Context) {
	orderID := c.Param("order_id")
	userID := c.Param("user_id")

	var status string
	err := db.PG.QueryRow(`SELECT status FROM orders WHERE id = $1 AND user_id = $2`, orderID, userID).Scan(&status)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "订单不存在"})
		return
	}

	if status != "pending" && status != "paid" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "该订单无法取消"})
		return
	}

	// 更新订单状态
	_, err = db.PG.Exec(`
		UPDATE orders SET status = 'cancelled', cancel_time = NOW(), updated_at = NOW() WHERE id = $1
	`, orderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "取消失败"})
		return
	}

	// 恢复库存
	restoreStock(orderID)

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "订单已取消"})
}

// 恢复库存
func restoreStock(orderID string) {
	db.PG.Exec(`
		UPDATE products p SET stock = stock + oi.quantity
		FROM order_items oi WHERE oi.order_id = $1 AND oi.product_id = p.id
	`, orderID)
}

// 完成订单（商家核销）
func CompleteOrder(c *gin.Context) {
	orderID := c.Param("order_id")

	var status string
	var userID int
	var discountAmount float64
	err := db.PG.QueryRow(`SELECT status, user_id, COALESCE(discount_amount, 0) FROM orders WHERE id = $1`, orderID).Scan(&status, &userID, &discountAmount)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "订单不存在"})
		return
	}

	if status != "paid" {
		statusMsg := map[string]string{
			"pending":   "订单未支付，无法核销",
			"completed": "订单已核销",
			"cancelled": "订单已取消",
			"expired":   "订单已过期",
		}
		msg := statusMsg[status]
		if msg == "" {
			msg = "订单状态不正确"
		}
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": msg})
		return
	}

	// 更新订单状态
	_, err = db.PG.Exec(`
		UPDATE orders SET status = 'completed', complete_time = NOW(), pickup_time = NOW(), updated_at = NOW() WHERE id = $1
	`, orderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "核销失败"})
		return
	}

	// 更新商家销售额
	db.PG.Exec(`
		UPDATE merchants m SET 
			total_sales = total_sales + o.pay_amount,
			total_saved_loss = total_saved_loss + o.discount_amount,
			total_products_sold = total_products_sold + (SELECT SUM(quantity) FROM order_items WHERE order_id = $1)
		FROM orders o WHERE o.id = $1 AND o.merchant_id = m.id
	`, orderID)

	// 更新用户环保数据（省钱 + 拯救食物重量）
	var orderQuantity int
	db.PG.QueryRow(`SELECT COALESCE(SUM(quantity), 0) FROM order_items WHERE order_id = $1`, orderID).Scan(&orderQuantity)
	estimatedWeight := float64(orderQuantity) * 0.3
	if estimatedWeight < 0.1 {
		estimatedWeight = 0.1
	}
	db.PG.Exec(`
		UPDATE users SET 
			total_saved_money = total_saved_money + $1,
			total_saved_weight = total_saved_weight + $2,
			total_orders = total_orders + 1
		WHERE id = $3
	`, discountAmount, estimatedWeight, userID)

	// 更新用户段位等级（基于累计拯救重量）
	var totalWeight float64
	db.PG.QueryRow(`SELECT total_saved_weight FROM users WHERE id = $1`, userID).Scan(&totalWeight)
	newRank := "newcomer"
	if totalWeight >= 200 {
		newRank = "protector"
	} else if totalWeight >= 50 {
		newRank = "guardian"
	} else if totalWeight >= 10 {
		newRank = "expert"
	}
	db.PG.Exec(`UPDATE users SET rank_level = $1 WHERE id = $2`, newRank, userID)

	// 更新小队贡献（如果用户在小队中）
	db.PG.Exec(`
		UPDATE team_members SET contribution = contribution + $1
		WHERE user_id = $2
	`, discountAmount, userID)
	db.PG.Exec(`
		UPDATE teams SET 
			total_saved = total_saved + $1,
			weekly_saved = weekly_saved + $1
		WHERE id = (SELECT team_id FROM team_members WHERE user_id = $2)
	`, discountAmount, userID)

	// 记录好友动态
	var firstProductID int
	var firstDiscount float64
	err = db.PG.QueryRow(`
		SELECT product_id, CASE WHEN original_price > 0 THEN current_price / original_price ELSE 1 END
		FROM order_items WHERE order_id = $1 LIMIT 1
	`, orderID).Scan(&firstProductID, &firstDiscount)
	if err == nil {
		RecordPurchaseActivity(userID, firstProductID, firstDiscount, discountAmount)
	}

	// 注意：挑战任务进度已在支付时更新，这里不再重复更新

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "核销成功"})
}

// 获取用户订单列表
func GetUserOrders(c *gin.Context) {
	userID := c.Param("user_id")
	status := c.Query("status") // 可选筛选

	query := `
		SELECT o.id, o.order_no, o.merchant_id, m.shop_name, o.total_amount, o.discount_amount, 
			o.delivery_fee, o.pay_amount, o.delivery_type, o.status, o.pickup_code, o.remark, 
			o.expire_time, o.pay_time, o.complete_time, o.created_at,
			o.address_name, o.address_phone, o.address_province, o.address_city, o.address_district, o.address_detail,
			(SELECT COUNT(*) FROM order_items WHERE order_id = o.id) as item_count
		FROM orders o
		JOIN merchants m ON o.merchant_id = m.id
		WHERE o.user_id = $1
	`
	args := []interface{}{userID}

	if status != "" {
		query += ` AND o.status = $2`
		args = append(args, status)
	}
	query += ` ORDER BY o.created_at DESC`

	rows, err := db.PG.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "查询失败"})
		return
	}
	defer rows.Close()

	var orders []gin.H
	for rows.Next() {
		var id, merchantID, itemCount int
		var orderNo, shopName, deliveryType, orderStatus, pickupCode string
		var totalAmount, discountAmount, deliveryFee, payAmount float64
		var remark, addrName, addrPhone, addrProvince, addrCity, addrDistrict, addrDetail sql.NullString
		var expireTime, createdAt time.Time
		var payTime, completeTime sql.NullTime

		err := rows.Scan(&id, &orderNo, &merchantID, &shopName, &totalAmount, &discountAmount,
			&deliveryFee, &payAmount, &deliveryType, &orderStatus, &pickupCode, &remark, 
			&expireTime, &payTime, &completeTime, &createdAt,
			&addrName, &addrPhone, &addrProvince, &addrCity, &addrDistrict, &addrDetail, &itemCount)
		if err != nil {
			continue
		}

		// 检查是否过期
		if orderStatus == "pending" && time.Now().After(expireTime) {
			orderStatus = "expired"
			db.PG.Exec(`UPDATE orders SET status = 'expired' WHERE id = $1`, id)
			restoreStock(fmt.Sprintf("%d", id))
		}

		order := gin.H{
			"id":              id,
			"order_no":        orderNo,
			"merchant_id":     merchantID,
			"shop_name":       shopName,
			"total_amount":    totalAmount,
			"discount_amount": discountAmount,
			"delivery_fee":    deliveryFee,
			"pay_amount":      payAmount,
			"delivery_type":   deliveryType,
			"status":          orderStatus,
			"pickup_code":     pickupCode,
			"remark":          remark.String,
			"expire_time":     expireTime,
			"pay_time":        payTime.Time,
			"complete_time":   completeTime.Time,
			"created_at":      createdAt,
			"item_count":      itemCount,
		}

		if deliveryType == "delivery" {
			order["address"] = gin.H{
				"name":     addrName.String,
				"phone":    addrPhone.String,
				"province": addrProvince.String,
				"city":     addrCity.String,
				"district": addrDistrict.String,
				"detail":   addrDetail.String,
			}
		}

		orders = append(orders, order)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": orders})
}

// 获取订单详情
func GetOrderDetail(c *gin.Context) {
	orderID := c.Param("order_id")

	var id, merchantID int
	var orderNo, shopName, deliveryType, orderStatus string
	var totalAmount, discountAmount, deliveryFee, payAmount float64
	var remark, addrName, addrPhone, addrProvince, addrCity, addrDistrict, addrDetail sql.NullString
	var shopAddress, pickupCode sql.NullString
	var expireTime, createdAt time.Time
	var payTime, completeTime sql.NullTime

	err := db.PG.QueryRow(`
		SELECT o.id, o.order_no, o.merchant_id, m.shop_name, COALESCE(m.address, ''), o.total_amount, 
			o.discount_amount, o.delivery_fee, o.pay_amount, o.delivery_type, o.status, COALESCE(o.pickup_code, ''), o.remark, 
			o.expire_time, o.pay_time, o.complete_time, o.created_at,
			o.address_name, o.address_phone, o.address_province, o.address_city, o.address_district, o.address_detail
		FROM orders o
		JOIN merchants m ON o.merchant_id = m.id
		WHERE o.id = $1
	`, orderID).Scan(&id, &orderNo, &merchantID, &shopName, &shopAddress, &totalAmount,
		&discountAmount, &deliveryFee, &payAmount, &deliveryType, &orderStatus, &pickupCode, &remark,
		&expireTime, &payTime, &completeTime, &createdAt,
		&addrName, &addrPhone, &addrProvince, &addrCity, &addrDistrict, &addrDetail)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "订单不存在: " + err.Error()})
		return
	}

	// 检查是否过期
	if orderStatus == "pending" && time.Now().After(expireTime) {
		orderStatus = "expired"
		db.PG.Exec(`UPDATE orders SET status = 'expired' WHERE id = $1`, id)
		restoreStock(orderID)
	}

	// 获取订单商品
	rows, _ := db.PG.Query(`
		SELECT product_id, product_name, product_image, original_price, current_price, quantity, subtotal
		FROM order_items WHERE order_id = $1
	`, orderID)
	defer rows.Close()

	var items []gin.H
	for rows.Next() {
		var productID, quantity int
		var productName string
		var productImage sql.NullString
		var originalPrice, currentPrice, subtotal float64

		rows.Scan(&productID, &productName, &productImage, &originalPrice, &currentPrice, &quantity, &subtotal)
		items = append(items, gin.H{
			"product_id":     productID,
			"product_name":   productName,
			"product_image":  productImage.String,
			"original_price": originalPrice,
			"current_price":  currentPrice,
			"quantity":       quantity,
			"subtotal":       subtotal,
		})
	}

	result := gin.H{
		"id":              id,
		"order_no":        orderNo,
		"merchant_id":     merchantID,
		"shop_name":       shopName,
		"shop_address":    shopAddress.String,
		"total_amount":    totalAmount,
		"discount_amount": discountAmount,
		"delivery_fee":    deliveryFee,
		"pay_amount":      payAmount,
		"delivery_type":   deliveryType,
		"status":          orderStatus,
		"pickup_code":     pickupCode.String,
		"remark":          remark.String,
		"expire_time":     expireTime,
		"pay_time":        payTime.Time,
		"complete_time":   completeTime.Time,
		"created_at":      createdAt,
		"items":           items,
	}

	if deliveryType == "delivery" {
		result["address"] = gin.H{
			"name":     addrName.String,
			"phone":    addrPhone.String,
			"province": addrProvince.String,
			"city":     addrCity.String,
			"district": addrDistrict.String,
			"detail":   addrDetail.String,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// 获取商家订单列表
func GetMerchantOrders(c *gin.Context) {
	merchantID := c.Param("merchant_id")
	status := c.Query("status")

	query := `
		SELECT o.id, o.order_no, o.user_id, u.nickname, u.phone, o.total_amount, 
			o.discount_amount, o.delivery_fee, o.pay_amount, o.delivery_type, o.status, o.pickup_code, o.remark,
			o.expire_time, o.pay_time, o.complete_time, o.created_at,
			o.address_name, o.address_phone, o.address_province, o.address_city, o.address_district, o.address_detail,
			(SELECT COUNT(*) FROM order_items WHERE order_id = o.id) as item_count,
			COALESCE((SELECT bool_or(p.is_blind_box) FROM order_items oi JOIN products p ON oi.product_id = p.id WHERE oi.order_id = o.id), false) as is_blind_box
		FROM orders o
		JOIN users u ON o.user_id = u.id
		WHERE o.merchant_id = $1
	`
	args := []interface{}{merchantID}

	if status != "" {
		query += ` AND o.status = $2`
		args = append(args, status)
	}
	query += ` ORDER BY o.created_at DESC`

	rows, err := db.PG.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "查询失败"})
		return
	}
	defer rows.Close()

	var orders []gin.H
	for rows.Next() {
		var id, userID, itemCount int
		var orderNo, nickname, phone, deliveryType, orderStatus, pickupCode string
		var totalAmount, discountAmount, deliveryFee, payAmount float64
		var remark, addrName, addrPhone, addrProvince, addrCity, addrDistrict, addrDetail sql.NullString
		var expireTime, createdAt time.Time
		var payTime, completeTime sql.NullTime
		var isBlindBox bool

		err := rows.Scan(&id, &orderNo, &userID, &nickname, &phone, &totalAmount, &discountAmount,
			&deliveryFee, &payAmount, &deliveryType, &orderStatus, &pickupCode, &remark, &expireTime, &payTime,
			&completeTime, &createdAt, &addrName, &addrPhone, &addrProvince, &addrCity, &addrDistrict, &addrDetail, &itemCount, &isBlindBox)
		if err != nil {
			continue
		}

		order := gin.H{
			"id":              id,
			"order_no":        orderNo,
			"user_id":         userID,
			"user_nickname":   nickname,
			"user_phone":      phone,
			"total_amount":    totalAmount,
			"discount_amount": discountAmount,
			"delivery_fee":    deliveryFee,
			"pay_amount":      payAmount,
			"delivery_type":   deliveryType,
			"is_blind_box":    isBlindBox,
			"status":          orderStatus,
			"pickup_code":     pickupCode,
			"remark":          remark.String,
			"expire_time":     expireTime,
			"pay_time":        payTime.Time,
			"complete_time":   completeTime.Time,
			"created_at":      createdAt,
			"item_count":      itemCount,
		}

		if deliveryType == "delivery" {
			order["address"] = gin.H{
				"name":     addrName.String,
				"phone":    addrPhone.String,
				"province": addrProvince.String,
				"city":     addrCity.String,
				"district": addrDistrict.String,
				"detail":   addrDetail.String,
			}
		}

		orders = append(orders, order)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": orders})
}
