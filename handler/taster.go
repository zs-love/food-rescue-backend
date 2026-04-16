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

// 试吃官信息
type TasterInfo struct {
	ID             int     `json:"id"`
	UserID         int     `json:"user_id"`
	RealName       string  `json:"real_name"`
	Reason         string  `json:"reason"`
	Status         string  `json:"status"`
	Level          string  `json:"level"`
	TotalTasks     int     `json:"total_tasks"`
	CompletedTasks int     `json:"completed_tasks"`
	TotalReviews   int     `json:"total_reviews"`
	AvgRating      float64 `json:"avg_rating"`
	ApprovedAt     string  `json:"approved_at"`
	CreatedAt      string  `json:"created_at"`
	// 用户信息
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

// 试吃任务
type TastingTask struct {
	ID            int     `json:"id"`
	ProductID     int     `json:"product_id"`
	MerchantID    int     `json:"merchant_id"`
	Title         string  `json:"title"`
	Description   string  `json:"description"`
	Requirements  string  `json:"requirements"`
	TotalSlots    int     `json:"total_slots"`
	FilledSlots   int     `json:"filled_slots"`
	OriginalPrice float64 `json:"original_price"`
	TastingPrice  float64 `json:"tasting_price"`
	StartTime     string  `json:"start_time"`
	EndTime       string  `json:"end_time"`
	Status        string  `json:"status"`
	CreatedAt     string  `json:"created_at"`
	// 关联信息
	ProductName string `json:"product_name"`
	ProductImage string `json:"product_image"`
	ShopName    string `json:"shop_name"`
	// 用户申请状态
	ApplicationStatus string `json:"application_status,omitempty"`
	ApplicationID     int    `json:"application_id,omitempty"`
}

// ApplyTaster 申请成为试吃官
func ApplyTaster(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的用户ID"})
		return
	}

	var req struct {
		RealName string `json:"real_name" binding:"required"`
		Reason   string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请填写真实姓名"})
		return
	}

	// 检查是否已申请
	var existingStatus sql.NullString
	db.PG.QueryRow(`SELECT status FROM tasters WHERE user_id = $1`, userID).Scan(&existingStatus)
	if existingStatus.Valid {
		if existingStatus.String == "pending" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "您的申请正在审核中"})
			return
		}
		if existingStatus.String == "approved" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "您已是试吃官"})
			return
		}
		// rejected 状态允许重新申请，更新记录
		if existingStatus.String == "rejected" {
			_, err = db.PG.Exec(`
				UPDATE tasters SET real_name = $2, reason = $3, status = 'pending', approved_at = NULL
				WHERE user_id = $1
			`, userID, req.RealName, req.Reason)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "重新申请失败"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "message": "重新申请已提交，请等待审核"})
			return
		}
	}

	var tasterID int
	err = db.PG.QueryRow(`
		INSERT INTO tasters (user_id, real_name, reason)
		VALUES ($1, $2, $3)
		RETURNING id
	`, userID, req.RealName, req.Reason).Scan(&tasterID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "申请失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "申请已提交，请等待审核", "data": gin.H{"taster_id": tasterID}})
}

// GetTasterStatus 获取试吃官状态
func GetTasterStatus(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的用户ID"})
		return
	}

	var info TasterInfo
	var approvedAt sql.NullTime
	var reason sql.NullString
	var createdAt time.Time

	err = db.PG.QueryRow(`
		SELECT t.id, t.user_id, t.real_name, t.reason, t.status, t.level,
		       t.total_tasks, t.completed_tasks, t.total_reviews, t.avg_rating,
		       t.approved_at, t.created_at, u.nickname, u.avatar
		FROM tasters t
		JOIN users u ON t.user_id = u.id
		WHERE t.user_id = $1
	`, userID).Scan(
		&info.ID, &info.UserID, &info.RealName, &reason, &info.Status, &info.Level,
		&info.TotalTasks, &info.CompletedTasks, &info.TotalReviews, &info.AvgRating,
		&approvedAt, &createdAt, &info.Nickname, &info.Avatar,
	)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "获取状态失败"})
		return
	}

	info.Reason = reason.String
	info.CreatedAt = createdAt.Format("2006-01-02 15:04:05")
	if approvedAt.Valid {
		info.ApprovedAt = approvedAt.Time.Format("2006-01-02 15:04:05")
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": info})
}

// GetTastingTasks 获取试吃任务列表
func GetTastingTasks(c *gin.Context) {
	userID := c.Query("user_id")
	status := c.DefaultQuery("status", "active")

	query := `
		SELECT t.id, t.product_id, t.merchant_id, t.title, t.description, t.requirements,
		       t.total_slots, t.filled_slots, t.original_price, t.tasting_price,
		       t.start_time, t.end_time, t.status, t.created_at,
		       p.name as product_name, COALESCE(p.images[1], '') as product_image,
		       m.shop_name
		FROM tasting_tasks t
		JOIN products p ON t.product_id = p.id
		JOIN merchants m ON t.merchant_id = m.id
		WHERE t.status = $1
		ORDER BY t.created_at DESC
	`

	rows, err := db.PG.Query(query, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "获取任务失败"})
		return
	}
	defer rows.Close()

	var tasks []TastingTask
	for rows.Next() {
		var task TastingTask
		var startTime, endTime sql.NullTime
		var desc, req sql.NullString
		var taskCreatedAt time.Time

		err := rows.Scan(
			&task.ID, &task.ProductID, &task.MerchantID, &task.Title, &desc, &req,
			&task.TotalSlots, &task.FilledSlots, &task.OriginalPrice, &task.TastingPrice,
			&startTime, &endTime, &task.Status, &taskCreatedAt,
			&task.ProductName, &task.ProductImage, &task.ShopName,
		)
		if err != nil {
			continue
		}

		task.CreatedAt = taskCreatedAt.Format("2006-01-02 15:04:05")
		task.Description = desc.String
		task.Requirements = req.String
		if startTime.Valid {
			task.StartTime = startTime.Time.Format("2006-01-02 15:04")
		}
		if endTime.Valid {
			task.EndTime = endTime.Time.Format("2006-01-02 15:04")
		}

		// 如果有用户ID，查询申请状态
		if userID != "" {
			var appStatus sql.NullString
			var appID sql.NullInt64
			db.PG.QueryRow(`
				SELECT status, id FROM tasting_applications 
				WHERE task_id = $1 AND user_id = $2
			`, task.ID, userID).Scan(&appStatus, &appID)
			task.ApplicationStatus = appStatus.String
			task.ApplicationID = int(appID.Int64)
		}

		tasks = append(tasks, task)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": tasks})
}

// ApplyTastingTask 申请试吃任务
func ApplyTastingTask(c *gin.Context) {
	taskID, err := strconv.Atoi(c.Param("task_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的任务ID"})
		return
	}

	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的用户ID"})
		return
	}

	// 检查任务是否存在且有名额
	var totalSlots, filledSlots int
	var taskStatus string
	err = db.PG.QueryRow(`
		SELECT total_slots, filled_slots, status FROM tasting_tasks WHERE id = $1
	`, taskID).Scan(&totalSlots, &filledSlots, &taskStatus)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "任务不存在"})
		return
	}
	if taskStatus != "active" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "任务已结束"})
		return
	}
	if filledSlots >= totalSlots {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "名额已满"})
		return
	}

	// 检查是否已申请
	var exists bool
	db.PG.QueryRow(`SELECT EXISTS(SELECT 1 FROM tasting_applications WHERE task_id = $1 AND user_id = $2)`, taskID, userID).Scan(&exists)
	if exists {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "您已申请过此任务"})
		return
	}

	// 获取试吃官ID（如果是试吃官）
	var tasterID sql.NullInt64
	db.PG.QueryRow(`SELECT id FROM tasters WHERE user_id = $1 AND status = 'approved'`, userID).Scan(&tasterID)

	// 创建申请
	var appID int
	err = db.PG.QueryRow(`
		INSERT INTO tasting_applications (task_id, user_id, taster_id)
		VALUES ($1, $2, $3)
		RETURNING id
	`, taskID, userID, tasterID).Scan(&appID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "申请失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "申请已提交", "data": gin.H{"application_id": appID}})
}

// CreateTastingTask 商家创建试吃任务
func CreateTastingTask(c *gin.Context) {
	merchantID, err := strconv.Atoi(c.Param("merchant_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的商家ID"})
		return
	}

	var req struct {
		ProductID     int     `json:"product_id" binding:"required"`
		Title         string  `json:"title" binding:"required"`
		Description   string  `json:"description"`
		Requirements  string  `json:"requirements"`
		TotalSlots    int     `json:"total_slots"`
		OriginalPrice float64 `json:"original_price"`
		TastingPrice  float64 `json:"tasting_price"`
		StartTime     string  `json:"start_time"`
		EndTime       string  `json:"end_time"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误"})
		return
	}

	if req.TotalSlots <= 0 {
		req.TotalSlots = 5
	}

	var taskID int
	err = db.PG.QueryRow(`
		INSERT INTO tasting_tasks (
			product_id, merchant_id, title, description, requirements,
			total_slots, original_price, tasting_price, start_time, end_time
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, '')::TIMESTAMP, NULLIF($10, '')::TIMESTAMP)
		RETURNING id
	`, req.ProductID, merchantID, req.Title, req.Description, req.Requirements,
		req.TotalSlots, req.OriginalPrice, req.TastingPrice, req.StartTime, req.EndTime,
	).Scan(&taskID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "创建失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "创建成功", "data": gin.H{"task_id": taskID}})
}

// GetMerchantTastingTasks 获取商家的试吃任务
func GetMerchantTastingTasks(c *gin.Context) {
	merchantID, err := strconv.Atoi(c.Param("merchant_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的商家ID"})
		return
	}

	rows, err := db.PG.Query(`
		SELECT t.id, t.product_id, t.merchant_id, t.title, t.description, t.requirements,
		       t.total_slots, t.filled_slots, t.original_price, t.tasting_price,
		       t.start_time, t.end_time, t.status, t.created_at,
		       p.name as product_name, COALESCE(p.images[1], '') as product_image
		FROM tasting_tasks t
		JOIN products p ON t.product_id = p.id
		WHERE t.merchant_id = $1
		ORDER BY t.created_at DESC
	`, merchantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "获取任务失败"})
		return
	}
	defer rows.Close()

	var tasks []TastingTask
	for rows.Next() {
		var task TastingTask
		var startTime, endTime sql.NullTime
		var desc, req sql.NullString
		var mTaskCreatedAt time.Time

		err := rows.Scan(
			&task.ID, &task.ProductID, &task.MerchantID, &task.Title, &desc, &req,
			&task.TotalSlots, &task.FilledSlots, &task.OriginalPrice, &task.TastingPrice,
			&startTime, &endTime, &task.Status, &mTaskCreatedAt,
			&task.ProductName, &task.ProductImage,
		)
		if err != nil {
			continue
		}

		task.CreatedAt = mTaskCreatedAt.Format("2006-01-02 15:04:05")
		task.Description = desc.String
		task.Requirements = req.String
		if startTime.Valid {
			task.StartTime = startTime.Time.Format("2006-01-02 15:04")
		}
		if endTime.Valid {
			task.EndTime = endTime.Time.Format("2006-01-02 15:04")
		}

		tasks = append(tasks, task)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": tasks})
}

// ApproveTastingApplication 审核试吃申请
func ApproveTastingApplication(c *gin.Context) {
	appID, err := strconv.Atoi(c.Param("app_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的申请ID"})
		return
	}

	var req struct {
		Approved bool `json:"approved"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误"})
		return
	}

	status := "rejected"
	if req.Approved {
		status = "approved"
	}

	// 更新申请状态
	result, err := db.PG.Exec(`
		UPDATE tasting_applications 
		SET status = $1, approved_at = $2 
		WHERE id = $3 AND status = 'pending'
	`, status, time.Now(), appID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "操作失败"})
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "申请不存在或已处理"})
		return
	}

	// 如果通过，更新任务已填名额
	if req.Approved {
		var taskID int
		db.PG.QueryRow(`SELECT task_id FROM tasting_applications WHERE id = $1`, appID).Scan(&taskID)
		db.PG.Exec(`UPDATE tasting_tasks SET filled_slots = filled_slots + 1 WHERE id = $1`, taskID)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "操作成功"})
}

// GetTastingApplications 获取试吃申请列表
func GetTastingApplications(c *gin.Context) {
	taskID, err := strconv.Atoi(c.Param("task_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的任务ID"})
		return
	}

	rows, err := db.PG.Query(`
		SELECT a.id, a.task_id, a.user_id, a.status, a.created_at,
		       u.nickname, u.avatar, u.total_orders,
		       t.level, t.total_reviews, t.avg_rating
		FROM tasting_applications a
		JOIN users u ON a.user_id = u.id
		LEFT JOIN tasters t ON a.taster_id = t.id
		WHERE a.task_id = $1
		ORDER BY a.created_at DESC
	`, taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "获取申请失败"})
		return
	}
	defer rows.Close()

	type Application struct {
		ID           int     `json:"id"`
		TaskID       int     `json:"task_id"`
		UserID       int     `json:"user_id"`
		Status       string  `json:"status"`
		CreatedAt    string  `json:"created_at"`
		Nickname     string  `json:"nickname"`
		Avatar       string  `json:"avatar"`
		TotalOrders  int     `json:"total_orders"`
		IsTaster     bool    `json:"is_taster"`
		TasterLevel  string  `json:"taster_level"`
		TotalReviews int     `json:"total_reviews"`
		AvgRating    float64 `json:"avg_rating"`
	}

	var apps []Application
	for rows.Next() {
		var app Application
		var nickname, avatar sql.NullString
		var tasterLevel sql.NullString
		var totalReviews sql.NullInt64
		var avgRating sql.NullFloat64

		err := rows.Scan(
			&app.ID, &app.TaskID, &app.UserID, &app.Status, &app.CreatedAt,
			&nickname, &avatar, &app.TotalOrders,
			&tasterLevel, &totalReviews, &avgRating,
		)
		if err != nil {
			continue
		}

		app.Nickname = nickname.String
		app.Avatar = avatar.String
		app.IsTaster = tasterLevel.Valid
		app.TasterLevel = tasterLevel.String
		app.TotalReviews = int(totalReviews.Int64)
		app.AvgRating = avgRating.Float64

		apps = append(apps, app)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": apps})
}

// SubmitTastingReview 提交试吃评价
func SubmitTastingReview(c *gin.Context) {
	appID, err := strconv.Atoi(c.Param("app_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的申请ID"})
		return
	}

	var req struct {
		Rating  int      `json:"rating" binding:"required"`
		Content string   `json:"content"`
		Images  []string `json:"images"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请填写评分"})
		return
	}

	if req.Rating < 1 || req.Rating > 5 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "评分范围为1-5"})
		return
	}

	// 获取申请信息
	var productID, userID int
	var appStatus string
	err = db.PG.QueryRow(`
		SELECT ta.status, tt.product_id, ta.user_id
		FROM tasting_applications ta
		JOIN tasting_tasks tt ON ta.task_id = tt.id
		WHERE ta.id = $1
	`, appID).Scan(&appStatus, &productID, &userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "申请不存在"})
		return
	}
	if appStatus != "approved" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "申请未通过审核"})
		return
	}

	// 创建评价
	var reviewID int
	err = db.PG.QueryRow(`
		INSERT INTO product_reviews (product_id, user_id, rating, content, images, is_taster_review, tasting_application_id)
		VALUES ($1, $2, $3, $4, $5, true, $6)
		RETURNING id
	`, productID, userID, req.Rating, req.Content, pq.Array(req.Images), appID).Scan(&reviewID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "提交评价失败"})
		return
	}

	// 更新申请状态
	db.PG.Exec(`UPDATE tasting_applications SET status = 'completed', review_id = $1, completed_at = $2 WHERE id = $3`,
		reviewID, time.Now(), appID)

	// 更新试吃官统计
	db.PG.Exec(`
		UPDATE tasters SET 
			completed_tasks = completed_tasks + 1,
			total_reviews = total_reviews + 1
		WHERE user_id = $1
	`, userID)

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "评价已提交", "data": gin.H{"review_id": reviewID}})
}

// SubmitOrderReview 消费者对已完成订单提交评价
func SubmitOrderReview(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的用户ID"})
		return
	}
	orderID, err := strconv.Atoi(c.Param("order_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的订单ID"})
		return
	}

	var req struct {
		Rating  int      `json:"rating" binding:"required"`
		Content string   `json:"content"`
		Images  []string `json:"images"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请选择评分"})
		return
	}
	if req.Rating < 1 || req.Rating > 5 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "评分范围1-5"})
		return
	}

	// 验证订单属于该用户且已完成
	var orderStatus string
	var merchantID int
	err = db.PG.QueryRow(`SELECT status, merchant_id FROM orders WHERE id = $1 AND user_id = $2`, orderID, userID).Scan(&orderStatus, &merchantID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "订单不存在"})
		return
	}
	if orderStatus != "completed" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "只能评价已完成的订单"})
		return
	}

	// 检查是否已评价
	var existingReview int
	db.PG.QueryRow(`SELECT COUNT(*) FROM product_reviews WHERE order_id = $1`, orderID).Scan(&existingReview)
	if existingReview > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "该订单已评价"})
		return
	}

	// 检查是否是试吃官
	var isTaster bool
	db.PG.QueryRow(`SELECT EXISTS(SELECT 1 FROM tasters WHERE user_id = $1 AND status = 'approved')`, userID).Scan(&isTaster)

	// 获取订单商品
	rows, err := db.PG.Query(`SELECT product_id FROM order_items WHERE order_id = $1`, orderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "获取订单商品失败"})
		return
	}
	defer rows.Close()

	var reviewIDs []int
	for rows.Next() {
		var productID int
		rows.Scan(&productID)

		var reviewID int
		err = db.PG.QueryRow(`
			INSERT INTO product_reviews (product_id, user_id, order_id, rating, content, images, is_taster_review)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id
		`, productID, userID, orderID, req.Rating, req.Content, pq.Array(req.Images), isTaster).Scan(&reviewID)
		if err != nil {
			continue
		}
		reviewIDs = append(reviewIDs, reviewID)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "评价成功", "data": gin.H{"review_ids": reviewIDs}})
}

// GetProductReviews 获取商品评价
func GetProductReviews(c *gin.Context) {
	productID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的商品ID"})
		return
	}

	rows, err := db.PG.Query(`
		SELECT r.id, r.product_id, r.user_id, r.rating, r.content, r.images,
		       r.is_taster_review, r.merchant_reply, r.reply_time, r.created_at,
		       u.nickname, u.avatar, t.level as taster_level
		FROM product_reviews r
		JOIN users u ON r.user_id = u.id
		LEFT JOIN tasters t ON r.user_id = t.user_id AND t.status = 'approved'
		WHERE r.product_id = $1 AND r.status = 'visible'
		ORDER BY r.is_taster_review DESC, r.created_at DESC
	`, productID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "获取评价失败"})
		return
	}
	defer rows.Close()

	type Review struct {
		ID             int      `json:"id"`
		ProductID      int      `json:"product_id"`
		UserID         int      `json:"user_id"`
		Rating         int      `json:"rating"`
		Content        string   `json:"content"`
		Images         []string `json:"images"`
		IsTasterReview bool     `json:"is_taster_review"`
		MerchantReply  string   `json:"merchant_reply"`
		ReplyTime      string   `json:"reply_time"`
		CreatedAt      string   `json:"created_at"`
		Nickname       string   `json:"nickname"`
		Avatar         string   `json:"avatar"`
		TasterLevel    string   `json:"taster_level"`
	}

	var reviews []Review
	for rows.Next() {
		var r Review
		var content, reply sql.NullString
		var replyTime sql.NullTime
		var images pq.StringArray
		var nickname, avatar, tasterLevel sql.NullString

		err := rows.Scan(
			&r.ID, &r.ProductID, &r.UserID, &r.Rating, &content, &images,
			&r.IsTasterReview, &reply, &replyTime, &r.CreatedAt,
			&nickname, &avatar, &tasterLevel,
		)
		if err != nil {
			continue
		}

		r.Content = content.String
		r.Images = images
		r.MerchantReply = reply.String
		r.Nickname = nickname.String
		r.Avatar = avatar.String
		r.TasterLevel = tasterLevel.String
		if replyTime.Valid {
			r.ReplyTime = replyTime.Time.Format("2006-01-02 15:04")
		}

		reviews = append(reviews, r)
	}

	// 获取评价统计
	var totalCount int
	var avgRating float64
	db.PG.QueryRow(`
		SELECT COUNT(*), COALESCE(AVG(rating), 0) 
		FROM product_reviews WHERE product_id = $1 AND status = 'visible'
	`, productID).Scan(&totalCount, &avgRating)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"reviews":     reviews,
			"total_count": totalCount,
			"avg_rating":  avgRating,
		},
	})
}


// GetMerchantReviews 获取商家所有商品的评价
func GetMerchantReviews(c *gin.Context) {
	merchantID, err := strconv.Atoi(c.Param("merchant_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的商家ID"})
		return
	}

	rows, err := db.PG.Query(`
		SELECT r.id, r.product_id, r.user_id, r.rating, r.content, r.images,
		       r.is_taster_review, r.merchant_reply, r.reply_time, r.created_at,
		       u.nickname, u.avatar, t.level as taster_level,
		       p.name as product_name
		FROM product_reviews r
		JOIN users u ON r.user_id = u.id
		JOIN products p ON r.product_id = p.id
		LEFT JOIN tasters t ON r.user_id = t.user_id AND t.status = 'approved'
		WHERE p.merchant_id = $1 AND r.status = 'visible'
		ORDER BY r.created_at DESC
	`, merchantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "获取评价失败"})
		return
	}
	defer rows.Close()

	type MerchantReview struct {
		ID             int      `json:"id"`
		ProductID      int      `json:"product_id"`
		UserID         int      `json:"user_id"`
		Rating         int      `json:"rating"`
		Content        string   `json:"content"`
		Images         []string `json:"images"`
		IsTasterReview bool     `json:"is_taster_review"`
		MerchantReply  string   `json:"merchant_reply"`
		ReplyTime      string   `json:"reply_time"`
		CreatedAt      string   `json:"created_at"`
		Nickname       string   `json:"nickname"`
		Avatar         string   `json:"avatar"`
		TasterLevel    string   `json:"taster_level"`
		ProductName    string   `json:"product_name"`
	}

	var reviews []MerchantReview
	for rows.Next() {
		var r MerchantReview
		var content, reply sql.NullString
		var replyTime sql.NullTime
		var images pq.StringArray
		var nickname, avatar, tasterLevel sql.NullString

		err := rows.Scan(
			&r.ID, &r.ProductID, &r.UserID, &r.Rating, &content, &images,
			&r.IsTasterReview, &reply, &replyTime, &r.CreatedAt,
			&nickname, &avatar, &tasterLevel, &r.ProductName,
		)
		if err != nil {
			continue
		}

		r.Content = content.String
		r.Images = images
		r.MerchantReply = reply.String
		r.Nickname = nickname.String
		r.Avatar = avatar.String
		r.TasterLevel = tasterLevel.String
		if replyTime.Valid {
			r.ReplyTime = replyTime.Time.Format("2006-01-02 15:04")
		}

		reviews = append(reviews, r)
	}

	// 统计
	var totalCount int
	var avgRating float64
	var goodCount int
	db.PG.QueryRow(`
		SELECT COUNT(*), COALESCE(AVG(rating), 0)
		FROM product_reviews r
		JOIN products p ON r.product_id = p.id
		WHERE p.merchant_id = $1 AND r.status = 'visible'
	`, merchantID).Scan(&totalCount, &avgRating)

	db.PG.QueryRow(`
		SELECT COUNT(*)
		FROM product_reviews r
		JOIN products p ON r.product_id = p.id
		WHERE p.merchant_id = $1 AND r.status = 'visible' AND r.rating >= 4
	`, merchantID).Scan(&goodCount)

	goodRate := 0
	if totalCount > 0 {
		goodRate = goodCount * 100 / totalCount
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"reviews":     reviews,
			"total_count": totalCount,
			"avg_rating":  avgRating,
			"good_rate":   goodRate,
		},
	})
}

// ReplyReview 商家回复评价
func ReplyReview(c *gin.Context) {
	reviewID, err := strconv.Atoi(c.Param("review_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的评价ID"})
		return
	}

	var req struct {
		Reply string `json:"reply" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请填写回复内容"})
		return
	}

	result, err := db.PG.Exec(`
		UPDATE product_reviews SET merchant_reply = $1, reply_time = NOW()
		WHERE id = $2
	`, req.Reply, reviewID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "回复失败"})
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "评价不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "回复成功"})
}
