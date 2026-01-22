package models

import (
	"time"
)

// SpeedProbeStatus 探测状态
type SpeedProbeStatus string

const (
	SpeedProbeStatusSuccess SpeedProbeStatus = "success" // 成功
	SpeedProbeStatusFailed  SpeedProbeStatus = "failed"  // 失败
	SpeedProbeStatusTimeout SpeedProbeStatus = "timeout" // 超时
)

// SpeedProbeResult 速度探测结果模型
type SpeedProbeResult struct {
	ID             uint             `json:"id" gorm:"primaryKey"`
	URL            string           `json:"url" gorm:"type:varchar(1000);not null;index:idx_url"`                       // 探测的URL
	ClientIP       string           `json:"client_ip" gorm:"type:varchar(50);not null;index:idx_client_ip"`             // 客户端IP地址
	SpeedKbps      float64          `json:"speed_kbps" gorm:"type:decimal(10,2);not null"`                              // 下载速度 KB/s
	FileSize       *int64           `json:"file_size,omitempty" gorm:"default:null"`                                    // 文件大小（字节）
	DownloadTimeMs *int64           `json:"download_time_ms,omitempty" gorm:"default:null"`                             // 下载耗时（毫秒）
	Status         SpeedProbeStatus `json:"status" gorm:"type:varchar(20);not null;default:'success';index:idx_status"` // 探测状态
	ErrorMessage   string           `json:"error_message,omitempty" gorm:"type:text"`                                   // 错误信息
	UserAgent      string           `json:"user_agent,omitempty" gorm:"type:varchar(500)"`                              // 客户端User-Agent
	CreatedAt      time.Time        `json:"created_at" gorm:"index:idx_created_at;index:idx_url_ip_created"`            // 创建时间
}

// TableName 指定表名
func (SpeedProbeResult) TableName() string {
	return "speed_probe_results"
}

// SpeedAlertLog 速度告警记录模型（按URL维度）
type SpeedAlertLog struct {
	ID              uint      `json:"id" gorm:"primaryKey"`
	URL             string    `json:"url" gorm:"type:varchar(1000);not null;index:idx_url"`            // 告警的URL
	TimeWindowStart time.Time `json:"time_window_start" gorm:"not null"`                               // 时间窗口开始
	TimeWindowEnd   time.Time `json:"time_window_end" gorm:"not null"`                                 // 时间窗口结束
	TotalIPs        int       `json:"total_ips" gorm:"not null"`                                       // 探测该URL的IP总数
	FailedIPs       int       `json:"failed_ips" gorm:"not null"`                                      // 未达标的IP数量
	SuccessIPs      int       `json:"success_ips" gorm:"not null"`                                     // 达标的IP数量
	FailedRate      float64   `json:"failed_rate" gorm:"type:decimal(5,2);not null"`                   // 未达标IP比例（百分比）
	AvgSpeedKbps    *float64  `json:"avg_speed_kbps,omitempty" gorm:"type:decimal(10,2);default:null"` // 所有IP的平均速度 KB/s
	AlertSent       bool      `json:"alert_sent" gorm:"default:false;index:idx_alert_sent"`            // 是否已发送告警
	AlertMessage    string    `json:"alert_message,omitempty" gorm:"type:text"`                        // 告警消息
	IPDetails       string    `json:"ip_details,omitempty" gorm:"type:text"`                           // IP探测详情（JSON格式）
	CreatedAt       time.Time `json:"created_at" gorm:"index:idx_created_at"`                          // 创建时间
}

// TableName 指定表名
func (SpeedAlertLog) TableName() string {
	return "speed_alert_logs"
}
