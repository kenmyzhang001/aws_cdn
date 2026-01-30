package services

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/models"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

type CustomDownloadLinkService struct {
	db *gorm.DB
}

func NewCustomDownloadLinkService(db *gorm.DB) *CustomDownloadLinkService {
	return &CustomDownloadLinkService{db: db}
}

// CreateCustomDownloadLink 创建单个自定义下载链接
func (s *CustomDownloadLinkService) CreateCustomDownloadLink(link *models.CustomDownloadLink) error {
	if link.URL == "" {
		return fmt.Errorf("链接URL不能为空")
	}
	return s.db.Create(link).Error
}

// BatchCreateCustomDownloadLinks 批量创建自定义下载链接
// urls: 链接列表，支持换行符或逗号分隔
func (s *CustomDownloadLinkService) BatchCreateCustomDownloadLinks(urlsText string) ([]models.CustomDownloadLink, error) {
	// 解析链接（支持换行符和逗号分隔）
	urls := parseURLs(urlsText)
	if len(urls) == 0 {
		return nil, fmt.Errorf("没有有效的链接")
	}

	links := make([]models.CustomDownloadLink, 0, len(urls))
	for _, url := range urls {
		link := models.CustomDownloadLink{
			URL:    url,
			Status: models.CustomDownloadLinkStatusActive,
		}
		links = append(links, link)
	}

	// 批量插入
	if err := s.db.Create(&links).Error; err != nil {
		return nil, fmt.Errorf("批量创建链接失败: %w", err)
	}

	return links, nil
}

// parseURLs 解析URL字符串（支持换行符和逗号分隔）
func parseURLs(text string) []string {
	var urls []string

	// 先按换行符分割
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 每行再按逗号分割
		parts := strings.Split(line, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				urls = append(urls, part)
			}
		}
	}

	return urls
}

// GetCustomDownloadLink 获取自定义下载链接
func (s *CustomDownloadLinkService) GetCustomDownloadLink(id uint) (*models.CustomDownloadLink, error) {
	var link models.CustomDownloadLink
	if err := s.db.Preload("Group").First(&link, id).Error; err != nil {
		return nil, fmt.Errorf("链接不存在: %w", err)
	}
	return &link, nil
}

// ListAllCustomDownloadLinks 列出所有自定义下载链接（不分页）
func (s *CustomDownloadLinkService) ListAllCustomDownloadLinks() ([]models.CustomDownloadLink, error) {
	var links []models.CustomDownloadLink

	if err := s.db.Preload("Group").
		Where("deleted_at IS NULL").
		Order("created_at DESC").
		Find(&links).Error; err != nil {
		return nil, err
	}

	return links, nil
}

// ListCustomDownloadLinks 列出所有自定义下载链接，支持分页、分组筛选和搜索
func (s *CustomDownloadLinkService) ListCustomDownloadLinks(page, pageSize int, groupID *uint, search *string, status *models.CustomDownloadLinkStatus) ([]models.CustomDownloadLink, int64, error) {
	var links []models.CustomDownloadLink
	var total int64

	offset := (page - 1) * pageSize

	query := s.db.Model(&models.CustomDownloadLink{}).Where("deleted_at IS NULL")

	if groupID != nil {
		query = query.Where("group_id = ?", *groupID)
	}

	if search != nil && *search != "" {
		searchPattern := "%" + *search + "%"
		query = query.Where("url LIKE ? OR name LIKE ? OR description LIKE ?", searchPattern, searchPattern, searchPattern)
	}

	if status != nil {
		query = query.Where("status = ?", *status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Preload("Group").Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&links).Error; err != nil {
		return nil, 0, err
	}

	return links, total, nil
}

// UpdateCustomDownloadLink 更新自定义下载链接
func (s *CustomDownloadLinkService) UpdateCustomDownloadLink(id uint, updates map[string]interface{}) error {
	return s.db.Model(&models.CustomDownloadLink{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteCustomDownloadLink 删除自定义下载链接（软删除）
func (s *CustomDownloadLinkService) DeleteCustomDownloadLink(id uint) error {
	return s.db.Delete(&models.CustomDownloadLink{}, id).Error
}

// BatchDeleteCustomDownloadLinks 批量删除自定义下载链接
func (s *CustomDownloadLinkService) BatchDeleteCustomDownloadLinks(ids []uint) error {
	return s.db.Delete(&models.CustomDownloadLink{}, ids).Error
}

// IncrementClickCount 增加点击次数
func (s *CustomDownloadLinkService) IncrementClickCount(id uint) error {
	return s.db.Model(&models.CustomDownloadLink{}).Where("id = ?", id).UpdateColumn("click_count", gorm.Expr("click_count + ?", 1)).Error
}

// UpdateActualURLsForAllLinks 更新所有链接的 actual_url（检查301/302重定向）
func (s *CustomDownloadLinkService) UpdateActualURLsForAllLinks() error {
	log := logger.GetLogger()
	log.Info("开始更新所有自定义下载链接的 actual_url")

	// 查询所有启用的链接
	var links []models.CustomDownloadLink
	if err := s.db.Where("status = ? AND deleted_at IS NULL", models.CustomDownloadLinkStatusActive).
		Find(&links).Error; err != nil {
		log.WithError(err).Error("查询自定义下载链接失败")
		return fmt.Errorf("查询自定义下载链接失败: %w", err)
	}

	if len(links) == 0 {
		log.Info("没有需要更新的链接")
		return nil
	}

	log.WithField("count", len(links)).Info("找到需要更新的链接")

	var wg sync.WaitGroup
	// 限制并发数为 20
	semaphore := make(chan struct{}, 20)

	for _, link := range links {
		wg.Add(1)
		go func(l models.CustomDownloadLink) {
			defer wg.Done()

			// 获取信号量，控制并发数
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			actualURL, err := s.getActualURL(l.URL)
			if err != nil {
				log.WithError(err).WithField("url", l.URL).Warn("获取实际URL失败")
				return
			}

			if err := s.db.Model(&models.CustomDownloadLink{}).
				Where("id = ?", l.ID).
				Update("actual_url", actualURL).Error; err != nil {
				log.WithError(err).WithFields(map[string]interface{}{
					"link_id": l.ID,
					"url":     l.URL,
				}).Error("更新 actual_url 失败")
				return
			}

			log.WithFields(map[string]interface{}{
				"link_id":    l.ID,
				"url":        l.URL,
				"actual_url": actualURL,
			}).Info("更新 actual_url 成功")

		}(link)
	}

	// 等待所有 goroutine 完成
	wg.Wait()

	log.Info("所有链接的 actual_url 更新完成")
	return nil
}

// getActualURL 获取URL的实际地址（跟踪301/302重定向）
func (s *CustomDownloadLinkService) getActualURL(url string) (string, error) {
	if strings.HasSuffix(strings.ToLower(url), ".apk") {
		return url, nil
	}
	log := logger.GetLogger()

	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// 最多跟踪10次重定向
			if len(via) >= 10 {
				log.WithField("url", url).Error("重定向次数过多")
				return fmt.Errorf("重定向次数过多")
			}
			return nil
		},
	}

	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		log.WithError(err).WithField("url", url).Error("创建请求失败")
		return url, nil
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; URL-Checker/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		// 如果HEAD请求失败，尝试GET请求
		req, err = http.NewRequest("GET", url, nil)
		if err != nil {
			log.WithError(err).WithField("url", url).Error("创建GET请求失败")
			return url, nil
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; URL-Checker/1.0)")

		resp, err = client.Do(req)
		if err != nil {
			log.WithError(err).WithField("url", url).Debug("URL 请求失败")
			return url, nil
		}
	}
	defer resp.Body.Close()

	// 返回最终的URL（如果有重定向）
	finalURL := resp.Request.URL.String()

	if finalURL != url {
		log.WithFields(map[string]interface{}{
			"original_url": url,
			"final_url":    finalURL,
			"status_code":  resp.StatusCode,
		}).Info("检测到URL重定向")
	}

	return finalURL, nil
}
