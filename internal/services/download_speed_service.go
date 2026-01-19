package services

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/models"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"gorm.io/gorm"
)

// DownloadSpeedService 下载速度探测服务
type DownloadSpeedService struct {
	db             *gorm.DB
	client         *http.Client
	telegram       *TelegramService
	speedThreshold float64 // 速度阈值（KB/s），低于此值将发送告警
}

// NewDownloadSpeedService 创建下载速度探测服务
func NewDownloadSpeedService(db *gorm.DB, telegram *TelegramService) *DownloadSpeedService {
	return &DownloadSpeedService{
		db: db,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		telegram:       telegram,
		speedThreshold: 100.0, // 默认阈值：100 KB/s
	}
}

// SetSpeedThreshold 设置速度阈值
func (s *DownloadSpeedService) SetSpeedThreshold(threshold float64) {
	s.speedThreshold = threshold
}

// SpeedResult 速度测试结果
type SpeedResult struct {
	URL       string
	Speed     float64 // KB/s
	Duration  time.Duration
	Error     error
	PackageID uint   // 包ID
	FileName  string // 文件名
}

// CheckDownloadSpeed 检查所有 DownloadPackage 的下载速度
func (s *DownloadSpeedService) CheckDownloadSpeed() error {
	log := logger.GetLogger()
	log.Info("开始检查 DownloadPackage 的下载速度")

	var packages []models.DownloadPackage
	if err := s.db.Where("download_url != '' AND deleted_at IS NULL").
		Find(&packages).Error; err != nil {
		log.WithError(err).Error("查询 DownloadPackage 失败")
		return fmt.Errorf("查询 DownloadPackage 失败: %w", err)
	}

	if len(packages) == 0 {
		log.Info("没有找到需要检查的下载链接")
		message := "下载速度探测完成\n\n没有找到需要检查的下载链接"
		if err := s.telegram.SendMessage(message); err != nil {
			log.WithError(err).Error("发送 Telegram 消息失败")
		}
		return nil
	}

	var results []SpeedResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	// 创建并发控制 channel，限制并发数为 10
	semaphore := make(chan struct{}, 10)

	for _, pkg := range packages {
		wg.Add(1)
		go func(p models.DownloadPackage) {
			defer wg.Done()

			// 获取信号量，控制并发数
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result := s.measureSpeed(p.DownloadURL)
			// 添加包信息
			result.PackageID = p.ID
			result.FileName = p.FileName
			mu.Lock()
			results = append(results, result)
			mu.Unlock()

			if result.Error != nil {
				log.WithFields(map[string]interface{}{
					"package_id":   p.ID,
					"download_url": p.DownloadURL,
					"error":        result.Error.Error(),
				}).Warn("下载速度测试失败")
			} else {
				log.WithFields(map[string]interface{}{
					"package_id":   p.ID,
					"download_url": p.DownloadURL,
					"speed":        fmt.Sprintf("%.2f KB/s", result.Speed),
					"duration":     result.Duration,
				}).Info("下载速度测试完成")
			}
		}(pkg)
	}

	// 等待所有 goroutine 完成
	wg.Wait()

	// 计算平均值（只统计成功的）
	var totalSpeed float64
	var successCount int
	for _, result := range results {
		if result.Error == nil {
			totalSpeed += result.Speed
			successCount++
		}
	}

	var avgSpeed float64
	if successCount > 0 {
		avgSpeed = totalSpeed / float64(successCount)
	}

	// 检查速度告警（只检查成功的测试）
	var slowURLs []SpeedResult
	for _, result := range results {
		if result.Error == nil && result.Speed < s.speedThreshold {
			slowURLs = append(slowURLs, result)
		}
	}

	// 构建消息
	message := "📊 下载速度探测报告\n\n"
	if s.telegram.GetSitename() != "" {
		message = fmt.Sprintf("[%s] 📊 下载速度探测报告\n\n", s.telegram.GetSitename())
	}
	message += fmt.Sprintf("总链接数: %d\n", len(packages))
	message += fmt.Sprintf("成功测试: %d\n", successCount)
	message += fmt.Sprintf("失败数量: %d\n", len(packages)-successCount)
	if successCount > 0 {
		message += fmt.Sprintf("平均速度: %.2f KB/s\n", avgSpeed)
	} else {
		message += "平均速度: 无可用数据\n"
	}
	message += fmt.Sprintf("慢速链接: %d 个（低于 %.2f KB/s）\n", len(slowURLs), s.speedThreshold)
	message += "\n⚠️ 提示：慢速链接将单独发送告警\n"

	// 发送到 Telegram
	if err := s.telegram.SendMessage(message); err != nil {
		log.WithError(err).Error("发送 Telegram 消息失败")
		return fmt.Errorf("发送 Telegram 消息失败: %w", err)
	}

	// 如果有慢速链接，发送告警
	if len(slowURLs) > 0 {
		log.WithFields(map[string]interface{}{
			"slow_count":      len(slowURLs),
			"speed_threshold": s.speedThreshold,
		}).Warn("检测到下载速度低于阈值的链接")

		if err := s.sendSpeedAlerts(slowURLs); err != nil {
			log.WithError(err).Error("发送速度告警失败")
			// 不返回错误，因为主报告已发送成功
		}
	}

	log.WithFields(map[string]interface{}{
		"total_count":   len(packages),
		"success_count": successCount,
		"avg_speed":     avgSpeed,
		"slow_count":    len(slowURLs),
	}).Info("下载速度探测完成")

	return nil
}

// sendSpeedAlerts 发送速度告警消息（每5条合并发送）
func (s *DownloadSpeedService) sendSpeedAlerts(slowURLs []SpeedResult) error {
	if len(slowURLs) == 0 {
		return nil
	}

	const batchSize = 5
	totalBatches := (len(slowURLs) + batchSize - 1) / batchSize

	for i := 0; i < totalBatches; i++ {
		start := i * batchSize
		end := start + batchSize
		if end > len(slowURLs) {
			end = len(slowURLs)
		}

		batch := slowURLs[start:end]
		message := fmt.Sprintf("⚠️ 下载速度告警（阈值: %.2f KB/s）\n\n", s.speedThreshold)
		if s.telegram.GetSitename() != "" {
			message = fmt.Sprintf("[%s] ⚠️ 下载速度告警（阈值: %.2f KB/s）\n\n", s.telegram.GetSitename(), s.speedThreshold)
		}

		for j, result := range batch {
			message += fmt.Sprintf("%d. %s\n", start+j+1, result.FileName)
			message += fmt.Sprintf("   速度: %.2f KB/s\n", result.Speed)
			message += fmt.Sprintf("   URL: %s\n", result.URL)
			if j < len(batch)-1 {
				message += "\n"
			}
		}

		if err := s.telegram.SendMessage(message); err != nil {
			return fmt.Errorf("发送第 %d 批告警消息失败: %w", i+1, err)
		}

		// 如果不是最后一批，等待1秒
		if i < totalBatches-1 {
			time.Sleep(1 * time.Second)
		}
	}

	return nil
}

// measureSpeed 测量单个 URL 的下载速度（只下载前100KB）
func (s *DownloadSpeedService) measureSpeed(url string) SpeedResult {
	log := logger.GetLogger()
	startTime := time.Now()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return SpeedResult{
			URL:      url,
			Speed:    0,
			Duration: time.Since(startTime),
			Error:    fmt.Errorf("创建请求失败: %w", err),
		}
	}

	// 设置 Range 头，只下载前100KB
	req.Header.Set("Range", "bytes=0-102399")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Speed-Checker/1.0)")

	resp, err := s.client.Do(req)
	if err != nil {
		return SpeedResult{
			URL:      url,
			Speed:    0,
			Duration: time.Since(startTime),
			Error:    fmt.Errorf("请求失败: %w", err),
		}
	}
	defer resp.Body.Close()

	// 检查状态码
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return SpeedResult{
			URL:      url,
			Speed:    0,
			Duration: time.Since(startTime),
			Error:    fmt.Errorf("HTTP 状态码错误: %d", resp.StatusCode),
		}
	}

	// 读取数据（限制100KB）
	limitReader := io.LimitReader(resp.Body, 100*1024)
	data, err := io.ReadAll(limitReader)
	if err != nil {
		return SpeedResult{
			URL:      url,
			Speed:    0,
			Duration: time.Since(startTime),
			Error:    fmt.Errorf("读取数据失败: %w", err),
		}
	}

	duration := time.Since(startTime)
	bytesDownloaded := len(data)

	// 计算速度（KB/s）
	var speed float64
	if duration.Seconds() > 0 {
		speed = float64(bytesDownloaded) / 1024.0 / duration.Seconds()
	}

	log.WithFields(map[string]interface{}{
		"url":              url,
		"bytes_downloaded": bytesDownloaded,
		"duration":         duration,
		"speed":            fmt.Sprintf("%.2f KB/s", speed),
	}).Debug("下载速度测试完成")

	return SpeedResult{
		URL:      url,
		Speed:    speed,
		Duration: duration,
		Error:    nil,
	}
}
