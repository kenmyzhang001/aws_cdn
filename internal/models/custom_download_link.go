package models

import (
	"time"

	"gorm.io/gorm"
)

// CustomDownloadLinkStatus 自定义下载链接状态
type CustomDownloadLinkStatus string

const (
	CustomDownloadLinkStatusActive   CustomDownloadLinkStatus = "active"   // 启用
	CustomDownloadLinkStatusInactive CustomDownloadLinkStatus = "inactive" // 禁用
)

// CustomDownloadLink 自定义下载链接模型
type CustomDownloadLink struct {
	ID          uint                     `json:"id" gorm:"primaryKey"`
	URL         string                   `json:"url" gorm:"type:varchar(1000);not null"`         // 下载链接URL
	Name        string                   `json:"name" gorm:"type:varchar(255);default:''"`       // 链接名称
	Description string                   `json:"description" gorm:"type:text"`                   // 链接描述
	GroupID     *uint                    `json:"group_id" gorm:"index"`                          // 所属分组ID
	Group       *Group                   `json:"group,omitempty" gorm:"foreignKey:GroupID"`      // 分组关联
	Status      CustomDownloadLinkStatus `json:"status" gorm:"default:'active'"`                 // 状态
	ClickCount  uint                     `json:"click_count" gorm:"default:0"`                   // 点击次数
	CreatedAt   time.Time                `json:"created_at"`
	UpdatedAt   time.Time                `json:"updated_at"`
	DeletedAt   gorm.DeletedAt           `json:"-" gorm:"index"`
}

// TableName 指定表名
func (CustomDownloadLink) TableName() string {
	return "custom_download_links"
}
