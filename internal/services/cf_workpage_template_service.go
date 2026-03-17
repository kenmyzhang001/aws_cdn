package services

import (
	"aws_cdn/internal/models"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

type CFWorkpageTemplateService struct {
	db *gorm.DB
}

func NewCFWorkpageTemplateService(db *gorm.DB) *CFWorkpageTemplateService {
	return &CFWorkpageTemplateService{db: db}
}

// List 列表，支持关键词筛选（name_zh/name_my）与分页
func (s *CFWorkpageTemplateService) List(keyword string, page, pageSize int) ([]models.CFWorkpageTemplate, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	var list []models.CFWorkpageTemplate
	var total int64
	query := s.db.Model(&models.CFWorkpageTemplate{})
	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("name_zh LIKE ? OR name_my LIKE ?", like, like)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("查询总数失败: %w", err)
	}
	offset := (page - 1) * pageSize
	if err := query.Order("id DESC").Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("查询列表失败: %w", err)
	}
	return list, total, nil
}

// Get 获取单条
func (s *CFWorkpageTemplateService) Get(id uint) (*models.CFWorkpageTemplate, error) {
	var t models.CFWorkpageTemplate
	if err := s.db.First(&t, id).Error; err != nil {
		return nil, fmt.Errorf("模版不存在: %w", err)
	}
	return &t, nil
}

// Create 创建
func (s *CFWorkpageTemplateService) Create(nameZh, nameMy, defaultLang string) (*models.CFWorkpageTemplate, error) {
	if defaultLang != "zh" && defaultLang != "my" {
		defaultLang = "zh"
	}
	t := &models.CFWorkpageTemplate{
		NameZh:      strings.TrimSpace(nameZh),
		NameMy:      strings.TrimSpace(nameMy),
		DefaultLang: defaultLang,
	}
	if err := s.db.Create(t).Error; err != nil {
		return nil, fmt.Errorf("创建模版失败: %w", err)
	}
	return t, nil
}

// Update 更新
func (s *CFWorkpageTemplateService) Update(id uint, nameZh, nameMy, defaultLang *string) (*models.CFWorkpageTemplate, error) {
	t, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if nameZh != nil {
		t.NameZh = strings.TrimSpace(*nameZh)
	}
	if nameMy != nil {
		t.NameMy = strings.TrimSpace(*nameMy)
	}
	if defaultLang != nil && (*defaultLang == "zh" || *defaultLang == "my") {
		t.DefaultLang = *defaultLang
	}
	if err := s.db.Save(t).Error; err != nil {
		return nil, fmt.Errorf("更新模版失败: %w", err)
	}
	return t, nil
}

// Delete 删除
func (s *CFWorkpageTemplateService) Delete(id uint) error {
	var count int64
	if err := s.db.Model(&models.CFWorkpageSite{}).Where("template_id = ?", id).Count(&count).Error; err != nil {
		return fmt.Errorf("检查关联站点失败: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("该模版已被 %d 个站点使用，请先解除关联再删除", count)
	}
	return s.db.Delete(&models.CFWorkpageTemplate{}, id).Error
}
