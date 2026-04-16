package handler

import (
	cryptoRand "crypto/rand"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"

	"food-rescue/db"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

// 使用独立的随机数生成器，避免全局状态竞争
var (
	blindBoxRand     *rand.Rand
	blindBoxRandOnce sync.Once
	blindBoxRandMu   sync.Mutex
)

// 初始化盲盒专用的随机数生成器
func getBlindBoxRand() *rand.Rand {
	blindBoxRandOnce.Do(func() {
		// 使用 crypto/rand 获取真正的随机种子
		var seed int64
		if err := binary.Read(cryptoRand.Reader, binary.LittleEndian, &seed); err != nil {
			// 如果 crypto/rand 失败，使用时间+进程信息作为后备
			seed = time.Now().UnixNano()
		}
		blindBoxRand = rand.New(rand.NewSource(seed))
	})
	return blindBoxRand
}

// 获取一个线程安全的随机整数
func blindBoxRandInt(n int) int {
	blindBoxRandMu.Lock()
	defer blindBoxRandMu.Unlock()
	r := getBlindBoxRand()
	// 每次调用时混入当前时间增加随机性
	r.Seed(r.Int63() ^ time.Now().UnixNano())
	return r.Intn(n)
}

// 获取一个线程安全的随机浮点数
func blindBoxRandFloat64() float64 {
	blindBoxRandMu.Lock()
	defer blindBoxRandMu.Unlock()
	r := getBlindBoxRand()
	r.Seed(r.Int63() ^ time.Now().UnixNano())
	return r.Float64()
}

type BlindBoxProduct struct {
	ID          int                   `json:"id"`
	MerchantID  int                   `json:"merchant_id"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Images      []string              `json:"images"`
	Price       float64               `json:"price"`
	MinValue    float64               `json:"min_value"`
	MaxValue    float64               `json:"max_value"`
	Stock       int                   `json:"stock"`
	SoldCount   int                   `json:"sold_count"`
	ShopName    string                `json:"shop_name"`
	ShopLogo    string                `json:"shop_logo"`
	ExpiryDate  string                `json:"expiry_date"`
	DaysLeft    int                   `json:"days_left"`
	Contents    []BlindBoxContentItem `json:"contents,omitempty"`
}

type BlindBoxContentItem struct {
	ID               int     `json:"id"`
	Name             string  `json:"name"`
	Value            float64 `json:"value"`
	Image            string  `json:"image"`
	Description      string  `json:"description"`
	ExpiryDate       string  `json:"expiry_date"`
	StorageCondition string  `json:"storage_condition"`
	Quantity         int     `json:"quantity"`
	Probability      int     `json:"probability"`
}

type BlindBoxPurchase struct {
	ID         int                `json:"id"`
	ProductID  int                `json:"product_id"`
	MerchantID int                `json:"merchant_id"`
	Price      float64            `json:"price"`
	Status     string             `json:"status"`
	OpenedAt   string             `json:"opened_at,omitempty"`
	CreatedAt  string             `json:"created_at"`
	Product    *BlindBoxProduct   `json:"product,omitempty"`
	Items      []BlindBoxItem     `json:"items,omitempty"`
	TotalValue float64            `json:"total_value,omitempty"`
	Profit     float64            `json:"profit,omitempty"`
}

type BlindBoxItem struct {
	ID               int     `json:"id"`
	Name             string  `json:"name"`
	Value            float64 `json:"value"`
	Image            string  `json:"image"`
	ProductID        int     `json:"product_id,omitempty"`
	Description      string  `json:"description,omitempty"`
	ExpiryDate       string  `json:"expiry_date,omitempty"`
	StorageCondition string  `json:"storage_condition,omitempty"`
	Quantity         int     `json:"quantity,omitempty"`
}

// GetBlindBoxes 获取可购买的盲盒列表
func GetBlindBoxes(c *gin.Context) {
	rows, err := db.PG.Query(`
		SELECT p.id, p.merchant_id, p.name, p.description, p.images,
		       p.original_price, p.blind_box_min_value, p.blind_box_max_value,
		       p.stock, p.sold_count, p.expiry_date,
		       m.shop_name, m.shop_logo
		FROM products p
		JOIN merchants m ON p.merchant_id = m.id
		WHERE p.is_blind_box = true AND p.status = 'active' AND p.stock > 0
		  AND p.expiry_date >= CURRENT_DATE
		ORDER BY p.created_at DESC
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "获取盲盒失败"})
		return
	}
	defer rows.Close()

	var boxes []BlindBoxProduct
	now := time.Now()

	for rows.Next() {
		var b BlindBoxProduct
		var images pq.StringArray
		var desc sql.NullString
		var minVal, maxVal sql.NullFloat64
		var expiryDate time.Time
		var shopLogo sql.NullString

		err := rows.Scan(&b.ID, &b.MerchantID, &b.Name, &desc, &images,
			&b.Price, &minVal, &maxVal, &b.Stock, &b.SoldCount, &expiryDate,
			&b.ShopName, &shopLogo)
		if err != nil {
			continue
		}

		b.Description = desc.String
		b.Images = images
		b.MinValue = minVal.Float64
		b.MaxValue = maxVal.Float64
		b.ShopLogo = shopLogo.String
		b.ExpiryDate = expiryDate.Format("2006-01-02")
		b.DaysLeft = int(math.Ceil(expiryDate.Sub(now).Hours() / 24))
		if b.DaysLeft < 0 {
			b.DaysLeft = 0
		}

		boxes = append(boxes, b)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": boxes})
}

// PurchaseBlindBox 购买盲盒 - 创建订单
func PurchaseBlindBox(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的用户ID"})
		return
	}

	var req struct {
		ProductID int `json:"product_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误"})
		return
	}

	// 检查盲盒商品
	var merchantID int
	var price float64
	var stock int
	var productName string
	err = db.PG.QueryRow(`
		SELECT merchant_id, original_price, stock, name 
		FROM products 
		WHERE id = $1 AND is_blind_box = true AND status = 'active'
	`, req.ProductID).Scan(&merchantID, &price, &stock, &productName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "盲盒不存在"})
		return
	}

	if stock <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "盲盒已售罄"})
		return
	}

	// 生成订单号和取货码
	orderNo := time.Now().Format("20060102150405") + strconv.Itoa(userID) + strconv.Itoa(rand.Intn(1000))
	pickupCode := fmt.Sprintf("%06d", rand.Intn(1000000))

	// 开始事务
	tx, err := db.PG.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "系统错误"})
		return
	}
	defer tx.Rollback()

	// 创建订单（盲盒订单）
	var orderID int
	expireAt := time.Now().Add(30 * time.Minute) // 30分钟支付超时
	err = tx.QueryRow(`
		INSERT INTO orders (user_id, merchant_id, order_no, total_amount, pay_amount, status, delivery_type, pickup_code, expire_time)
		VALUES ($1, $2, $3, $4, $5, 'pending', 'pickup', $6, $7)
		RETURNING id
	`, userID, merchantID, orderNo, price, price, pickupCode, expireAt).Scan(&orderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "创建订单失败: " + err.Error()})
		return
	}

	// 创建订单商品（盲盒）
	_, err = tx.Exec(`
		INSERT INTO order_items (order_id, product_id, product_name, original_price, current_price, quantity, subtotal)
		VALUES ($1, $2, $3, $4, $5, 1, $6)
	`, orderID, req.ProductID, productName, price, price, price)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "创建订单商品失败: " + err.Error()})
		return
	}

	// 扣减盲盒库存（与CreateOrder保持一致，在创建订单时扣库存）
	result, err := tx.Exec(`UPDATE products SET stock = stock - 1 WHERE id = $1 AND stock > 0`, req.ProductID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "扣减库存失败"})
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "盲盒已售罄"})
		return
	}

	if err = tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "创建订单失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "订单创建成功，请在30分钟内完成支付",
		"data": gin.H{
			"order_id": orderID,
			"order_no": orderNo,
			"amount":   price,
		},
	})
}

// OpenBlindBox 开启盲盒
func OpenBlindBox(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的用户ID"})
		return
	}

	purchaseID, err := strconv.Atoi(c.Param("purchase_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的购买ID"})
		return
	}

	// 检查购买记录
	var productID int
	var price float64
	var status string
	err = db.PG.QueryRow(`
		SELECT product_id, price, status FROM blind_box_purchases 
		WHERE id = $1 AND user_id = $2
	`, purchaseID, userID).Scan(&productID, &price, &status)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "购买记录不存在"})
		return
	}

	if status != "unopened" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "盲盒已开启"})
		return
	}

	// 获取盲盒价值范围和预设内容
	var minValue, maxValue sql.NullFloat64
	db.PG.QueryRow(`
		SELECT blind_box_min_value, blind_box_max_value FROM products WHERE id = $1
	`, productID).Scan(&minValue, &maxValue)

	minVal := minValue.Float64
	maxVal := maxValue.Float64
	if minVal == 0 {
		minVal = price * 1.2
	}
	if maxVal == 0 {
		maxVal = price * 2.5
	}

	// 获取商家预设的盲盒内容
	var presetItems []BlindBoxContentItem
	contentRows, _ := db.PG.Query(`
		SELECT id, product_name, product_value, product_image, 
		       COALESCE(product_description, ''), 
		       product_expiry_date,
		       COALESCE(product_storage_condition, ''),
		       quantity, probability
		FROM blind_box_contents WHERE blind_box_id = $1
	`, productID)
	if contentRows != nil {
		for contentRows.Next() {
			var item BlindBoxContentItem
			var img, desc, storage sql.NullString
			var expiryDate sql.NullTime
			contentRows.Scan(&item.ID, &item.Name, &item.Value, &img, &desc, &expiryDate, &storage, &item.Quantity, &item.Probability)
			item.Image = img.String
			item.Description = desc.String
			item.StorageCondition = storage.String
			if expiryDate.Valid {
				item.ExpiryDate = expiryDate.Time.Format("2006-01-02")
			}
			presetItems = append(presetItems, item)
		}
		contentRows.Close()
	}

	// 生成盲盒内容
	var items []BlindBoxItem
	if len(presetItems) > 0 {
		// 使用商家预设的内容
		items = selectFromPresetItems(presetItems, minVal, maxVal)
	} else {
		// 使用随机生成的内容
		items = generateRandomItems(minVal, maxVal)
	}

	// 开始事务
	tx, err := db.PG.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "系统错误"})
		return
	}
	defer tx.Rollback()

	// 更新购买状态
	_, err = tx.Exec(`
		UPDATE blind_box_purchases SET status = 'opened', opened_at = $1 WHERE id = $2
	`, time.Now(), purchaseID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "开启失败"})
		return
	}

	// 插入盲盒内容
	var totalValue float64
	var resultItems []BlindBoxItem
	for _, item := range items {
		var itemID int
		err = tx.QueryRow(`
			INSERT INTO blind_box_items (purchase_id, item_name, item_value, item_image, item_description, item_expiry_date, item_storage_condition, item_quantity)
			VALUES ($1, $2, $3, $4, $5, NULLIF($6, '')::DATE, $7, $8)
			RETURNING id
		`, purchaseID, item.Name, item.Value, item.Image, item.Description, item.ExpiryDate, item.StorageCondition, item.Quantity).Scan(&itemID)
		if err != nil {
			continue
		}
		totalValue += item.Value * float64(item.Quantity)
		resultItems = append(resultItems, BlindBoxItem{
			ID:               itemID,
			Name:             item.Name,
			Value:            item.Value,
			Image:            item.Image,
			Description:      item.Description,
			ExpiryDate:       item.ExpiryDate,
			StorageCondition: item.StorageCondition,
			Quantity:         item.Quantity,
		})
	}

	if err = tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "开启失败"})
		return
	}

	profit := totalValue - price

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "开启成功",
		"data": gin.H{
			"items":       resultItems,
			"total_value": math.Round(totalValue*100) / 100,
			"price":       price,
			"profit":      math.Round(profit*100) / 100,
		},
	})
}

// GetUserBlindBoxes 获取用户的盲盒购买记录
func GetUserBlindBoxes(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的用户ID"})
		return
	}

	status := c.Query("status")

	query := `
		SELECT bp.id, bp.product_id, bp.merchant_id, bp.price, bp.status, bp.opened_at, bp.created_at,
		       p.name, p.images, p.blind_box_min_value, p.blind_box_max_value,
		       m.shop_name
		FROM blind_box_purchases bp
		JOIN products p ON bp.product_id = p.id
		JOIN merchants m ON bp.merchant_id = m.id
		WHERE bp.user_id = $1
	`
	args := []interface{}{userID}

	if status != "" && status != "all" {
		query += " AND bp.status = $2"
		args = append(args, status)
	}
	query += " ORDER BY bp.created_at DESC"

	rows, err := db.PG.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "获取失败"})
		return
	}
	defer rows.Close()

	var purchases []BlindBoxPurchase
	for rows.Next() {
		var p BlindBoxPurchase
		var openedAt sql.NullTime
		var createdAt time.Time
		var productName string
		var images pq.StringArray
		var minVal, maxVal sql.NullFloat64
		var shopName string

		err := rows.Scan(&p.ID, &p.ProductID, &p.MerchantID, &p.Price, &p.Status, &openedAt, &createdAt,
			&productName, &images, &minVal, &maxVal, &shopName)
		if err != nil {
			continue
		}

		p.CreatedAt = createdAt.Format("2006-01-02 15:04:05")
		if openedAt.Valid {
			p.OpenedAt = openedAt.Time.Format("2006-01-02 15:04:05")
		}

		p.Product = &BlindBoxProduct{
			ID:       p.ProductID,
			Name:     productName,
			Images:   images,
			MinValue: minVal.Float64,
			MaxValue: maxVal.Float64,
			ShopName: shopName,
		}

		// 如果已开启，获取内容
		if p.Status == "opened" {
			itemRows, _ := db.PG.Query(`
				SELECT id, item_name, item_value, item_image, 
				       COALESCE(item_description, ''),
				       item_expiry_date,
				       COALESCE(item_storage_condition, ''),
				       COALESCE(item_quantity, 1)
				FROM blind_box_items WHERE purchase_id = $1
			`, p.ID)
			if itemRows != nil {
				var totalValue float64
				for itemRows.Next() {
					var item BlindBoxItem
					var img, desc, storage sql.NullString
					var expiryDate sql.NullTime
					var qty int
					itemRows.Scan(&item.ID, &item.Name, &item.Value, &img, &desc, &expiryDate, &storage, &qty)
					item.Image = img.String
					item.Description = desc.String
					item.StorageCondition = storage.String
					item.Quantity = qty
					if item.Quantity == 0 {
						item.Quantity = 1
					}
					if expiryDate.Valid {
						item.ExpiryDate = expiryDate.Time.Format("2006-01-02")
					}
					p.Items = append(p.Items, item)
					totalValue += item.Value * float64(item.Quantity)
				}
				itemRows.Close()
				p.TotalValue = math.Round(totalValue*100) / 100
				p.Profit = math.Round((totalValue-p.Price)*100) / 100
			}
		}

		purchases = append(purchases, p)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": purchases})
}

// SetBlindBoxContents 商家设置盲盒内容
func SetBlindBoxContents(c *gin.Context) {
	productID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的商品ID"})
		return
	}

	var req struct {
		Contents []struct {
			ProductID        int     `json:"product_id"`
			Name             string  `json:"name"`
			Value            float64 `json:"value"`
			Image            string  `json:"image"`
			Description      string  `json:"description"`
			ExpiryDate       string  `json:"expiry_date"`
			StorageCondition string  `json:"storage_condition"`
			Quantity         int     `json:"quantity"`
			Probability      int     `json:"probability"`
		} `json:"contents"`
		DeductStock bool `json:"deduct_stock"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误"})
		return
	}

	// 验证是盲盒商品
	var isBlindBox bool
	err = db.PG.QueryRow(`SELECT is_blind_box FROM products WHERE id = $1`, productID).Scan(&isBlindBox)
	if err != nil || !isBlindBox {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "该商品不是盲盒"})
		return
	}

	// 开始事务
	tx, err := db.PG.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "系统错误"})
		return
	}
	defer tx.Rollback()

	// 删除旧内容
	_, err = tx.Exec(`DELETE FROM blind_box_contents WHERE blind_box_id = $1`, productID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "更新失败"})
		return
	}

	// 插入新内容并扣库存
	var minVal, maxVal float64
	for _, item := range req.Contents {
		quantity := item.Quantity
		if quantity <= 0 {
			quantity = 1
		}
		probability := item.Probability
		if probability <= 0 {
			probability = 100
		}

		// 如果有商品ID且需要扣库存
		if req.DeductStock && item.ProductID > 0 {
			result, err := tx.Exec(`
				UPDATE products SET stock = stock - $1 
				WHERE id = $2 AND stock >= $1 AND is_blind_box = false
			`, quantity, item.ProductID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "扣减库存失败"})
				return
			}
			affected, _ := result.RowsAffected()
			if affected == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "商品库存不足: " + item.Name})
				return
			}
		}

		_, err = tx.Exec(`
			INSERT INTO blind_box_contents (blind_box_id, source_product_id, product_name, product_value, product_image, product_description, product_expiry_date, product_storage_condition, quantity, probability)
			VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, '')::DATE, $8, $9, $10)
		`, productID, item.ProductID, item.Name, item.Value, item.Image, item.Description, item.ExpiryDate, item.StorageCondition, quantity, probability)
		if err != nil {
			continue
		}

		minVal += item.Value * float64(quantity)
		maxVal += item.Value * float64(quantity)
	}

	// 最小值取80%，最大值取120%
	minVal = minVal * 0.8
	maxVal = maxVal * 1.2

	_, err = tx.Exec(`
		UPDATE products SET blind_box_min_value = $1, blind_box_max_value = $2 WHERE id = $3
	`, minVal, maxVal, productID)

	if err = tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "设置成功"})
}

// GetBlindBoxContents 获取盲盒内容设置
func GetBlindBoxContents(c *gin.Context) {
	productID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的商品ID"})
		return
	}

	rows, err := db.PG.Query(`
		SELECT id, COALESCE(source_product_id, 0), product_name, product_value, product_image, 
		       COALESCE(product_description, ''), product_expiry_date, COALESCE(product_storage_condition, ''),
		       quantity, probability
		FROM blind_box_contents WHERE blind_box_id = $1
	`, productID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "获取失败"})
		return
	}
	defer rows.Close()

	var contents []map[string]interface{}
	for rows.Next() {
		var id, sourceProductID, quantity, probability int
		var name, description, storageCondition string
		var value float64
		var img sql.NullString
		var expiryDate sql.NullTime

		err := rows.Scan(&id, &sourceProductID, &name, &value, &img, &description, &expiryDate, &storageCondition, &quantity, &probability)
		if err != nil {
			continue
		}
		
		item := map[string]interface{}{
			"id":                id,
			"product_id":        sourceProductID,
			"name":              name,
			"value":             value,
			"image":             img.String,
			"description":       description,
			"storage_condition": storageCondition,
			"quantity":          quantity,
			"probability":       probability,
		}
		if expiryDate.Valid {
			item["expiry_date"] = expiryDate.Time.Format("2006-01-02")
		} else {
			item["expiry_date"] = ""
		}
		contents = append(contents, item)
	}

	// 确保返回空数组而不是null
	if contents == nil {
		contents = []map[string]interface{}{}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": contents})
}

// 从预设内容中随机选择商品 - 盲盒的核心是不确定性
// 概率(probability)表示该商品的稀有度：100=普通，50=稀有，10=超稀有
// 算法：使用加权随机选择，概率越低的商品越难抽到
func selectFromPresetItems(presetItems []BlindBoxContentItem, minVal, maxVal float64) []BlindBoxItem {
	var items []BlindBoxItem
	var totalValue float64

	if len(presetItems) == 0 {
		return items
	}

	// 计算总权重
	totalWeight := 0
	for _, item := range presetItems {
		totalWeight += item.Probability
	}

	// 创建可用商品池（复制一份）
	availablePool := make([]BlindBoxContentItem, len(presetItems))
	copy(availablePool, presetItems)

	// 目标：选择3-6个商品，总价值在minVal到maxVal之间
	targetCount := 3 + blindBoxRandInt(4) // 3-6个
	maxAttempts := 20                     // 防止无限循环

	for len(items) < targetCount && len(availablePool) > 0 && maxAttempts > 0 {
		maxAttempts--

		// 重新计算当前池的总权重
		currentTotalWeight := 0
		for _, item := range availablePool {
			currentTotalWeight += item.Probability
		}

		if currentTotalWeight == 0 {
			break
		}

		// 加权随机选择
		randomWeight := blindBoxRandInt(currentTotalWeight)
		cumulativeWeight := 0
		selectedIdx := -1

		for idx, item := range availablePool {
			cumulativeWeight += item.Probability
			if randomWeight < cumulativeWeight {
				selectedIdx = idx
				break
			}
		}

		if selectedIdx == -1 {
			selectedIdx = len(availablePool) - 1
		}

		selected := availablePool[selectedIdx]

		// 添加到结果
		items = append(items, BlindBoxItem{
			Name:             selected.Name,
			Value:            selected.Value,
			Image:            selected.Image,
			Description:      selected.Description,
			ExpiryDate:       selected.ExpiryDate,
			StorageCondition: selected.StorageCondition,
			Quantity:         selected.Quantity,
		})
		totalValue += selected.Value * float64(selected.Quantity)

		// 从池中移除已选商品（每种商品只能出现一次）
		availablePool = append(availablePool[:selectedIdx], availablePool[selectedIdx+1:]...)
	}

	// 如果总价值不够最低要求，继续从剩余池中添加
	for totalValue < minVal && len(availablePool) > 0 {
		// 选择价值最高的商品补充
		maxValueIdx := 0
		maxItemValue := availablePool[0].Value * float64(availablePool[0].Quantity)
		for idx, item := range availablePool {
			itemValue := item.Value * float64(item.Quantity)
			if itemValue > maxItemValue {
				maxItemValue = itemValue
				maxValueIdx = idx
			}
		}

		selected := availablePool[maxValueIdx]
		items = append(items, BlindBoxItem{
			Name:             selected.Name,
			Value:            selected.Value,
			Image:            selected.Image,
			Description:      selected.Description,
			ExpiryDate:       selected.ExpiryDate,
			StorageCondition: selected.StorageCondition,
			Quantity:         selected.Quantity,
		})
		totalValue += selected.Value * float64(selected.Quantity)
		availablePool = append(availablePool[:maxValueIdx], availablePool[maxValueIdx+1:]...)
	}

	// 如果还是没有商品，至少选一个
	if len(items) == 0 && len(presetItems) > 0 {
		preset := presetItems[blindBoxRandInt(len(presetItems))]
		items = append(items, BlindBoxItem{
			Name:             preset.Name,
			Value:            preset.Value,
			Image:            preset.Image,
			Description:      preset.Description,
			ExpiryDate:       preset.ExpiryDate,
			StorageCondition: preset.StorageCondition,
			Quantity:         preset.Quantity,
		})
	}

	return items
}

// 生成随机商品内容
func generateRandomItems(minValue, maxValue float64) []BlindBoxItem {
	itemPool := []struct {
		Name             string
		Image            string
		Description      string
		StorageCondition string
	}{
		{"法式牛角包", "", "外酥内软，奶香浓郁", "常温保存"},
		{"巧克力蛋糕", "", "浓郁巧克力风味，口感绵密", "冷藏保存"},
		{"草莓慕斯", "", "新鲜草莓制作，入口即化", "冷藏保存"},
		{"抹茶卷", "", "日式抹茶风味，清新不腻", "冷藏保存"},
		{"芝士蛋糕", "", "进口芝士，香浓细腻", "冷藏保存"},
		{"红豆面包", "", "精选红豆，甜而不腻", "常温保存"},
		{"肉松小贝", "", "酥脆肉松，咸香可口", "常温保存"},
		{"蛋黄酥", "", "咸蛋黄馅，层层酥脆", "常温保存"},
		{"提拉米苏", "", "意式经典，咖啡香浓", "冷藏保存"},
		{"奶油泡芙", "", "新鲜奶油，外脆内软", "冷藏保存"},
		{"蓝莓马芬", "", "新鲜蓝莓，松软可口", "常温保存"},
		{"椰蓉面包", "", "椰香四溢，口感丰富", "常温保存"},
		{"菠萝包", "", "港式经典，酥脆香甜", "常温保存"},
		{"蜂蜜蛋糕", "", "天然蜂蜜，绵软香甜", "常温保存"},
		{"黑森林蛋糕", "", "樱桃巧克力，经典搭配", "冷藏保存"},
	}

	itemCount := 3 + blindBoxRandInt(4) // 3-6个商品

	targetValue := minValue + blindBoxRandFloat64()*(maxValue-minValue)
	avgValue := targetValue / float64(itemCount)

	// 计算3天后的日期作为默认过期日期
	expiryDate := time.Now().AddDate(0, 0, 3).Format("2006-01-02")

	var items []BlindBoxItem
	usedIndexes := make(map[int]bool)

	for i := 0; i < itemCount; i++ {
		var idx int
		for {
			idx = blindBoxRandInt(len(itemPool))
			if !usedIndexes[idx] {
				usedIndexes[idx] = true
				break
			}
		}

		value := avgValue * (0.7 + blindBoxRandFloat64()*0.6)
		value = math.Round(value*100) / 100

		items = append(items, BlindBoxItem{
			Name:             itemPool[idx].Name,
			Value:            value,
			Image:            itemPool[idx].Image,
			Description:      itemPool[idx].Description,
			ExpiryDate:       expiryDate,
			StorageCondition: itemPool[idx].StorageCondition,
			Quantity:         1,
		})
	}

	return items
}

// GetMerchantBlindBoxStats 获取商家盲盒销售统计
func GetMerchantBlindBoxStats(c *gin.Context) {
	merchantID, err := strconv.Atoi(c.Param("merchant_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的商家ID"})
		return
	}

	// 获取时间范围参数
	days := 7 // 默认7天
	if d := c.Query("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			days = parsed
		}
	}

	startDate := time.Now().AddDate(0, 0, -days)

	// 统计盲盒销售数据
	var totalSales float64
	var totalCount int
	var openedCount int

	err = db.PG.QueryRow(`
		SELECT 
			COALESCE(SUM(price), 0),
			COUNT(*),
			COUNT(CASE WHEN status = 'opened' THEN 1 END)
		FROM blind_box_purchases 
		WHERE merchant_id = $1 AND created_at >= $2
	`, merchantID, startDate).Scan(&totalSales, &totalCount, &openedCount)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "获取统计失败"})
		return
	}

	// 获取每日销售数据
	rows, err := db.PG.Query(`
		SELECT 
			DATE(created_at) as date,
			COALESCE(SUM(price), 0) as sales,
			COUNT(*) as count
		FROM blind_box_purchases 
		WHERE merchant_id = $1 AND created_at >= $2
		GROUP BY DATE(created_at)
		ORDER BY date
	`, merchantID, startDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "获取统计失败"})
		return
	}
	defer rows.Close()

	var dailyData []map[string]interface{}
	for rows.Next() {
		var date time.Time
		var sales float64
		var count int
		if err := rows.Scan(&date, &sales, &count); err != nil {
			continue
		}
		dailyData = append(dailyData, map[string]interface{}{
			"date":  date.Format("01-02"),
			"sales": sales,
			"count": count,
		})
	}

	// 获取盲盒商品统计
	var activeBoxes int
	var totalStock int
	err = db.PG.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(stock), 0)
		FROM products 
		WHERE merchant_id = $1 AND is_blind_box = true AND status = 'active'
	`, merchantID).Scan(&activeBoxes, &totalStock)
	if err != nil {
		activeBoxes = 0
		totalStock = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"total_sales":   math.Round(totalSales*100) / 100,
			"total_count":   totalCount,
			"opened_count":  openedCount,
			"open_rate":     func() float64 { if totalCount > 0 { return float64(openedCount) / float64(totalCount) * 100 }; return 0 }(),
			"active_boxes":  activeBoxes,
			"total_stock":   totalStock,
			"daily_data":    dailyData,
		},
	})
}
