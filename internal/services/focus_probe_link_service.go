package services

import (
	"aws_cdn/internal/models"
	"errors"

	"gorm.io/gorm"
)

type FocusProbeLinkService struct {
	db *gorm.DB
}

func NewFocusProbeLinkService(db *gorm.DB) *FocusProbeLinkService {
	return &FocusProbeLinkService{db: db}
}

// GetFocusProbeLinks 获取重点探测链接列表（分页）
func (s *FocusProbeLinkService) GetFocusProbeLinks(page, pageSize int, linkType string, enabled *bool, search string) ([]models.FocusProbeLink, int64, error) {
	var links []models.FocusProbeLink
	var total int64

	query := s.db.Model(&models.FocusProbeLink{})

	// 过滤条件
	if linkType != "" {
		query = query.Where("link_type = ?", linkType)
	}
	if enabled != nil {
		query = query.Where("enabled = ?", *enabled)
	}
	if search != "" {
		query = query.Where("url LIKE ? OR name LIKE ? OR description LIKE ?",
			"%"+search+"%", "%"+search+"%", "%"+search+"%")
	}

	// 统计总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&links).Error; err != nil {
		return nil, 0, err
	}

	return links, total, nil
}

// GetFocusProbeLinkByID 根据ID获取重点探测链接
func (s *FocusProbeLinkService) GetFocusProbeLinkByID(id uint) (*models.FocusProbeLink, error) {
	var link models.FocusProbeLink
	if err := s.db.First(&link, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("重点探测链接不存在")
		}
		return nil, err
	}
	return &link, nil
}

// CreateFocusProbeLink 创建重点探测链接
func (s *FocusProbeLinkService) CreateFocusProbeLink(link *models.FocusProbeLink) error {
	// 检查URL是否已存在
	var existingLink models.FocusProbeLink
	if err := s.db.Where("url = ?", link.URL).First(&existingLink).Error; err == nil {
		return errors.New("该URL已存在于重点探测链接中")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	// 创建链接
	if err := s.db.Create(link).Error; err != nil {
		return err
	}

	return nil
}

// UpdateFocusProbeLink 更新重点探测链接
func (s *FocusProbeLinkService) UpdateFocusProbeLink(id uint, updates map[string]interface{}) error {
	result := s.db.Model(&models.FocusProbeLink{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("重点探测链接不存在")
	}
	return nil
}

// DeleteFocusProbeLink 删除重点探测链接（软删除）
func (s *FocusProbeLinkService) DeleteFocusProbeLink(id uint) error {
	result := s.db.Delete(&models.FocusProbeLink{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("重点探测链接不存在")
	}
	return nil
}

// BatchDeleteFocusProbeLinks 批量删除重点探测链接
func (s *FocusProbeLinkService) BatchDeleteFocusProbeLinks(ids []uint) error {
	if len(ids) == 0 {
		return errors.New("未指定要删除的链接")
	}

	result := s.db.Delete(&models.FocusProbeLink{}, ids)
	if result.Error != nil {
		return result.Error
	}

	return nil
}

// BatchUpdateProbeInterval 批量更新探测间隔
func (s *FocusProbeLinkService) BatchUpdateProbeInterval(ids []uint, intervalMinutes int) error {
	if len(ids) == 0 {
		return errors.New("未指定要更新的链接")
	}

	if intervalMinutes <= 0 {
		return errors.New("探测间隔必须大于0")
	}

	result := s.db.Model(&models.FocusProbeLink{}).
		Where("id IN ?", ids).
		Update("probe_interval_minutes", intervalMinutes)

	if result.Error != nil {
		return result.Error
	}

	return nil
}

// UpdateAllProbeInterval 更新所有重点探测链接的探测间隔
func (s *FocusProbeLinkService) UpdateAllProbeInterval(intervalMinutes int) error {
	if intervalMinutes <= 0 {
		return errors.New("探测间隔必须大于0")
	}

	result := s.db.Model(&models.FocusProbeLink{}).
		Where("enabled = ?", true).
		Update("probe_interval_minutes", intervalMinutes)

	if result.Error != nil {
		return result.Error
	}

	return nil
}

// ToggleEnabled 切换启用状态
func (s *FocusProbeLinkService) ToggleEnabled(id uint) error {
	var link models.FocusProbeLink
	if err := s.db.First(&link, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("重点探测链接不存在")
		}
		return err
	}

	link.Enabled = !link.Enabled
	if err := s.db.Save(&link).Error; err != nil {
		return err
	}

	return nil
}

// GetEnabledLinks 获取所有启用的重点探测链接
func (s *FocusProbeLinkService) GetEnabledLinks() ([]models.FocusProbeLink, error) {
	var links []models.FocusProbeLink
	if err := s.db.Where("enabled = ?", true).Find(&links).Error; err != nil {
		return nil, err
	}
	return links, nil
}

// CheckIfURLExists 检查URL是否已存在
func (s *FocusProbeLinkService) CheckIfURLExists(url string) (bool, error) {
	var count int64
	if err := s.db.Model(&models.FocusProbeLink{}).Where("url = ?", url).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetStatistics 获取统计信息
func (s *FocusProbeLinkService) GetStatistics() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// 总数
	var total int64
	if err := s.db.Model(&models.FocusProbeLink{}).Count(&total).Error; err != nil {
		return nil, err
	}
	stats["total"] = total

	// 启用数
	var enabled int64
	if err := s.db.Model(&models.FocusProbeLink{}).Where("enabled = ?", true).Count(&enabled).Error; err != nil {
		return nil, err
	}
	stats["enabled"] = enabled

	// 按类型统计
	var typeCounts []struct {
		LinkType string
		Count    int64
	}
	if err := s.db.Model(&models.FocusProbeLink{}).
		Select("link_type, COUNT(*) as count").
		Group("link_type").
		Scan(&typeCounts).Error; err != nil {
		return nil, err
	}

	typeStats := make(map[string]int64)
	for _, tc := range typeCounts {
		typeStats[tc.LinkType] = tc.Count
	}
	stats["by_type"] = typeStats

	return stats, nil
}

// AddFromDownloadPackage 从下载包添加到重点探测
func (s *FocusProbeLinkService) AddFromDownloadPackage(packageID uint, url, name string) error {
	link := &models.FocusProbeLink{
		LinkType:             models.LinkTypeDownloadPackage,
		LinkID:               &packageID,
		URL:                  url,
		Name:                 name,
		ProbeIntervalMinutes: 30, // 默认30分钟
		Enabled:              true,
	}
	return s.CreateFocusProbeLink(link)
}

// AddFromCustomDownloadLink 从自定义下载链接添加到重点探测
func (s *FocusProbeLinkService) AddFromCustomDownloadLink(linkID uint, url, name string) error {
	link := &models.FocusProbeLink{
		LinkType:             models.LinkTypeCustomDownloadLink,
		LinkID:               &linkID,
		URL:                  url,
		Name:                 name,
		ProbeIntervalMinutes: 30, // 默认30分钟
		Enabled:              true,
	}
	return s.CreateFocusProbeLink(link)
}

// AddFromR2File 从R2文件添加到重点探测
func (s *FocusProbeLinkService) AddFromR2File(url, name, description string) error {
	link := &models.FocusProbeLink{
		LinkType:             models.LinkTypeR2File,
		URL:                  url,
		Name:                 name,
		Description:          description,
		ProbeIntervalMinutes: 30, // 默认30分钟
		Enabled:              true,
	}
	return s.CreateFocusProbeLink(link)
}

// UpdateProbeResult 更新探测结果
func (s *FocusProbeLinkService) UpdateProbeResult(url string, status string, speedKbps float64) error {
	var link models.FocusProbeLink
	if err := s.db.Where("url = ?", url).First(&link).Error; err != nil {
		// 如果链接不存在，不报错
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	updates := map[string]interface{}{
		"last_probe_time":       gorm.Expr("NOW()"),
		"last_probe_status":     status,
		"last_probe_speed_kbps": speedKbps,
	}

	if err := s.db.Model(&link).Updates(updates).Error; err != nil {
		return err
	}

	return nil
}

// GetLinksByType 按类型获取链接
func (s *FocusProbeLinkService) GetLinksByType(linkType models.LinkType) ([]models.FocusProbeLink, error) {
	var links []models.FocusProbeLink
	if err := s.db.Where("link_type = ?", linkType).Order("created_at DESC").Find(&links).Error; err != nil {
		return nil, err
	}
	return links, nil
}

// GetLinksNeedingProbe 获取需要探测的链接（基于探测间隔）
func (s *FocusProbeLinkService) GetLinksNeedingProbe() ([]models.FocusProbeLink, error) {
	var links []models.FocusProbeLink

	// 查询启用的链接，且满足以下条件之一：
	// 1. 从未探测过（last_probe_time为空）
	// 2. 距离上次探测时间超过探测间隔
	query := s.db.Where("enabled = ?", true).Where(
		s.db.Where("last_probe_time IS NULL").Or(
			"TIMESTAMPDIFF(MINUTE, last_probe_time, NOW()) >= probe_interval_minutes",
		),
	)

	if err := query.Find(&links).Error; err != nil {
		return nil, err
	}

	return links, nil
}

// ExportLinks 导出链接列表（用于配置探测任务）
func (s *FocusProbeLinkService) ExportLinks() ([]string, error) {
	var links []models.FocusProbeLink
	if err := s.db.Where("enabled = ?", true).Find(&links).Error; err != nil {
		return nil, err
	}

	urls := make([]string, len(links))
	for i, link := range links {
		urls[i] = link.URL
	}

	return urls, nil
}

// GetProbeIntervalForURL 获取URL的探测间隔（分钟）
// 如果URL在FocusProbeLink中存在，返回其设置的间隔；否则返回默认的30分钟
func (s *FocusProbeLinkService) GetProbeIntervalForURL(url string) (int, error) {
	var link models.FocusProbeLink
	err := s.db.Where("url = ?", url).First(&link).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 如果不存在，返回默认的30分钟
			return 30, nil
		}
		return 0, err
	}

	return link.ProbeIntervalMinutes, nil
}
