package models

import (
	"time"

	"gorm.io/gorm"
)

// LinkType 链接类型
type LinkType string

const (
	LinkTypeDownloadPackage   LinkType = "download_package"   // AWS CDN下载包
	LinkTypeCustomDownloadLink LinkType = "custom_download_link" // 自定义下载链接
	LinkTypeR2File             LinkType = "r2_file"            // R2文件
)

// FocusProbeLink 重点探测链接模型
type FocusProbeLink struct {
	ID                  uint           `json:"id" gorm:"primaryKey"`
	LinkType            LinkType       `json:"link_type" gorm:"type:varchar(50);not null;index:idx_link_type"` // 链接类型
	LinkID              *uint          `json:"link_id" gorm:"index:idx_link_id"`                                // 关联的原始链接ID（可选）
	URL                 string         `json:"url" gorm:"type:varchar(1000);not null;uniqueIndex:idx_url"`     // 探测的URL
	Name                string         `json:"name" gorm:"type:varchar(255)"`                                   // 链接名称
	Description         string         `json:"description" gorm:"type:text"`                                    // 链接描述
	ProbeIntervalMinutes int           `json:"probe_interval_minutes" gorm:"default:10"`                        // 探测间隔（分钟）
	Enabled             bool           `json:"enabled" gorm:"default:true;index:idx_enabled"`                   // 是否启用
	LastProbeTime       *time.Time     `json:"last_probe_time"`                                                 // 最后探测时间
	LastProbeStatus     string         `json:"last_probe_status" gorm:"type:varchar(20)"`                       // 最后探测状态
	LastProbeSpeedKbps  *float64       `json:"last_probe_speed_kbps" gorm:"type:decimal(10,2)"`                 // 最后探测速度
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (FocusProbeLink) TableName() string {
	return "focus_probe_links"
}
