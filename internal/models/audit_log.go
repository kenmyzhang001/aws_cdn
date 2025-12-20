package models

import (
	"time"
)

// AuditLog 审计日志模型
type AuditLog struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"user_id" gorm:"index;not null"`           // 操作用户ID
	Username  string    `json:"username" gorm:"type:varchar(191);index"` // 操作用户名
	Action    string    `json:"action" gorm:"type:varchar(100);index;not null"` // 操作类型
	Resource  string    `json:"resource" gorm:"type:varchar(100);index"`  // 资源类型（domain, redirect, cloudfront, download_package等）
	ResourceID string   `json:"resource_id" gorm:"type:varchar(100);index"` // 资源ID
	Method    string    `json:"method" gorm:"type:varchar(10)"`         // HTTP方法
	Path      string    `json:"path" gorm:"type:varchar(500)"`           // 请求路径
	IP        string    `json:"ip" gorm:"type:varchar(50);index"`        // 客户端IP
	UserAgent string    `json:"user_agent" gorm:"type:varchar(500)"`    // 用户代理
	Request   string    `json:"request" gorm:"type:text"`               // 请求数据（JSON格式）
	Response  string    `json:"response" gorm:"type:text"`               // 响应数据（JSON格式，仅记录关键信息）
	Status    int       `json:"status" gorm:"index"`                     // HTTP状态码
	Message   string    `json:"message" gorm:"type:text"`                // 操作描述
	Error     string    `json:"error" gorm:"type:text"`                  // 错误信息（如果有）
	Duration  int64     `json:"duration"`                                // 操作耗时（毫秒）
	CreatedAt time.Time `json:"created_at" gorm:"index"`
}

// TableName 指定表名
func (AuditLog) TableName() string {
	return "audit_logs"
}

