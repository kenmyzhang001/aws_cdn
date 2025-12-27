package models

import (
	"time"

	"gorm.io/gorm"
)

// Group 分组模型
type Group struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	Name      string         `json:"name" gorm:"type:varchar(255);not null;uniqueIndex"`
	IsDefault bool           `json:"is_default" gorm:"default:false"` // 是否为默认分组
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (Group) TableName() string {
	return "groups"
}

