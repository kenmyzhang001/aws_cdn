package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// Config Agent配置
type Config struct {
	ServerURL       string        // 服务器地址
	ProbeInterval   time.Duration // 探测间隔
	TimeoutDuration time.Duration // 单次探测超时时间
	MaxFileSize     int64         // 最大下载文件大小（字节）
	SpeedThreshold  float64       // 速度阈值（KB/s），用于判断是否成功
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
	DownloadPackages    []LinkItem `json:"download_packages"`
	CustomDownloadLinks []LinkItem `json:"custom_download_links"`
	R2CustomDomains     []LinkItem `json:"r2_custom_domains"`
	Total               int        `json:"total"`
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
	serverURL := flag.String("server", "http://localhost:8080", "服务器地址")
	interval := flag.Duration("interval", 20*time.Minute, "探测间隔")
	timeout := flag.Duration("timeout", 30*time.Second, "单次探测超时时间")
	maxSize := flag.Int64("max-size", 10*1024*1024, "最大下载文件大小（字节）")
	speedThreshold := flag.Float64("speed-threshold", 100.0, "速度阈值（KB/s）")
	flag.Parse()

	config := Config{
		ServerURL:       *serverURL,
		ProbeInterval:   *interval,
		TimeoutDuration: *timeout,
		MaxFileSize:     *maxSize,
		SpeedThreshold:  *speedThreshold,
	}

	log.Printf("🚀 Agent 启动")
	log.Printf("   服务器地址: %s", config.ServerURL)
	log.Printf("   探测间隔: %v", config.ProbeInterval)
	log.Printf("   探测超时: %v", config.TimeoutDuration)
	log.Printf("   最大文件大小: %d MB", config.MaxFileSize/(1024*1024))
	log.Printf("   速度阈值: %.2f KB/s", config.SpeedThreshold)

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

	// 2. 收集所有URL（去重）
	urlMap := make(map[string]bool)
	var urls []string

	for _, link := range links.DownloadPackages {
		if link.URL != "" && !urlMap[link.URL] {
			urlMap[link.URL] = true
			urls = append(urls, link.URL)
		}
	}
	for _, link := range links.CustomDownloadLinks {
		if link.URL != "" && !urlMap[link.URL] {
			urlMap[link.URL] = true
			urls = append(urls, link.URL)
		}
	}
	for _, link := range links.R2CustomDomains {
		if link.URL != "" && !urlMap[link.URL] {
			urlMap[link.URL] = true
			urls = append(urls, link.URL)
		}
	}

	log.Printf("🔍 去重后需要探测 %d 个URL", len(urls))

	// 3. 探测所有URL
	results := make([]ProbeResult, 0, len(urls))
	successCount := 0
	failedCount := 0

	for i, url := range urls {
		log.Printf("   [%d/%d] 探测: %s", i+1, len(urls), url)

		result := probeURL(url, config)
		results = append(results, result)

		if result.Status == "success" {
			successCount++
			log.Printf("   ✓ 成功 | 速度: %.2f KB/s | 耗时: %d ms",
				result.SpeedKbps, *result.DownloadTimeMs)
		} else {
			failedCount++
			log.Printf("   ✗ 失败 | 原因: %s", result.ErrorMessage)
		}
	}

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

// probeURL 探测单个URL的下载速度
func probeURL(url string, config *Config) ProbeResult {
	result := ProbeResult{
		URL:       url,
		UserAgent: "SpeedProbeAgent/1.0",
		Status:    "failed",
	}

	// 创建HTTP客户端
	client := &http.Client{
		Timeout: config.TimeoutDuration,
	}

	// 记录开始时间
	startTime := time.Now()

	// 发起请求
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("创建请求失败: %v", err)
		return result
	}
	req.Header.Set("User-Agent", result.UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("请求失败: %v", err)
		result.Status = "timeout"
		return result
	}
	defer resp.Body.Close()

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
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

			// 检查是否超过最大文件大小
			if totalSize > config.MaxFileSize {
				break
			}
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

	// 判断是否成功（基于速度阈值）
	if speedKbps >= config.SpeedThreshold {
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
