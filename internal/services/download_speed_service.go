package services

import (
	"fmt"
	"net/http"
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
