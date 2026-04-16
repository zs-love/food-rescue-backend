package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"food-rescue/db"
	"food-rescue/model"

	"github.com/gin-gonic/gin"
)

type RegisterRequest struct {
	Phone    string `json:"phone" binding:"required"`
	Password string `json:"password" binding:"required"`
	Role     string `json:"role" binding:"required"` // consumer 或 merchant
	Nickname string `json:"nickname"`
	ShopName string `json:"shop_name"` // 商家注册时必填
}

type LoginRequest struct {
	Phone    string `json:"phone" binding:"required"`
	Password string `json:"password" binding:"required"`
	Role     string `json:"role" binding:"required"` // consumer 或 merchant
}

type AuthResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type UserLoginData struct {
	UserID           int     `json:"user_id"`
	Role             string  `json:"role"`
	Phone            string  `json:"phone"`
	Nickname         string  `json:"nickname"`
	Avatar           string  `json:"avatar"`
	TotalSavedWeight float64 `json:"total_saved_weight"`
	TotalSavedMoney  float64 `json:"total_saved_money"`
	TotalOrders      int     `json:"total_orders"`
	RankLevel        string  `json:"rank_level"`
}

type MerchantLoginData struct {
	MerchantID        int     `json:"merchant_id"`
	Role              string  `json:"role"`
	Phone             string  `json:"phone"`
	ShopName          string  `json:"shop_name"`
	ShopLogo          string  `json:"shop_logo"`
	IsVerified        bool    `json:"is_verified"`
	Status            string  `json:"status"`
	TotalSales        float64 `json:"total_sales"`
	TotalSavedLoss    float64 `json:"total_saved_loss"`
	TotalProductsSold int     `json:"total_products_sold"`
	Rating            float64 `json:"rating"`
}

func Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, AuthResponse{Success: false, Message: "参数错误"})
		return
	}

	if req.Role == "consumer" {
		registerUser(c, req)
	} else if req.Role == "merchant" {
		registerMerchant(c, req)
	} else {
		c.JSON(http.StatusBadRequest, AuthResponse{Success: false, Message: "无效的角色类型"})
	}
}

func registerUser(c *gin.Context, req RegisterRequest) {
	nickname := req.Nickname
	if nickname == "" {
		nickname = "用户" + req.Phone[len(req.Phone)-4:]
	}

	var userID int
	err := db.PG.QueryRow(`
		INSERT INTO users (phone, password, nickname, rank_level, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, req.Phone, req.Password, nickname, model.RankNewcomer, model.UserStatusActive).Scan(&userID)

	if err != nil {
		c.JSON(http.StatusBadRequest, AuthResponse{Success: false, Message: "手机号已注册"})
		return
	}

	c.JSON(http.StatusOK, AuthResponse{
		Success: true,
		Message: "注册成功",
		Data: UserLoginData{
			UserID:    userID,
			Role:      "consumer",
			Phone:     req.Phone,
			Nickname:  nickname,
			RankLevel: model.RankNewcomer,
		},
	})
}

func registerMerchant(c *gin.Context, req RegisterRequest) {
	if req.ShopName == "" {
		c.JSON(http.StatusBadRequest, AuthResponse{Success: false, Message: "店铺名称不能为空"})
		return
	}

	var merchantID int
	err := db.PG.QueryRow(`
		INSERT INTO merchants (phone, password, shop_name, status)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, req.Phone, req.Password, req.ShopName, model.MerchantStatusPending).Scan(&merchantID)

	if err != nil {
		c.JSON(http.StatusBadRequest, AuthResponse{Success: false, Message: "手机号已注册"})
		return
	}

	c.JSON(http.StatusOK, AuthResponse{
		Success: true,
		Message: "注册成功，请等待审核",
		Data: MerchantLoginData{
			MerchantID: merchantID,
			Role:       "merchant",
			Phone:      req.Phone,
			ShopName:   req.ShopName,
			Status:     model.MerchantStatusPending,
		},
	})
}

func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, AuthResponse{Success: false, Message: "参数错误"})
		return
	}

	if req.Role == "consumer" {
		loginUser(c, req)
	} else if req.Role == "merchant" {
		loginMerchant(c, req)
	} else {
		c.JSON(http.StatusBadRequest, AuthResponse{Success: false, Message: "无效的角色类型"})
	}
}

func loginUser(c *gin.Context, req LoginRequest) {
	// 先检查用户是否存在
	var exists bool
	err := db.PG.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE phone = $1)`, req.Phone).Scan(&exists)
	if err != nil || !exists {
		c.JSON(http.StatusUnauthorized, AuthResponse{Success: false, Message: "该手机号未注册，请先注册账号"})
		return
	}

	var user struct {
		ID               int
		Phone            string
		Nickname         sql.NullString
		Avatar           sql.NullString
		TotalSavedWeight float64
		TotalSavedMoney  float64
		TotalOrders      int
		RankLevel        string
		Status           string
	}

	err = db.PG.QueryRow(`
		SELECT id, phone, nickname, avatar, total_saved_weight, total_saved_money, 
		       total_orders, rank_level, status
		FROM users 
		WHERE phone = $1 AND password = $2
	`, req.Phone, req.Password).Scan(
		&user.ID, &user.Phone, &user.Nickname, &user.Avatar,
		&user.TotalSavedWeight, &user.TotalSavedMoney, &user.TotalOrders,
		&user.RankLevel, &user.Status,
	)

	if err != nil {
		c.JSON(http.StatusUnauthorized, AuthResponse{Success: false, Message: "密码错误，请重新输入"})
		return
	}

	if user.Status == model.UserStatusBanned {
		c.JSON(http.StatusForbidden, AuthResponse{Success: false, Message: "账号已被封禁，请联系客服"})
		return
	}

	// 更新最后登录时间
	db.PG.Exec("UPDATE users SET updated_at = $1 WHERE id = $2", time.Now(), user.ID)

	c.JSON(http.StatusOK, AuthResponse{
		Success: true,
		Message: "登录成功",
		Data: UserLoginData{
			UserID:           user.ID,
			Role:             "consumer",
			Phone:            user.Phone,
			Nickname:         user.Nickname.String,
			Avatar:           user.Avatar.String,
			TotalSavedWeight: user.TotalSavedWeight,
			TotalSavedMoney:  user.TotalSavedMoney,
			TotalOrders:      user.TotalOrders,
			RankLevel:        user.RankLevel,
		},
	})
}

func loginMerchant(c *gin.Context, req LoginRequest) {
	// 先检查商家是否存在
	var exists bool
	err := db.PG.QueryRow(`SELECT EXISTS(SELECT 1 FROM merchants WHERE phone = $1)`, req.Phone).Scan(&exists)
	if err != nil || !exists {
		c.JSON(http.StatusUnauthorized, AuthResponse{Success: false, Message: "该手机号未注册商家账号，请先注册"})
		return
	}

	var merchant struct {
		ID                int
		Phone             string
		ShopName          sql.NullString
		ShopLogo          sql.NullString
		IsVerified        bool
		Status            string
		TotalSales        float64
		TotalSavedLoss    float64
		TotalProductsSold int
		Rating            float64
	}

	err = db.PG.QueryRow(`
		SELECT id, phone, shop_name, shop_logo, is_verified, status,
		       total_sales, total_saved_loss, total_products_sold, rating
		FROM merchants 
		WHERE phone = $1 AND password = $2
	`, req.Phone, req.Password).Scan(
		&merchant.ID, &merchant.Phone, &merchant.ShopName, &merchant.ShopLogo,
		&merchant.IsVerified, &merchant.Status, &merchant.TotalSales,
		&merchant.TotalSavedLoss, &merchant.TotalProductsSold, &merchant.Rating,
	)

	if err != nil {
		c.JSON(http.StatusUnauthorized, AuthResponse{Success: false, Message: "密码错误，请重新输入"})
		return
	}

	if merchant.Status == model.MerchantStatusBanned {
		c.JSON(http.StatusForbidden, AuthResponse{Success: false, Message: "店铺已被封禁，请联系客服"})
		return
	}

	if merchant.Status == model.MerchantStatusPending {
		c.JSON(http.StatusOK, AuthResponse{
			Success: true,
			Message: "登录成功，店铺审核中",
			Data: MerchantLoginData{
				MerchantID:        merchant.ID,
				Role:              "merchant",
				Phone:             merchant.Phone,
				ShopName:          merchant.ShopName.String,
				ShopLogo:          merchant.ShopLogo.String,
				IsVerified:        merchant.IsVerified,
				Status:            merchant.Status,
				TotalSales:        merchant.TotalSales,
				TotalSavedLoss:    merchant.TotalSavedLoss,
				TotalProductsSold: merchant.TotalProductsSold,
				Rating:            merchant.Rating,
			},
		})
		return
	}

	// 更新最后登录时间
	db.PG.Exec("UPDATE merchants SET updated_at = $1 WHERE id = $2", time.Now(), merchant.ID)

	c.JSON(http.StatusOK, AuthResponse{
		Success: true,
		Message: "登录成功",
		Data: MerchantLoginData{
			MerchantID:        merchant.ID,
			Role:              "merchant",
			Phone:             merchant.Phone,
			ShopName:          merchant.ShopName.String,
			ShopLogo:          merchant.ShopLogo.String,
			IsVerified:        merchant.IsVerified,
			Status:            merchant.Status,
			TotalSales:        merchant.TotalSales,
			TotalSavedLoss:    merchant.TotalSavedLoss,
			TotalProductsSold: merchant.TotalProductsSold,
			Rating:            merchant.Rating,
		},
	})
}


// UpdateUserProfile 更新用户资料
func UpdateUserProfile(c *gin.Context) {
	userID := c.Param("user_id")

	var req struct {
		Nickname string `json:"nickname"`
		Avatar   string `json:"avatar"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误"})
		return
	}

	// 构建更新语句
	updates := []string{}
	args := []interface{}{}
	argIndex := 1

	if req.Nickname != "" {
		updates = append(updates, fmt.Sprintf("nickname = $%d", argIndex))
		args = append(args, req.Nickname)
		argIndex++
	}

	if req.Avatar != "" {
		updates = append(updates, fmt.Sprintf("avatar = $%d", argIndex))
		args = append(args, req.Avatar)
		argIndex++
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "没有要更新的内容"})
		return
	}

	// 添加 updated_at
	updates = append(updates, fmt.Sprintf("updated_at = $%d", argIndex))
	args = append(args, time.Now())
	argIndex++

	// 添加 user_id 条件
	args = append(args, userID)

	query := fmt.Sprintf("UPDATE users SET %s WHERE id = $%d", joinStrings(updates, ", "), argIndex)

	result, err := db.PG.Exec(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "更新失败"})
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "用户不存在"})
		return
	}

	// 返回更新后的用户信息
	var user struct {
		ID               int
		Phone            string
		Nickname         sql.NullString
		Avatar           sql.NullString
		TotalSavedWeight float64
		TotalSavedMoney  float64
		TotalOrders      int
		RankLevel        string
	}

	err = db.PG.QueryRow(`
		SELECT id, phone, nickname, avatar, total_saved_weight, total_saved_money, 
		       total_orders, rank_level
		FROM users WHERE id = $1
	`, userID).Scan(
		&user.ID, &user.Phone, &user.Nickname, &user.Avatar,
		&user.TotalSavedWeight, &user.TotalSavedMoney, &user.TotalOrders, &user.RankLevel,
	)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "更新成功"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "更新成功",
		"data": UserLoginData{
			UserID:           user.ID,
			Role:             "consumer",
			Phone:            user.Phone,
			Nickname:         user.Nickname.String,
			Avatar:           user.Avatar.String,
			TotalSavedWeight: user.TotalSavedWeight,
			TotalSavedMoney:  user.TotalSavedMoney,
			TotalOrders:      user.TotalOrders,
			RankLevel:        user.RankLevel,
		},
	})
}

// 辅助函数：连接字符串
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
