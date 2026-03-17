package models

import (
	"time"

	"gorm.io/gorm"
)

// CFWorkpageSite CF-WorkPage 站点（关联 CF 账号、模版，绑定主域名与子域名）
type CFWorkpageSite struct {
	ID           uint                  `json:"id" gorm:"primaryKey"`
	CFAccountID  uint                  `json:"cf_account_id" gorm:"not null"`
	CFAccount    *CFAccount             `json:"cf_account,omitempty" gorm:"foreignKey:CFAccountID"`
	TemplateID   uint                  `json:"template_id" gorm:"not null"`
	Template     *CFWorkpageTemplate    `json:"template,omitempty" gorm:"foreignKey:TemplateID"`
	ZoneID       string                `json:"zone_id" gorm:"type:varchar(64);not null"`   // CF Zone ID（主域名所属）
	MainDomain   string                `json:"main_domain" gorm:"type:varchar(255);not null"` // 主域名（来自 Zone，下拉选择）
	Subdomain    string                `json:"subdomain" gorm:"type:varchar(128)"`         // 子域名前缀，如 www、app；空表示仅用主域名
	Status       string                `json:"status" gorm:"type:varchar(32);default:'pending'"` // pending | deployed | failed
	CreatedAt    time.Time             `json:"created_at"`
	UpdatedAt    time.Time             `json:"updated_at"`
	DeletedAt    gorm.DeletedAt        `json:"-" gorm:"index"`
}

// TableName 指定表名
func (CFWorkpageSite) TableName() string {
	return "cf_workpage_sites"
}
