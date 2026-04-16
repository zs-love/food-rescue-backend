package handler

import (
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// 允许的图片类型
var allowedImageTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

// 最大文件大小 5MB
const maxFileSize = 5 * 1024 * 1024

// UploadFile 通用文件上传接口
func UploadFile(c *gin.Context) {
	// 获取上传类型 (avatar, product, shop, general)
	uploadType := c.DefaultPostForm("type", "general")

	// 获取上传的文件
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请选择要上传的文件"})
		return
	}
	defer file.Close()

	// 检查文件大小
	if header.Size > maxFileSize {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "文件大小不能超过5MB"})
		return
	}

	// 读取文件内容检测类型
	buffer := make([]byte, 512)
	_, err = file.Read(buffer)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "读取文件失败"})
		return
	}

	// 检测文件类型
	contentType := http.DetectContentType(buffer)
	if !allowedImageTypes[contentType] {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "只支持 JPG、PNG、GIF、WebP 格式的图片"})
		return
	}

	// 重置文件指针
	file.Seek(0, 0)

	// 生成唯一文件名
	ext := filepath.Ext(header.Filename)
	if ext == "" {
		// 根据内容类型推断扩展名
		switch contentType {
		case "image/jpeg":
			ext = ".jpg"
		case "image/png":
			ext = ".png"
		case "image/gif":
			ext = ".gif"
		case "image/webp":
			ext = ".webp"
		}
	}

	// 使用时间戳+MD5生成唯一文件名
	hash := md5.New()
	io.Copy(hash, file)
	file.Seek(0, 0)
	hashStr := fmt.Sprintf("%x", hash.Sum(nil))[:8]
	filename := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), hashStr, strings.ToLower(ext))

	// 根据类型确定存储目录
	var subDir string
	switch uploadType {
	case "avatar":
		subDir = "avatars"
	case "product":
		subDir = "products"
	case "shop":
		subDir = "shops"
	default:
		subDir = "general"
	}

	// 创建上传目录
	uploadDir := filepath.Join("uploads", subDir)
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "创建目录失败"})
		return
	}

	// 保存文件
	filePath := filepath.Join(uploadDir, filename)
	out, err := os.Create(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "保存文件失败"})
		return
	}
	defer out.Close()

	_, err = io.Copy(out, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "写入文件失败"})
		return
	}

	// 返回文件URL (相对路径)
	fileURL := fmt.Sprintf("/uploads/%s/%s", subDir, filename)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"url":      fileURL,
			"filename": filename,
			"size":     header.Size,
			"type":     contentType,
		},
	})
}
