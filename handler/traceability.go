package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"food-rescue/db"

	"github.com/gin-gonic/gin"
)

// 商品溯源信息
type TraceabilityInfo struct {
	ProductID              int    `json:"product_id"`
	SupplierName           string `json:"supplier_name"`
	SupplierLicense        string `json:"supplier_license"`
	PurchaseDate           string `json:"purchase_date"`
	PurchaseBatch          string `json:"purchase_batch"`
	PurchaseQuantity       int    `json:"purchase_quantity"`
	PurchaseCertificate    string `json:"purchase_certificate"`
	InspectionDate         string `json:"inspection_date"`
	InspectionResult       string `json:"inspection_result"`
	InspectionCertificate  string `json:"inspection_certificate"`
	StorageTemperature     string `json:"storage_temperature"`
	StorageHumidity        string `json:"storage_humidity"`
	StorageLocation        string `json:"storage_location"`
}

// GetProductTraceability 获取商品溯源信息
func GetProductTraceability(c *gin.Context) {
	productID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的商品ID"})
		return
	}

	var info TraceabilityInfo
	var purchaseDate, inspectionDate sql.NullTime
	var supplierName, supplierLicense, purchaseBatch, purchaseCert sql.NullString
	var inspectionResult, inspectionCert sql.NullString
	var storageTemp, storageHumidity, storageLocation sql.NullString
	var purchaseQty sql.NullInt64

	err = db.PG.QueryRow(`
		SELECT product_id, supplier_name, supplier_license, 
		       purchase_date, purchase_batch, purchase_quantity, purchase_certificate,
		       inspection_date, inspection_result, inspection_certificate,
		       storage_temperature, storage_humidity, storage_location
		FROM product_traceability WHERE product_id = $1
	`, productID).Scan(
		&info.ProductID, &supplierName, &supplierLicense,
		&purchaseDate, &purchaseBatch, &purchaseQty, &purchaseCert,
		&inspectionDate, &inspectionResult, &inspectionCert,
		&storageTemp, &storageHumidity, &storageLocation,
	)

	if err == sql.ErrNoRows {
		// 返回空的溯源信息
		c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "获取溯源信息失败"})
		return
	}

	info.SupplierName = supplierName.String
	info.SupplierLicense = supplierLicense.String
	info.PurchaseBatch = purchaseBatch.String
	info.PurchaseQuantity = int(purchaseQty.Int64)
	info.PurchaseCertificate = purchaseCert.String
	info.InspectionResult = inspectionResult.String
	info.InspectionCertificate = inspectionCert.String
	info.StorageTemperature = storageTemp.String
	info.StorageHumidity = storageHumidity.String
	info.StorageLocation = storageLocation.String

	if purchaseDate.Valid {
		info.PurchaseDate = purchaseDate.Time.Format("2006-01-02")
	}
	if inspectionDate.Valid {
		info.InspectionDate = inspectionDate.Time.Format("2006-01-02")
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": info})
}

// SetProductTraceability 设置商品溯源信息
func SetProductTraceability(c *gin.Context) {
	productID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的商品ID"})
		return
	}

	var req TraceabilityInfo
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "参数错误"})
		return
	}

	// 使用 UPSERT
	_, err = db.PG.Exec(`
		INSERT INTO product_traceability (
			product_id, supplier_name, supplier_license,
			purchase_date, purchase_batch, purchase_quantity, purchase_certificate,
			inspection_date, inspection_result, inspection_certificate,
			storage_temperature, storage_humidity, storage_location, updated_at
		) VALUES ($1, $2, $3, NULLIF($4, '')::DATE, $5, $6, $7, NULLIF($8, '')::DATE, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (product_id) DO UPDATE SET
			supplier_name = EXCLUDED.supplier_name,
			supplier_license = EXCLUDED.supplier_license,
			purchase_date = EXCLUDED.purchase_date,
			purchase_batch = EXCLUDED.purchase_batch,
			purchase_quantity = EXCLUDED.purchase_quantity,
			purchase_certificate = EXCLUDED.purchase_certificate,
			inspection_date = EXCLUDED.inspection_date,
			inspection_result = EXCLUDED.inspection_result,
			inspection_certificate = EXCLUDED.inspection_certificate,
			storage_temperature = EXCLUDED.storage_temperature,
			storage_humidity = EXCLUDED.storage_humidity,
			storage_location = EXCLUDED.storage_location,
			updated_at = EXCLUDED.updated_at
	`, productID, req.SupplierName, req.SupplierLicense,
		req.PurchaseDate, req.PurchaseBatch, req.PurchaseQuantity, req.PurchaseCertificate,
		req.InspectionDate, req.InspectionResult, req.InspectionCertificate,
		req.StorageTemperature, req.StorageHumidity, req.StorageLocation, time.Now())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "保存溯源信息失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "保存成功"})
}

// GenerateTraceQRCode 生成溯源二维码数据（模拟）
func GenerateTraceQRCode(c *gin.Context) {
	productID := c.Param("id")
	
	// 生成溯源链接
	qrData := "https://trace.foodrescue.com/product/" + productID
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"qr_data": qrData,
			"product_id": productID,
		},
	})
}
