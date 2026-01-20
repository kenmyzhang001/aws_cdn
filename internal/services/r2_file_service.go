package services

import (
	"aws_cdn/internal/models"
	"aws_cdn/internal/services/cloudflare"
	"fmt"
	"io"

	"gorm.io/gorm"
)

type R2FileService struct {
	db               *gorm.DB
	cfAccountService *CFAccountService
}

func NewR2FileService(db *gorm.DB, cfAccountService *CFAccountService) *R2FileService {
	return &R2FileService{
		db:               db,
		cfAccountService: cfAccountService,
	}
}

// getR2S3Service 获取 R2 S3 服务
func (s *R2FileService) getR2S3Service(bucket *models.R2Bucket) (*cloudflare.R2S3Service, error) {
	// 从 CF 账号获取 R2 凭证（账号维度）
	cfAccount, err := s.cfAccountService.GetCFAccount(bucket.CFAccountID)
	if err != nil {
		return nil, fmt.Errorf("获取 Cloudflare 账号失败: %w", err)
	}

	// 检查是否配置了 R2 Access Key
	r2AccessKeyID := s.cfAccountService.GetR2AccessKeyID(cfAccount)
	r2SecretAccessKey := s.cfAccountService.GetR2SecretAccessKey(cfAccount)

	if r2AccessKeyID == "" || r2SecretAccessKey == "" {
		return nil, fmt.Errorf("CF 账号未配置 R2 Access Key 和 Secret Key。请在 CF 账号管理中配置：1) 进入 Cloudflare Dashboard → R2 → Manage R2 API Tokens 2) 创建 API Token（选择 Read and Write 权限）3) 将 Access Key ID 和 Secret Access Key 填入 CF 账号设置")
	}

	// 获取 Account ID（优先使用账号中的 Account ID）
	accountID := cfAccount.AccountID
	if accountID == "" {
		// 如果账号没有 Account ID，尝试通过 API Token 获取
		apiToken := s.cfAccountService.GetAPIToken(cfAccount)
		if apiToken == "" {
			return nil, fmt.Errorf("CF 账号未配置 Account ID 和 API Token。请至少配置其中一个")
		}

		r2API := cloudflare.NewR2APIService(apiToken)
		var err2 error
		accountID, err2 = r2API.GetAccountID()
		if err2 != nil {
			return nil, fmt.Errorf("获取账户ID失败: %w。建议在 CF 账号设置中直接配置 Account ID", err2)
		}
	}

	// 验证 Account ID 格式（应该是32位十六进制字符串）
	if len(accountID) != 32 {
		return nil, fmt.Errorf("Account ID 格式不正确（应该是32位字符），当前值: %s (长度: %d)。请检查 CF 账号设置中的 Account ID", accountID, len(accountID))
	}

	// 创建 R2 S3 服务
	cfg := &cloudflare.R2S3Config{
		AccountID:       accountID,
		AccessKeyID:     r2AccessKeyID,
		SecretAccessKey: r2SecretAccessKey,
		BucketName:      bucket.BucketName,
	}

	return cloudflare.NewR2S3Service(cfg)
}

// UploadFile 上传文件到 R2 存储桶
func (s *R2FileService) UploadFile(r2BucketID uint, key string, body io.ReadSeeker, contentType string) error {
	// 获取存储桶信息
	var bucket models.R2Bucket
	if err := s.db.Preload("CFAccount").First(&bucket, r2BucketID).Error; err != nil {
		return fmt.Errorf("R2存储桶不存在: %w", err)
	}

	// 获取 R2 S3 服务
	r2S3, err := s.getR2S3Service(&bucket)
	if err != nil {
		return err
	}

	// 上传文件
	return r2S3.UploadFile(key, body, contentType)
}

// CreateDirectory 创建目录
func (s *R2FileService) CreateDirectory(r2BucketID uint, prefix string) error {
	// 获取存储桶信息
	var bucket models.R2Bucket
	if err := s.db.Preload("CFAccount").First(&bucket, r2BucketID).Error; err != nil {
		return fmt.Errorf("R2存储桶不存在: %w", err)
	}

	// 获取 R2 S3 服务
	r2S3, err := s.getR2S3Service(&bucket)
	if err != nil {
		return err
	}

	// 创建目录
	return r2S3.CreateDirectory(prefix)
}

// ListFiles 列出文件
func (s *R2FileService) ListFiles(r2BucketID uint, prefix string) ([]string, error) {
	// 获取存储桶信息
	var bucket models.R2Bucket
	if err := s.db.Preload("CFAccount").First(&bucket, r2BucketID).Error; err != nil {
		return nil, fmt.Errorf("R2存储桶不存在: %w", err)
	}

	// 获取 R2 S3 服务
	r2S3, err := s.getR2S3Service(&bucket)
	if err != nil {
		return nil, err
	}

	// 列出文件
	return r2S3.ListFiles(prefix)
}

// DeleteFile 删除文件
func (s *R2FileService) DeleteFile(r2BucketID uint, key string) error {
	// 获取存储桶信息
	var bucket models.R2Bucket
	if err := s.db.Preload("CFAccount").First(&bucket, r2BucketID).Error; err != nil {
		return fmt.Errorf("R2存储桶不存在: %w", err)
	}

	// 获取 R2 S3 服务
	r2S3, err := s.getR2S3Service(&bucket)
	if err != nil {
		return err
	}

	// 删除文件
	return r2S3.DeleteFile(key)
}

// FileExists 检查文件是否存在
func (s *R2FileService) FileExists(r2BucketID uint, key string) (bool, error) {
	// 获取存储桶信息
	var bucket models.R2Bucket
	if err := s.db.Preload("CFAccount").First(&bucket, r2BucketID).Error; err != nil {
		return false, fmt.Errorf("R2存储桶不存在: %w", err)
	}

	// 获取 R2 S3 服务
	r2S3, err := s.getR2S3Service(&bucket)
	if err != nil {
		return false, err
	}

	// 检查文件是否存在
	return r2S3.FileExists(key)
}
