package handler

import (
	"database/sql"
	"fmt"
	"food-rescue/db"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

// ========== 管理员认证 ==========

func AdminLogin(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请输入用户名和密码"})
		return
	}

	var id int
	var password, nickname, role, status string
	err := db.PG.QueryRow(`SELECT id, password, nickname, role, status FROM admins WHERE username = $1`, req.Username).
		Scan(&id, &password, &nickname, &role, &status)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "用户名或密码错误"})
		return
	}
	if password != req.Password {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "用户名或密码错误"})
		return
	}
	if status != "active" {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "账号已被禁用"})
		return
	}

	db.PG.Exec(`UPDATE admins SET last_login_at = NOW() WHERE id = $1`, id)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"admin_id": id,
			"username": req.Username,
			"nickname": nickname,
			"role":     role,
		},
	})
}

// ========== 用户管理 ==========

func AdminGetUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	keyword := c.Query("keyword")
	status := c.Query("status")

	if page < 1 { page = 1 }
	offset := (page - 1) * pageSize

	// 连表查询：用户 + 订单统计
	where := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if keyword != "" {
		where += fmt.Sprintf(` AND (u.phone ILIKE $%d OR u.nickname ILIKE $%d)`, argIdx, argIdx)
		args = append(args, "%"+keyword+"%")
		argIdx++
	}
	if status != "" {
		where += fmt.Sprintf(` AND u.status = $%d`, argIdx)
		args = append(args, status)
		argIdx++
	}

	// 总数
	var total int
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM users u %s`, where)
	db.PG.QueryRow(countQuery, args...).Scan(&total)

	// 列表（连表查询订单数和消费金额）
	query := fmt.Sprintf(`
		SELECT u.id, u.phone, u.nickname, u.avatar, u.total_saved_weight, u.total_saved_money,
		       u.total_orders, u.rank_level, u.status, u.created_at,
		       COALESCE(order_stats.order_count, 0) as real_order_count,
		       COALESCE(order_stats.total_paid, 0) as total_paid
		FROM users u
		LEFT JOIN (
			SELECT user_id, COUNT(*) as order_count, SUM(pay_amount) as total_paid
			FROM orders WHERE status IN ('paid', 'completed')
			GROUP BY user_id
		) order_stats ON u.id = order_stats.user_id
		%s
		ORDER BY u.created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, pageSize, offset)

	rows, err := db.PG.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"list": []gin.H{}, "total": 0}})
		return
	}
	defer rows.Close()

	var users []gin.H
	for rows.Next() {
		var id, totalOrders, realOrderCount int
		var phone string
		var nickname, avatar, rankLevel, userStatus sql.NullString
		var totalWeight, totalMoney, totalPaid float64
		var createdAt time.Time

		rows.Scan(&id, &phone, &nickname, &avatar, &totalWeight, &totalMoney,
			&totalOrders, &rankLevel, &userStatus, &createdAt, &realOrderCount, &totalPaid)

		users = append(users, gin.H{
			"id": id, "phone": phone, "nickname": nickname.String, "avatar": avatar.String,
			"total_saved_weight": totalWeight, "total_saved_money": totalMoney,
			"total_orders": totalOrders, "real_order_count": realOrderCount,
			"total_paid": totalPaid, "rank_level": rankLevel.String,
			"status": userStatus.String, "created_at": createdAt.Format("2006-01-02 15:04:05"),
		})
	}
	if users == nil { users = []gin.H{} }

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"list": users, "total": total, "page": page, "page_size": pageSize}})
}

func AdminUpdateUserStatus(c *gin.Context) {
	userID := c.Param("id")
	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误"})
		return
	}
	_, err := db.PG.Exec(`UPDATE users SET status = $1, updated_at = NOW() WHERE id = $2`, req.Status, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "操作失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "更新成功"})
}

// ========== 商家管理 ==========

func AdminGetMerchants(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	keyword := c.Query("keyword")
	status := c.Query("status")

	if page < 1 { page = 1 }
	offset := (page - 1) * pageSize

	where := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if keyword != "" {
		where += fmt.Sprintf(` AND (m.phone ILIKE $%d OR m.shop_name ILIKE $%d)`, argIdx, argIdx)
		args = append(args, "%"+keyword+"%")
		argIdx++
	}
	if status != "" {
		where += fmt.Sprintf(` AND m.status = $%d`, argIdx)
		args = append(args, status)
		argIdx++
	}

	var total int
	db.PG.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM merchants m %s`, where), args...).Scan(&total)

	// 连表查询：商家 + 商品数 + 订单数 + 销售额
	query := fmt.Sprintf(`
		SELECT m.id, m.phone, m.shop_name, m.shop_logo, m.is_verified, m.province, m.city, m.district,
		       m.rating, m.rating_count, m.total_sales, m.total_products_sold, m.status, m.created_at,
		       COALESCE(p_stats.product_count, 0) as product_count,
		       COALESCE(p_stats.active_count, 0) as active_product_count,
		       COALESCE(o_stats.order_count, 0) as order_count,
		       COALESCE(o_stats.revenue, 0) as revenue
		FROM merchants m
		LEFT JOIN (
			SELECT merchant_id, COUNT(*) as product_count,
			       COUNT(*) FILTER (WHERE status = 'active') as active_count
			FROM products GROUP BY merchant_id
		) p_stats ON m.id = p_stats.merchant_id
		LEFT JOIN (
			SELECT merchant_id, COUNT(*) as order_count, SUM(pay_amount) as revenue
			FROM orders WHERE status IN ('paid', 'completed')
			GROUP BY merchant_id
		) o_stats ON m.id = o_stats.merchant_id
		%s
		ORDER BY m.created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, pageSize, offset)

	rows, err := db.PG.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"list": []gin.H{}, "total": 0}})
		return
	}
	defer rows.Close()

	var merchants []gin.H
	for rows.Next() {
		var id, ratingCount, totalProductsSold, productCount, activeProductCount, orderCount int
		var phone string
		var shopName, shopLogo, province, city, district, mStatus sql.NullString
		var isVerified bool
		var rating, totalSales, revenue float64
		var createdAt time.Time

		rows.Scan(&id, &phone, &shopName, &shopLogo, &isVerified, &province, &city, &district,
			&rating, &ratingCount, &totalSales, &totalProductsSold, &mStatus, &createdAt,
			&productCount, &activeProductCount, &orderCount, &revenue)

		merchants = append(merchants, gin.H{
			"id": id, "phone": phone, "shop_name": shopName.String, "shop_logo": shopLogo.String,
			"is_verified": isVerified, "province": province.String, "city": city.String, "district": district.String,
			"rating": rating, "rating_count": ratingCount, "total_sales": totalSales,
			"total_products_sold": totalProductsSold, "status": mStatus.String,
			"created_at": createdAt.Format("2006-01-02 15:04:05"),
			"product_count": productCount, "active_product_count": activeProductCount,
			"order_count": orderCount, "revenue": revenue,
		})
	}
	if merchants == nil { merchants = []gin.H{} }

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"list": merchants, "total": total, "page": page, "page_size": pageSize}})
}

func AdminUpdateMerchantStatus(c *gin.Context) {
	merchantID := c.Param("id")
	var req struct {
		Status     string `json:"status"`
		IsVerified *bool  `json:"is_verified"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误"})
		return
	}
	if req.Status != "" {
		db.PG.Exec(`UPDATE merchants SET status = $1, updated_at = NOW() WHERE id = $2`, req.Status, merchantID)
	}
	if req.IsVerified != nil {
		db.PG.Exec(`UPDATE merchants SET is_verified = $1, updated_at = NOW() WHERE id = $2`, *req.IsVerified, merchantID)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "更新成功"})
}

// ========== 统计概览 ==========

func AdminDashboard(c *gin.Context) {
	var userCount, merchantCount, orderCount, productCount int
	var totalRevenue, totalSaved float64

	db.PG.QueryRow(`SELECT COUNT(*) FROM users WHERE status = 'active'`).Scan(&userCount)
	db.PG.QueryRow(`SELECT COUNT(*) FROM merchants WHERE status = 'active'`).Scan(&merchantCount)
	db.PG.QueryRow(`SELECT COUNT(*) FROM orders WHERE status IN ('paid', 'completed')`).Scan(&orderCount)
	db.PG.QueryRow(`SELECT COUNT(*) FROM products WHERE status = 'active'`).Scan(&productCount)
	db.PG.QueryRow(`SELECT COALESCE(SUM(pay_amount), 0) FROM orders WHERE status IN ('paid', 'completed')`).Scan(&totalRevenue)
	db.PG.QueryRow(`SELECT COALESCE(SUM(discount_amount), 0) FROM orders WHERE status IN ('paid', 'completed')`).Scan(&totalSaved)

	// 今日数据
	var todayOrders int
	var todayRevenue float64
	db.PG.QueryRow(`SELECT COUNT(*) FROM orders WHERE status IN ('paid', 'completed') AND DATE(created_at) = CURRENT_DATE`).Scan(&todayOrders)
	db.PG.QueryRow(`SELECT COALESCE(SUM(pay_amount), 0) FROM orders WHERE status IN ('paid', 'completed') AND DATE(created_at) = CURRENT_DATE`).Scan(&todayRevenue)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"user_count": userCount, "merchant_count": merchantCount,
			"order_count": orderCount, "product_count": productCount,
			"total_revenue": totalRevenue, "total_saved": totalSaved,
			"today_orders": todayOrders, "today_revenue": todayRevenue,
		},
	})
}

// ========== 订单管理 ==========

func AdminGetOrders(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	keyword := c.Query("keyword")     // 订单号/用户手机号/商家名
	status := c.Query("status")
	deliveryType := c.Query("delivery_type")

	if page < 1 { page = 1 }
	offset := (page - 1) * pageSize

	where := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if keyword != "" {
		where += fmt.Sprintf(` AND (o.order_no ILIKE $%d OR u.phone ILIKE $%d OR m.shop_name ILIKE $%d)`, argIdx, argIdx, argIdx)
		args = append(args, "%"+keyword+"%")
		argIdx++
	}
	if status != "" {
		where += fmt.Sprintf(` AND o.status = $%d`, argIdx)
		args = append(args, status)
		argIdx++
	}
	if deliveryType != "" {
		where += fmt.Sprintf(` AND o.delivery_type = $%d`, argIdx)
		args = append(args, deliveryType)
		argIdx++
	}

	// 总数
	var total int
	countQ := fmt.Sprintf(`
		SELECT COUNT(*) FROM orders o
		JOIN users u ON o.user_id = u.id
		JOIN merchants m ON o.merchant_id = m.id
		%s
	`, where)
	db.PG.QueryRow(countQ, args...).Scan(&total)

	// 连表查询：订单 + 用户 + 商家 + 商品数量
	query := fmt.Sprintf(`
		SELECT o.id, o.order_no, o.user_id, u.phone as user_phone, u.nickname as user_nickname,
		       o.merchant_id, m.shop_name,
		       o.total_amount, o.discount_amount, o.delivery_fee, o.pay_amount,
		       o.delivery_type, o.status, o.pickup_code,
		       o.remark, o.pay_time, o.complete_time, o.created_at,
		       COALESCE(oi.item_count, 0) as item_count,
		       COALESCE(oi.product_names, '') as product_names
		FROM orders o
		JOIN users u ON o.user_id = u.id
		JOIN merchants m ON o.merchant_id = m.id
		LEFT JOIN (
			SELECT order_id, COUNT(*) as item_count,
			       STRING_AGG(product_name, '、' ORDER BY id) as product_names
			FROM order_items GROUP BY order_id
		) oi ON o.id = oi.order_id
		%s
		ORDER BY o.created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, pageSize, offset)

	rows, err := db.PG.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"list": []gin.H{}, "total": 0}})
		return
	}
	defer rows.Close()

	var orders []gin.H
	for rows.Next() {
		var id, userID, merchantID, itemCount int
		var orderNo, userPhone, delivType, oStatus string
		var userNickname, shopName, pickupCode, remark, productNames sql.NullString
		var totalAmount, discountAmount, deliveryFee, payAmount float64
		var payTime, completeTime sql.NullTime
		var createdAt time.Time

		rows.Scan(&id, &orderNo, &userID, &userPhone, &userNickname,
			&merchantID, &shopName,
			&totalAmount, &discountAmount, &deliveryFee, &payAmount,
			&delivType, &oStatus, &pickupCode,
			&remark, &payTime, &completeTime, &createdAt,
			&itemCount, &productNames)

		order := gin.H{
			"id": id, "order_no": orderNo,
			"user_id": userID, "user_phone": userPhone, "user_nickname": userNickname.String,
			"merchant_id": merchantID, "shop_name": shopName.String,
			"total_amount": totalAmount, "discount_amount": discountAmount,
			"delivery_fee": deliveryFee, "pay_amount": payAmount,
			"delivery_type": delivType, "status": oStatus,
			"pickup_code": pickupCode.String, "remark": remark.String,
			"item_count": itemCount, "product_names": productNames.String,
			"created_at": createdAt.Format("2006-01-02 15:04:05"),
		}
		if payTime.Valid { order["pay_time"] = payTime.Time.Format("2006-01-02 15:04:05") }
		if completeTime.Valid { order["complete_time"] = completeTime.Time.Format("2006-01-02 15:04:05") }

		orders = append(orders, order)
	}
	if orders == nil { orders = []gin.H{} }

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"list": orders, "total": total, "page": page, "page_size": pageSize}})
}

// AdminGetOrderDetail 管理端订单详情（含商品明细）
func AdminGetOrderDetail(c *gin.Context) {
	orderID := c.Param("id")

	// 订单基本信息 + 用户 + 商家
	var id, userID, merchantID int
	var orderNo, userPhone, delivType, oStatus string
	var userNickname, shopName, pickupCode, remark sql.NullString
	var totalAmount, discountAmount, deliveryFee, payAmount float64
	var payTime, completeTime sql.NullTime
	var createdAt time.Time
	// 配送地址
	var addrName, addrPhone, addrProvince, addrCity, addrDistrict, addrDetail sql.NullString

	err := db.PG.QueryRow(`
		SELECT o.id, o.order_no, o.user_id, u.phone, u.nickname,
		       o.merchant_id, m.shop_name,
		       o.total_amount, o.discount_amount, o.delivery_fee, o.pay_amount,
		       o.delivery_type, o.status, o.pickup_code, o.remark,
		       o.pay_time, o.complete_time, o.created_at,
		       o.address_name, o.address_phone, o.address_province, o.address_city, o.address_district, o.address_detail
		FROM orders o
		JOIN users u ON o.user_id = u.id
		JOIN merchants m ON o.merchant_id = m.id
		WHERE o.id = $1
	`, orderID).Scan(&id, &orderNo, &userID, &userPhone, &userNickname,
		&merchantID, &shopName,
		&totalAmount, &discountAmount, &deliveryFee, &payAmount,
		&delivType, &oStatus, &pickupCode, &remark,
		&payTime, &completeTime, &createdAt,
		&addrName, &addrPhone, &addrProvince, &addrCity, &addrDistrict, &addrDetail)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "订单不存在"})
		return
	}

	order := gin.H{
		"id": id, "order_no": orderNo,
		"user_id": userID, "user_phone": userPhone, "user_nickname": userNickname.String,
		"merchant_id": merchantID, "shop_name": shopName.String,
		"total_amount": totalAmount, "discount_amount": discountAmount,
		"delivery_fee": deliveryFee, "pay_amount": payAmount,
		"delivery_type": delivType, "status": oStatus,
		"pickup_code": pickupCode.String, "remark": remark.String,
		"created_at": createdAt.Format("2006-01-02 15:04:05"),
	}
	if payTime.Valid { order["pay_time"] = payTime.Time.Format("2006-01-02 15:04:05") }
	if completeTime.Valid { order["complete_time"] = completeTime.Time.Format("2006-01-02 15:04:05") }
	if addrName.Valid {
		order["address"] = gin.H{
			"name": addrName.String, "phone": addrPhone.String,
			"province": addrProvince.String, "city": addrCity.String,
			"district": addrDistrict.String, "detail": addrDetail.String,
		}
	}

	// 商品明细
	itemRows, _ := db.PG.Query(`
		SELECT product_id, product_name, original_price, current_price, quantity, subtotal
		FROM order_items WHERE order_id = $1 ORDER BY id
	`, orderID)
	defer itemRows.Close()

	var items []gin.H
	for itemRows.Next() {
		var productID, qty int
		var productName string
		var origPrice, curPrice, subtotal float64
		itemRows.Scan(&productID, &productName, &origPrice, &curPrice, &qty, &subtotal)
		items = append(items, gin.H{
			"product_id": productID, "product_name": productName,
			"original_price": origPrice, "current_price": curPrice,
			"quantity": qty, "subtotal": subtotal,
		})
	}
	if items == nil { items = []gin.H{} }
	order["items"] = items

	c.JSON(http.StatusOK, gin.H{"success": true, "data": order})
}

// ========== 商品管理 ==========

func AdminGetProducts(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	keyword := c.Query("keyword")
	status := c.Query("status")
	categoryID := c.Query("category_id")
	merchantID := c.Query("merchant_id")
	expiring := c.Query("expiring") // "1" = 3天内到期

	if page < 1 { page = 1 }
	offset := (page - 1) * pageSize

	where := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if keyword != "" {
		where += fmt.Sprintf(` AND (p.name ILIKE $%d OR m.shop_name ILIKE $%d)`, argIdx, argIdx)
		args = append(args, "%"+keyword+"%")
		argIdx++
	}
	if status != "" {
		where += fmt.Sprintf(` AND p.status = $%d`, argIdx)
		args = append(args, status)
		argIdx++
	}
	if categoryID != "" {
		where += fmt.Sprintf(` AND p.category_id = $%d`, argIdx)
		args = append(args, categoryID)
		argIdx++
	}
	if merchantID != "" {
		where += fmt.Sprintf(` AND p.merchant_id = $%d`, argIdx)
		args = append(args, merchantID)
		argIdx++
	}
	if expiring == "1" {
		where += ` AND p.expiry_date <= CURRENT_DATE + INTERVAL '3 days' AND p.expiry_date >= CURRENT_DATE`
	}

	var total int
	db.PG.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM products p JOIN merchants m ON p.merchant_id = m.id %s`, where), args...).Scan(&total)

	query := fmt.Sprintf(`
		SELECT p.id, p.name, p.images[1] as image, p.original_price, p.expiry_date, p.stock, p.sold_count,
		       p.price_rule_type, p.is_blind_box, p.status, p.created_at,
		       m.id as merchant_id, m.shop_name,
		       c.name as category_name,
		       COALESCE(oi.order_count, 0) as order_count
		FROM products p
		JOIN merchants m ON p.merchant_id = m.id
		LEFT JOIN categories c ON p.category_id = c.id
		LEFT JOIN (
			SELECT product_id, COUNT(DISTINCT order_id) as order_count
			FROM order_items GROUP BY product_id
		) oi ON p.id = oi.product_id
		%s
		ORDER BY p.created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, pageSize, offset)

	rows, err := db.PG.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"list": []gin.H{}, "total": 0}})
		return
	}
	defer rows.Close()

	var products []gin.H
	now := time.Now()
	for rows.Next() {
		var id, stock, soldCount, mID, orderCount int
		var pName, priceRuleType, pStatus string
		var image, shopName, categoryName sql.NullString
		var originalPrice float64
		var expiryDate time.Time
		var isBlindBox bool
		var createdAt time.Time

		rows.Scan(&id, &pName, &image, &originalPrice, &expiryDate, &stock, &soldCount,
			&priceRuleType, &isBlindBox, &pStatus, &createdAt,
			&mID, &shopName, &categoryName, &orderCount)

		daysLeft := int(expiryDate.Sub(now).Hours()/24) + 1
		if daysLeft < 0 { daysLeft = 0 }

		products = append(products, gin.H{
			"id": id, "name": pName, "image": image.String,
			"original_price": originalPrice, "expiry_date": expiryDate.Format("2006-01-02"),
			"days_left": daysLeft, "stock": stock, "sold_count": soldCount,
			"price_rule_type": priceRuleType, "is_blind_box": isBlindBox,
			"status": pStatus, "created_at": createdAt.Format("2006-01-02 15:04"),
			"merchant_id": mID, "shop_name": shopName.String,
			"category_name": categoryName.String, "order_count": orderCount,
		})
	}
	if products == nil { products = []gin.H{} }

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"list": products, "total": total, "page": page, "page_size": pageSize}})
}

func AdminUpdateProductStatus(c *gin.Context) {
	productID := c.Param("id")
	var req struct { Status string `json:"status" binding:"required"` }
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误"})
		return
	}
	_, err := db.PG.Exec(`UPDATE products SET status = $1, updated_at = NOW() WHERE id = $2`, req.Status, productID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "操作失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "更新成功"})
}

// ========== 分类管理 ==========

func AdminGetCategories(c *gin.Context) {
	rows, err := db.PG.Query(`
		SELECT c.id, c.name, c.icon, c.sort_order, c.is_active, c.created_at,
		       COALESCE(p_count.cnt, 0) as product_count
		FROM categories c
		LEFT JOIN (SELECT category_id, COUNT(*) as cnt FROM products GROUP BY category_id) p_count ON c.id = p_count.category_id
		ORDER BY c.sort_order, c.id
	`)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []gin.H{}})
		return
	}
	defer rows.Close()

	var cats []gin.H
	for rows.Next() {
		var id, sortOrder, productCount int
		var name string
		var icon sql.NullString
		var isActive bool
		var createdAt time.Time
		rows.Scan(&id, &name, &icon, &sortOrder, &isActive, &createdAt, &productCount)
		cats = append(cats, gin.H{
			"id": id, "name": name, "icon": icon.String, "sort_order": sortOrder,
			"is_active": isActive, "product_count": productCount,
			"created_at": createdAt.Format("2006-01-02 15:04"),
		})
	}
	if cats == nil { cats = []gin.H{} }
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cats})
}

func AdminCreateCategory(c *gin.Context) {
	var req struct {
		Name      string `json:"name" binding:"required"`
		Icon      string `json:"icon"`
		SortOrder int    `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请输入分类名称"})
		return
	}
	var id int
	err := db.PG.QueryRow(`INSERT INTO categories (name, icon, sort_order) VALUES ($1, $2, $3) RETURNING id`,
		req.Name, req.Icon, req.SortOrder).Scan(&id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "创建失败，分类名可能已存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "创建成功", "data": gin.H{"id": id}})
}

func AdminUpdateCategory(c *gin.Context) {
	catID := c.Param("id")
	var req struct {
		Name      string `json:"name"`
		Icon      string `json:"icon"`
		SortOrder *int   `json:"sort_order"`
		IsActive  *bool  `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误"})
		return
	}
	if req.Name != "" {
		db.PG.Exec(`UPDATE categories SET name = $1 WHERE id = $2`, req.Name, catID)
	}
	if req.Icon != "" {
		db.PG.Exec(`UPDATE categories SET icon = $1 WHERE id = $2`, req.Icon, catID)
	}
	if req.SortOrder != nil {
		db.PG.Exec(`UPDATE categories SET sort_order = $1 WHERE id = $2`, *req.SortOrder, catID)
	}
	if req.IsActive != nil {
		db.PG.Exec(`UPDATE categories SET is_active = $1 WHERE id = $2`, *req.IsActive, catID)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "更新成功"})
}

func AdminDeleteCategory(c *gin.Context) {
	catID := c.Param("id")
	// 检查是否有商品使用此分类
	var count int
	db.PG.QueryRow(`SELECT COUNT(*) FROM products WHERE category_id = $1`, catID).Scan(&count)
	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": fmt.Sprintf("该分类下有 %d 个商品，无法删除", count)})
		return
	}
	db.PG.Exec(`DELETE FROM categories WHERE id = $1`, catID)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "删除成功"})
}

// ========== 评价管理 ==========

func AdminGetReviews(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	keyword := c.Query("keyword")
	rating := c.Query("rating")         // "low" = 1-2星
	isTaster := c.Query("is_taster")    // "1" = 试吃官评价
	hasReply := c.Query("has_reply")    // "0" = 未回复, "1" = 已回复

	if page < 1 { page = 1 }
	offset := (page - 1) * pageSize

	where := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if keyword != "" {
		where += fmt.Sprintf(` AND (r.content ILIKE $%d OR p.name ILIKE $%d OR u.nickname ILIKE $%d)`, argIdx, argIdx, argIdx)
		args = append(args, "%"+keyword+"%")
		argIdx++
	}
	if rating == "low" {
		where += ` AND r.rating <= 2`
	} else if rating == "high" {
		where += ` AND r.rating >= 4`
	}
	if isTaster == "1" {
		where += ` AND r.is_taster_review = true`
	}
	if hasReply == "0" {
		where += ` AND (r.merchant_reply IS NULL OR r.merchant_reply = '')`
	} else if hasReply == "1" {
		where += ` AND r.merchant_reply IS NOT NULL AND r.merchant_reply != ''`
	}

	var total int
	countQ := fmt.Sprintf(`
		SELECT COUNT(*) FROM product_reviews r
		JOIN users u ON r.user_id = u.id
		JOIN products p ON r.product_id = p.id
		%s
	`, where)
	db.PG.QueryRow(countQ, args...).Scan(&total)

	query := fmt.Sprintf(`
		SELECT r.id, r.rating, r.content, r.images, r.is_taster_review, r.merchant_reply, r.created_at,
		       u.id as user_id, u.nickname, u.avatar,
		       p.id as product_id, p.name as product_name,
		       m.id as merchant_id, m.shop_name
		FROM product_reviews r
		JOIN users u ON r.user_id = u.id
		JOIN products p ON r.product_id = p.id
		JOIN merchants m ON p.merchant_id = m.id
		%s
		ORDER BY r.created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, pageSize, offset)

	rows, err := db.PG.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"list": []gin.H{}, "total": 0}})
		return
	}
	defer rows.Close()

	var reviews []gin.H
	for rows.Next() {
		var id, ratingVal, userID, productID, merchantID int
		var content, productName, shopName string
		var nickname, avatar, merchantReply sql.NullString
		var images []string
		var isTasterReview bool
		var createdAt time.Time

		rows.Scan(&id, &ratingVal, &content, pq.Array(&images), &isTasterReview, &merchantReply, &createdAt,
			&userID, &nickname, &avatar,
			&productID, &productName,
			&merchantID, &shopName)

		reviews = append(reviews, gin.H{
			"id": id, "rating": ratingVal, "content": content, "images": images,
			"is_taster_review": isTasterReview, "merchant_reply": merchantReply.String,
			"created_at": createdAt.Format("2006-01-02 15:04"),
			"user_id": userID, "nickname": nickname.String, "avatar": avatar.String,
			"product_id": productID, "product_name": productName,
			"merchant_id": merchantID, "shop_name": shopName,
		})
	}
	if reviews == nil { reviews = []gin.H{} }

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"list": reviews, "total": total, "page": page, "page_size": pageSize}})
}

func AdminDeleteReview(c *gin.Context) {
	reviewID := c.Param("id")
	_, err := db.PG.Exec(`DELETE FROM product_reviews WHERE id = $1`, reviewID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "删除失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "删除成功"})
}

// ========== 公益捐赠管理 ==========

func AdminGetDonations(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	keyword := c.Query("keyword")

	if page < 1 { page = 1 }
	offset := (page - 1) * pageSize

	where := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if keyword != "" {
		where += fmt.Sprintf(` AND (u.nickname ILIKE $%d OR u.phone ILIKE $%d OR fb.name ILIKE $%d)`, argIdx, argIdx, argIdx)
		args = append(args, "%"+keyword+"%")
		argIdx++
	}

	var total int
	db.PG.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM donations d JOIN users u ON d.user_id = u.id LEFT JOIN food_banks fb ON d.food_bank_id = fb.id %s`, where), args...).Scan(&total)

	// 汇总
	var totalAmount float64
	var totalDonors int
	db.PG.QueryRow(`SELECT COALESCE(SUM(amount),0), COUNT(DISTINCT user_id) FROM donations`).Scan(&totalAmount, &totalDonors)

	query := fmt.Sprintf(`
		SELECT d.id, d.amount, d.message, d.created_at,
		       u.id as user_id, u.nickname, u.phone,
		       fb.id as fb_id, fb.name as fb_name
		FROM donations d
		JOIN users u ON d.user_id = u.id
		LEFT JOIN food_banks fb ON d.food_bank_id = fb.id
		%s
		ORDER BY d.created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, pageSize, offset)

	rows, err := db.PG.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"list": []gin.H{}, "total": 0}})
		return
	}
	defer rows.Close()

	var list []gin.H
	for rows.Next() {
		var id, userID int
		var amount float64
		var msgN, nickname, phone, fbName sql.NullString
		var fbID sql.NullInt64
		var createdAt time.Time

		rows.Scan(&id, &amount, &msgN, &createdAt, &userID, &nickname, &phone, &fbID, &fbName)
		list = append(list, gin.H{
			"id": id, "amount": amount, "message": msgN.String,
			"created_at": createdAt.Format("2006-01-02 15:04"),
			"user_id": userID, "nickname": nickname.String, "phone": phone.String,
			"food_bank_id": fbID.Int64, "food_bank_name": fbName.String,
		})
	}
	if list == nil { list = []gin.H{} }

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"list": list, "total": total, "page": page, "page_size": pageSize,
		"total_amount": totalAmount, "total_donors": totalDonors,
	}})
}

// ========== 试吃官管理 ==========

func AdminGetTasters(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	keyword := c.Query("keyword")
	status := c.Query("status") // pending, approved, rejected

	if page < 1 { page = 1 }
	offset := (page - 1) * pageSize

	where := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if keyword != "" {
		where += fmt.Sprintf(` AND (t.real_name ILIKE $%d OR u.phone ILIKE $%d OR u.nickname ILIKE $%d)`, argIdx, argIdx, argIdx)
		args = append(args, "%"+keyword+"%")
		argIdx++
	}
	if status != "" {
		where += fmt.Sprintf(` AND t.status = $%d`, argIdx)
		args = append(args, status)
		argIdx++
	}

	var total int
	db.PG.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM tasters t JOIN users u ON t.user_id = u.id %s`, where), args...).Scan(&total)

	query := fmt.Sprintf(`
		SELECT t.id, t.real_name, t.reason, t.status, t.level,
		       t.total_tasks, t.completed_tasks, t.total_reviews, t.avg_rating,
		       t.approved_at, t.created_at,
		       u.id as user_id, u.nickname, u.phone, u.avatar, u.total_orders
		FROM tasters t
		JOIN users u ON t.user_id = u.id
		%s
		ORDER BY CASE t.status WHEN 'pending' THEN 0 WHEN 'approved' THEN 1 ELSE 2 END, t.created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, pageSize, offset)

	rows, err := db.PG.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"list": []gin.H{}, "total": 0}})
		return
	}
	defer rows.Close()

	var list []gin.H
	for rows.Next() {
		var id, totalTasks, completedTasks, totalReviews, userID, totalOrders int
		var realName, reason, tStatus, level string
		var nickname, phone, avatar sql.NullString
		var avgRating float64
		var approvedAt sql.NullTime
		var createdAt time.Time

		rows.Scan(&id, &realName, &reason, &tStatus, &level,
			&totalTasks, &completedTasks, &totalReviews, &avgRating,
			&approvedAt, &createdAt,
			&userID, &nickname, &phone, &avatar, &totalOrders)

		item := gin.H{
			"id": id, "real_name": realName, "reason": reason, "status": tStatus, "level": level,
			"total_tasks": totalTasks, "completed_tasks": completedTasks,
			"total_reviews": totalReviews, "avg_rating": avgRating,
			"created_at": createdAt.Format("2006-01-02 15:04"),
			"user_id": userID, "nickname": nickname.String, "phone": phone.String,
			"avatar": avatar.String, "total_orders": totalOrders,
		}
		if approvedAt.Valid { item["approved_at"] = approvedAt.Time.Format("2006-01-02 15:04") }
		list = append(list, item)
	}
	if list == nil { list = []gin.H{} }

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"list": list, "total": total, "page": page, "page_size": pageSize}})
}

func AdminApproveTaster(c *gin.Context) {
	tasterID := c.Param("id")
	var req struct {
		Approved bool `json:"approved"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误"})
		return
	}
	if req.Approved {
		db.PG.Exec(`UPDATE tasters SET status = 'approved', approved_at = NOW() WHERE id = $1`, tasterID)
	} else {
		db.PG.Exec(`UPDATE tasters SET status = 'rejected' WHERE id = $1`, tasterID)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "操作成功"})
}

// ========== 盲盒管理 ==========

func AdminGetBlindBoxes(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	keyword := c.Query("keyword")

	if page < 1 { page = 1 }
	offset := (page - 1) * pageSize

	where := "WHERE p.is_blind_box = true"
	args := []interface{}{}
	argIdx := 1

	if keyword != "" {
		where += fmt.Sprintf(` AND (p.name ILIKE $%d OR m.shop_name ILIKE $%d)`, argIdx, argIdx)
		args = append(args, "%"+keyword+"%")
		argIdx++
	}

	var total int
	db.PG.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM products p JOIN merchants m ON p.merchant_id = m.id %s`, where), args...).Scan(&total)

	query := fmt.Sprintf(`
		SELECT p.id, p.name, p.original_price, p.blind_box_min_value, p.blind_box_max_value,
		       p.stock, p.sold_count, p.expiry_date, p.status, p.created_at,
		       m.id as merchant_id, m.shop_name,
		       COALESCE(bp.purchase_count, 0) as purchase_count,
		       COALESCE(bp.opened_count, 0) as opened_count,
		       COALESCE(bp.total_revenue, 0) as total_revenue
		FROM products p
		JOIN merchants m ON p.merchant_id = m.id
		LEFT JOIN (
			SELECT product_id,
			       COUNT(*) as purchase_count,
			       COUNT(*) FILTER (WHERE status = 'opened') as opened_count,
			       SUM(price) as total_revenue
			FROM blind_box_purchases GROUP BY product_id
		) bp ON p.id = bp.product_id
		%s
		ORDER BY p.created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, pageSize, offset)

	rows, err := db.PG.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"list": []gin.H{}, "total": 0}})
		return
	}
	defer rows.Close()

	now := time.Now()
	var list []gin.H
	for rows.Next() {
		var id, stock, soldCount, mID, purchaseCount, openedCount int
		var pName, pStatus string
		var shopName sql.NullString
		var price, minVal, maxVal, totalRevenue float64
		var expiryDate, createdAt time.Time

		rows.Scan(&id, &pName, &price, &minVal, &maxVal,
			&stock, &soldCount, &expiryDate, &pStatus, &createdAt,
			&mID, &shopName, &purchaseCount, &openedCount, &totalRevenue)

		daysLeft := int(expiryDate.Sub(now).Hours()/24) + 1
		if daysLeft < 0 { daysLeft = 0 }

		openRate := 0.0
		if purchaseCount > 0 { openRate = float64(openedCount) / float64(purchaseCount) * 100 }

		list = append(list, gin.H{
			"id": id, "name": pName, "price": price,
			"min_value": minVal, "max_value": maxVal,
			"stock": stock, "sold_count": soldCount,
			"expiry_date": expiryDate.Format("2006-01-02"), "days_left": daysLeft,
			"status": pStatus, "created_at": createdAt.Format("2006-01-02 15:04"),
			"merchant_id": mID, "shop_name": shopName.String,
			"purchase_count": purchaseCount, "opened_count": openedCount,
			"open_rate": openRate, "total_revenue": totalRevenue,
		})
	}
	if list == nil { list = []gin.H{} }

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"list": list, "total": total, "page": page, "page_size": pageSize}})
}

// ========== 管理员账号管理 ==========

func AdminGetAdmins(c *gin.Context) {
	rows, err := db.PG.Query(`SELECT id, username, nickname, role, status, last_login_at, created_at FROM admins ORDER BY id`)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []gin.H{}})
		return
	}
	defer rows.Close()

	var list []gin.H
	for rows.Next() {
		var id int
		var username, nickname, role, status string
		var lastLogin sql.NullTime
		var createdAt time.Time
		rows.Scan(&id, &username, &nickname, &role, &status, &lastLogin, &createdAt)
		item := gin.H{
			"id": id, "username": username, "nickname": nickname,
			"role": role, "status": status,
			"created_at": createdAt.Format("2006-01-02 15:04"),
		}
		if lastLogin.Valid { item["last_login_at"] = lastLogin.Time.Format("2006-01-02 15:04") }
		list = append(list, item)
	}
	if list == nil { list = []gin.H{} }
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list})
}

func AdminCreateAdmin(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
		Nickname string `json:"nickname"`
		Role     string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请填写用户名和密码"})
		return
	}
	if req.Role == "" { req.Role = "admin" }
	if req.Nickname == "" { req.Nickname = req.Username }

	var id int
	err := db.PG.QueryRow(`INSERT INTO admins (username, password, nickname, role) VALUES ($1,$2,$3,$4) RETURNING id`,
		req.Username, req.Password, req.Nickname, req.Role).Scan(&id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "创建失败，用户名可能已存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "创建成功"})
}

func AdminUpdateAdmin(c *gin.Context) {
	adminID := c.Param("id")
	var req struct {
		Nickname *string `json:"nickname"`
		Password *string `json:"password"`
		Status   *string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误"})
		return
	}
	if req.Nickname != nil { db.PG.Exec(`UPDATE admins SET nickname=$1 WHERE id=$2`, *req.Nickname, adminID) }
	if req.Password != nil && *req.Password != "" { db.PG.Exec(`UPDATE admins SET password=$1 WHERE id=$2`, *req.Password, adminID) }
	if req.Status != nil { db.PG.Exec(`UPDATE admins SET status=$1 WHERE id=$2`, *req.Status, adminID) }
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "更新成功"})
}

// ========== 拼团管理 ==========

func AdminGetGroupBuys(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	keyword := c.Query("keyword")
	status := c.Query("status")

	if page < 1 { page = 1 }
	offset := (page - 1) * pageSize

	where := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if keyword != "" {
		where += fmt.Sprintf(` AND (p.name ILIKE $%d OR u.nickname ILIKE $%d)`, argIdx, argIdx)
		args = append(args, "%"+keyword+"%")
		argIdx++
	}
	if status != "" {
		where += fmt.Sprintf(` AND g.status = $%d`, argIdx)
		args = append(args, status)
		argIdx++
	}

	var total int
	db.PG.QueryRow(fmt.Sprintf(`
		SELECT COUNT(*) FROM group_buys g
		JOIN users u ON g.creator_id = u.id
		JOIN products p ON g.product_id = p.id %s
	`, where), args...).Scan(&total)

	query := fmt.Sprintf(`
		SELECT g.id, g.target_count, g.current_count, g.original_price, g.group_price,
		       g.invite_code, g.status, g.expire_time, g.created_at,
		       u.id as user_id, u.nickname as creator_name,
		       p.id as product_id, p.name as product_name,
		       m.shop_name
		FROM group_buys g
		JOIN users u ON g.creator_id = u.id
		JOIN products p ON g.product_id = p.id
		JOIN merchants m ON g.merchant_id = m.id
		%s
		ORDER BY g.created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, pageSize, offset)

	rows, err := db.PG.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"list": []gin.H{}, "total": 0}})
		return
	}
	defer rows.Close()

	var list []gin.H
	for rows.Next() {
		var id, targetCount, currentCount, userID, productID int
		var origPrice, groupPrice float64
		var inviteCode, gStatus, creatorName, productName, shopName string
		var expireTime, createdAt time.Time

		rows.Scan(&id, &targetCount, &currentCount, &origPrice, &groupPrice,
			&inviteCode, &gStatus, &expireTime, &createdAt,
			&userID, &creatorName, &productID, &productName, &shopName)

		remaining := int(expireTime.Sub(time.Now()).Seconds())
		if remaining < 0 { remaining = 0 }

		list = append(list, gin.H{
			"id": id, "target_count": targetCount, "current_count": currentCount,
			"original_price": origPrice, "group_price": groupPrice,
			"invite_code": inviteCode, "status": gStatus,
			"remaining_seconds": remaining,
			"expire_time": expireTime.Format("2006-01-02 15:04"),
			"created_at": createdAt.Format("2006-01-02 15:04"),
			"user_id": userID, "creator_name": creatorName,
			"product_id": productID, "product_name": productName,
			"shop_name": shopName,
		})
	}
	if list == nil { list = []gin.H{} }
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"list": list, "total": total, "page": page, "page_size": pageSize}})
}

// ========== 紧急清仓管理 ==========

func AdminGetUrgentEvents(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 { page = 1 }
	offset := (page - 1) * pageSize

	var total int
	db.PG.QueryRow(`SELECT COUNT(*) FROM urgent_events`).Scan(&total)

	rows, err := db.PG.Query(`
		SELECT e.id, e.title, e.description, e.extra_discount, e.total_slots,
		       e.total_slots - COALESCE(ep.joined, 0) as remaining_slots,
		       COALESCE(ep.joined, 0) as participant_count,
		       e.start_time, e.end_time, e.status, e.created_at,
		       m.id as merchant_id, m.shop_name,
		       p.id as product_id, p.name as product_name
		FROM urgent_events e
		JOIN merchants m ON e.merchant_id = m.id
		LEFT JOIN products p ON e.product_id = p.id
		LEFT JOIN (
			SELECT event_id, COUNT(*) as joined FROM event_participants GROUP BY event_id
		) ep ON e.id = ep.event_id
		ORDER BY e.created_at DESC
		LIMIT $1 OFFSET $2
	`, pageSize, offset)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"list": []gin.H{}, "total": 0}})
		return
	}
	defer rows.Close()

	var list []gin.H
	now := time.Now()
	for rows.Next() {
		var id, extraDiscount, totalSlots, remainingSlots, participantCount, mID int
		var title, eStatus string
		var desc, shopName sql.NullString
		var productID sql.NullInt64
		var productName sql.NullString
		var startTime, endTime, createdAt time.Time

		rows.Scan(&id, &title, &desc, &extraDiscount, &totalSlots,
			&remainingSlots, &participantCount,
			&startTime, &endTime, &eStatus, &createdAt,
			&mID, &shopName, &productID, &productName)

		remaining := int(endTime.Sub(now).Seconds())
		if remaining < 0 { remaining = 0 }

		// 自动判断状态
		displayStatus := eStatus
		if now.After(endTime) { displayStatus = "ended" }

		list = append(list, gin.H{
			"id": id, "title": title, "description": desc.String,
			"extra_discount": extraDiscount, "total_slots": totalSlots,
			"remaining_slots": remainingSlots, "participant_count": participantCount,
			"remaining_seconds": remaining,
			"start_time": startTime.Format("2006-01-02 15:04"),
			"end_time": endTime.Format("2006-01-02 15:04"),
			"status": displayStatus, "created_at": createdAt.Format("2006-01-02 15:04"),
			"merchant_id": mID, "shop_name": shopName.String,
			"product_id": productID.Int64, "product_name": productName.String,
		})
	}
	if list == nil { list = []gin.H{} }
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"list": list, "total": total, "page": page, "page_size": pageSize}})
}
