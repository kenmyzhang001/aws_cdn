package services

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/models"
	"fmt"
	"net/http"
	"time"

	"gorm.io/gorm"
)

// URLCheckerService URL 检查服务
type URLCheckerService struct {
	db       *gorm.DB
	client   *http.Client
	telegram *TelegramService
}

// NewURLCheckerService 创建 URL 检查服务
func NewURLCheckerService(db *gorm.DB, telegram *TelegramService) *URLCheckerService {
	return &URLCheckerService{
		db: db,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		telegram: telegram,
	}
}

// CheckDownloadURLs 检查所有 DownloadPackage 的 download_url
func (s *URLCheckerService) CheckDownloadURLs() error {
	log := logger.GetLogger()
	log.Info("开始检查 DownloadPackage 的 download_url")

	var packages []models.DownloadPackage
	if err := s.db.Where("download_url != '' AND deleted_at IS NULL").
		Find(&packages).Error; err != nil {
		log.WithError(err).Error("查询 DownloadPackage 失败")
		return fmt.Errorf("查询 DownloadPackage 失败: %w", err)
	}

	var invalidURLs []string
	for _, pkg := range packages {
		if !s.isURLValid(pkg.DownloadURL) {
			invalidURLs = append(invalidURLs, pkg.DownloadURL)
			log.WithFields(map[string]interface{}{
				"package_id":   pkg.ID,
				"download_url": pkg.DownloadURL,
			}).Warn("检测到不可用的下载链接")
		}
	}

	if len(invalidURLs) > 0 {
		log.WithField("count", len(invalidURLs)).Info("发现不可用的下载链接，准备发送到 Telegram")
		if err := s.telegram.SendMessagesBatch(invalidURLs); err != nil {
			log.WithError(err).Error("发送 Telegram 消息失败")
			return fmt.Errorf("发送 Telegram 消息失败: %w", err)
		}
		log.WithField("count", len(invalidURLs)).Info("已将所有不可用的链接发送到 Telegram")
	} else {
		log.Info("所有下载链接均可用")
	}

	return nil
}

// isURLValid 检查 URL 是否可用
func (s *URLCheckerService) isURLValid(url string) bool {
	log := logger.GetLogger()

	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		log.WithError(err).WithField("url", url).Warn("创建请求失败")
		return false
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; URL-Checker/1.0)")

	resp, err := s.client.Do(req)
	if err != nil {
		log.WithError(err).WithField("url", url).Debug("URL 检查失败")
		return false
	}
	defer resp.Body.Close()

	// 200 和 206 (Partial Content) 都认为是可用的
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent {
		return true
	}

	log.WithFields(map[string]interface{}{
		"url":         url,
		"status_code": resp.StatusCode,
	}).Debug("URL 返回非成功状态码")
	return false
}
