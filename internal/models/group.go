package models

import (
	"time"

	"gorm.io/gorm"
)

// Group 分组模型
type Group struct {
	ID                   uint           `json:"id" gorm:"primaryKey"`
	Name                 string         `json:"name" gorm:"type:varchar(255);not null;uniqueIndex"`
	IsDefault            bool           `json:"is_default" gorm:"default:false"`                  // 是否为默认分组
	ProbeEnabled         bool           `json:"probe_enabled" gorm:"default:true"`                // 是否启用探测
	ProbeIntervalMinutes int            `json:"probe_interval_minutes" gorm:"default:10"`         // 探测间隔（分钟）
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
	DeletedAt            gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (Group) TableName() string {
	return "groups"
}
