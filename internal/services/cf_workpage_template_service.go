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

// Delete 删除（会级联删除模版下的所有表格行）
func (s *CFWorkpageTemplateService) Delete(id uint) error {
	var count int64
	if err := s.db.Model(&models.CFWorkpageSite{}).Where("template_id = ?", id).Count(&count).Error; err != nil {
		return fmt.Errorf("检查关联站点失败: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("该模版已被 %d 个站点使用，请先解除关联再删除", count)
	}
	_ = s.db.Where("template_id = ?", id).Delete(&models.CFWorkpageTemplateRow{})
	return s.db.Delete(&models.CFWorkpageTemplate{}, id).Error
}

// ListRows 获取模版下所有表格行（按 sort_order 排序）
func (s *CFWorkpageTemplateService) ListRows(templateID uint) ([]models.CFWorkpageTemplateRow, error) {
	var rows []models.CFWorkpageTemplateRow
	if err := s.db.Where("template_id = ?", templateID).Order("sort_order ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("查询表格行失败: %w", err)
	}
	return rows, nil
}

// SaveRows 批量保存模版表格行（先删后增；若任一行 auto_popup=true，同模版下其余行会被设为 false）
func (s *CFWorkpageTemplateService) SaveRows(templateID uint, rows []models.CFWorkpageTemplateRow) ([]models.CFWorkpageTemplateRow, error) {
	if _, err := s.Get(templateID); err != nil {
		return nil, err
	}
	if err := s.db.Where("template_id = ?", templateID).Delete(&models.CFWorkpageTemplateRow{}).Error; err != nil {
		return nil, fmt.Errorf("清空原表格行失败: %w", err)
	}
	for i := range rows {
		rows[i].ID = 0
		rows[i].TemplateID = templateID
		rows[i].SortOrder = i
		if rows[i].AutoPopup {
			// 同模版只允许一个 auto_popup，前面已删光，无需再清
			break
		}
	}
	// 若有多行 auto_popup，只保留第一个
	hasAuto := false
	for i := range rows {
		if rows[i].AutoPopup {
			if hasAuto {
				rows[i].AutoPopup = false
			} else {
				hasAuto = true
			}
		}
	}
	for i := range rows {
		if err := s.db.Create(&rows[i]).Error; err != nil {
			return nil, fmt.Errorf("创建表格行失败: %w", err)
		}
	}
	return s.ListRows(templateID)
}
