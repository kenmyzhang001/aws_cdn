package models

import (
	"time"

	"gorm.io/gorm"
)

// DownloadPackageStatus 下载包状态
type DownloadPackageStatus string

const (
	DownloadPackageStatusPending    DownloadPackageStatus = "pending"    // 待处理
	DownloadPackageStatusUploading  DownloadPackageStatus = "uploading"  // 上传中
	DownloadPackageStatusProcessing DownloadPackageStatus = "processing" // 处理中（创建CloudFront等）
	DownloadPackageStatusCompleted  DownloadPackageStatus = "completed"  // 已完成
	DownloadPackageStatusFailed     DownloadPackageStatus = "failed"     // 失败
)

// DownloadPackage 下载包模型
type DownloadPackage struct {
	ID               uint                  `json:"id" gorm:"primaryKey"`
	DomainID         uint                  `json:"domain_id" gorm:"not null;index"`
	Domain           Domain                `json:"domain" gorm:"foreignKey:DomainID"`
	DomainName       string                `json:"domain_name" gorm:"type:varchar(255);not null"` // 下载域名
	FileName         string                `json:"file_name" gorm:"type:varchar(255);not null"`   // 文件名
	FileSize         int64                 `json:"file_size" gorm:"not null"`                     // 文件大小（字节）
	FileType         string                `json:"file_type" gorm:"type:varchar(100)"`            // 文件类型（如：application/vnd.android.package-archive）
	S3Key            string                `json:"s3_key" gorm:"type:varchar(500);not null"`                    // S3对象键
	CloudFrontID     string                `json:"cloudfront_id" gorm:"column:cloudfront_id;type:varchar(255)"` // CloudFront分发ID
	CloudFrontDomain string                `json:"cloudfront_domain" gorm:"column:cloudfront_domain;type:varchar(255)"` // CloudFront域名
	DownloadURL      string                `json:"download_url" gorm:"type:varchar(500)"`                      // 下载URL（通过域名访问）
	Status           DownloadPackageStatus `json:"status" gorm:"default:'pending'"`
	ErrorMessage     string                `json:"error_message" gorm:"type:text"` // 错误信息
	CreatedAt        time.Time             `json:"created_at"`
	UpdatedAt        time.Time             `json:"updated_at"`
	DeletedAt        gorm.DeletedAt        `json:"-" gorm:"index"`
}

// TableName 指定表名
func (DownloadPackage) TableName() string {
	return "download_packages"
}
