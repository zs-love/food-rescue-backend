package main

import (
	"log"
	"net/http"
	"os"

	"food-rescue/db"
	"food-rescue/handler"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// 加载环境变量
	godotenv.Load()

	// 初始化数据库
	if err := db.InitPostgres(); err != nil {
		log.Fatal("Failed to connect to PostgreSQL:", err)
	}
	defer db.ClosePostgres()

	if err := db.InitRedis(); err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	}
	defer db.CloseRedis()

	// 初始化路由
	r := gin.Default()

	// CORS
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	// 静态文件服务 - 上传的文件
	r.Static("/uploads", "./uploads")

	// 文件上传
	r.POST("/api/upload", handler.UploadFile)

	// 路由
	r.POST("/api/register", handler.Register)
	r.POST("/api/login", handler.Login)

	// 管理后台
	r.POST("/api/admin/login", handler.AdminLogin)
	r.GET("/api/admin/dashboard", handler.AdminDashboard)
	r.GET("/api/admin/users", handler.AdminGetUsers)
	r.PUT("/api/admin/users/:id/status", handler.AdminUpdateUserStatus)
	r.GET("/api/admin/merchants", handler.AdminGetMerchants)
	r.PUT("/api/admin/merchants/:id/status", handler.AdminUpdateMerchantStatus)
	r.GET("/api/admin/orders", handler.AdminGetOrders)
	r.GET("/api/admin/orders/:id", handler.AdminGetOrderDetail)
	r.GET("/api/admin/products", handler.AdminGetProducts)
	r.PUT("/api/admin/products/:id/status", handler.AdminUpdateProductStatus)
	r.GET("/api/admin/categories", handler.AdminGetCategories)
	r.POST("/api/admin/categories", handler.AdminCreateCategory)
	r.PUT("/api/admin/categories/:id", handler.AdminUpdateCategory)
	r.DELETE("/api/admin/categories/:id", handler.AdminDeleteCategory)
	r.GET("/api/admin/reviews", handler.AdminGetReviews)
	r.DELETE("/api/admin/reviews/:id", handler.AdminDeleteReview)
	r.GET("/api/admin/donations", handler.AdminGetDonations)
	r.GET("/api/admin/tasters", handler.AdminGetTasters)
	r.POST("/api/admin/tasters/:id/approve", handler.AdminApproveTaster)
	r.GET("/api/admin/blindboxes", handler.AdminGetBlindBoxes)
	r.GET("/api/admin/admins", handler.AdminGetAdmins)
	r.POST("/api/admin/admins", handler.AdminCreateAdmin)
	r.PUT("/api/admin/admins/:id", handler.AdminUpdateAdmin)
	r.GET("/api/admin/groupbuys", handler.AdminGetGroupBuys)
	r.GET("/api/admin/urgent-events", handler.AdminGetUrgentEvents)

	// 商家店铺
	r.GET("/api/shops/:id", handler.GetShopInfo)
	r.PUT("/api/shops/:id", handler.UpdateShopInfo)

	// 商品分类
	r.GET("/api/categories", handler.GetCategories)

	// 商品管理
	r.GET("/api/shops/:id/products", handler.GetMerchantProducts)
	r.GET("/api/shops/:id/products/stats", handler.GetProductStats)
	r.POST("/api/shops/:id/products", handler.CreateProduct)
	r.PUT("/api/products/:id", handler.UpdateProduct)
	r.DELETE("/api/products/:id", handler.DeleteProduct)
	r.PUT("/api/products/:id/status", handler.UpdateProductStatus)

	// 消费者端
	r.GET("/api/home/stats", handler.GetHomeStats)
	r.GET("/api/home", handler.GetHomeData)
	r.GET("/api/nearby-shops", handler.GetNearbyShops)
	r.GET("/api/expiring-products", handler.GetExpiringProducts)
	r.GET("/api/flash-sale", handler.GetFlashSaleProducts)
	r.GET("/api/category/:id/products", handler.GetProductsByCategory)
	r.GET("/api/shop/:id/products", handler.GetShopProducts)
	r.GET("/api/product/:id", handler.GetProductDetail)
	r.GET("/api/search", handler.SearchProducts)

	// 地址搜索代理（高德地图）
	r.GET("/api/poi/search", handler.SearchPOI)

	// 挑战赛
	r.GET("/api/user/:user_id/challenge", handler.GetChallengeData)
	r.POST("/api/user/:user_id/checkin", handler.DailyCheckin)
	r.POST("/api/user/:user_id/tasks/:task_id/claim", handler.ClaimTaskReward)
	r.GET("/api/urgent-events", handler.GetUrgentEvents)
	r.POST("/api/user/:user_id/events/:event_id/join", handler.JoinUrgentEvent)
	r.GET("/api/leaderboard", handler.GetLeaderboard)

	// 社交拼救
	r.POST("/api/user/:user_id/group-buys", handler.CreateGroupBuy)
	r.POST("/api/user/:user_id/group-buys/join", handler.JoinGroupBuy)
	r.GET("/api/user/:user_id/group-buys", handler.GetUserGroupBuys)
	r.GET("/api/group-buys/:group_id", handler.GetGroupBuyDetail)
	r.GET("/api/group-buys/hot", handler.GetHotGroupBuys)
	r.GET("/api/product/:id/group-buys", handler.GetProductGroupBuys)

	// 小队系统
	r.POST("/api/user/:user_id/team", handler.CreateTeam)
	r.POST("/api/user/:user_id/team/join", handler.JoinTeam)
	r.POST("/api/user/:user_id/team/leave", handler.LeaveTeam)
	r.GET("/api/user/:user_id/team", handler.GetUserTeam)
	r.GET("/api/teams/:team_id", handler.GetTeamDetail)
	r.GET("/api/teams/leaderboard", handler.GetTeamLeaderboard)

	// 好友动态
	r.GET("/api/user/:user_id/activities", handler.GetFriendActivities)

	// 通知系统
	r.GET("/api/user/:user_id/notifications", handler.GetNotifications)
	r.GET("/api/user/:user_id/notifications/unread-count", handler.GetUnreadCount)
	r.POST("/api/user/:user_id/notifications/:notification_id/read", handler.MarkNotificationRead)
	r.GET("/api/user/:user_id/notification-settings", handler.GetNotificationSettings)
	r.PUT("/api/user/:user_id/notification-settings", handler.UpdateNotificationSettings)
	r.POST("/api/user/:user_id/watch-product", handler.WatchProduct)
	r.DELETE("/api/user/:user_id/watch-product/:product_id", handler.UnwatchProduct)
	r.GET("/api/user/:user_id/watch-product/:product_id/check", handler.CheckProductWatch)
	r.GET("/api/flash-sales", handler.GetFlashSales)

	// 公益系统
	r.GET("/api/food-banks", handler.GetFoodBanks)
	r.GET("/api/charity/stats", handler.GetCharityStats)
	r.GET("/api/charity/recent", handler.GetRecentDonations)
	r.GET("/api/charity/leaderboard", handler.GetDonationLeaderboard)
	r.GET("/api/user/:user_id/donations", handler.GetUserDonations)
	r.POST("/api/user/:user_id/donations", handler.CreateDonation)

	// 收藏
	r.GET("/api/user/:user_id/favorites", handler.GetFavorites)
	r.POST("/api/user/:user_id/favorites", handler.AddFavorite)
	r.DELETE("/api/user/:user_id/favorites/:product_id", handler.RemoveFavorite)
	r.GET("/api/user/:user_id/favorites/:product_id/check", handler.CheckFavorite)

	// 用户资料
	r.PUT("/api/user/:user_id/profile", handler.UpdateUserProfile)

	// 购物车
	r.GET("/api/user/:user_id/cart", handler.GetCart)
	r.GET("/api/user/:user_id/cart/count", handler.GetCartCount)
	r.POST("/api/user/:user_id/cart", handler.AddToCart)
	r.PUT("/api/user/:user_id/cart/:product_id", handler.UpdateCartItem)
	r.DELETE("/api/user/:user_id/cart/:product_id", handler.RemoveFromCart)
	r.DELETE("/api/user/:user_id/cart", handler.ClearCart)

	// 订单
	r.POST("/api/user/:user_id/orders", handler.CreateOrder)
	r.GET("/api/user/:user_id/orders", handler.GetUserOrders)
	r.GET("/api/orders/:order_id", handler.GetOrderDetail)
	r.POST("/api/orders/:order_id/pay", handler.PayOrder)
	r.POST("/api/user/:user_id/orders/:order_id/cancel", handler.CancelOrder)
	r.POST("/api/orders/:order_id/complete", handler.CompleteOrder)
	r.GET("/api/merchant/:merchant_id/orders", handler.GetMerchantOrders)

	// 盲盒
	r.GET("/api/blind-boxes", handler.GetBlindBoxes)
	r.POST("/api/user/:user_id/blind-boxes/purchase", handler.PurchaseBlindBox)
	r.POST("/api/user/:user_id/blind-boxes/:purchase_id/open", handler.OpenBlindBox)
	r.GET("/api/user/:user_id/blind-boxes", handler.GetUserBlindBoxes)
	r.POST("/api/products/:id/blind-box-contents", handler.SetBlindBoxContents)
	r.GET("/api/products/:id/blind-box-contents", handler.GetBlindBoxContents)
	r.GET("/api/merchant/:merchant_id/blind-box-stats", handler.GetMerchantBlindBoxStats)

	// 商品溯源
	r.GET("/api/products/:id/traceability", handler.GetProductTraceability)
	r.POST("/api/products/:id/traceability", handler.SetProductTraceability)
	r.GET("/api/products/:id/trace-qrcode", handler.GenerateTraceQRCode)

	// 商品评价
	r.GET("/api/products/:id/reviews", handler.GetProductReviews)
	r.POST("/api/user/:user_id/orders/:order_id/review", handler.SubmitOrderReview)
	r.GET("/api/merchant/:merchant_id/reviews", handler.GetMerchantReviews)
	r.POST("/api/reviews/:review_id/reply", handler.ReplyReview)

	// 商家紧急清仓事件
	r.POST("/api/merchant/:merchant_id/urgent-events", handler.CreateUrgentEvent)
	r.GET("/api/merchant/:merchant_id/urgent-events", handler.GetMerchantUrgentEvents)

	// 试吃官
	r.POST("/api/user/:user_id/taster/apply", handler.ApplyTaster)
	r.GET("/api/user/:user_id/taster/status", handler.GetTasterStatus)
	r.GET("/api/tasting-tasks", handler.GetTastingTasks)
	r.POST("/api/user/:user_id/tasting-tasks/:task_id/apply", handler.ApplyTastingTask)
	r.POST("/api/tasting-applications/:app_id/review", handler.SubmitTastingReview)

	// 商家试吃任务管理
	r.POST("/api/merchant/:merchant_id/tasting-tasks", handler.CreateTastingTask)
	r.GET("/api/merchant/:merchant_id/tasting-tasks", handler.GetMerchantTastingTasks)
	r.GET("/api/tasting-tasks/:task_id/applications", handler.GetTastingApplications)
	r.POST("/api/tasting-applications/:app_id/approve", handler.ApproveTastingApplication)

	// 启动服务
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "9090"
	}
	log.Printf("Server running on port %s", port)
	r.Run("0.0.0.0:" + port)
}
