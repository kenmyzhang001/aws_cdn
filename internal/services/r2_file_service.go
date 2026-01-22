package services

import (
	"aws_cdn/internal/logger"
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
	log := logger.GetLogger()

	log.WithFields(map[string]interface{}{
		"bucket_id":     bucket.ID,
		"bucket_name":   bucket.BucketName,
		"cf_account_id": bucket.CFAccountID,
	}).Info("开始获取 R2 S3 服务")

	// 从 CF 账号获取 R2 凭证（账号维度）
	cfAccount, err := s.cfAccountService.GetCFAccount(bucket.CFAccountID)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"bucket_id":     bucket.ID,
			"cf_account_id": bucket.CFAccountID,
		}).Error("获取 Cloudflare 账号失败")
		return nil, fmt.Errorf("获取 Cloudflare 账号失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"bucket_id":   bucket.ID,
		"bucket_name": bucket.BucketName,
		"cf_account":  cfAccount.Email,
	}).Info("成功获取 CF 账号信息")

	// 检查是否配置了 R2 Access Key
	r2AccessKeyID := s.cfAccountService.GetR2AccessKeyID(cfAccount)
	r2SecretAccessKey := s.cfAccountService.GetR2SecretAccessKey(cfAccount)

	log.WithFields(map[string]interface{}{
		"bucket_id":      bucket.ID,
		"has_access_key": r2AccessKeyID != "",
		"has_secret_key": r2SecretAccessKey != "",
		"access_key_len": len(r2AccessKeyID),
		"secret_key_len": len(r2SecretAccessKey),
		"access_key_prefix": func() string {
			if len(r2AccessKeyID) > 8 {
				return r2AccessKeyID[:8] + "..."
			}
			return "empty"
		}(),
	}).Info("检查 R2 Access Key 配置")

	if r2AccessKeyID == "" || r2SecretAccessKey == "" {
		log.WithFields(map[string]interface{}{
			"bucket_id":      bucket.ID,
			"cf_account_id":  bucket.CFAccountID,
			"has_access_key": r2AccessKeyID != "",
			"has_secret_key": r2SecretAccessKey != "",
		}).Error("CF 账号未配置 R2 Access Key 和 Secret Key")
		return nil, fmt.Errorf("CF 账号未配置 R2 Access Key 和 Secret Key。请在 CF 账号管理中配置：1) 进入 Cloudflare Dashboard → R2 → Manage R2 API Tokens 2) 创建 API Token（选择 Read and Write 权限）3) 将 Access Key ID 和 Secret Access Key 填入 CF 账号设置")
	}

	// 获取 Account ID（优先使用账号中的 Account ID）
	accountID := cfAccount.AccountID
	log.WithFields(map[string]interface{}{
		"bucket_id":      bucket.ID,
		"account_id":     accountID,
		"account_id_len": len(accountID),
	}).Info("获取 Account ID")

	if accountID == "" {
		log.WithFields(map[string]interface{}{
			"bucket_id":     bucket.ID,
			"cf_account_id": bucket.CFAccountID,
		}).Info("Account ID 为空，尝试通过 API Token 获取")

		// 如果账号没有 Account ID，尝试通过 API Token 获取
		apiToken := s.cfAccountService.GetAPIToken(cfAccount)
		if apiToken == "" {
			log.WithFields(map[string]interface{}{
				"bucket_id":     bucket.ID,
				"cf_account_id": bucket.CFAccountID,
			}).Error("CF 账号未配置 Account ID 和 API Token")
			return nil, fmt.Errorf("CF 账号未配置 Account ID 和 API Token。请至少配置其中一个")
		}

		log.WithFields(map[string]interface{}{
			"bucket_id":     bucket.ID,
			"has_api_token": len(apiToken) > 0,
		}).Info("通过 API Token 获取 Account ID")

		// 优先使用 R2APIToken，如果没有则使用 APIToken
		r2APIToken := s.cfAccountService.GetR2APIToken(cfAccount)
		if r2APIToken == "" {
			r2APIToken = apiToken
		}

		accountID := cfAccount.AccountID

		log.WithFields(map[string]interface{}{
			"bucket_id":      bucket.ID,
			"account_id":     accountID,
			"account_id_len": len(accountID),
		}).Info("成功通过 API Token 获取 Account ID")
	}

	// 验证 Account ID 格式（应该是32位十六进制字符串）
	if len(accountID) != 32 {
		log.WithFields(map[string]interface{}{
			"bucket_id":      bucket.ID,
			"account_id":     accountID,
			"account_id_len": len(accountID),
			"expected_len":   32,
		}).Error("Account ID 格式不正确")
		return nil, fmt.Errorf("Account ID 格式不正确（应该是32位字符），当前值: %s (长度: %d)。请检查 CF 账号设置中的 Account ID", accountID, len(accountID))
	}

	log.WithFields(map[string]interface{}{
		"bucket_id":      bucket.ID,
		"bucket_name":    bucket.BucketName,
		"account_id":     accountID,
		"access_key_id":  r2AccessKeyID[:min(8, len(r2AccessKeyID))] + "...",
		"has_secret_key": r2SecretAccessKey != "",
	}).Info("准备创建 R2 S3 服务")

	// 创建 R2 S3 服务
	cfg := &cloudflare.R2S3Config{
		AccountID:       accountID,
		AccessKeyID:     r2AccessKeyID,
		SecretAccessKey: r2SecretAccessKey,
		BucketName:      bucket.BucketName,
	}

	r2S3Service, err := cloudflare.NewR2S3Service(cfg)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"bucket_id":   bucket.ID,
			"bucket_name": bucket.BucketName,
			"account_id":  accountID,
		}).Error("创建 R2 S3 服务失败")
		return nil, err
	}

	log.WithFields(map[string]interface{}{
		"bucket_id":   bucket.ID,
		"bucket_name": bucket.BucketName,
		"account_id":  accountID,
	}).Info("成功创建 R2 S3 服务")

	return r2S3Service, nil
}

// min 辅助函数
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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

// === 数据库操作方法 ===

// CreateR2FileRecord 创建文件记录
func (s *R2FileService) CreateR2FileRecord(file *models.R2File) error {
	return s.db.Create(file).Error
}

// UpdateR2FileRecord 更新文件记录
func (s *R2FileService) UpdateR2FileRecord(file *models.R2File) error {
	return s.db.Save(file).Error
}

// DeleteR2FileRecord 删除文件记录（软删除：更新状态为deleted）
func (s *R2FileService) DeleteR2FileRecord(r2BucketID uint, filePath string) error {
	return s.db.Model(&models.R2File{}).
		Where("r2_bucket_id = ? AND file_path = ?", r2BucketID, filePath).
		Update("status", "deleted").Error
}

// GetR2FileRecord 根据桶ID和文件路径获取文件记录
func (s *R2FileService) GetR2FileRecord(r2BucketID uint, filePath string) (*models.R2File, error) {
	var file models.R2File
	err := s.db.Where("r2_bucket_id = ? AND file_path = ? AND status = ?", r2BucketID, filePath, "active").
		First(&file).Error
	if err != nil {
		return nil, err
	}
	return &file, nil
}

// ListR2FileRecords 列出指定桶的所有文件记录
func (s *R2FileService) ListR2FileRecords(r2BucketID uint) ([]models.R2File, error) {
	var files []models.R2File
	err := s.db.Where("r2_bucket_id = ? AND status = ?", r2BucketID, "active").
		Order("created_at DESC").
		Find(&files).Error
	return files, err
}

// ListAllAPKFileRecords 列出所有APK文件记录
func (s *R2FileService) ListAllAPKFileRecords() ([]models.R2File, error) {
	var files []models.R2File
	err := s.db.Where("file_path LIKE ? AND status = ?", "%.apk", "active").
		Order("created_at DESC").
		Find(&files).Error
	return files, err
}

// SyncFileRecord 同步文件记录（上传或更新时调用）
func (s *R2FileService) SyncFileRecord(r2BucketID uint, filePath, fileName string, fileSize *int64, contentType *string, etag *string) error {
	// 先查找是否已存在
	var existing models.R2File
	err := s.db.Where("r2_bucket_id = ? AND file_path = ?", r2BucketID, filePath).First(&existing).Error

	if err == gorm.ErrRecordNotFound {
		// 不存在，创建新记录
		newFile := &models.R2File{
			R2BucketID:  r2BucketID,
			FilePath:    filePath,
			FileName:    fileName,
			FileSize:    fileSize,
			ContentType: contentType,
			ETag:        etag,
			Status:      "active",
		}
		return s.db.Create(newFile).Error
	} else if err != nil {
		return err
	}

	// 已存在，更新记录
	existing.FileName = fileName
	existing.FileSize = fileSize
	existing.ContentType = contentType
	existing.ETag = etag
	existing.Status = "active" // 恢复为active状态
	return s.db.Save(&existing).Error
}
