package handler

import (
	"database/sql"
	"fmt"
	"food-rescue/db"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// ========== 拼救团 ==========

// 创建拼救团
func CreateGroupBuy(c *gin.Context) {
	userID := c.Param("user_id")
	var req struct {
		ProductID   int `json:"product_id"`
		TargetCount int `json:"target_count"` // 目标人数 2-5
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误"})
		return
	}

	if req.TargetCount < 2 || req.TargetCount > 5 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "拼团人数需在2-5人之间"})
		return
	}

	// 获取商品信息（含库存检查）
	var productName string
	var originalPrice, currentPrice float64
	var merchantID, stock int
	err := db.PG.QueryRow(`
		SELECT name, original_price, 
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
			merchant_id, stock
		FROM products WHERE id = $1 AND status = 'active'
	`, req.ProductID).Scan(&productName, &originalPrice, &currentPrice, &merchantID, &stock)
	if err != nil {
		log.Printf("CreateGroupBuy: 查询商品失败 product_id=%d, err=%v", req.ProductID, err)
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "商品不存在"})
		return
	}

	// 库存检查：需要足够 target_count 人购买
	if stock < req.TargetCount {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": fmt.Sprintf("库存不足，当前仅剩%d件", stock)})
		return
	}

	// 计算拼团价格（人数越多折扣越大）
	groupDiscount := 0.95 - float64(req.TargetCount-2)*0.05 // 2人95折，3人90折，4人85折，5人80折
	groupPrice := currentPrice * groupDiscount

	// 生成邀请码（带重试避免碰撞）
	var inviteCode string
	for i := 0; i < 5; i++ {
		inviteCode = fmt.Sprintf("%06d", rand.Intn(1000000))
		var exists bool
		db.PG.QueryRow(`SELECT EXISTS(SELECT 1 FROM group_buys WHERE invite_code = $1)`, inviteCode).Scan(&exists)
		if !exists {
			break
		}
	}

	// 创建拼团（original_price 存真正的原价，用于前端展示折扣对比）
	var groupID int
	expireTime := time.Now().Add(24 * time.Hour)
	err = db.PG.QueryRow(`
		INSERT INTO group_buys (creator_id, product_id, merchant_id, target_count, current_count,
			original_price, group_price, invite_code, expire_time)
		VALUES ($1, $2, $3, $4, 1, $5, $6, $7, $8)
		RETURNING id
	`, userID, req.ProductID, merchantID, req.TargetCount, originalPrice, groupPrice, inviteCode, expireTime).Scan(&groupID)
	if err != nil {
		log.Printf("CreateGroupBuy: 插入拼团失败 err=%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "创建拼团失败"})
		return
	}

	// 创建者自动加入
	db.PG.Exec(`INSERT INTO group_buy_members (group_id, user_id, is_creator) VALUES ($1, $2, true)`, groupID, userID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "拼团创建成功",
		"data": gin.H{
			"group_id":     groupID,
			"invite_code":  inviteCode,
			"target_count": req.TargetCount,
			"group_price":  groupPrice,
			"expire_time":  expireTime,
		},
	})
}

// 加入拼救团
func JoinGroupBuy(c *gin.Context) {
	userID := c.Param("user_id")
	var req struct {
		InviteCode string `json:"invite_code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误"})
		return
	}

	// 检查是否已加入（事务外快速检查，减少锁竞争）
	var groupID int
	err := db.PG.QueryRow(`SELECT id FROM group_buys WHERE invite_code = $1`, req.InviteCode).Scan(&groupID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "拼团不存在"})
		return
	}

	var alreadyJoined bool
	db.PG.QueryRow(`SELECT EXISTS(SELECT 1 FROM group_buy_members WHERE group_id=$1 AND user_id=$2)`,
		groupID, userID).Scan(&alreadyJoined)
	if alreadyJoined {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "已加入此拼团"})
		return
	}

	// 开启事务，使用 FOR UPDATE 锁定拼团行防止并发超员
	tx, err := db.PG.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "系统错误"})
		return
	}
	defer tx.Rollback()

	var targetCount, currentCount int
	var status string
	var expireTime time.Time
	var productID int
	err = tx.QueryRow(`
		SELECT id, target_count, current_count, status, expire_time, product_id
		FROM group_buys WHERE id = $1 FOR UPDATE
	`, groupID).Scan(&groupID, &targetCount, &currentCount, &status, &expireTime, &productID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "拼团不存在"})
		return
	}

	if status != "pending" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "拼团已结束"})
		return
	}
	if time.Now().After(expireTime) {
		tx.Exec(`UPDATE group_buys SET status = 'expired' WHERE id = $1`, groupID)
		tx.Commit()
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "拼团已过期"})
		return
	}
	if currentCount >= targetCount {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "拼团人数已满"})
		return
	}

	// 加入拼团
	_, err = tx.Exec(`INSERT INTO group_buy_members (group_id, user_id) VALUES ($1, $2)`, groupID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "加入失败"})
		return
	}
	_, err = tx.Exec(`UPDATE group_buys SET current_count = current_count + 1 WHERE id = $1`, groupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "更新拼团失败"})
		return
	}

	// 检查是否成团
	newCount := currentCount + 1
	if newCount >= targetCount {
		// 成团前检查库存
		var stock int
		tx.QueryRow(`SELECT stock FROM products WHERE id = $1 FOR UPDATE`, productID).Scan(&stock)
		if stock < targetCount {
			// 库存不足，无法成团，回滚
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "商品库存不足，无法成团"})
			return
		}

		tx.Exec(`UPDATE group_buys SET status = 'success' WHERE id = $1`, groupID)

		// 成团后为所有成员创建订单
		createGroupBuyOrders(tx, groupID)

		// 发送好友动态
		createFriendActivity(tx, groupID, "group_success")
	}

	if err = tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "操作失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "加入成功",
		"data": gin.H{
			"group_id":      groupID,
			"current_count": newCount,
			"target_count":  targetCount,
			"is_success":    newCount >= targetCount,
		},
	})
}

// 获取拼团详情
func GetGroupBuyDetail(c *gin.Context) {
	groupID := c.Param("group_id")

	var g struct {
		ID           int       `json:"id"`
		CreatorID    int       `json:"creator_id"`
		ProductID    int       `json:"product_id"`
		TargetCount  int       `json:"target_count"`
		CurrentCount int       `json:"current_count"`
		OriginalPrice float64  `json:"original_price"`
		GroupPrice   float64   `json:"group_price"`
		InviteCode   string    `json:"invite_code"`
		Status       string    `json:"status"`
		ExpireTime   time.Time `json:"expire_time"`
	}

	err := db.PG.QueryRow(`
		SELECT id, creator_id, product_id, target_count, current_count,
			original_price, group_price, invite_code, status, expire_time
		FROM group_buys WHERE id = $1
	`, groupID).Scan(&g.ID, &g.CreatorID, &g.ProductID, &g.TargetCount, &g.CurrentCount,
		&g.OriginalPrice, &g.GroupPrice, &g.InviteCode, &g.Status, &g.ExpireTime)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "拼团不存在"})
		return
	}

	// 获取商品信息
	var productName string
	var productImage sql.NullString
	db.PG.QueryRow(`SELECT name, images[1] FROM products WHERE id = $1`, g.ProductID).Scan(&productName, &productImage)

	// 获取成员列表
	rows, _ := db.PG.Query(`
		SELECT u.id, u.nickname, u.avatar, m.is_creator, m.joined_at
		FROM group_buy_members m
		JOIN users u ON m.user_id = u.id
		WHERE m.group_id = $1
		ORDER BY m.joined_at
	`, groupID)
	defer rows.Close()

	var members []gin.H
	for rows.Next() {
		var uid int
		var nickname, avatar sql.NullString
		var isCreator bool
		var joinedAt time.Time
		rows.Scan(&uid, &nickname, &avatar, &isCreator, &joinedAt)
		members = append(members, gin.H{
			"user_id":    uid,
			"nickname":   nickname.String,
			"avatar":     avatar.String,
			"is_creator": isCreator,
			"joined_at":  joinedAt,
		})
	}

	// 计算剩余时间
	remainingSeconds := int(time.Until(g.ExpireTime).Seconds())
	if remainingSeconds < 0 {
		remainingSeconds = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":                g.ID,
			"product_id":        g.ProductID,
			"product_name":      productName,
			"product_image":     productImage.String,
			"target_count":      g.TargetCount,
			"current_count":     g.CurrentCount,
			"original_price":    g.OriginalPrice,
			"group_price":       g.GroupPrice,
			"invite_code":       g.InviteCode,
			"status":            g.Status,
			"remaining_seconds": remainingSeconds,
			"members":           members,
		},
	})
}

// 获取用户的拼团列表
func GetUserGroupBuys(c *gin.Context) {
	userID := c.Param("user_id")
	status := c.DefaultQuery("status", "") // pending, success, expired

	query := `
		SELECT g.id, g.product_id, p.name, p.images[1], g.target_count, g.current_count,
			g.group_price, g.status, g.expire_time, m.is_creator
		FROM group_buys g
		JOIN group_buy_members m ON g.id = m.group_id
		JOIN products p ON g.product_id = p.id
		WHERE m.user_id = $1
	`
	args := []interface{}{userID}
	if status != "" {
		query += ` AND g.status = $2`
		args = append(args, status)
	}
	query += ` ORDER BY g.created_at DESC`

	rows, err := db.PG.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "查询失败"})
		return
	}
	defer rows.Close()

	var groups []gin.H
	for rows.Next() {
		var id, productID, targetCount, currentCount int
		var productName string
		var productImage sql.NullString
		var groupPrice float64
		var gStatus string
		var expireTime time.Time
		var isCreator bool

		rows.Scan(&id, &productID, &productName, &productImage, &targetCount, &currentCount,
			&groupPrice, &gStatus, &expireTime, &isCreator)

		remainingSeconds := int(time.Until(expireTime).Seconds())
		if remainingSeconds < 0 {
			remainingSeconds = 0
		}

		groups = append(groups, gin.H{
			"id":                id,
			"product_id":        productID,
			"product_name":      productName,
			"product_image":     productImage.String,
			"target_count":      targetCount,
			"current_count":     currentCount,
			"group_price":       groupPrice,
			"status":            gStatus,
			"remaining_seconds": remainingSeconds,
			"is_creator":        isCreator,
		})
	}

	if groups == nil {
		groups = []gin.H{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": groups})
}

// ========== 小队系统 ==========

// 创建小队
func CreateTeam(c *gin.Context) {
	userID := c.Param("user_id")
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请输入小队名称"})
		return
	}

	// 检查用户是否已有小队
	var existingTeam int
	db.PG.QueryRow(`SELECT team_id FROM team_members WHERE user_id = $1`, userID).Scan(&existingTeam)
	if existingTeam > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "您已加入其他小队"})
		return
	}

	// 生成邀请码
	inviteCode := fmt.Sprintf("T%05d", rand.Intn(100000))

	var teamID int
	err := db.PG.QueryRow(`
		INSERT INTO teams (name, captain_id, invite_code) VALUES ($1, $2, $3) RETURNING id
	`, req.Name, userID, inviteCode).Scan(&teamID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "创建失败"})
		return
	}

	// 队长自动加入
	db.PG.Exec(`INSERT INTO team_members (team_id, user_id, is_captain) VALUES ($1, $2, true)`, teamID, userID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "小队创建成功",
		"data": gin.H{
			"team_id":     teamID,
			"name":        req.Name,
			"invite_code": inviteCode,
		},
	})
}

// 加入小队
func JoinTeam(c *gin.Context) {
	userID := c.Param("user_id")
	var req struct {
		InviteCode string `json:"invite_code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误"})
		return
	}

	// 检查用户是否已有小队
	var existingTeam int
	db.PG.QueryRow(`SELECT team_id FROM team_members WHERE user_id = $1`, userID).Scan(&existingTeam)
	if existingTeam > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "您已加入其他小队"})
		return
	}

	// 查找小队
	var teamID, memberCount int
	err := db.PG.QueryRow(`SELECT id, member_count FROM teams WHERE invite_code = $1`, req.InviteCode).Scan(&teamID, &memberCount)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "小队不存在"})
		return
	}
	if memberCount >= 5 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "小队人数已满"})
		return
	}

	// 加入小队
	db.PG.Exec(`INSERT INTO team_members (team_id, user_id) VALUES ($1, $2)`, teamID, userID)
	db.PG.Exec(`UPDATE teams SET member_count = member_count + 1 WHERE id = $1`, teamID)

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "加入成功"})
}

// 退出小队
func LeaveTeam(c *gin.Context) {
	userID := c.Param("user_id")

	// 查找用户所在小队
	var teamID int
	var isCaptain bool
	err := db.PG.QueryRow(`SELECT team_id, is_captain FROM team_members WHERE user_id = $1`, userID).Scan(&teamID, &isCaptain)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "您未加入任何小队"})
		return
	}

	// 队长不能直接退出（需要先转让或解散）
	if isCaptain {
		// 检查是否只剩队长一人
		var memberCount int
		db.PG.QueryRow(`SELECT member_count FROM teams WHERE id = $1`, teamID).Scan(&memberCount)
		if memberCount > 1 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "队长请先转让队长后再退出"})
			return
		}
		// 只剩一人，解散小队
		db.PG.Exec(`DELETE FROM team_members WHERE team_id = $1`, teamID)
		db.PG.Exec(`DELETE FROM teams WHERE id = $1`, teamID)
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "小队已解散"})
		return
	}

	// 普通成员退出
	db.PG.Exec(`DELETE FROM team_members WHERE team_id = $1 AND user_id = $2`, teamID, userID)
	db.PG.Exec(`UPDATE teams SET member_count = member_count - 1 WHERE id = $1`, teamID)

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已退出小队"})
}


// 获取小队详情
func GetTeamDetail(c *gin.Context) {
	teamID := c.Param("team_id")

	var name, inviteCode string
	var captainID, memberCount int
	var totalSaved float64
	var weeklyRank int
	var createdAt time.Time

	err := db.PG.QueryRow(`
		SELECT name, captain_id, invite_code, member_count, total_saved, weekly_rank, created_at
		FROM teams WHERE id = $1
	`, teamID).Scan(&name, &captainID, &inviteCode, &memberCount, &totalSaved, &weeklyRank, &createdAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "小队不存在"})
		return
	}

	// 获取成员列表
	rows, _ := db.PG.Query(`
		SELECT u.id, u.nickname, u.avatar, m.is_captain, m.contribution, m.joined_at
		FROM team_members m
		JOIN users u ON m.user_id = u.id
		WHERE m.team_id = $1
		ORDER BY m.contribution DESC
	`, teamID)
	defer rows.Close()

	var members []gin.H
	for rows.Next() {
		var uid int
		var nickname, avatar sql.NullString
		var isCaptain bool
		var contribution float64
		var joinedAt time.Time
		rows.Scan(&uid, &nickname, &avatar, &isCaptain, &contribution, &joinedAt)
		members = append(members, gin.H{
			"user_id":      uid,
			"nickname":     nickname.String,
			"avatar":       avatar.String,
			"is_captain":   isCaptain,
			"contribution": contribution,
			"joined_at":    joinedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":           teamID,
			"name":         name,
			"captain_id":   captainID,
			"invite_code":  inviteCode,
			"member_count": memberCount,
			"total_saved":  totalSaved,
			"weekly_rank":  weeklyRank,
			"members":      members,
			"created_at":   createdAt,
		},
	})
}

// 获取用户的小队
func GetUserTeam(c *gin.Context) {
	userID := c.Param("user_id")

	var teamID int
	err := db.PG.QueryRow(`SELECT team_id FROM team_members WHERE user_id = $1`, userID).Scan(&teamID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
		return
	}

	// 直接查询小队详情
	var name, inviteCode string
	var captainID, memberCount int
	var totalSaved, weeklySaved float64
	var weeklyRank int

	err = db.PG.QueryRow(`
		SELECT name, captain_id, invite_code, member_count, total_saved, weekly_saved, weekly_rank
		FROM teams WHERE id = $1
	`, teamID).Scan(&name, &captainID, &inviteCode, &memberCount, &totalSaved, &weeklySaved, &weeklyRank)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
		return
	}

	// 获取成员列表
	rows, _ := db.PG.Query(`
		SELECT u.id, u.nickname, u.avatar, m.is_captain, m.contribution
		FROM team_members m
		JOIN users u ON m.user_id = u.id
		WHERE m.team_id = $1
		ORDER BY m.contribution DESC
	`, teamID)
	defer rows.Close()

	var members []gin.H
	for rows.Next() {
		var uid int
		var nickname, avatar sql.NullString
		var isCaptain bool
		var contribution float64
		rows.Scan(&uid, &nickname, &avatar, &isCaptain, &contribution)
		members = append(members, gin.H{
			"user_id":      uid,
			"nickname":     nickname.String,
			"avatar":       avatar.String,
			"is_captain":   isCaptain,
			"contribution": contribution,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":           teamID,
			"name":         name,
			"captain_id":   captainID,
			"invite_code":  inviteCode,
			"member_count": memberCount,
			"total_saved":  totalSaved,
			"weekly_saved": weeklySaved,
			"weekly_rank":  weeklyRank,
			"members":      members,
		},
	})
}

// 小队排行榜
func GetTeamLeaderboard(c *gin.Context) {
	period := c.DefaultQuery("period", "weekly")
	limit := c.DefaultQuery("limit", "20")
	limitNum, _ := strconv.Atoi(limit)

	var orderBy string
	if period == "weekly" {
		orderBy = "weekly_saved DESC"
	} else {
		orderBy = "total_saved DESC"
	}

	rows, err := db.PG.Query(fmt.Sprintf(`
		SELECT id, name, member_count, total_saved, weekly_saved, weekly_rank
		FROM teams
		ORDER BY %s
		LIMIT $1
	`, orderBy), limitNum)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []gin.H{}})
		return
	}
	defer rows.Close()

	var teams []gin.H
	rank := 1
	for rows.Next() {
		var id, memberCount, weeklyRank int
		var name string
		var totalSaved, weeklySaved float64
		rows.Scan(&id, &name, &memberCount, &totalSaved, &weeklySaved, &weeklyRank)
		teams = append(teams, gin.H{
			"rank":         rank,
			"id":           id,
			"name":         name,
			"member_count": memberCount,
			"total_saved":  totalSaved,
			"weekly_saved": weeklySaved,
		})
		rank++
	}

	if teams == nil {
		teams = []gin.H{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": teams})
}

// ========== 好友动态 ==========

// 获取好友动态
func GetFriendActivities(c *gin.Context) {
	userID := c.Param("user_id")
	limit := c.DefaultQuery("limit", "20")
	limitNum, _ := strconv.Atoi(limit)

	// 获取好友动态（包括自己小队成员和关注的人）
	rows, err := db.PG.Query(`
		SELECT a.id, a.user_id, u.nickname, u.avatar, a.activity_type, 
			a.product_id, p.name as product_name, p.images[1] as product_image,
			a.discount, a.saved_money, a.message, a.created_at
		FROM friend_activities a
		JOIN users u ON a.user_id = u.id
		LEFT JOIN products p ON a.product_id = p.id
		WHERE a.user_id IN (
			SELECT user_id FROM team_members WHERE team_id = (
				SELECT team_id FROM team_members WHERE user_id = $1
			)
		) OR a.user_id = $1
		ORDER BY a.created_at DESC
		LIMIT $2
	`, userID, limitNum)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []gin.H{}})
		return
	}
	defer rows.Close()

	var activities []gin.H
	for rows.Next() {
		var id, uid int
		var productID sql.NullInt64
		var nickname, avatar, activityType, message string
		var productName, productImage sql.NullString
		var discount, savedMoney sql.NullFloat64
		var createdAt time.Time

		rows.Scan(&id, &uid, &nickname, &avatar, &activityType,
			&productID, &productName, &productImage, &discount, &savedMoney, &message, &createdAt)

		activities = append(activities, gin.H{
			"id":            id,
			"user_id":       uid,
			"nickname":      nickname,
			"avatar":        avatar,
			"activity_type": activityType,
			"product_id":    productID.Int64,
			"product_name":  productName.String,
			"product_image": productImage.String,
			"discount":      discount.Float64,
			"saved_money":   savedMoney.Float64,
			"message":       message,
			"created_at":    createdAt,
			"time_ago":      formatTimeAgo(createdAt),
		})
	}

	if activities == nil {
		activities = []gin.H{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": activities})
}

// 格式化时间
func formatTimeAgo(t time.Time) string {
	diff := time.Since(t)
	if diff < time.Minute {
		return "刚刚"
	} else if diff < time.Hour {
		return fmt.Sprintf("%d分钟前", int(diff.Minutes()))
	} else if diff < 24*time.Hour {
		return fmt.Sprintf("%d小时前", int(diff.Hours()))
	} else if diff < 7*24*time.Hour {
		return fmt.Sprintf("%d天前", int(diff.Hours()/24))
	}
	return t.Format("01-02")
}

// 创建好友动态（内部调用）
func createFriendActivity(tx *sql.Tx, relatedID int, activityType string) {
	// 根据类型创建不同动态
	switch activityType {
	case "group_success":
		// 拼团成功
		tx.Exec(`
			INSERT INTO friend_activities (user_id, activity_type, product_id, message)
			SELECT m.user_id, 'group_success', g.product_id, '拼团成功！'
			FROM group_buy_members m
			JOIN group_buys g ON m.group_id = g.id
			WHERE g.id = $1
		`, relatedID)
	}
}

// 成团后为所有成员创建订单（内部调用）
func createGroupBuyOrders(tx *sql.Tx, groupID int) {
	// 获取拼团信息
	var productID, merchantID int
	var groupPrice, originalPrice float64
	var productName string
	err := tx.QueryRow(`
		SELECT g.product_id, g.merchant_id, g.group_price, g.original_price, p.name
		FROM group_buys g
		JOIN products p ON g.product_id = p.id
		WHERE g.id = $1
	`, groupID).Scan(&productID, &merchantID, &groupPrice, &originalPrice, &productName)
	if err != nil {
		log.Printf("createGroupBuyOrders: 获取拼团信息失败 group_id=%d, err=%v", groupID, err)
		return
	}

	// 获取所有成员
	rows, err := tx.Query(`SELECT user_id FROM group_buy_members WHERE group_id = $1`, groupID)
	if err != nil {
		log.Printf("createGroupBuyOrders: 获取成员失败 group_id=%d, err=%v", groupID, err)
		return
	}
	defer rows.Close()

	var memberIDs []int
	for rows.Next() {
		var uid int
		rows.Scan(&uid)
		memberIDs = append(memberIDs, uid)
	}

	// 为每个成员创建订单
	for _, uid := range memberIDs {
		orderNo := time.Now().Format("20060102150405") + fmt.Sprintf("%d%03d", uid, rand.Intn(1000))
		pickupCode := fmt.Sprintf("%06d", rand.Intn(1000000))
		discountAmount := originalPrice - groupPrice
		if discountAmount < 0 {
			discountAmount = 0
		}

		var orderID int
		err := tx.QueryRow(`
			INSERT INTO orders (user_id, merchant_id, order_no, total_amount, discount_amount, pay_amount, status, delivery_type, pickup_code, expire_time)
			VALUES ($1, $2, $3, $4, $5, $6, 'pending', 'pickup', $7, NOW() + INTERVAL '30 minutes')
			RETURNING id
		`, uid, merchantID, orderNo, originalPrice, discountAmount, groupPrice, pickupCode).Scan(&orderID)
		if err != nil {
			log.Printf("createGroupBuyOrders: 创建订单失败 user_id=%d, err=%v", uid, err)
			continue
		}

		// 创建订单商品（original_price 用真正的原价，current_price 用拼团价）
		tx.Exec(`
			INSERT INTO order_items (order_id, product_id, product_name, original_price, current_price, quantity, subtotal)
			VALUES ($1, $2, $3, $4, $5, 1, $6)
		`, orderID, productID, productName, originalPrice, groupPrice, groupPrice)
	}

	// 扣减库存（按成员数量）
	tx.Exec(`UPDATE products SET stock = stock - $1, sold_count = sold_count + $2 WHERE id = $3 AND stock >= $4`,
		len(memberIDs), len(memberIDs), productID, len(memberIDs))
}

// 记录购买动态（订单完成时调用）
func RecordPurchaseActivity(userID int, productID int, discount float64, savedMoney float64) {
	message := fmt.Sprintf("抢到了%.0f折好物", discount*10)
	db.PG.Exec(`
		INSERT INTO friend_activities (user_id, activity_type, product_id, discount, saved_money, message)
		VALUES ($1, 'purchase', $2, $3, $4, $5)
	`, userID, productID, discount, savedMoney, message)
}

// ========== 热门拼团 ==========

// 获取热门拼团（可加入的）
func GetHotGroupBuys(c *gin.Context) {
	limit := c.DefaultQuery("limit", "10")
	limitNum, _ := strconv.Atoi(limit)

	// 批量清理过期拼团
	db.PG.Exec(`UPDATE group_buys SET status = 'expired' WHERE status = 'pending' AND expire_time < NOW()`)

	rows, err := db.PG.Query(`
		SELECT g.id, g.product_id, p.name, p.images[1], g.target_count, g.current_count,
			g.original_price, g.group_price, g.expire_time,
			u.nickname as creator_name, u.avatar as creator_avatar
		FROM group_buys g
		JOIN products p ON g.product_id = p.id
		JOIN users u ON g.creator_id = u.id
		WHERE g.status = 'pending' AND g.expire_time > NOW()
		ORDER BY g.current_count DESC, g.created_at DESC
		LIMIT $1
	`, limitNum)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []gin.H{}})
		return
	}
	defer rows.Close()

	var groups []gin.H
	for rows.Next() {
		var id, productID, targetCount, currentCount int
		var productName, creatorName string
		var productImage, creatorAvatar sql.NullString
		var originalPrice, groupPrice float64
		var expireTime time.Time

		rows.Scan(&id, &productID, &productName, &productImage, &targetCount, &currentCount,
			&originalPrice, &groupPrice, &expireTime, &creatorName, &creatorAvatar)

		remainingSeconds := int(time.Until(expireTime).Seconds())
		if remainingSeconds < 0 {
			continue
		}

		groups = append(groups, gin.H{
			"id":                id,
			"product_id":        productID,
			"product_name":      productName,
			"product_image":     productImage.String,
			"target_count":      targetCount,
			"current_count":     currentCount,
			"original_price":    originalPrice,
			"group_price":       groupPrice,
			"remaining_seconds": remainingSeconds,
			"creator_name":      creatorName,
			"creator_avatar":    creatorAvatar.String,
			"discount":          int((1 - groupPrice/originalPrice) * 100),
		})
	}

	if groups == nil {
		groups = []gin.H{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": groups})
}

// 获取商品的进行中拼团
func GetProductGroupBuys(c *gin.Context) {
	productID := c.Param("id")

	rows, err := db.PG.Query(`
		SELECT g.id, g.target_count, g.current_count, g.group_price, g.invite_code, g.expire_time,
			u.nickname as creator_name, u.avatar as creator_avatar
		FROM group_buys g
		JOIN users u ON g.creator_id = u.id
		WHERE g.product_id = $1 AND g.status = 'pending' AND g.expire_time > NOW()
		ORDER BY g.current_count DESC, g.created_at DESC
		LIMIT 5
	`, productID)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []gin.H{}})
		return
	}
	defer rows.Close()

	var groups []gin.H
	for rows.Next() {
		var id, targetCount, currentCount int
		var groupPrice float64
		var inviteCode, creatorName string
		var creatorAvatar sql.NullString
		var expireTime time.Time

		rows.Scan(&id, &targetCount, &currentCount, &groupPrice, &inviteCode, &expireTime, &creatorName, &creatorAvatar)

		remainingSeconds := int(time.Until(expireTime).Seconds())
		if remainingSeconds < 0 {
			continue
		}

		groups = append(groups, gin.H{
			"id":                id,
			"target_count":      targetCount,
			"current_count":     currentCount,
			"group_price":       groupPrice,
			"invite_code":       inviteCode,
			"remaining_seconds": remainingSeconds,
			"creator_name":      creatorName,
			"creator_avatar":    creatorAvatar.String,
		})
	}

	if groups == nil {
		groups = []gin.H{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": groups})
}
