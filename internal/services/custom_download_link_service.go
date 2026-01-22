package services

import (
	"aws_cdn/internal/models"
	"fmt"
	"strings"

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
func (s *CustomDownloadLinkService) BatchCreateCustomDownloadLinks(urlsText string, groupID *uint) ([]models.CustomDownloadLink, error) {
	// 解析链接（支持换行符和逗号分隔）
	urls := parseURLs(urlsText)
	if len(urls) == 0 {
		return nil, fmt.Errorf("没有有效的链接")
	}

	links := make([]models.CustomDownloadLink, 0, len(urls))
	for _, url := range urls {
		link := models.CustomDownloadLink{
			URL:     url,
			GroupID: groupID,
			Status:  models.CustomDownloadLinkStatusActive,
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
