package handler

import (
	"database/sql"
	"fmt"
	"food-rescue/db"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// 获取食物银行列表
func GetFoodBanks(c *gin.Context) {
	rows, err := db.PG.Query(`
		SELECT id, name, description, logo, address, contact_phone, 
		       total_received, people_helped
		FROM food_banks
		WHERE is_active = true
		ORDER BY total_received DESC
	`)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []gin.H{}})
		return
	}
	defer rows.Close()

	var banks []gin.H
	for rows.Next() {
		var id, peopleHelped int
		var name, description string
		var logo, address, contactPhone sql.NullString
		var totalReceived float64

		rows.Scan(&id, &name, &description, &logo, &address, &contactPhone, &totalReceived, &peopleHelped)

		banks = append(banks, gin.H{
			"id":             id,
			"name":           name,
			"description":    description,
			"logo":           logo.String,
			"address":        address.String,
			"contact_phone":  contactPhone.String,
			"total_received": totalReceived,
			"people_helped":  peopleHelped,
		})
	}

	if banks == nil {
		banks = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": banks})
}

// 获取公益统计
func GetCharityStats(c *gin.Context) {
	// 平台总捐赠
	var totalDonations float64
	var totalDonors, totalPeopleHelped int

	db.PG.QueryRow(`SELECT COALESCE(SUM(amount), 0) FROM donations`).Scan(&totalDonations)
	db.PG.QueryRow(`SELECT COUNT(DISTINCT user_id) FROM donations`).Scan(&totalDonors)
	db.PG.QueryRow(`SELECT COALESCE(SUM(people_helped), 0) FROM food_banks`).Scan(&totalPeopleHelped)

	// 今日捐赠
	var todayDonations float64
	var todayDonors int
	db.PG.QueryRow(`SELECT COALESCE(SUM(amount), 0) FROM donations WHERE DATE(created_at) = CURRENT_DATE`).Scan(&todayDonations)
	db.PG.QueryRow(`SELECT COUNT(DISTINCT user_id) FROM donations WHERE DATE(created_at) = CURRENT_DATE`).Scan(&todayDonors)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"total_donations":    totalDonations,
			"total_donors":       totalDonors,
			"total_people_helped": totalPeopleHelped,
			"today_donations":    todayDonations,
			"today_donors":       todayDonors,
		},
	})
}

// 获取用户捐赠记录
func GetUserDonations(c *gin.Context) {
	userID := c.Param("user_id")
	limit := c.DefaultQuery("limit", "20")
	limitNum, _ := strconv.Atoi(limit)

	// 用户捐赠统计
	var totalAmount float64
	var donationCount int
	db.PG.QueryRow(`SELECT COALESCE(total_donations, 0), COALESCE(donation_count, 0) FROM users WHERE id = $1`, userID).Scan(&totalAmount, &donationCount)

	// 捐赠记录
	rows, err := db.PG.Query(`
		SELECT d.id, d.amount, d.message, d.created_at,
		       fb.name as food_bank_name
		FROM donations d
		LEFT JOIN food_banks fb ON d.food_bank_id = fb.id
		WHERE d.user_id = $1
		ORDER BY d.created_at DESC
		LIMIT $2
	`, userID, limitNum)

	var records []gin.H
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int
			var amount float64
			var message, foodBankName sql.NullString
			var createdAt time.Time

			rows.Scan(&id, &amount, &message, &createdAt, &foodBankName)

			records = append(records, gin.H{
				"id":             id,
				"amount":         amount,
				"message":        message.String,
				"food_bank_name": foodBankName.String,
				"created_at":     createdAt,
				"time_ago":       formatTimeAgo(createdAt),
			})
		}
	}

	if records == nil {
		records = []gin.H{}
	}

	// 计算爱心等级
	loveLevel := calculateLoveLevel(donationCount)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"total_amount":    totalAmount,
			"donation_count":  donationCount,
			"love_level":      loveLevel,
			"people_helped":   donationCount * 2, // 假设每次捐赠帮助2人
			"records":         records,
		},
	})
}

// 创建捐赠（下单时调用）
func CreateDonation(c *gin.Context) {
	userID := c.Param("user_id")
	var req struct {
		OrderID    int     `json:"order_id"`
		Amount     float64 `json:"amount"`
		FoodBankID *int    `json:"food_bank_id"`
		Message    string  `json:"message"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误"})
		return
	}

	if req.Amount <= 0 {
		req.Amount = 1.0 // 默认1元
	}

	tx, err := db.PG.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "系统错误"})
		return
	}
	defer tx.Rollback()

	// 创建捐赠记录
	var donationID int
	err = tx.QueryRow(`
		INSERT INTO donations (user_id, order_id, amount, food_bank_id, message)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''))
		RETURNING id
	`, userID, req.OrderID, req.Amount, req.FoodBankID, req.Message).Scan(&donationID)

	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "捐赠失败"})
		return
	}

	// 更新用户捐赠统计
	tx.Exec(`
		UPDATE users SET 
			total_donations = COALESCE(total_donations, 0) + $2,
			donation_count = COALESCE(donation_count, 0) + 1
		WHERE id = $1
	`, userID, req.Amount)

	// 更新食物银行统计
	if req.FoodBankID != nil {
		tx.Exec(`
			UPDATE food_banks SET 
				total_received = total_received + $2,
				people_helped = people_helped + 2
			WHERE id = $1
		`, *req.FoodBankID, req.Amount)
	}

	tx.Commit()

	// 发送通知
	userIDInt, _ := strconv.Atoi(userID)
	SendNotification(userIDInt, NotifyTypeDonation,
		"爱心已送达 ❤️",
		fmt.Sprintf("感谢您捐赠 ¥%.2f，您的爱心将帮助有需要的人", req.Amount),
		donationID, "donation")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "感谢您的爱心捐赠！",
		"data": gin.H{
			"donation_id": donationID,
		},
	})
}

// 获取捐赠排行榜
func GetDonationLeaderboard(c *gin.Context) {
	period := c.DefaultQuery("period", "all") // weekly, monthly, all
	limit := c.DefaultQuery("limit", "20")
	limitNum, _ := strconv.Atoi(limit)

	var dateFilter string
	switch period {
	case "weekly":
		dateFilter = "AND d.created_at >= CURRENT_DATE - INTERVAL '7 days'"
	case "monthly":
		dateFilter = "AND d.created_at >= CURRENT_DATE - INTERVAL '30 days'"
	default:
		dateFilter = ""
	}

	query := `
		SELECT u.id, u.nickname, u.avatar, 
		       COALESCE(SUM(d.amount), 0) as total_donated,
		       COUNT(d.id) as donation_count
		FROM users u
		LEFT JOIN donations d ON u.id = d.user_id ` + dateFilter + `
		WHERE u.status = 'active'
		GROUP BY u.id, u.nickname, u.avatar
		HAVING COUNT(d.id) > 0
		ORDER BY total_donated DESC
		LIMIT $1
	`

	rows, err := db.PG.Query(query, limitNum)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []gin.H{}})
		return
	}
	defer rows.Close()

	var leaderboard []gin.H
	rank := 1
	for rows.Next() {
		var id, donationCount int
		var nickname, avatar sql.NullString
		var totalDonated float64

		rows.Scan(&id, &nickname, &avatar, &totalDonated, &donationCount)

		leaderboard = append(leaderboard, gin.H{
			"rank":           rank,
			"user_id":        id,
			"nickname":       nickname.String,
			"avatar":         avatar.String,
			"total_donated":  totalDonated,
			"donation_count": donationCount,
			"love_level":     calculateLoveLevel(donationCount),
		})
		rank++
	}

	if leaderboard == nil {
		leaderboard = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": leaderboard})
}

// 计算爱心等级
func calculateLoveLevel(donationCount int) string {
	if donationCount >= 100 {
		return "爱心大使"
	} else if donationCount >= 50 {
		return "公益达人"
	} else if donationCount >= 20 {
		return "爱心使者"
	} else if donationCount >= 5 {
		return "热心市民"
	} else if donationCount >= 1 {
		return "爱心新人"
	}
	return "尚未捐赠"
}

// 获取最近捐赠动态（公益墙）
func GetRecentDonations(c *gin.Context) {
	limit := c.DefaultQuery("limit", "10")
	limitNum, _ := strconv.Atoi(limit)

	rows, err := db.PG.Query(`
		SELECT d.id, u.nickname, u.avatar, d.amount, d.message, d.created_at,
		       fb.name as food_bank_name
		FROM donations d
		JOIN users u ON d.user_id = u.id
		LEFT JOIN food_banks fb ON d.food_bank_id = fb.id
		ORDER BY d.created_at DESC
		LIMIT $1
	`, limitNum)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []gin.H{}})
		return
	}
	defer rows.Close()

	var donations []gin.H
	for rows.Next() {
		var id int
		var nickname string
		var avatar sql.NullString
		var amount float64
		var message, foodBankName sql.NullString
		var createdAt time.Time

		rows.Scan(&id, &nickname, &avatar, &amount, &message, &createdAt, &foodBankName)

		// 隐藏部分昵称（按字符处理，支持中文）
		displayName := nickname
		runes := []rune(nickname)
		if len(runes) > 2 {
			displayName = string(runes[:1]) + "**" + string(runes[len(runes)-1:])
		} else if len(runes) == 2 {
			displayName = string(runes[:1]) + "*"
		}

		donations = append(donations, gin.H{
			"id":             id,
			"nickname":       displayName,
			"avatar":         avatar.String,
			"amount":         amount,
			"message":        message.String,
			"food_bank_name": foodBankName.String,
			"time_ago":       formatTimeAgo(createdAt),
		})
	}

	if donations == nil {
		donations = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": donations})
}
