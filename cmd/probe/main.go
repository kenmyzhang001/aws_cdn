package main

import (
	"aws_cdn/internal/config"
	"aws_cdn/internal/database"
	"aws_cdn/internal/models"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ProbeRequest 请求结构体
type ProbeRequest struct {
	URLs []string `json:"urls" binding:"required"`
}

// ProbeResponse 响应结构体
type ProbeResponse struct {
	AvailableURLs []string `json:"available_urls"`
}

// probeURLs 从数据库查询链接的探测结果，按照 speed_kbps 倒序返回
func probeURLs(db *gorm.DB, urls []string) []string {
	var availableURLs []string

	// 对于每个URL，查询数据库中的探测结果
	for _, url := range urls {
		var result models.SpeedProbeResult

		// 查询该URL的探测结果，按照 speed_kbps 倒序，只取第一条（最快的记录）
		err := db.Where("url = ? AND status = ? AND created_at > ?", url, models.SpeedProbeStatusSuccess, time.Now().Add(-time.Minute*35)).
			Order("speed_kbps DESC").
			First(&result).Error

		// 如果查询成功，说明该URL有可用的探测记录
		if err == nil {
			fmt.Println("url:", result.URL, ",speed:", result.SpeedKbps, ",created_at:", result.CreatedAt)
			availableURLs = append(availableURLs, result.URL)
		}
	}

	return availableURLs
}

// probeHandler 探活接口处理函数
func probeHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ProbeRequest

		// 绑定JSON请求体
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "无效的请求格式，需要urls字段（字符串数组）",
			})
			return
		}

		// 从数据库查询链接
		availableURLs := probeURLs(db, req.URLs)

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
