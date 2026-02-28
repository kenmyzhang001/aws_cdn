package models

import (
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"
)

// WorkerDomainBinding 单个 Worker 域名的 CF 绑定信息（Zone/Route/CustomDomain）
type WorkerDomainBinding struct {
	Domain         string `json:"domain"`
	ZoneID         string `json:"zone_id"`
	WorkerRoute    string `json:"worker_route"`
	CustomDomainID string `json:"custom_domain_id"`
}

// CFWorker Cloudflare Worker 模型
type CFWorker struct {
	ID               uint           `json:"id" gorm:"primaryKey"`
	CFAccountID      uint           `json:"cf_account_id" gorm:"not null;index"`                    // 关联的 CF 账号 ID
	CFAccount        *CFAccount     `json:"cf_account,omitempty" gorm:"foreignKey:CFAccountID"`    // 关联的 CF 账号
	WorkerName       string         `json:"worker_name" gorm:"type:varchar(255);not null"`         // Worker 名称（脚本名称）
	WorkerDomain     string         `json:"worker_domain" gorm:"type:varchar(255);index"`          // 首个/主 Worker 域名（展示与兼容，多域名时取列表第一个）
	WorkerDomains    string         `json:"-" gorm:"type:longtext"`                                // 域名列表 JSON（存库用，接口返回用 WorkerDomainsArray）
	TargetDomain     string         `json:"target_domain" gorm:"type:varchar(255)"`                // 目标跳转（单链接时使用；多目标时存首个用于展示）
	Targets          string         `json:"-" gorm:"type:longtext"`                                 // 目标链接列表 JSON（存库用，接口返回用 TargetsArray）
	FallbackURL      string         `json:"fallback_url" gorm:"type:varchar(512)"`                   // 兜底链接（可选）
	Mode             string         `json:"mode" gorm:"type:varchar(20);default:single"`           // single / time / random / probe
	BusinessMode     string         `json:"business_mode" gorm:"column:business_mode;type:varchar(20);default:推广;index"` // 业务模式：下载、推广
	RotateDays       int            `json:"rotate_days" gorm:"default:0"`                           // 时间轮播每 N 天轮换
	BaseDate         string         `json:"base_date" gorm:"type:varchar(20)"`                     // 时间轮播基准日期
	ZoneID           string         `json:"zone_id" gorm:"type:varchar(100)"`                      // 首个域名所在 Zone（兼容旧数据）
	WorkerRoute      string         `json:"worker_route" gorm:"type:varchar(500)"`                 // 首个域名路由 ID（兼容旧数据）
	CustomDomainID   string         `json:"custom_domain_id" gorm:"type:varchar(100)"`             // 首个域名自定义域名 ID（兼容旧数据）
	DomainBindings   string         `json:"-" gorm:"type:longtext"`                                 // 各域名绑定信息 JSON []WorkerDomainBinding
	Status           string         `json:"status" gorm:"type:varchar(50);default:'active'"`       // 状态：active, inactive
	Description      string         `json:"description" gorm:"type:text"`                          // 描述
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `json:"-" gorm:"index"`

	// 仅用于 JSON 输出，由 AfterFind 解析
	TargetsArray        []string              `json:"targets" gorm:"-"`
	WorkerDomainsArray  []string              `json:"worker_domains" gorm:"-"`  // 绑定的域名列表
	DomainBindingsArray []WorkerDomainBinding `json:"-" gorm:"-"`               // 业务用，不直接序列化到 API
}

// AfterFind 查询后解析 Targets/WorkerDomains/DomainBindings 到对应 Array
func (w *CFWorker) AfterFind(_ *gorm.DB) error {
	w.TargetsArray = w.targetsList()
	w.WorkerDomainsArray = w.domainsList()
	w.DomainBindingsArray = w.bindingsList()
	// 兼容：若仅有 WorkerDomain 无 WorkerDomains，则列表为首域名
	if len(w.WorkerDomainsArray) == 0 && w.WorkerDomain != "" {
		w.WorkerDomainsArray = []string{w.WorkerDomain}
	}
	if len(w.WorkerDomainsArray) > 0 && w.WorkerDomain == "" {
		w.WorkerDomain = w.WorkerDomainsArray[0]
	}
	return nil
}

// BeforeSave 保存前将 WorkerDomainsArray 同步回 WorkerDomains，并同步首个到 WorkerDomain
func (w *CFWorker) BeforeSave(_ *gorm.DB) error {
	if len(w.WorkerDomainsArray) > 0 {
		data, _ := json.Marshal(w.WorkerDomainsArray)
		w.WorkerDomains = string(data)
		w.WorkerDomain = w.WorkerDomainsArray[0]
	} else if w.WorkerDomain != "" {
		w.WorkerDomainsArray = []string{w.WorkerDomain}
		data, _ := json.Marshal(w.WorkerDomainsArray)
		w.WorkerDomains = string(data)
	}
	if len(w.DomainBindingsArray) > 0 {
		data, _ := json.Marshal(w.DomainBindingsArray)
		w.DomainBindings = string(data)
	}
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

func (w *CFWorker) domainsList() []string {
	if w.WorkerDomains == "" {
		return nil
	}
	var list []string
	_ = json.Unmarshal([]byte(w.WorkerDomains), &list)
	return list
}

func (w *CFWorker) bindingsList() []WorkerDomainBinding {
	if w.DomainBindings == "" {
		return nil
	}
	var list []WorkerDomainBinding
	_ = json.Unmarshal([]byte(w.DomainBindings), &list)
	return list
}

// TargetsList 用于业务逻辑的解析后目标列表
func (w *CFWorker) TargetsList() []string {
	return w.targetsList()
}

// DomainsList 返回已绑定的 Worker 域名列表（用于业务逻辑）
func (w *CFWorker) DomainsList() []string {
	list := w.domainsList()
	if len(list) == 0 && w.WorkerDomain != "" {
		return []string{w.WorkerDomain}
	}
	return list
}

// BindDomain 绑定一个域名（仅修改内存列表，需调用方 Save）。若已存在则忽略。不包含 CF 侧操作。
func (w *CFWorker) BindDomain(domain string) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return
	}
	list := w.DomainsList()
	for _, d := range list {
		if d == domain {
			return
		}
	}
	w.WorkerDomainsArray = append(list, domain)
	if w.WorkerDomain == "" {
		w.WorkerDomain = domain
	}
}

// UnbindDomain 解绑一个域名（仅修改内存列表，需调用方 Save）。不包含 CF 侧操作。
func (w *CFWorker) UnbindDomain(domain string) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return
	}
	list := w.DomainsList()
	var newList []string
	for _, d := range list {
		if d != domain {
			newList = append(newList, d)
		}
	}
	w.WorkerDomainsArray = newList
	if len(newList) > 0 {
		w.WorkerDomain = newList[0]
	} else {
		w.WorkerDomain = ""
	}
	// 同时从绑定信息中移除
	var newBindings []WorkerDomainBinding
	for _, b := range w.DomainBindingsArray {
		if b.Domain != domain {
			newBindings = append(newBindings, b)
		}
	}
	w.DomainBindingsArray = newBindings
}

// GetBinding 获取某域名对应的 CF 绑定信息
func (w *CFWorker) GetBinding(domain string) (WorkerDomainBinding, bool) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	for _, b := range w.bindingsList() {
		if strings.ToLower(b.Domain) == domain {
			return b, true
		}
	}
	return WorkerDomainBinding{}, false
}

// SetBinding 设置或更新某域名的绑定信息（仅内存，需 Save）
func (w *CFWorker) SetBinding(binding WorkerDomainBinding) {
	binding.Domain = strings.TrimSpace(strings.ToLower(binding.Domain))
	if binding.Domain == "" {
		return
	}
	list := w.DomainBindingsArray
	found := false
	for i := range list {
		if list[i].Domain == binding.Domain {
			list[i] = binding
			found = true
			break
		}
	}
	if !found {
		w.DomainBindingsArray = append(list, binding)
	} else {
		w.DomainBindingsArray = list
	}
}

// TableName 指定表名
func (CFWorker) TableName() string {
	return "cf_workers"
}
