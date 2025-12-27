package services

import (
	"aws_cdn/internal/models"
	"fmt"

	"gorm.io/gorm"
)

type GroupService struct {
	db *gorm.DB
}

func NewGroupService(db *gorm.DB) *GroupService {
	return &GroupService{db: db}
}

// GetDefaultGroup 获取默认分组
func (s *GroupService) GetDefaultGroup() (*models.Group, error) {
	var group models.Group
	if err := s.db.Where("is_default = ?", true).First(&group).Error; err != nil {
		return nil, fmt.Errorf("获取默认分组失败: %w", err)
	}
	return &group, nil
}

// GetOrCreateDefaultGroup 获取或创建默认分组
func (s *GroupService) GetOrCreateDefaultGroup() (*models.Group, error) {
	var group models.Group
	if err := s.db.Where("is_default = ?", true).First(&group).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// 创建默认分组
			group = models.Group{
				Name:      "默认分组",
				IsDefault: true,
			}
			if err := s.db.Create(&group).Error; err != nil {
				return nil, fmt.Errorf("创建默认分组失败: %w", err)
			}
			return &group, nil
		}
		return nil, fmt.Errorf("获取默认分组失败: %w", err)
	}
	return &group, nil
}

// ListGroups 列出所有分组
func (s *GroupService) ListGroups() ([]models.Group, error) {
	var groups []models.Group
	if err := s.db.Order("is_default DESC, id ASC").Find(&groups).Error; err != nil {
		return nil, fmt.Errorf("获取分组列表失败: %w", err)
	}
	return groups, nil
}

// GetGroup 获取分组信息
func (s *GroupService) GetGroup(id uint) (*models.Group, error) {
	var group models.Group
	if err := s.db.First(&group, id).Error; err != nil {
		return nil, fmt.Errorf("分组不存在: %w", err)
	}
	return &group, nil
}

// CreateGroup 创建分组
func (s *GroupService) CreateGroup(name string) (*models.Group, error) {
	group := &models.Group{
		Name:      name,
		IsDefault: false,
	}
	if err := s.db.Create(group).Error; err != nil {
		return nil, fmt.Errorf("创建分组失败: %w", err)
	}
	return group, nil
}

// UpdateGroup 更新分组
func (s *GroupService) UpdateGroup(id uint, name string) (*models.Group, error) {
	group, err := s.GetGroup(id)
	if err != nil {
		return nil, err
	}
	group.Name = name
	if err := s.db.Save(group).Error; err != nil {
		return nil, fmt.Errorf("更新分组失败: %w", err)
	}
	return group, nil
}

// DeleteGroup 删除分组
func (s *GroupService) DeleteGroup(id uint) error {
	group, err := s.GetGroup(id)
	if err != nil {
		return err
	}
	if group.IsDefault {
		return fmt.Errorf("不能删除默认分组")
	}

	// 检查是否有域名关联到此分组
	var domainCount int64
	if err := s.db.Model(&models.Domain{}).Where("group_id = ? AND deleted_at IS NULL", id).Count(&domainCount).Error; err != nil {
		return fmt.Errorf("检查域名关联失败: %w", err)
	}
	if domainCount > 0 {
		return fmt.Errorf("该分组下还有 %d 个域名，无法删除。请先将这些域名转移到其他分组后再删除", domainCount)
	}

	// 检查是否有重定向规则关联到此分组
	var redirectCount int64
	if err := s.db.Table("redirect_rules").Where("group_id = ? AND deleted_at IS NULL", id).Count(&redirectCount).Error; err != nil {
		return fmt.Errorf("检查重定向规则关联失败: %w", err)
	}
	if redirectCount > 0 {
		return fmt.Errorf("该分组下还有 %d 个重定向规则，无法删除。请先将这些重定向规则转移到其他分组后再删除", redirectCount)
	}

	// 检查是否有下载包关联到此分组
	var downloadPackageCount int64
	if err := s.db.Table("download_packages").Where("group_id = ? AND deleted_at IS NULL", id).Count(&downloadPackageCount).Error; err != nil {
		return fmt.Errorf("检查下载包关联失败: %w", err)
	}
	if downloadPackageCount > 0 {
		return fmt.Errorf("该分组下还有 %d 个下载包，无法删除。请先将这些下载包转移到其他分组后再删除", downloadPackageCount)
	}

	if err := s.db.Delete(group).Error; err != nil {
		return fmt.Errorf("删除分组失败: %w", err)
	}
	return nil
}
