package models

import (
	"time"

	"gorm.io/gorm"
)

// R2Bucket R2存储桶模型
type R2Bucket struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	CFAccountID uint           `json:"cf_account_id" gorm:"not null;index"` // 关联的 Cloudflare 账号 ID
	BucketName  string         `json:"bucket_name" gorm:"type:varchar(255);not null"` // 存储桶名称
	Location    string         `json:"location" gorm:"type:varchar(100)"`              // 存储位置
	Note        string         `json:"note" gorm:"type:text"`                          // 备注
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`

	// 关联
	CFAccount CFAccount `json:"cf_account,omitempty" gorm:"foreignKey:CFAccountID"`
}

// TableName 指定表名
func (R2Bucket) TableName() string {
	return "r2_buckets"
}
