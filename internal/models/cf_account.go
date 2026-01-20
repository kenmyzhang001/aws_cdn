package models

import (
	"time"

	"gorm.io/gorm"
)

// CFAccount Cloudflare账号模型
type CFAccount struct {
	ID                uint           `json:"id" gorm:"primaryKey"`
	Email             string         `json:"email" gorm:"type:varchar(255);not null;uniqueIndex"` // Cloudflare账号邮箱
	Password          string         `json:"-" gorm:"type:varchar(255);not null"`                 // bcrypt 哈希密码
	APIToken          string         `json:"-" gorm:"type:text"`                                  // Cloudflare API Token（用于管理 R2 存储桶、自定义域名等，加密存储）
	R2APIToken        string         `json:"-" gorm:"type:text"`                                  // R2 API Token（用于 R2 API 操作，加密存储）
	AccountID         string         `json:"account_id" gorm:"type:varchar(100)"`                 // Cloudflare Account ID
	R2AccessKeyID     string         `json:"-" gorm:"type:varchar(255)"`                          // R2 Access Key ID（账号维度，加密存储）
	R2SecretAccessKey string         `json:"-" gorm:"type:text"`                                  // R2 Secret Access Key（账号维度，加密存储）
	Note              string         `json:"note" gorm:"type:text"`                               // 备注
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (CFAccount) TableName() string {
	return "cf_accounts"
}
