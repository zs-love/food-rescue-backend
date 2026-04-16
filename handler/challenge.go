package handler

import (
	"database/sql"
	"food-rescue/db"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// 任务类型
const (
	TaskTypeDaily   = "daily"
	TaskTypeWeekly  = "weekly"
	TaskTypeSpecial = "special"
)

// 任务目标类型
const (
	GoalTypeSaveWeight = "save_weight"  // 拯救重量
	GoalTypeSaveMoney  = "save_money"   // 节省金额
	GoalTypeOrderCount = "order_count"  // 下单次数
	GoalTypeCategoryBuy = "category_buy" // 购买指定分类
)

// 每日任务模板
var dailyTaskTemplates = []map[string]interface{}{
	{"title": "今日拯救 1kg 食物", "goal_type": GoalTypeSaveWeight, "goal_value": 1.0, "reward_points": 10, "reward_desc": "10积分"},
	{"title": "今日拯救 2kg 食物", "goal_type": GoalTypeSaveWeight, "goal_value": 2.0, "reward_points": 25, "reward_desc": "25积分"},
	{"title": "今日节省 ¥10", "goal_type": GoalTypeSaveMoney, "goal_value": 10.0, "reward_points": 15, "reward_desc": "15积分"},
	{"title": "今日节省 ¥20", "goal_type": GoalTypeSaveMoney, "goal_value": 20.0, "reward_points": 30, "reward_desc": "30积分"},
	{"title": "今日下单 1 次", "goal_type": GoalTypeOrderCount, "goal_value": 1.0, "reward_points": 5, "reward_desc": "5积分"},
	{"title": "今日下单 2 次", "goal_type": GoalTypeOrderCount, "goal_value": 2.0, "reward_points": 15, "reward_desc": "15积分"},
}

// 获取用户挑战数据
func GetChallengeData(c *gin.Context) {
	userID := c.Param("user_id")

	// 获取用户打卡信息
	var checkinStreak int
	var lastCheckin sql.NullTime
	var totalCheckins int
	var totalPoints int

	err := db.PG.QueryRow(`
		SELECT COALESCE(checkin_streak, 0), last_checkin_date, 
		       COALESCE(total_checkins, 0), COALESCE(challenge_points, 0)
		FROM users WHERE id = $1
	`, userID).Scan(&checkinStreak, &lastCheckin, &totalCheckins, &totalPoints)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "用户不存在"})
		return
	}

	// 检查今日是否已打卡
	todayChecked := false
	if lastCheckin.Valid {
		todayChecked = lastCheckin.Time.Format("2006-01-02") == time.Now().Format("2006-01-02")
	}

	// 获取今日任务
	tasks := getTodayTasks(userID)

	// 获取活跃的紧急事件
	events := getActiveEvents()

	// 计算连续打卡奖励
	streakBonus := calculateStreakBonus(checkinStreak)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"checkin_streak":  checkinStreak,
			"today_checked":   todayChecked,
			"total_checkins":  totalCheckins,
			"total_points":    totalPoints,
			"streak_bonus":    streakBonus,
			"tasks":           tasks,
			"events":          events,
		},
	})
}

// 每日打卡
func DailyCheckin(c *gin.Context) {
	userID := c.Param("user_id")

	// 检查今日是否已打卡
	var lastCheckin sql.NullTime
	var checkinStreak int
	err := db.PG.QueryRow(`
		SELECT last_checkin_date, COALESCE(checkin_streak, 0) FROM users WHERE id = $1
	`, userID).Scan(&lastCheckin, &checkinStreak)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "用户不存在"})
		return
	}

	today := time.Now().Format("2006-01-02")
	if lastCheckin.Valid && lastCheckin.Time.Format("2006-01-02") == today {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "今日已打卡"})
		return
	}

	// 计算新的连续天数
	newStreak := 1
	if lastCheckin.Valid {
		yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		if lastCheckin.Time.Format("2006-01-02") == yesterday {
			newStreak = checkinStreak + 1
		}
	}

	// 计算打卡奖励积分
	basePoints := 5
	streakBonus := calculateStreakBonus(newStreak)
	totalReward := basePoints + streakBonus

	// 更新用户数据
	_, err = db.PG.Exec(`
		UPDATE users SET 
			last_checkin_date = CURRENT_DATE,
			checkin_streak = $2,
			total_checkins = COALESCE(total_checkins, 0) + 1,
			challenge_points = COALESCE(challenge_points, 0) + $3
		WHERE id = $1
	`, userID, newStreak, totalReward)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "打卡失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "打卡成功",
		"data": gin.H{
			"streak":       newStreak,
			"base_points":  basePoints,
			"streak_bonus": streakBonus,
			"total_reward": totalReward,
		},
	})
}

// 领取任务奖励
func ClaimTaskReward(c *gin.Context) {
	userID := c.Param("user_id")
	taskID := c.Param("task_id")

	// 检查任务是否完成
	var status string
	var rewardPoints int
	err := db.PG.QueryRow(`
		SELECT status, reward_points FROM user_tasks 
		WHERE id = $1 AND user_id = $2
	`, taskID, userID).Scan(&status, &rewardPoints)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "任务不存在"})
		return
	}

	if status != "completed" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "任务未完成"})
		return
	}

	// 更新任务状态并发放奖励
	tx, err := db.PG.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "系统错误"})
		return
	}
	defer tx.Rollback()

	_, err = tx.Exec(`UPDATE user_tasks SET status = 'claimed' WHERE id = $1`, taskID)
	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "领取失败"})
		return
	}

	_, err = tx.Exec(`
		UPDATE users SET challenge_points = COALESCE(challenge_points, 0) + $2 WHERE id = $1
	`, userID, rewardPoints)
	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "领取失败"})
		return
	}

	tx.Commit()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "领取成功",
		"data": gin.H{
			"points": rewardPoints,
		},
	})
}

// 获取紧急清仓事件
func GetUrgentEvents(c *gin.Context) {
	events := getActiveEvents()
	c.JSON(http.StatusOK, gin.H{"success": true, "data": events})
}

// 参与紧急事件
func JoinUrgentEvent(c *gin.Context) {
	userID := c.Param("user_id")
	eventID := c.Param("event_id")

	// 检查事件是否有效
	var remainingSlots int
	var extraDiscount float64
	err := db.PG.QueryRow(`
		SELECT remaining_slots, extra_discount FROM urgent_events 
		WHERE id = $1 AND status = 'active' AND end_time > NOW()
	`, eventID).Scan(&remainingSlots, &extraDiscount)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "活动已结束或不存在"})
		return
	}

	if remainingSlots <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "名额已满"})
		return
	}

	// 检查用户是否已参与
	var exists bool
	db.PG.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM event_participants WHERE event_id = $1 AND user_id = $2)
	`, eventID, userID).Scan(&exists)

	if exists {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "已参与此活动"})
		return
	}

	// 加入活动
	tx, err := db.PG.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "系统错误"})
		return
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO event_participants (event_id, user_id) VALUES ($1, $2)
	`, eventID, userID)
	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "参与失败"})
		return
	}

	_, err = tx.Exec(`
		UPDATE urgent_events SET remaining_slots = remaining_slots - 1 WHERE id = $1
	`, eventID)
	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "参与失败"})
		return
	}

	tx.Commit()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "参与成功",
		"data": gin.H{
			"extra_discount": extraDiscount,
		},
	})
}

// 获取排行榜
func GetLeaderboard(c *gin.Context) {
	period := c.DefaultQuery("period", "weekly") // weekly, monthly, all
	limit := c.DefaultQuery("limit", "20")
	limitNum, _ := strconv.Atoi(limit)

	var dateFilter string
	switch period {
	case "weekly":
		dateFilter = "AND o.created_at >= CURRENT_DATE - INTERVAL '7 days'"
	case "monthly":
		dateFilter = "AND o.created_at >= CURRENT_DATE - INTERVAL '30 days'"
	default:
		dateFilter = ""
	}

	// 使用 discount_amount 字段计算节省金额
	// users表只存消费者，不需要role过滤
	query := `
		SELECT u.id, u.nickname, u.avatar, u.rank_level,
		       COALESCE(SUM(o.discount_amount), 0) as saved_money,
		       COUNT(o.id) as order_count
		FROM users u
		LEFT JOIN orders o ON u.id = o.user_id AND o.status IN ('completed', 'paid') ` + dateFilter + `
		WHERE u.status = 'active'
		GROUP BY u.id, u.nickname, u.avatar, u.rank_level
		HAVING COUNT(o.id) > 0
		ORDER BY saved_money DESC, order_count DESC
		LIMIT $1
	`

	rows, err := db.PG.Query(query, limitNum)
	if err != nil {
		// 返回空数组而不是错误，避免前端崩溃
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []gin.H{}})
		return
	}
	defer rows.Close()

	var leaderboard []gin.H
	rank := 1
	for rows.Next() {
		var id int
		var nickname, avatar, rankLevel sql.NullString
		var savedMoney float64
		var orderCount int

		if err := rows.Scan(&id, &nickname, &avatar, &rankLevel, &savedMoney, &orderCount); err != nil {
			continue
		}

		leaderboard = append(leaderboard, gin.H{
			"rank":        rank,
			"user_id":     id,
			"nickname":    nickname.String,
			"avatar":      avatar.String,
			"rank_level":  rankLevel.String,
			"saved_money": savedMoney,
			"order_count": orderCount,
		})
		rank++
	}

	// 确保返回空数组而不是 null
	if leaderboard == nil {
		leaderboard = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": leaderboard})
}

// 辅助函数

func getTodayTasks(userID string) []gin.H {
	today := time.Now().Format("2006-01-02")

	// 检查今日任务是否已生成
	var count int
	db.PG.QueryRow(`
		SELECT COUNT(*) FROM user_tasks 
		WHERE user_id = $1 AND DATE(created_at) = $2
	`, userID, today).Scan(&count)

	if count == 0 {
		// 生成今日任务（随机选择3个）
		generateDailyTasks(userID)
	}

	// 获取今日任务
	rows, err := db.PG.Query(`
		SELECT id, title, goal_type, goal_value, current_value, 
		       reward_points, reward_desc, status
		FROM user_tasks 
		WHERE user_id = $1 AND DATE(created_at) = $2
		ORDER BY id
	`, userID, today)

	if err != nil {
		return []gin.H{}
	}
	defer rows.Close()

	var tasks []gin.H
	for rows.Next() {
		var id int
		var title, goalType, rewardDesc, status string
		var goalValue, currentValue float64
		var rewardPoints int

		rows.Scan(&id, &title, &goalType, &goalValue, &currentValue, &rewardPoints, &rewardDesc, &status)

		progress := currentValue / goalValue
		if progress > 1 {
			progress = 1
		}

		tasks = append(tasks, gin.H{
			"id":            id,
			"title":         title,
			"goal_type":     goalType,
			"goal_value":    goalValue,
			"current_value": currentValue,
			"progress":      progress,
			"reward_points": rewardPoints,
			"reward_desc":   rewardDesc,
			"status":        status,
		})
	}

	return tasks
}

func generateDailyTasks(userID string) {
	// 随机选择3个任务模板
	rand.Seed(time.Now().UnixNano())
	indices := rand.Perm(len(dailyTaskTemplates))[:3]

	for _, idx := range indices {
		task := dailyTaskTemplates[idx]
		db.PG.Exec(`
			INSERT INTO user_tasks (user_id, task_type, title, goal_type, goal_value, 
			                        reward_points, reward_desc, status)
			VALUES ($1, 'daily', $2, $3, $4, $5, $6, 'active')
		`, userID, task["title"], task["goal_type"], task["goal_value"],
			task["reward_points"], task["reward_desc"])
	}
}

func getActiveEvents() []gin.H {
	rows, err := db.PG.Query(`
		SELECT e.id, e.title, e.description, e.merchant_id, e.product_id,
		       e.extra_discount, e.total_slots, e.remaining_slots,
		       e.start_time, e.end_time,
		       m.shop_name, p.name as product_name, p.original_price
		FROM urgent_events e
		JOIN merchants m ON e.merchant_id = m.id
		LEFT JOIN products p ON e.product_id = p.id
		WHERE e.status = 'active' AND e.end_time > NOW()
		ORDER BY e.end_time ASC
		LIMIT 5
	`)

	if err != nil {
		return []gin.H{}
	}
	defer rows.Close()

	var events []gin.H
	for rows.Next() {
		var id, merchantID int
		var productID sql.NullInt64
		var title, description, shopName string
		var productName sql.NullString
		var extraDiscount, originalPrice sql.NullFloat64
		var totalSlots, remainingSlots int
		var startTime, endTime time.Time

		rows.Scan(&id, &title, &description, &merchantID, &productID,
			&extraDiscount, &totalSlots, &remainingSlots,
			&startTime, &endTime, &shopName, &productName, &originalPrice)

		// 计算剩余时间（秒）
		remainingSeconds := int(time.Until(endTime).Seconds())
		if remainingSeconds < 0 {
			remainingSeconds = 0
		}

		events = append(events, gin.H{
			"id":                id,
			"title":             title,
			"description":       description,
			"merchant_id":       merchantID,
			"product_id":        productID.Int64,
			"shop_name":         shopName,
			"product_name":      productName.String,
			"original_price":    originalPrice.Float64,
			"extra_discount":    extraDiscount.Float64,
			"total_slots":       totalSlots,
			"remaining_slots":   remainingSlots,
			"remaining_seconds": remainingSeconds,
			"start_time":        startTime.Format("2006-01-02 15:04:05"),
			"end_time":          endTime.Format("2006-01-02 15:04:05"),
		})
	}

	return events
}

func calculateStreakBonus(streak int) int {
	// 连续打卡奖励：3天+5, 7天+10, 14天+20, 30天+50
	if streak >= 30 {
		return 50
	} else if streak >= 14 {
		return 20
	} else if streak >= 7 {
		return 10
	} else if streak >= 3 {
		return 5
	}
	return 0
}

// 商家创建紧急清仓事件
func CreateUrgentEvent(c *gin.Context) {
	merchantID, err := strconv.Atoi(c.Param("merchant_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的商家ID"})
		return
	}

	var req struct {
		ProductID     int     `json:"product_id"`
		Title         string  `json:"title" binding:"required"`
		Description   string  `json:"description"`
		ExtraDiscount float64 `json:"extra_discount"`
		TotalSlots    int     `json:"total_slots"`
		StartTime     string  `json:"start_time"`
		EndTime       string  `json:"end_time" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请填写必要信息"})
		return
	}

	if req.ExtraDiscount <= 0 {
		req.ExtraDiscount = 10
	}
	if req.TotalSlots <= 0 {
		req.TotalSlots = 10
	}
	if req.StartTime == "" {
		req.StartTime = time.Now().Format("2006-01-02 15:04:05")
	}

	var eventID int
	err = db.PG.QueryRow(`
		INSERT INTO urgent_events (merchant_id, product_id, title, description, extra_discount, total_slots, remaining_slots, start_time, end_time)
		VALUES ($1, NULLIF($2, 0), $3, $4, $5, $6, $6, $7::TIMESTAMP, $8::TIMESTAMP)
		RETURNING id
	`, merchantID, req.ProductID, req.Title, req.Description, req.ExtraDiscount, req.TotalSlots, req.StartTime, req.EndTime).Scan(&eventID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "创建失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "创建成功", "data": gin.H{"event_id": eventID}})
}

// 获取商家的紧急清仓事件
func GetMerchantUrgentEvents(c *gin.Context) {
	merchantID, err := strconv.Atoi(c.Param("merchant_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的商家ID"})
		return
	}

	rows, err := db.PG.Query(`
		SELECT e.id, e.title, e.description, e.product_id, e.extra_discount,
		       e.total_slots, e.remaining_slots, e.start_time, e.end_time, e.status,
		       e.created_at, COALESCE(p.name, '') as product_name
		FROM urgent_events e
		LEFT JOIN products p ON e.product_id = p.id
		WHERE e.merchant_id = $1
		ORDER BY e.created_at DESC
	`, merchantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "获取失败"})
		return
	}
	defer rows.Close()

	var events []gin.H
	for rows.Next() {
		var id int
		var title, description, status, createdAt, productName string
		var productID sql.NullInt64
		var extraDiscount float64
		var totalSlots, remainingSlots int
		var startTime, endTime time.Time

		err := rows.Scan(&id, &title, &description, &productID, &extraDiscount,
			&totalSlots, &remainingSlots, &startTime, &endTime, &status,
			&createdAt, &productName)
		if err != nil {
			continue
		}

		remainingSeconds := int(time.Until(endTime).Seconds())
		if remainingSeconds < 0 {
			remainingSeconds = 0
		}

		participantCount := totalSlots - remainingSlots

		events = append(events, gin.H{
			"id":                id,
			"title":             title,
			"description":       description,
			"product_id":        productID.Int64,
			"product_name":      productName,
			"extra_discount":    extraDiscount,
			"total_slots":       totalSlots,
			"remaining_slots":   remainingSlots,
			"participant_count": participantCount,
			"remaining_seconds": remainingSeconds,
			"start_time":        startTime.Format("2006-01-02 15:04"),
			"end_time":          endTime.Format("2006-01-02 15:04"),
			"status":            status,
			"created_at":        createdAt,
		})
	}

	if events == nil {
		events = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": events})
}

// 更新任务进度（订单完成时调用）
func UpdateTaskProgress(userID int, savedMoney float64, savedWeight float64) {
	today := time.Now().Format("2006-01-02")

	// 更新节省金额类任务
	db.PG.Exec(`
		UPDATE user_tasks SET 
			current_value = current_value + $3,
			status = CASE WHEN current_value + $3 >= goal_value THEN 'completed' ELSE status END
		WHERE user_id = $1 AND DATE(created_at) = $2 
		  AND goal_type = 'save_money' AND status = 'active'
	`, userID, today, savedMoney)

	// 更新拯救重量类任务
	db.PG.Exec(`
		UPDATE user_tasks SET 
			current_value = current_value + $3,
			status = CASE WHEN current_value + $3 >= goal_value THEN 'completed' ELSE status END
		WHERE user_id = $1 AND DATE(created_at) = $2 
		  AND goal_type = 'save_weight' AND status = 'active'
	`, userID, today, savedWeight)

	// 更新下单次数类任务
	db.PG.Exec(`
		UPDATE user_tasks SET 
			current_value = current_value + 1,
			status = CASE WHEN current_value + 1 >= goal_value THEN 'completed' ELSE status END
		WHERE user_id = $1 AND DATE(created_at) = $2 
		  AND goal_type = 'order_count' AND status = 'active'
	`, userID, today)
}
