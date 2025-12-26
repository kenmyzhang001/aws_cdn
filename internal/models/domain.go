package models

import (
	"time"

	"gorm.io/gorm"
)

// DomainStatus 域名状态
type DomainStatus string

const (
	DomainStatusPending    DomainStatus = "pending"     // 待转入
	DomainStatusInProgress DomainStatus = "in_progress" // 转入中
	DomainStatusCompleted  DomainStatus = "completed"   // 已完成
	DomainStatusFailed     DomainStatus = "failed"      // 失败
)

// DNSProvider DNS提供商类型
type DNSProvider string

const (
	DNSProviderAWS       DNSProvider = "aws"       // AWS Route53
	DNSProviderCloudflare DNSProvider = "cloudflare" // Cloudflare
)

// Domain 域名模型
type Domain struct {
	ID                uint           `json:"id" gorm:"primaryKey"`
	DomainName        string         `json:"domain_name" gorm:"type:varchar(255);uniqueIndex;not null"`
	Registrar         string         `json:"registrar"` // 原注册商
	DNSProvider       DNSProvider    `json:"dns_provider" gorm:"type:varchar(20);default:'aws'"` // DNS提供商: aws, cloudflare
	Status            DomainStatus   `json:"status" gorm:"default:'pending'"`
	NServers          string         `json:"n_servers" gorm:"type:text"`                  // NS 服务器配置，JSON 格式
	CertificateStatus string         `json:"certificate_status" gorm:"default:'pending'"` // 证书状态: pending, issued, failed
	CertificateARN    string         `json:"certificate_arn"`                             // ACM 证书 ARN
	HostedZoneID      string         `json:"hosted_zone_id"`                              // Route53 Hosted Zone ID 或 Cloudflare Zone ID
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (Domain) TableName() string {
	return "domains"
}
