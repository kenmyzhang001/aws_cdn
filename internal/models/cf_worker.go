package models

import (
	"time"

	"gorm.io/gorm"
)

// CFWorker Cloudflare Worker 模型
type CFWorker struct {
	ID               uint           `json:"id" gorm:"primaryKey"`
	CFAccountID      uint           `json:"cf_account_id" gorm:"not null;index"`                    // 关联的 CF 账号 ID
	CFAccount        *CFAccount     `json:"cf_account,omitempty" gorm:"foreignKey:CFAccountID"`    // 关联的 CF 账号
	WorkerName       string         `json:"worker_name" gorm:"type:varchar(255);not null"`         // Worker 名称（脚本名称）
	WorkerDomain     string         `json:"worker_domain" gorm:"type:varchar(255);not null;index"` // Worker 域名（域名 A）
	TargetDomain     string         `json:"target_domain" gorm:"type:varchar(255);not null"`       // 目标跳转域名（域名 B）
	ZoneID           string         `json:"zone_id" gorm:"type:varchar(100)"`                      // Worker 域名所在的 Zone ID
	WorkerRoute      string         `json:"worker_route" gorm:"type:varchar(500)"`                 // Worker 路由 ID（如果使用路由模式）
	CustomDomainID   string         `json:"custom_domain_id" gorm:"type:varchar(100)"`             // Worker 自定义域名 ID（如果使用自定义域名模式）
	Status           string         `json:"status" gorm:"type:varchar(50);default:'active'"`       // 状态：active, inactive
	Description      string         `json:"description" gorm:"type:text"`                          // 描述
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (CFWorker) TableName() string {
	return "cf_workers"
}
