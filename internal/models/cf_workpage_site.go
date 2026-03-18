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
	Status       string                `json:"status" gorm:"type:varchar(32);default:'pending'"` // pending | deploying | deployed | failed

	PagesProjectName string `json:"pages_project_name" gorm:"type:varchar(128)"` // Pages 项目名（自动生成/可复用）
	DeploymentID     string `json:"deployment_id" gorm:"type:varchar(64)"`        // Pages Deployment ID
	DeploymentURL    string `json:"deployment_url" gorm:"type:varchar(512)"`      // Pages 访问地址（pages.dev 或自定义域名）
	CustomDomain     string `json:"custom_domain" gorm:"type:varchar(255)"`       // 绑定的自定义域名（主域名或子域名）
	LastError        string `json:"last_error" gorm:"type:text"`                  // 最近一次部署错误
	DeployedAt       *time.Time `json:"deployed_at" gorm:"default:null"`          // 部署成功时间
	// 最近一次成功上传到 Pages 的 index.html 原文（仅接口单独查询，列表/详情 JSON 不返回以减小体积）
	DeployedIndexHTML string `json:"-" gorm:"type:longtext"`

	CreatedAt    time.Time             `json:"created_at"`
	UpdatedAt    time.Time             `json:"updated_at"`
	DeletedAt    gorm.DeletedAt        `json:"-" gorm:"index"`
}

// TableName 指定表名
func (CFWorkpageSite) TableName() string {
	return "cf_workpage_sites"
}
