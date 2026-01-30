package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Config Agent配置
type Config struct {
	ServerURL       string        // 服务器地址
	ProbeInterval   time.Duration // 探测间隔
	TimeoutDuration time.Duration // 单次探测超时时间
	MaxFileSize     int64         // 最大下载文件大小（字节）
	SpeedThreshold  float64       // 速度阈值（KB/s），用于判断是否成功
	Concurrency     int           // 并发探测数量
}

// LinkItem 链接项
type LinkItem struct {
	ID          uint   `json:"id"`
	URL         string `json:"url"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
}

// AllLinksResponse 所有链接的响应
type AllLinksResponse struct {
	Links []LinkItem `json:"links"`
	Total int        `json:"total"`
}

// ProbeResult 探测结果
type ProbeResult struct {
	URL            string  `json:"url"`
	SpeedKbps      float64 `json:"speed_kbps"`
	FileSize       *int64  `json:"file_size,omitempty"`
	DownloadTimeMs *int64  `json:"download_time_ms,omitempty"`
	Status         string  `json:"status"`
	ErrorMessage   string  `json:"error_message,omitempty"`
	UserAgent      string  `json:"user_agent"`
}

// BatchReportRequest 批量上报请求
type BatchReportRequest struct {
	Results []ProbeResult `json:"results"`
}

func main() {
	// 解析命令行参数
	serverURL := flag.String("server", "http://16.163.99.99:8080", "服务器地址")
	interval := flag.Duration("interval", 30*time.Minute, "探测间隔")
	timeout := flag.Duration("timeout", 60*time.Second, "单次探测超时时间")
	maxSize := flag.Int64("max-size", 10*1024*1024, "最大下载文件大小（字节）")
	speedThreshold := flag.Float64("speed-threshold", 10.0, "速度阈值（KB/s）")
	concurrency := flag.Int("concurrency", 50, "并发探测数量")
	flag.Parse()

	config := Config{
		ServerURL:       *serverURL,
		ProbeInterval:   *interval,
		TimeoutDuration: *timeout,
		MaxFileSize:     *maxSize,
		SpeedThreshold:  *speedThreshold,
		Concurrency:     *concurrency,
	}

	log.Printf("🚀 Agent 启动")
	log.Printf("   服务器地址: %s", config.ServerURL)
	log.Printf("   探测间隔: %v", config.ProbeInterval)
	log.Printf("   探测超时: %v", config.TimeoutDuration)
	log.Printf("   最大文件大小: %d MB", config.MaxFileSize/(1024*1024))
	log.Printf("   速度阈值: %.2f KB/s", config.SpeedThreshold)
	log.Printf("   并发数量: %d", config.Concurrency)

	// 立即执行一次
	log.Println("⏰ 开始首次探测...")
	runProbe(&config)

	// 定时执行
	ticker := time.NewTicker(config.ProbeInterval)
	defer ticker.Stop()

	for range ticker.C {
		log.Printf("⏰ 开始定时探测...")
		runProbe(&config)
	}
}

// runProbe 执行一次完整的探测流程
func runProbe(config *Config) {
	startTime := time.Now()

	// 1. 获取所有链接
	links, err := fetchAllLinks(config.ServerURL)
	if err != nil {
		log.Printf("❌ 获取链接失败: %v", err)
		return
	}

	log.Printf("📋 获取到 %d 个链接", links.Total)

	// 2. 提取所有需要探测的URL（去重）
	urlSet := make(map[string]bool)
	for _, link := range links.Links {
		if link.URL != "" {
			urlSet[link.URL] = true
		}
	}

	// 转换为数组
	urls := make([]string, 0, len(urlSet))
	for url := range urlSet {
		urls = append(urls, url)
	}

	log.Printf("🔍 需要探测 %d 个URL", len(urls))

	// 3. 并发探测所有URL
	results := make([]ProbeResult, 0, len(urls))
	var resultsMutex sync.Mutex
	var wg sync.WaitGroup

	// 创建并发控制的 semaphore channel
	semaphore := make(chan struct{}, config.Concurrency)

	successCount := 0
	failedCount := 0
	var statsMutex sync.Mutex

	completed := 0
	var completedMutex sync.Mutex

	for _, url := range urls {
		wg.Add(1)
		go func(targetURL string) {
			defer wg.Done()

			// 获取信号量
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// 获取当前进度
			completedMutex.Lock()
			completed++
			currentIndex := completed
			completedMutex.Unlock()

			log.Printf("   [%d/%d] 探测: %s", currentIndex, len(urls), targetURL)

			result := probeURL(targetURL, config)

			// 保存结果
			resultsMutex.Lock()
			results = append(results, result)
			resultsMutex.Unlock()

			// 更新统计
			statsMutex.Lock()
			if result.Status == "success" {
				successCount++
				log.Printf("   ✓ 成功 | 速度: %.2f KB/s | 耗时: %d ms",
					result.SpeedKbps, *result.DownloadTimeMs)
			} else {
				failedCount++
				log.Printf("   ✗ 失败 | 原因: %s", result.ErrorMessage)
			}
			statsMutex.Unlock()
		}(url)
	}

	// 等待所有探测完成
	wg.Wait()

	// 4. 批量上报结果
	log.Printf("📤 上报探测结果...")
	if err := reportResults(config.ServerURL, results); err != nil {
		log.Printf("❌ 上报失败: %v", err)
	} else {
		log.Printf("✅ 上报成功")
	}

	// 5. 输出统计
	elapsed := time.Since(startTime)
	log.Printf("📊 本次探测完成")
	log.Printf("   总耗时: %v", elapsed)
	log.Printf("   探测总数: %d", len(urls))
	log.Printf("   成功: %d (%.1f%%)", successCount, float64(successCount)*100/float64(len(urls)))
	log.Printf("   失败: %d (%.1f%%)", failedCount, float64(failedCount)*100/float64(len(urls)))
	log.Println()
}

// fetchAllLinks 获取所有链接
func fetchAllLinks(serverURL string) (*AllLinksResponse, error) {
	url := fmt.Sprintf("%s/api/v1/all-links", serverURL)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result AllLinksResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return &result, nil
}

// probeURL 探测单个URL的下载速度（支持重试）
func probeURL(url string, config *Config) ProbeResult {
	const maxRetries = 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		result := probeURLOnce(url, config)

		// 如果成功或非超时错误，直接返回
		if result.Status == "success" || result.Status != "timeout" {
			return result
		}

		// 超时且还有重试机会
		if attempt < maxRetries {
			log.Printf("   ⚠️  超时，%d秒后重试 (%d/%d)", 2, attempt, maxRetries)
			time.Sleep(2 * time.Second)
		} else {
			// 最后一次重试也失败了
			result.ErrorMessage = fmt.Sprintf("请求超时(已重试%d次): %s", maxRetries, result.ErrorMessage)
			return result
		}
	}

	// 不应该到这里，但为了安全返回失败
	return ProbeResult{
		URL:          url,
		UserAgent:    "SpeedProbeAgent/1.0",
		Status:       "failed",
		ErrorMessage: "未知错误",
	}
}

// probeRedirectTarget 探测重定向目标URL是否可下载
func probeRedirectTarget(url string, config *Config) ProbeResult {
	result := ProbeResult{
		URL:       url,
		UserAgent: "SpeedProbeAgent/1.0",
		Status:    "failed",
	}

	// 不跟随重定向的客户端
	client := &http.Client{
		Timeout: config.TimeoutDuration,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	const downloadSize = 1 * 1024 // 1KB
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("创建请求失败: %v", err)
		return result
	}
	req.Header.Set("User-Agent", result.UserAgent)
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", downloadSize-1))

	startTime := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("请求失败: %v", err)
		result.Status = "timeout"
		return result
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	contentDisposition := resp.Header.Get("Content-Disposition")

	// 判断是否为有效的下载链接
	isValid := false
	if resp.StatusCode == http.StatusPartialContent {
		isValid = true
	} else if resp.StatusCode == http.StatusOK {
		if strings.Contains(strings.ToLower(contentDisposition), ".apk") {
			isValid = true
		} else if strings.Contains(strings.ToLower(contentType), "application/vnd.android.package-archive") {
			isValid = true
		}
	}

	if !isValid {
		result.ErrorMessage = fmt.Sprintf("重定向目标不满足下载条件: 状态码=%d", resp.StatusCode)
		return result
	}

	// 读取实际下载的数据
	totalSize := int64(0)
	buffer := make([]byte, 32*1024) // 32KB buffer
	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			totalSize += int64(n)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			result.ErrorMessage = fmt.Sprintf("读取失败: %v", err)
			return result
		}
	}

	// 计算速度（基于实际下载的字节数）
	downloadTime := time.Since(startTime)
	downloadTimeMs := downloadTime.Milliseconds()
	speedKbps := float64(totalSize) / 1024.0 / downloadTime.Seconds()

	result.FileSize = &totalSize
	result.DownloadTimeMs = &downloadTimeMs
	result.SpeedKbps = speedKbps
	result.Status = "success"

	return result
}

// probeURLOnce 执行单次URL探测
func probeURLOnce(url string, config *Config) ProbeResult {
	result := ProbeResult{
		URL:       url,
		UserAgent: "SpeedProbeAgent/1.0",
		Status:    "failed",
	}

	// 创建HTTP客户端，允许跟随重定向
	client := &http.Client{
		Timeout: config.TimeoutDuration,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("重定向次数过多")
			}
			return nil
		},
	}

	// 记录开始时间
	startTime := time.Now()

	// 发起请求（使用 Range 头只请求前1KB）
	const maxDownloadSize = 1 * 1024 // 1KB
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("创建请求失败: %v", err)
		return result
	}
	req.Header.Set("User-Agent", result.UserAgent)
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", maxDownloadSize-1))

	resp, err := client.Do(req)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("请求失败: %v", err)
		result.Status = "timeout"
		return result
	}
	defer resp.Body.Close()

	// 获取响应头信息
	contentType := resp.Header.Get("Content-Type")
	contentDisposition := resp.Header.Get("Content-Disposition")

	// 判断是否为有效的下载链接（满足以下任一条件即可）：
	// 1. 状态码为 206 (Partial Content)
	// 2. Content-Disposition 包含 .apk 文件名
	// 3. Content-Type 为 application/vnd.android.package-archive
	// 4. 如果是重定向状态码，检查最终重定向后的URL是否可下载
	isValid := false

	if resp.StatusCode == http.StatusPartialContent {
		isValid = true
	} else if strings.Contains(strings.ToLower(contentDisposition), ".apk") {
		isValid = true
	} else if strings.Contains(strings.ToLower(contentType), "application/vnd.android.package-archive") {
		isValid = true
	} else if resp.StatusCode == http.StatusTemporaryRedirect ||
		resp.StatusCode == http.StatusMovedPermanently ||
		resp.StatusCode == http.StatusFound {
		// 处理重定向情况
		location := resp.Header.Get("Location")
		if location == "" {
			result.ErrorMessage = "重定向但未找到Location头"
			return result
		}

		// 对重定向后的URL进行探测
		redirectResult := probeRedirectTarget(location, config)
		return redirectResult
	} else if resp.StatusCode != http.StatusOK {
		result.ErrorMessage = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return result
	}

	// 下载内容并计算速度
	totalSize := int64(0)
	buffer := make([]byte, 32*1024) // 32KB buffer

	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			totalSize += int64(n)
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			result.ErrorMessage = fmt.Sprintf("读取失败: %v", err)
			return result
		}
	}

	// 计算耗时和速度
	downloadTime := time.Since(startTime)
	downloadTimeMs := downloadTime.Milliseconds()
	speedKbps := float64(totalSize) / 1024.0 / downloadTime.Seconds()

	result.FileSize = &totalSize
	result.DownloadTimeMs = &downloadTimeMs
	result.SpeedKbps = speedKbps

	// 判断是否成功（基于速度阈值或有效性检查）
	if isValid || speedKbps >= config.SpeedThreshold {
		result.Status = "success"
	} else {
		result.Status = "failed"
		result.ErrorMessage = fmt.Sprintf("速度过慢: %.2f KB/s < %.2f KB/s", speedKbps, config.SpeedThreshold)
	}

	return result
}

// reportResults 批量上报探测结果
func reportResults(serverURL string, results []ProbeResult) error {
	url := fmt.Sprintf("%s/api/v1/speed-probe/report-batch", serverURL)

	reqBody := BatchReportRequest{
		Results: results,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("序列化失败: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
