package models

import (
	"time"

	"gorm.io/gorm"
)

// R2CacheRule R2缓存规则模型
type R2CacheRule struct {
	ID              uint           `json:"id" gorm:"primaryKey"`
	R2CustomDomainID uint          `json:"r2_custom_domain_id" gorm:"not null;index"` // 关联的自定义域名 ID
	RuleName        string         `json:"rule_name" gorm:"type:varchar(255);not null"` // 规则名称
	Expression      string         `json:"expression" gorm:"type:text;not null"`        // 匹配表达式（如：http.host eq "assets.jjj0108.com" and http.request.uri.path.extension eq "apk"）
	CacheStatus     string         `json:"cache_status" gorm:"type:varchar(50)"`        // 缓存状态：Eligible, Bypass
	EdgeTTL         string         `json:"edge_ttl" gorm:"type:varchar(50)"`            // Edge TTL（如：1 month）
	BrowserTTL      string         `json:"browser_ttl" gorm:"type:varchar(50)"`         // Browser TTL
	CloudflareRuleID string        `json:"cloudflare_rule_id" gorm:"type:varchar(100)"` // Cloudflare 规则 ID
	Status          string         `json:"status" gorm:"type:varchar(50)"`              // 状态：active, pending, failed
	Note            string         `json:"note" gorm:"type:text"`                       // 备注
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `json:"-" gorm:"index"`

	// 关联
	R2CustomDomain R2CustomDomain `json:"r2_custom_domain,omitempty" gorm:"foreignKey:R2CustomDomainID"`
}

// TableName 指定表名
func (R2CacheRule) TableName() string {
	return "r2_cache_rules"
}
