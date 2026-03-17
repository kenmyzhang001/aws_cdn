package services

import (
	"aws_cdn/internal/models"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

type CFWorkpageSiteService struct {
	db *gorm.DB
}

func NewCFWorkpageSiteService(db *gorm.DB) *CFWorkpageSiteService {
	return &CFWorkpageSiteService{db: db}
}

// List 列表，支持 cf_account_id、template_id 筛选与分页
func (s *CFWorkpageSiteService) List(cfAccountID, templateID *uint, page, pageSize int) ([]models.CFWorkpageSite, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	var list []models.CFWorkpageSite
	var total int64
	query := s.db.Model(&models.CFWorkpageSite{})
	if cfAccountID != nil {
		query = query.Where("cf_account_id = ?", *cfAccountID)
	}
	if templateID != nil {
		query = query.Where("template_id = ?", *templateID)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("查询总数失败: %w", err)
	}
	offset := (page - 1) * pageSize
	if err := query.Preload("CFAccount").Preload("Template").Order("id DESC").Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("查询列表失败: %w", err)
	}
	return list, total, nil
}

// Get 获取单条
func (s *CFWorkpageSiteService) Get(id uint) (*models.CFWorkpageSite, error) {
	var site models.CFWorkpageSite
	if err := s.db.Preload("CFAccount").Preload("Template").First(&site, id).Error; err != nil {
		return nil, fmt.Errorf("站点不存在: %w", err)
	}
	return &site, nil
}

// Create 创建（仅落库，实际部署 WorkPage 可后续对接 CF API）
func (s *CFWorkpageSiteService) Create(cfAccountID, templateID uint, zoneID, mainDomain, subdomain string) (*models.CFWorkpageSite, error) {
	mainDomain = strings.TrimSpace(strings.ToLower(mainDomain))
	subdomain = strings.TrimSpace(strings.ToLower(subdomain))
	site := &models.CFWorkpageSite{
		CFAccountID: cfAccountID,
		TemplateID:  templateID,
		ZoneID:      zoneID,
		MainDomain:  mainDomain,
		Subdomain:   subdomain,
		Status:      "pending",
	}
	if err := s.db.Create(site).Error; err != nil {
		return nil, fmt.Errorf("创建站点失败: %w", err)
	}
	return s.Get(site.ID)
}

// Update 更新（仅允许改子域名等，主域名/zone 一般不变）
func (s *CFWorkpageSiteService) Update(id uint, subdomain *string) (*models.CFWorkpageSite, error) {
	site, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if subdomain != nil {
		site.Subdomain = strings.TrimSpace(strings.ToLower(*subdomain))
	}
	if err := s.db.Model(site).Update("subdomain", site.Subdomain).Error; err != nil {
		return nil, fmt.Errorf("更新站点失败: %w", err)
	}
	return s.Get(id)
}

// Delete 删除
func (s *CFWorkpageSiteService) Delete(id uint) error {
	return s.db.Delete(&models.CFWorkpageSite{}, id).Error
}
