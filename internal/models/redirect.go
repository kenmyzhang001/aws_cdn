package models

import (
	"time"

	"gorm.io/gorm"
)

// RedirectRuleStatus 重定向规则状态
type RedirectRuleStatus string

const (
	RedirectRuleStatusPending    RedirectRuleStatus = "pending"    // 待处理
	RedirectRuleStatusProcessing RedirectRuleStatus = "processing" // 处理中
	RedirectRuleStatusCompleted  RedirectRuleStatus = "completed"  // 已完成（所有配置通过）
	RedirectRuleStatusFailed     RedirectRuleStatus = "failed"     // 失败
)

// RedirectRule 重定向规则
type RedirectRule struct {
	ID           uint               `json:"id" gorm:"primaryKey"`
	SourceDomain string             `json:"source_domain" gorm:"type:varchar(255);uniqueIndex;not null"`
	GroupID      *uint              `json:"group_id" gorm:"index"`                     // 所属分组ID
	Group        *Group             `json:"group,omitempty" gorm:"foreignKey:GroupID"` // 分组关联
	Targets      []RedirectTarget   `json:"targets" gorm:"foreignKey:RuleID;constraint:OnDelete:CASCADE"`
	CloudFrontID string             `json:"cloudfront_id"`                                    // CloudFront Distribution ID
	Status       RedirectRuleStatus `json:"status" gorm:"type:varchar(32);default:'pending'"` // 状态
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
	DeletedAt    gorm.DeletedAt     `json:"-" gorm:"index"`
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
