package models

import (
	"time"

	"gorm.io/gorm"
)

// DomainRedirect 域名 302 重定向（Cloudflare Redirect Rules）
type DomainRedirect struct {
	ID            uint           `json:"id" gorm:"primaryKey"`
	CFAccountID   uint           `json:"cf_account_id" gorm:"not null"`
	CFAccount     *CFAccount     `json:"cf_account,omitempty" gorm:"foreignKey:CFAccountID"`
	ZoneID        string         `json:"zone_id" gorm:"type:varchar(64);not null"`
	SourceDomain  string         `json:"source_domain" gorm:"type:varchar(255);not null"`
	TargetDomain  string         `json:"target_domain" gorm:"type:varchar(255);not null"`
	PreservePath  bool           `json:"preserve_path" gorm:"default:true"` // 是否保留路径与查询串
	CFRuleID      string         `json:"cf_rule_id" gorm:"type:varchar(64)"`
	Status        string         `json:"status" gorm:"type:varchar(32);default:'active'"` // active, disabled
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (DomainRedirect) TableName() string {
	return "domain_redirects"
}
