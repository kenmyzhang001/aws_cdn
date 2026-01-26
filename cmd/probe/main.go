package main

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// ProbeRequest 请求结构体
type ProbeRequest struct {
	URLs []string `json:"urls" binding:"required"`
}

// ProbeResponse 响应结构体
type ProbeResponse struct {
	AvailableURLs []string `json:"available_urls"`
}

// probeURL 检测单个链接是否可用（返回200状态码）
func probeURL(url string) bool {
	client := &http.Client{
		Timeout: 500 * time.Millisecond, // 设置2秒超时
	}

	resp, err := client.Head(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// 检查状态码是否为200
	return resp.StatusCode == http.StatusOK
}

// probeURLs 并发检测多个链接
func probeURLs(urls []string) []string {
	var availableURLs []string
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, url := range urls {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			if probeURL(u) {
				mu.Lock()
				availableURLs = append(availableURLs, u)
				mu.Unlock()
			}
		}(url)
	}

	wg.Wait()
	return availableURLs
}

// probeHandler 探活接口处理函数
func probeHandler(c *gin.Context) {
	var req ProbeRequest

	// 绑定JSON请求体
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求格式，需要urls字段（字符串数组）",
		})
		return
	}

	// 检测链接
	availableURLs := probeURLs(req.URLs)

	// 返回可用的链接数组
	c.JSON(http.StatusOK, ProbeResponse{
		AvailableURLs: availableURLs,
	})
}

func main() {
	// 创建Gin路由
	r := gin.Default()

	// 定义探活接口
	r.POST("/probe", probeHandler)

	// 可选：添加健康检查接口
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	// 启动服务器，监听8080端口
	r.Run(":8080")
}
