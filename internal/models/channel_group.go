package models

import (
	"time"

	"gorm.io/gorm"
)

// ChannelGroup 渠道分组（用于站点日数据分组统计，与域名/下载包等 Group 区分）
type ChannelGroup struct {
	ID           uint           `json:"id" gorm:"primaryKey"`
	Name         string         `json:"name" gorm:"type:varchar(255);not null"`
	ChannelCodes []string       `json:"channel_codes" gorm:"type:json;serializer:json"` // 渠道编码列表
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (ChannelGroup) TableName() string {
	return "channel_groups"
}
