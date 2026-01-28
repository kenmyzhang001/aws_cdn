package models

import (
	"time"

	"gorm.io/gorm"
)

// R2CustomDomain R2自定义域名模型
type R2CustomDomain struct {
	ID              uint           `json:"id" gorm:"primaryKey"`
	R2BucketID      uint           `json:"r2_bucket_id" gorm:"not null;index"`       // 关联的 R2 存储桶 ID
	Domain          string         `json:"domain" gorm:"type:varchar(255);not null"` // 自定义域名（如 assets.jjj0108.com）
	ZoneID          string         `json:"zone_id" gorm:"type:varchar(100)"`         // Cloudflare Zone ID
	Status          string         `json:"status" gorm:"type:varchar(50)"`           // 状态：active, pending, failed
	Note            string         `json:"note" gorm:"type:text"`                    // 备注
	DefaultFilePath string         `json:"default_file_path" gorm:"type:varchar(500)"` // 默认文件路径，访问根路径时自动下载该文件
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `json:"-" gorm:"index"`

	// 关联
	R2Bucket R2Bucket `json:"r2_bucket,omitempty" gorm:"foreignKey:R2BucketID"`
}

// TableName 指定表名
func (R2CustomDomain) TableName() string {
	return "r2_custom_domains"
}
