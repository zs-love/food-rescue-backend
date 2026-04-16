package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
)

const baiduAK = "d9ZDU8MkMMgOV3xOqpnqHFAgdnMqUq9J"

// SearchPOI 百度地图 POI 搜索代理
func SearchPOI(c *gin.Context) {
	keyword := c.Query("keyword")
	city := c.DefaultQuery("city", "全国")

	if keyword == "" {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []gin.H{}})
		return
	}

	// 百度地点输入提示 API
	u := fmt.Sprintf("https://api.map.baidu.com/place/v2/suggestion?query=%s&region=%s&city_limit=false&output=json&ak=%s",
		url.QueryEscape(keyword), url.QueryEscape(city), baiduAK)

	resp, err := http.Get(u)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []gin.H{}})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Status int `json:"status"`
		Result []struct {
			Name     string `json:"name"`
			Address  string `json:"address"`
			Province string `json:"province"`
			City     string `json:"city"`
			District string `json:"district"`
		} `json:"result"`
	}

	if json.Unmarshal(body, &result) != nil || result.Status != 0 {
		// 兜底：百度地点搜索 API
		items := searchBaiduPlace(keyword, city)
		c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
		return
	}

	var items []gin.H
	for i, r := range result.Result {
		if r.Name == "" {
			continue
		}
		addr := r.Address
		if addr == "" {
			addr = r.Name
		}
		items = append(items, gin.H{
			"id":       fmt.Sprintf("%d", i),
			"title":    r.Name,
			"address":  addr,
			"province": r.Province,
			"city":     r.City,
			"district": r.District,
		})
		if len(items) >= 15 {
			break
		}
	}

	if items == nil {
		items = []gin.H{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

// 兜底：百度地点搜索 v2
func searchBaiduPlace(keyword, region string) []gin.H {
	u := fmt.Sprintf("https://api.map.baidu.com/place/v2/search?query=%s&region=%s&output=json&ak=%s&page_size=15",
		url.QueryEscape(keyword), url.QueryEscape(region), baiduAK)

	resp, err := http.Get(u)
	if err != nil {
		return []gin.H{}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Status  int `json:"status"`
		Results []struct {
			Name     string `json:"name"`
			Address  string `json:"address"`
			Province string `json:"province"`
			City     string `json:"city"`
			Area     string `json:"area"`
		} `json:"results"`
	}

	if json.Unmarshal(body, &result) != nil || result.Status != 0 {
		return []gin.H{}
	}

	var items []gin.H
	for i, r := range result.Results {
		items = append(items, gin.H{
			"id":       fmt.Sprintf("%d", i),
			"title":    r.Name,
			"address":  r.Address,
			"province": r.Province,
			"city":     r.City,
			"district": r.Area,
		})
	}
	if items == nil {
		items = []gin.H{}
	}
	return items
}
