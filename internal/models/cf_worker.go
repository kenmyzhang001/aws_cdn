package models

import (
	"encoding/json"
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
	TargetDomain     string         `json:"target_domain" gorm:"type:varchar(255)"`                // 目标跳转（单链接时使用；多目标时存首个用于展示）
	Targets          string         `json:"-" gorm:"type:longtext"`                                 // 目标链接列表 JSON（存库用，接口返回用 TargetsArray）
	FallbackURL      string         `json:"fallback_url" gorm:"type:varchar(512)"`                   // 兜底链接（可选）
	Mode             string         `json:"mode" gorm:"type:varchar(20);default:single"`           // single / time / random / probe
	RotateDays       int            `json:"rotate_days" gorm:"default:0"`                           // 时间轮播每 N 天轮换
	BaseDate         string         `json:"base_date" gorm:"type:varchar(20)"`                     // 时间轮播基准日期
	ZoneID           string         `json:"zone_id" gorm:"type:varchar(100)"`                      // Worker 域名所在的 Zone ID
	WorkerRoute      string         `json:"worker_route" gorm:"type:varchar(500)"`                 // Worker 路由 ID（如果使用路由模式）
	CustomDomainID   string         `json:"custom_domain_id" gorm:"type:varchar(100)"`             // Worker 自定义域名 ID（如果使用自定义域名模式）
	Status           string         `json:"status" gorm:"type:varchar(50);default:'active'"`       // 状态：active, inactive
	Description      string         `json:"description" gorm:"type:text"`                          // 描述
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `json:"-" gorm:"index"`

	// TargetsArray 仅用于 JSON 输出，由 AfterFind 从 Targets 解析
	TargetsArray []string `json:"targets" gorm:"-"`
}

// AfterFind 查询后解析 Targets JSON 到 TargetsArray
func (w *CFWorker) AfterFind(_ *gorm.DB) error {
	w.TargetsArray = w.targetsList()
	return nil
}

func (w *CFWorker) targetsList() []string {
	if w.Targets == "" {
		if w.TargetDomain != "" {
			return []string{w.TargetDomain}
		}
		return nil
	}
	var list []string
	_ = json.Unmarshal([]byte(w.Targets), &list)
	return list
}

// TargetsList 用于业务逻辑的解析后目标列表
func (w *CFWorker) TargetsList() []string {
	return w.targetsList()
}

// TableName 指定表名
func (CFWorker) TableName() string {
	return "cf_workers"
}
