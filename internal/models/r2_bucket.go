package models

import (
	"time"

	"gorm.io/gorm"
)

// R2Bucket R2存储桶模型
type R2Bucket struct {
	ID                uint           `json:"id" gorm:"primaryKey"`
	CFAccountID       uint           `json:"cf_account_id" gorm:"not null;index"`           // 关联的 Cloudflare 账号 ID
	BucketName        string         `json:"bucket_name" gorm:"type:varchar(255);not null"`  // 存储桶名称
	Location          string         `json:"location" gorm:"type:varchar(100)"`             // 存储位置
	AccountID         string         `json:"account_id" gorm:"type:varchar(100)"`            // Cloudflare Account ID（用于构建 R2 端点）
	R2AccessKeyID     string         `json:"-" gorm:"type:varchar(255)"`                    // R2 Access Key ID（加密存储）
	R2SecretAccessKey string         `json:"-" gorm:"type:text"`                            // R2 Secret Access Key（加密存储）
	Note              string         `json:"note" gorm:"type:text"`                         // 备注
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `json:"-" gorm:"index"`

	// 关联
	CFAccount CFAccount `json:"cf_account,omitempty" gorm:"foreignKey:CFAccountID"`
}

// TableName 指定表名
func (R2Bucket) TableName() string {
	return "r2_buckets"
}
