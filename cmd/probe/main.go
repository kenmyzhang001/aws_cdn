package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"gopkg.in/natefinch/lumberjack.v2"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const CheckTime = -time.Minute * 30

// SpeedProbeStatus 探测状态
type SpeedProbeStatus string

const (
	SpeedProbeStatusSuccess SpeedProbeStatus = "success" // 成功
	SpeedProbeStatusFailed  SpeedProbeStatus = "failed"  // 失败
	SpeedProbeStatusTimeout SpeedProbeStatus = "timeout" // 超时
)

type SpeedProbeResult struct {
	ID             uint             `json:"id" gorm:"primaryKey"`
	URL            string           `json:"url" gorm:"type:varchar(1000);not null;index:idx_url"`                       // 探测的URL
	ClientIP       string           `json:"client_ip" gorm:"type:varchar(50);not null;index:idx_client_ip"`             // 客户端IP地址
	SpeedKbps      float64          `json:"speed_kbps" gorm:"type:decimal(10,2);not null"`                              // 下载速度 KB/s
	FileSize       *int64           `json:"file_size,omitempty" gorm:"default:null"`                                    // 文件大小（字节）
	DownloadTimeMs *int64           `json:"download_time_ms,omitempty" gorm:"default:null"`                             // 下载耗时（毫秒）
	Status         SpeedProbeStatus `json:"status" gorm:"type:varchar(20);not null;default:'success';index:idx_status"` // 探测状态
	ErrorMessage   string           `json:"error_message,omitempty" gorm:"type:text"`                                   // 错误信息
	UserAgent      string           `json:"user_agent,omitempty" gorm:"type:varchar(500)"`                              // 客户端User-Agent
	CreatedAt      time.Time        `json:"created_at" gorm:"index:idx_created_at;index:idx_url_ip_created"`            // 创建时间
}

// TableName 指定表名
func (SpeedProbeResult) TableName() string {
	return "speed_probe_results"
}

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
func probeURLs(db *gorm.DB, urls []string, traceID string) []string {
	log.Printf("[TraceID: %s] probeURLs 开始 - URLs数量: %d, URLs: %v", traceID, len(urls), urls)
	startTime := time.Now()

	if len(urls) == 0 {
		log.Printf("[TraceID: %s] probeURLs 结束 - URLs为空，耗时: %v", traceID, time.Since(startTime))
		return []string{}
	}

	var results []SpeedProbeResult
	queryStartTime := time.Now()

	// 一次性查询所有URL的探测结果
	err := db.Where("url IN ? AND created_at > ?",
		urls,
		time.Now().Add(CheckTime)).
		Order("speed_kbps DESC").
		Find(&results).Error

	queryDuration := time.Since(queryStartTime)
	log.Printf("[TraceID: %s] 数据库查询完成 - 耗时: %v, 结果数量: %d, urls: %v", traceID, queryDuration, len(results), urls)

	if err != nil {
		log.Printf("[TraceID: %s] 查询探测结果失败 - 错误: %v, 耗时: %v, urls: %v", traceID, err, time.Since(startTime), urls)
		return []string{}
	}

	if len(results) == 0 {
		log.Printf("[TraceID: %s] 数据库无结果，切换到实时探测模式, urls: %v", traceID, urls)
		probeResults := realTimeProbe(urls, traceID)
		log.Printf("[TraceID: %s] probeURLs 结束（实时探测） - 结果数量: %d, 总耗时: %v, urls: %v", traceID, len(probeResults), time.Since(startTime), urls)
		return probeResults
	}

	// 使用map去重，每个URL只保留speed最高的那条记录
	urlMap := make(map[string]SpeedProbeResult)
	for _, result := range results {
		// 如果URL不存在，或者当前记录速度更快，则更新
		if existing, exists := urlMap[result.URL]; !exists || result.SpeedKbps > existing.SpeedKbps {
			urlMap[result.URL] = result
		}
	}
	log.Printf("[TraceID: %s] 去重后唯一URL数量: %d, urls: %v", traceID, len(urlMap), urls)

	// 检查10分钟内是否有speed_kbps=0的记录，标记为不可用
	unavailableCheckStart := time.Now()
	var unavailableURLs []string
	err = db.Select("DISTINCT url").
		Where("url IN ? AND created_at > ? AND speed_kbps = 0",
			urls,
			time.Now().Add(CheckTime)).
		Find(&unavailableURLs).Error

	if err != nil {
		log.Printf("[TraceID: %s] 查询不可用URL失败 - 错误: %v", traceID, err)
	}

	// 构建不可用URL的map，便于快速查找
	unavailableMap := make(map[string]bool)
	for _, url := range unavailableURLs {
		unavailableMap[url] = true
	}
	log.Printf("[TraceID: %s] %s内不可用URL数量: %d, 检查耗时: %v, 不可用URLs: %v",
		traceID, CheckTime.String(), len(unavailableURLs), time.Since(unavailableCheckStart), unavailableURLs)

	// 按原始urls顺序返回可用的URL
	var availableURLs []string
	for _, url := range urls {
		// 如果在10分钟内有speed_kbps=0的记录，跳过
		if unavailableMap[url] {
			log.Printf("[TraceID: %s] 跳过URL（%s内不可用）: %s, urls: %v", traceID, CheckTime.String(), url, urls)
			continue
		}

		if result, exists := urlMap[url]; exists {
			log.Printf("[TraceID: %s] 匹配到URL: %s, 速度: %.2f KB/s, 记录时间: %v, urls: %v",
				traceID, result.URL, result.SpeedKbps, result.CreatedAt.Format("2006-01-02 15:04:05"), urls)
			availableURLs = append(availableURLs, result.URL)
		} else {
			log.Printf("[TraceID: %s] 未匹配到URL: %s, urls: %v", traceID, url, urls)
		}
	}

	log.Printf("[TraceID: %s] probeURLs 结束 - 返回URLs数量: %d, 总耗时: %v, urls: %v", traceID, len(availableURLs), time.Since(startTime), urls)
	return availableURLs
}

func probeURLsByType(db *gorm.DB, urls []string, traceID string) []string {
	log.Printf("[TraceID: %s] probeURLsByType 开始 - URLs数量: %d, urls: %v", traceID, len(urls), urls)
	startTime := time.Now()

	if len(urls) == 0 {
		log.Printf("[TraceID: %s] probeURLsByType 结束 - URLs为空，耗时: %v, urls: %v", traceID, time.Since(startTime), urls)
		return []string{}
	}

	var results []SpeedProbeResult
	queryStartTime := time.Now()

	// 一次性查询所有URL的探测结果
	err := db.Where("url IN ? AND created_at > ?",
		urls,
		time.Now().Add(-time.Minute*35)).
		Order("speed_kbps DESC").
		Find(&results).Error

	queryDuration := time.Since(queryStartTime)
	log.Printf("[TraceID: %s] 数据库查询完成（按类型） - 耗时: %v, 结果数量: %d, urls: %v", traceID, queryDuration, len(results), urls)

	if err != nil {
		log.Printf("[TraceID: %s] 查询探测结果失败（按类型） - 错误: %v, 耗时: %v, urls: %v", traceID, err, time.Since(startTime), urls)
		return []string{}
	}

	if len(results) == 0 {
		log.Printf("[TraceID: %s] 数据库无结果（按类型），切换到实时探测模式, urls: %v", traceID, urls)
		probeResults := realTimeProbe(urls, traceID)
		log.Printf("[TraceID: %s] probeURLsByType 结束（实时探测） - 结果数量: %d, 总耗时: %v, urls: %v", traceID, len(probeResults), time.Since(startTime), urls)
		return probeResults
	}

	// 使用map去重，每个URL只保留speed最高的那条记录
	urlMap := make(map[string]SpeedProbeResult)
	for _, result := range results {
		// 如果URL不存在，或者当前记录速度更快，则更新
		if existing, exists := urlMap[result.URL]; !exists || result.SpeedKbps > existing.SpeedKbps {
			urlMap[result.URL] = result
		}
	}
	log.Printf("[TraceID: %s] 去重后唯一URL数量: %d, urls: %v", traceID, len(urlMap), urls)

	// 检查60分钟内是否有speed_kbps=0的记录，标记为不可用
	unavailableCheckStart := time.Now()
	var unavailableURLs []string
	err = db.Select("DISTINCT url").
		Where("url IN ? AND created_at > ? AND speed_kbps = 0",
			urls,
			time.Now().Add(CheckTime)).
		Find(&unavailableURLs).Error

	if err != nil {
		log.Printf("[TraceID: %s] 查询不可用URL失败（按类型） - 错误: %v", traceID, err)
	}

	// 构建不可用URL的map，便于快速查找
	unavailableMap := make(map[string]bool)
	for _, url := range unavailableURLs {
		unavailableMap[url] = true
	}
	log.Printf("[TraceID: %s] %s内不可用URL数量（按类型）: %d, 检查耗时: %v, 不可用URLs: %v",
		traceID, CheckTime.String(), len(unavailableURLs), time.Since(unavailableCheckStart), unavailableURLs)

	// 将结果转换为数组，过滤掉不可用的URL，按speed_kbps倒序排序
	var sortedResults []SpeedProbeResult
	for _, result := range urlMap {
		// 如果在%s内有speed_kbps=0的记录，跳过
		if unavailableMap[result.URL] {
			log.Printf("[TraceID: %s] 跳过URL（%s内不可用，按类型）: %s, urls: %v", traceID, CheckTime.String(), result.URL, urls)
			continue
		}
		sortedResults = append(sortedResults, result)
	}

	sort.Slice(sortedResults, func(i, j int) bool {
		return sortedResults[i].SpeedKbps > sortedResults[j].SpeedKbps
	})

	log.Printf("[TraceID: %s] 按速度排序完成（过滤后）, 结果数量: %d, urls: %v", traceID, len(sortedResults), urls)

	// 返回按速度排序的URL列表
	var availableURLs []string
	for idx, result := range sortedResults {
		log.Printf("[TraceID: %s] 排序结果 #%d: URL: %s, 速度: %.2f KB/s, 记录时间: %v, urls: %v",
			traceID, idx+1, result.URL, result.SpeedKbps, result.CreatedAt.Format("2006-01-02 15:04:05"), urls)
		availableURLs = append(availableURLs, result.URL)
	}

	log.Printf("[TraceID: %s] probeURLsByType 结束 - 返回URLs数量: %d, 总耗时: %v, urls: %v", traceID, len(availableURLs), time.Since(startTime), urls)
	return availableURLs
}

// probeRedirectTarget 探测重定向目标URL是否可下载
// 不再跟随重定向，直接检查目标URL的响应
func probeRedirectTarget(url string, traceID string) (float64, error) {
	log.Printf("[TraceID: %s] probeRedirectTarget 开始探测重定向目标 - URL: %s", traceID, url)
	overallStart := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		log.Printf("[TraceID: %s] probeRedirectTarget 失败 - URL: %s, 错误: 创建请求失败 - %v",
			traceID, url, err)
		return 0, fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置Range header
	req.Header.Set("Range", "bytes=0-1023")

	// 不跟随重定向的客户端
	client := &http.Client{
		Timeout: 2 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// 不跟随重定向
			return http.ErrUseLastResponse
		},
	}

	requestStartTime := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[TraceID: %s] probeRedirectTarget 失败 - URL: %s, 错误: HTTP请求失败 - %v",
			traceID, url, err)
		return 0, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	requestDuration := time.Since(requestStartTime)

	// 获取响应头信息
	contentType := resp.Header.Get("Content-Type")
	contentDisposition := resp.Header.Get("Content-Disposition")

	log.Printf("[TraceID: %s] 重定向目标响应 - URL: %s, 状态码: %d, Content-Type: %s, Content-Disposition: %s, 请求耗时: %v",
		traceID, url, resp.StatusCode, contentType, contentDisposition, requestDuration)

	// 判断是否为有效的下载链接
	isValid := false
	matchReason := ""

	if resp.StatusCode == http.StatusPartialContent {
		isValid = true
		matchReason = "重定向目标支持Range下载(206)"
	} else if resp.StatusCode == http.StatusOK {
		// 如果是200状态码，检查Content-Type和Content-Disposition
		if strings.Contains(strings.ToLower(contentDisposition), ".apk") {
			isValid = true
			matchReason = "重定向目标Content-Disposition包含.apk"
		} else if strings.Contains(strings.ToLower(contentType), "application/vnd.android.package-archive") {
			isValid = true
			matchReason = "重定向目标Content-Type为APK类型"
		}
	}

	if !isValid {
		log.Printf("[TraceID: %s] 重定向目标不满足下载条件 - URL: %s, 状态码: %d, Content-Type: %s, Content-Disposition: %s",
			traceID, url, resp.StatusCode, contentType, contentDisposition)
		return 0, fmt.Errorf("重定向目标不满足下载条件: 状态码=%d, Content-Type=%s, Content-Disposition=%s",
			resp.StatusCode, contentType, contentDisposition)
	}

	// 计算速度评分
	speedKbps := 1024.0 / float64(requestDuration.Milliseconds())
	log.Printf("[TraceID: %s] probeRedirectTarget 成功 - URL: %s, 匹配原因: %s, 状态码: %d, 评估速度: %.2f KB/s, 总耗时: %v",
		traceID, url, matchReason, resp.StatusCode, speedKbps, time.Since(overallStart))
	return speedKbps, nil
}

// probeURL 实时探测单个URL，通过以下任一条件判断可下载性：
// 1. HTTP 206 状态码 (支持Range下载)
// 2. Content-Disposition 包含 .apk 文件名
// 3. Content-Type 为 application/vnd.android.package-archive
func probeURL(url string, traceID string) (float64, error) {
	log.Printf("[TraceID: %s] probeURL 开始探测 - URL: %s", traceID, url)
	overallStart := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		log.Printf("[TraceID: %s] probeURL 失败 - URL: %s, 错误: 创建请求失败 - %v, 耗时: %v",
			traceID, url, err, time.Since(overallStart))
		return 0, fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置Range header，请求前1KB即可（只用于检测是否支持Range）
	req.Header.Set("Range", "bytes=0-1023")
	log.Printf("[TraceID: %s] 设置Range请求头 - URL: %s, Range: bytes=0-1023", traceID, url)

	// 自定义HTTP客户端，允许跟随重定向
	client := &http.Client{
		Timeout: 2 * time.Second, // 增加超时时间以支持重定向
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// 最多允许10次重定向
			if len(via) >= 10 {
				return fmt.Errorf("重定向次数过多")
			}
			return nil
		},
	}

	requestStartTime := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[TraceID: %s] probeURL 失败 - URL: %s, 错误: HTTP请求失败 - %v, 请求耗时: %v, 总耗时: %v",
			traceID, url, err, time.Since(requestStartTime), time.Since(overallStart))
		return 0, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	requestDuration := time.Since(requestStartTime)
	totalDuration := time.Since(overallStart)

	// 获取响应头信息
	contentType := resp.Header.Get("Content-Type")
	contentDisposition := resp.Header.Get("Content-Disposition")

	log.Printf("[TraceID: %s] HTTP响应收到 - URL: %s, 状态码: %d, Content-Length: %d, Content-Type: %s, Content-Disposition: %s, 请求耗时: %v",
		traceID, url, resp.StatusCode, resp.ContentLength, contentType, contentDisposition, requestDuration)

	// 判断是否为有效的APK下载链接（满足以下任一条件即可）：
	// 1. 状态码为 206 (Partial Content)
	// 2. Content-Disposition 包含 .apk 文件名
	// 3. Content-Type 为 application/vnd.android.package-archive
	// 4. 如果是重定向状态码，检查最终重定向后的URL是否可下载
	isValid := false
	matchReason := ""

	if resp.StatusCode == http.StatusPartialContent {
		isValid = true
		matchReason = "状态码206(支持Range下载)"
	} else if strings.Contains(strings.ToLower(contentDisposition), ".apk") {
		isValid = true
		matchReason = "Content-Disposition包含.apk文件名"
	} else if strings.Contains(strings.ToLower(contentType), "application/vnd.android.package-archive") {
		isValid = true
		matchReason = "Content-Type为APK类型"
	} else if resp.StatusCode == http.StatusTemporaryRedirect ||
		resp.StatusCode == http.StatusMovedPermanently ||
		resp.StatusCode == http.StatusFound {
		// 处理重定向情况
		location := resp.Header.Get("Location")
		if location == "" {
			log.Printf("[TraceID: %s] 重定向但未找到Location头 - URL: %s, 状态码: %d",
				traceID, url, resp.StatusCode)
			return 0, fmt.Errorf("重定向但未找到Location头")
		}

		log.Printf("[TraceID: %s] 检测到重定向 - 原URL: %s, 状态码: %d, 目标URL: %s",
			traceID, url, resp.StatusCode, location)

		// 对重定向后的URL进行探测
		finalSpeed, finalErr := probeRedirectTarget(location, traceID)
		if finalErr != nil {
			log.Printf("[TraceID: %s] 重定向目标不可用 - 目标URL: %s, 错误: %v",
				traceID, location, finalErr)
			return 0, fmt.Errorf("重定向目标不可用: %w", finalErr)
		}

		log.Printf("[TraceID: %s] 重定向目标可用 - 原URL: %s -> 目标URL: %s, 速度: %.2f KB/s",
			traceID, url, location, finalSpeed)
		return finalSpeed, nil
	}

	if isValid {
		speedKbps := 1024.0 / float64(requestDuration.Milliseconds())
		log.Printf("[TraceID: %s] probeURL 成功 - URL: %s, 匹配原因: %s, 状态码: %d, 响应时间: %v, 评估速度: %.2f KB/s, 总耗时: %v",
			traceID, url, matchReason, resp.StatusCode, requestDuration, speedKbps, totalDuration)
		return speedKbps, nil
	}

	// 如果不满足任何条件，记录详细信息并返回错误
	log.Printf("[TraceID: %s] probeURL 失败 - URL: %s, 状态码: %d, Content-Type: %s, Content-Disposition: %s, 响应时间: %v, 总耗时: %v",
		traceID, url, resp.StatusCode, contentType, contentDisposition, requestDuration, totalDuration)
	return 0, fmt.Errorf("不满足下载条件: 状态码=%d, Content-Type=%s, Content-Disposition=%s", resp.StatusCode, contentType, contentDisposition)
}

// realTimeProbe 并发实时探测多个URL，返回按原始顺序排序的URL列表
func realTimeProbe(urls []string, traceID string) []string {
	log.Printf("[TraceID: %s] realTimeProbe 开始 - 待探测URLs数量: %d", traceID, len(urls))
	startTime := time.Now()

	type urlSpeed struct {
		url   string
		speed float64
		index int // 添加index字段记录原始位置
	}

	var (
		wg         sync.WaitGroup
		mu         sync.Mutex
		results    []urlSpeed
		successCnt int
		failedCnt  int
	)

	// 并发探测所有 URL
	log.Printf("[TraceID: %s] 启动并发探测 - Goroutines数量: %d", traceID, len(urls))
	for idx, url := range urls {
		wg.Add(1)
		go func(index int, u string) {
			defer wg.Done()
			goroutineStart := time.Now()
			log.Printf("[TraceID: %s] Goroutine #%d 开始探测 - URL: %s", traceID, index+1, u)

			speed, err := probeURL(u, traceID)
			goroutineDuration := time.Since(goroutineStart)

			if err != nil {
				log.Printf("[TraceID: %s] Goroutine #%d 探测失败 - URL: %s, 错误: %v, 耗时: %v",
					traceID, index+1, u, err, goroutineDuration)
				mu.Lock()
				failedCnt++
				mu.Unlock()
				return
			}

			log.Printf("[TraceID: %s] Goroutine #%d 探测成功 - URL: %s, 速度: %.2f KB/s, 耗时: %v",
				traceID, index+1, u, speed, goroutineDuration)

			// 使用互斥锁保护共享数据
			mu.Lock()
			results = append(results, urlSpeed{url: u, speed: speed, index: index})
			successCnt++
			mu.Unlock()
		}(idx, url)
	}

	// 等待所有 goroutine 完成
	log.Printf("[TraceID: %s] 等待所有Goroutines完成...", traceID)
	wg.Wait()
	waitDuration := time.Since(startTime)
	log.Printf("[TraceID: %s] 所有Goroutines已完成 - 成功: %d, 失败: %d, 总耗时: %v",
		traceID, successCnt, failedCnt, waitDuration)

	// 按原始顺序排序结果
	sort.Slice(results, func(i, j int) bool {
		return results[i].index < results[j].index
	})

	// 提取URL列表（保持原始顺序）
	var sortedURLs []string
	for idx, result := range results {
		log.Printf("[TraceID: %s] 实时探测结果（按原始顺序）#%d: URL: %s, 速度: %.2f KB/s",
			traceID, idx+1, result.url, result.speed)
		sortedURLs = append(sortedURLs, result.url)
	}

	log.Printf("[TraceID: %s] realTimeProbe 结束 - 返回URLs数量: %d, 总耗时: %v, urls: %v",
		traceID, len(sortedURLs), time.Since(startTime), urls)
	return sortedURLs
}

// probeHandler 探活接口处理函数
func probeHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 生成 trace_id
		traceID := uuid.New().String()
		requestStartTime := time.Now()

		log.Printf("[TraceID: %s] ========== 新请求开始 ==========", traceID)
		log.Printf("[TraceID: %s] 请求信息 - 方法: %s, 路径: %s, 客户端IP: %s, User-Agent: %s",
			traceID, c.Request.Method, c.Request.URL.Path, c.ClientIP(), c.Request.UserAgent())

		var req ProbeRequest
		// 绑定JSON请求体
		bindStartTime := time.Now()
		if err := c.ShouldBindJSON(&req); err != nil {
			log.Printf("[TraceID: %s] 请求参数解析失败 - 错误: %v, 耗时: %v",
				traceID, err, time.Since(bindStartTime))
			c.JSON(http.StatusBadRequest, gin.H{
				"error":    "无效的请求格式，需要urls字段（字符串数组）",
				"trace_id": traceID,
			})
			return
		}
		log.Printf("[TraceID: %s] 请求参数解析成功 - Type: %s, URLs数量: %d, 耗时: %v",
			traceID, req.Type, len(req.URLs), time.Since(bindStartTime))
		log.Printf("[TraceID: %s] 请求URLs列表: %v", traceID, req.URLs)

		var availableURLs []string
		queryStartTime := time.Now()

		// 从数据库查询链接
		if req.Type == "" {
			log.Printf("[TraceID: %s] 使用 probeURLs 模式（保持原顺序）", traceID)
			availableURLs = probeURLs(db, req.URLs, traceID)
		} else {
			log.Printf("[TraceID: %s] 使用 probeURLsByType 模式（按速度排序）- Type: %s", traceID, req.Type)
			availableURLs = probeURLsByType(db, req.URLs, traceID)
		}

		queryDuration := time.Since(queryStartTime)
		log.Printf("[TraceID: %s] 查询处理完成 - 耗时: %v", traceID, queryDuration)

		totalDuration := time.Since(requestStartTime)
		log.Printf("[TraceID: %s] 请求参数解析成功 - Type: %s, URL: %v,响应结果 - 可用URLs数量: %d, 可用URLs: %v",
			traceID, req.Type, req.URLs, len(availableURLs), availableURLs)
		log.Printf("[TraceID: %s] ========== 请求完成 - 总耗时: %v ==========", traceID, totalDuration)
		// 返回可用的链接数组
		c.JSON(http.StatusOK, ProbeResponse{
			AvailableURLs: availableURLs,
		})
	}
}

func main() {
	// 确保 logs 目录存在
	if err := os.MkdirAll("logs", 0755); err != nil {
		fmt.Printf("创建 logs 目录失败: %v\n", err)
		os.Exit(1)
	}

	// 配置日志输出到文件，支持自动切割
	logFile := &lumberjack.Logger{
		Filename:   "logs/probe.log", // 日志文件路径
		MaxSize:    50,               // 每个日志文件最大50MB
		MaxBackups: 10,               // 最多保留10个旧日志文件
		MaxAge:     30,               // 日志文件最多保留30天
		Compress:   true,             // 是否压缩旧日志文件
		LocalTime:  true,             // 使用本地时间
	}

	// 同时输出到文件和控制台（方便调试）
	multiWriter := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(multiWriter)
	// 设置日志格式：日期 时间 文件名:行号
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// 同时配置 Gin 的日志输出
	gin.DefaultWriter = multiWriter
	gin.DefaultErrorWriter = multiWriter

	log.Println("========== Probe 服务启动 ==========")
	log.Printf("日志文件配置 - 路径: %s, 最大大小: %dMB, 最多保留: %d个文件",
		logFile.Filename, logFile.MaxSize, logFile.MaxBackups)

	// 加载配置
	// 加载环境变量
	if err := godotenv.Load(); err != nil {
		log.Println("未找到 .env 文件，使用环境变量")
	}
	cfg := Load()
	log.Printf("配置加载完成 - DB Host: %s, DB Port: %s, DB Name: %s",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.DBName)

	// 初始化数据库连接
	log.Println("开始连接数据库...")
	db, err := Initialize(DatabaseConfig{
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
	log.Println("数据库连接成功")

	// 创建Gin路由
	log.Println("初始化Gin路由...")
	r := gin.Default()

	// 定义探活接口
	r.POST("/probe", probeHandler(db))
	log.Println("注册路由 - POST /probe")

	// 可选：添加健康检查接口
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})
	log.Println("注册路由 - GET /health")

	// 启动服务器，监听8080端口
	log.Println("========== 服务器启动在 :8080 ==========")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

func Initialize(cfg DatabaseConfig) (*gorm.DB, error) {
	// MySQL DSN 格式: user:password@tcp(host:port)/dbname?charset=utf8mb4&parseTime=True&loc=Local
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.DBName,
	)
	log.Printf("数据库 DSN: %s:***@tcp(%s:%s)/%s", cfg.User, cfg.Host, cfg.Port, cfg.DBName)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}

	return db, nil
}

type Config struct {
	Database DatabaseConfig
}

type ServerConfig struct {
	Port     string
	Mode     string
	Sitename string
}

type JWTConfig struct {
	Secret      string
	ExpireHours int
}

type AWSConfig struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	CloudFrontID    string
	S3BucketName    string
}

type CloudflareConfig struct {
	APIEmail string
	APIKey   string
	APIToken string // 如果使用Token认证，优先使用Token
}

type ScheduledTaskConfig struct {
	EnableSpeedProbeAlert           bool // 是否启用速度探测告警检查任务
	EnableCleanOldResults           bool // 是否启用清理旧探测结果任务
	EnableUpdateCustomDownloadLinks bool // 是否启用更新自定义下载链接实际URL任务
}

func Load() *Config {
	return &Config{
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "38.60.244.146"),
			Port:     getEnv("DB_PORT", "3306"),
			User:     getEnv("DB_USER", "debian-sys-maint"),
			Password: getEnv("DB_PASSWORD", "DWRjndrcvb9A9zuF"),
			DBName:   getEnv("DB_NAME", "probe"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getBoolEnv(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		boolValue, err := strconv.ParseBool(value)
		if err != nil {
			return defaultValue
		}
		return boolValue
	}
	return defaultValue
}
