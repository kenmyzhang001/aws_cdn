package models

import (
	"time"

	"gorm.io/gorm"
)

// RedirectRule 重定向规则
type RedirectRule struct {
	ID           uint             `json:"id" gorm:"primaryKey"`
	SourceDomain string           `json:"source_domain" gorm:"uniqueIndex;not null"`
	Targets      []RedirectTarget `json:"targets" gorm:"foreignKey:RuleID;constraint:OnDelete:CASCADE"`
	CloudFrontID string           `json:"cloudfront_id"` // CloudFront Distribution ID
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
	DeletedAt    gorm.DeletedAt   `json:"-" gorm:"index"`
}

// RedirectTarget 重定向目标
type RedirectTarget struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	RuleID    uint           `json:"rule_id" gorm:"not null"`
	TargetURL string         `json:"target_url" gorm:"not null"`
	Weight    int            `json:"weight" gorm:"default:1"` // 权重，用于轮询
	IsActive  bool           `json:"is_active" gorm:"default:true"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (RedirectRule) TableName() string {
	return "redirect_rules"
}

func (RedirectTarget) TableName() string {
	return "redirect_targets"
}
