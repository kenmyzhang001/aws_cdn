package main

import (
	"aws_cdn/internal/config"
	"aws_cdn/internal/database"
	"aws_cdn/internal/models"
	"fmt"
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ProbeRequest 请求结构体
type ProbeRequest struct {
	URLs []string `json:"urls" binding:"required"`
	Type string   `json:"type"`
}

// ProbeResponse 响应结构体
type ProbeResponse struct {
	AvailableURLs []string `json:"available_urls"`
}

// probeURLs 从数据库查询链接的探测结果，按照 speed_kbps 倒序返回
func probeURLs(db *gorm.DB, urls []string) []string {
	if len(urls) == 0 {
		return []string{}
	}

	var results []models.SpeedProbeResult

	// 一次性查询所有URL的探测结果
	err := db.Where("url IN ? AND status = ? AND created_at > ?",
		urls,
		models.SpeedProbeStatusSuccess,
		time.Now().Add(-time.Minute*35)).
		Order("speed_kbps DESC").
		Find(&results).Error

	if err != nil {
		log.Printf("查询探测结果失败: %v", err)
		return []string{}
	}

	// 使用map去重，每个URL只保留speed最高的那条记录
	urlMap := make(map[string]models.SpeedProbeResult)
	for _, result := range results {
		// 如果URL不存在，或者当前记录速度更快，则更新
		if existing, exists := urlMap[result.URL]; !exists || result.SpeedKbps > existing.SpeedKbps {
			urlMap[result.URL] = result
		}
	}

	// 按原始urls顺序返回可用的URL
	var availableURLs []string
	for _, url := range urls {
		if result, exists := urlMap[url]; exists {
			fmt.Printf("url: %s, speed: %.2f, created_at: %v\n", result.URL, result.SpeedKbps, result.CreatedAt)
			availableURLs = append(availableURLs, result.URL)
		}
	}

	return availableURLs
}

func probeURLsByType(db *gorm.DB, urls []string) []string {
	if len(urls) == 0 {
		return []string{}
	}

	var results []models.SpeedProbeResult

	// 一次性查询所有URL的探测结果
	err := db.Where("url IN ? AND status = ? AND created_at > ?",
		urls,
		models.SpeedProbeStatusSuccess,
		time.Now().Add(-time.Minute*35)).
		Order("speed_kbps DESC").
		Find(&results).Error

	if err != nil {
		log.Printf("查询探测结果失败: %v", err)
		return []string{}
	}

	// 使用map去重，每个URL只保留speed最高的那条记录
	urlMap := make(map[string]models.SpeedProbeResult)
	for _, result := range results {
		// 如果URL不存在，或者当前记录速度更快，则更新
		if existing, exists := urlMap[result.URL]; !exists || result.SpeedKbps > existing.SpeedKbps {
			urlMap[result.URL] = result
		}
	}

	// 将结果转换为数组，按speed_kbps倒序排序
	var sortedResults []models.SpeedProbeResult
	for _, result := range urlMap {
		sortedResults = append(sortedResults, result)
	}

	sort.Slice(sortedResults, func(i, j int) bool {
		return sortedResults[i].SpeedKbps > sortedResults[j].SpeedKbps
	})

	// 返回按速度排序的URL列表
	var availableURLs []string
	for _, result := range sortedResults {
		fmt.Printf("url: %s, speed: %.2f, created_at: %v\n", result.URL, result.SpeedKbps, result.CreatedAt)
		availableURLs = append(availableURLs, result.URL)
	}

	return availableURLs
}

// probeHandler 探活接口处理函数
func probeHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ProbeRequest
		var availableURLs []string
		// 绑定JSON请求体
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "无效的请求格式，需要urls字段（字符串数组）",
			})
			return
		}

		// 从数据库查询链接
		if req.Type == "" {
			availableURLs = probeURLs(db, req.URLs)
		} else {
			availableURLs = probeURLsByType(db, req.URLs)
		}

		// 打印出参
		log.Printf("[ProbeHandler] 请求参数 - Type: %s, URLs数量: %d, URLs: %v, 响应结果 - 可用URLs数量: %d, 可用URLs: %v", req.Type, len(req.URLs), req.URLs, len(availableURLs), availableURLs)

		// 返回可用的链接数组
		c.JSON(http.StatusOK, ProbeResponse{
			AvailableURLs: availableURLs,
		})
	}
}

func main() {
	// 加载配置
	cfg := config.Load()

	// 初始化数据库连接
	db, err := database.Initialize(database.DatabaseConfig{
		Host:     cfg.Database.Host,
		Port:     cfg.Database.Port,
		User:     cfg.Database.User,
		Password: cfg.Database.Password,
		DBName:   cfg.Database.DBName,
		SSLMode:  cfg.Database.SSLMode,
	})
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}

	// 创建Gin路由
	r := gin.Default()

	// 定义探活接口
	r.POST("/probe", probeHandler(db))

	// 可选：添加健康检查接口
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	// 启动服务器，监听8080端口
	r.Run(":8080")
}
