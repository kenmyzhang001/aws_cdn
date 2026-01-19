package services

import (
	"aws_cdn/internal/models"
	"fmt"
	"io"

	"gorm.io/gorm"
)

type R2FileService struct {
	db              *gorm.DB
	cfAccountService *CFAccountService
}

func NewR2FileService(db *gorm.DB, cfAccountService *CFAccountService) *R2FileService {
	return &R2FileService{
		db:              db,
		cfAccountService: cfAccountService,
	}
}

// UploadFile 上传文件到 R2 存储桶
func (s *R2FileService) UploadFile(r2BucketID uint, key string, body io.ReadSeeker, contentType string) error {
	// 获取存储桶信息
	var bucket models.R2Bucket
	if err := s.db.Preload("CFAccount").First(&bucket, r2BucketID).Error; err != nil {
		return fmt.Errorf("R2存储桶不存在: %w", err)
	}

	// 获取 CF 账号信息
	cfAccount, err := s.cfAccountService.GetCFAccount(bucket.CFAccountID)
	if err != nil {
		return err
	}

	// 获取 API Token
	apiToken := s.cfAccountService.GetAPIToken(cfAccount)
	if apiToken == "" {
		return fmt.Errorf("Cloudflare账号未配置API Token")
	}

	// 注意：这里需要 R2 的 Access Key 和 Secret Key
	// 这些信息需要从 CF 账号中获取或单独配置
	// 暂时返回错误，提示需要配置 Access Key
	_ = bucket
	_ = cfAccount
	_ = apiToken
	return fmt.Errorf("需要配置 R2 Access Key 和 Secret Key 才能上传文件")
}

// CreateDirectory 创建目录
func (s *R2FileService) CreateDirectory(r2BucketID uint, prefix string) error {
	// 获取存储桶信息
	var bucket models.R2Bucket
	if err := s.db.Preload("CFAccount").First(&bucket, r2BucketID).Error; err != nil {
		return fmt.Errorf("R2存储桶不存在: %w", err)
	}

	// 获取 CF 账号信息
	cfAccount, err := s.cfAccountService.GetCFAccount(bucket.CFAccountID)
	if err != nil {
		return err
	}

	// 获取 API Token
	apiToken := s.cfAccountService.GetAPIToken(cfAccount)
	if apiToken == "" {
		return fmt.Errorf("Cloudflare账号未配置API Token")
	}

	// 注意：这里需要 R2 的 Access Key 和 Secret Key
	// 暂时返回错误
	_ = bucket
	_ = cfAccount
	_ = apiToken
	return fmt.Errorf("需要配置 R2 Access Key 和 Secret Key 才能创建目录")
}

// ListFiles 列出文件
func (s *R2FileService) ListFiles(r2BucketID uint, prefix string) ([]string, error) {
	// 获取存储桶信息
	var bucket models.R2Bucket
	if err := s.db.Preload("CFAccount").First(&bucket, r2BucketID).Error; err != nil {
		return nil, fmt.Errorf("R2存储桶不存在: %w", err)
	}

	// 获取 CF 账号信息
	cfAccount, err := s.cfAccountService.GetCFAccount(bucket.CFAccountID)
	if err != nil {
		return nil, err
	}

	// 获取 API Token
	apiToken := s.cfAccountService.GetAPIToken(cfAccount)
	if apiToken == "" {
		return nil, fmt.Errorf("Cloudflare账号未配置API Token")
	}

	// 注意：这里需要 R2 的 Access Key 和 Secret Key
	// 暂时返回错误
	_ = bucket
	_ = cfAccount
	_ = apiToken
	return nil, fmt.Errorf("需要配置 R2 Access Key 和 Secret Key 才能列出文件")
}
