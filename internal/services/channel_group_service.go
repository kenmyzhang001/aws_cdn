package services

import (
	"aws_cdn/internal/models"
	"fmt"

	"gorm.io/gorm"
)

type ChannelGroupService struct {
	db *gorm.DB
}

func NewChannelGroupService(db *gorm.DB) *ChannelGroupService {
	return &ChannelGroupService{db: db}
}

// List 列出所有渠道分组
func (s *ChannelGroupService) List() ([]models.ChannelGroup, error) {
	var list []models.ChannelGroup
	if err := s.db.Order("id ASC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("获取渠道分组列表失败: %w", err)
	}
	if list == nil {
		list = []models.ChannelGroup{}
	}
	return list, nil
}

// Get 获取单个渠道分组
func (s *ChannelGroupService) Get(id uint) (*models.ChannelGroup, error) {
	var g models.ChannelGroup
	if err := s.db.First(&g, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("渠道分组不存在")
		}
		return nil, fmt.Errorf("获取渠道分组失败: %w", err)
	}
	return &g, nil
}

// Create 创建渠道分组
func (s *ChannelGroupService) Create(name string, channelCodes []string) (*models.ChannelGroup, error) {
	if channelCodes == nil {
		channelCodes = []string{}
	}
	g := &models.ChannelGroup{
		Name:         name,
		ChannelCodes: channelCodes,
	}
	if err := s.db.Create(g).Error; err != nil {
		return nil, fmt.Errorf("创建渠道分组失败: %w", err)
	}
	return g, nil
}

// Update 更新渠道分组
func (s *ChannelGroupService) Update(id uint, name string, channelCodes []string) (*models.ChannelGroup, error) {
	g, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	g.Name = name
	if channelCodes != nil {
		g.ChannelCodes = channelCodes
	}
	if err := s.db.Save(g).Error; err != nil {
		return nil, fmt.Errorf("更新渠道分组失败: %w", err)
	}
	return g, nil
}

// Delete 删除渠道分组
func (s *ChannelGroupService) Delete(id uint) error {
	res := s.db.Delete(&models.ChannelGroup{}, id)
	if res.Error != nil {
		return fmt.Errorf("删除渠道分组失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("渠道分组不存在")
	}
	return nil
}
