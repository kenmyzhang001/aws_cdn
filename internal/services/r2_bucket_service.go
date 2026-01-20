package services

import (
	"aws_cdn/internal/models"
	"aws_cdn/internal/services/cloudflare"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

type R2BucketService struct {
	db               *gorm.DB
	cfAccountService *CFAccountService
}

func NewR2BucketService(db *gorm.DB, cfAccountService *CFAccountService) *R2BucketService {
	return &R2BucketService{
		db:               db,
		cfAccountService: cfAccountService,
	}
}

// ListR2Buckets 列出所有 R2 存储桶
func (s *R2BucketService) ListR2Buckets() ([]models.R2Bucket, error) {
	var buckets []models.R2Bucket
	if err := s.db.Preload("CFAccount").Order("id DESC").Find(&buckets).Error; err != nil {
		return nil, fmt.Errorf("获取R2存储桶列表失败: %w", err)
	}
	return buckets, nil
}

// GetR2Bucket 获取 R2 存储桶信息
func (s *R2BucketService) GetR2Bucket(id uint) (*models.R2Bucket, error) {
	var bucket models.R2Bucket
	if err := s.db.Preload("CFAccount").First(&bucket, id).Error; err != nil {
		return nil, fmt.Errorf("R2存储桶不存在: %w", err)
	}
	return &bucket, nil
}

// EnableR2 启用 R2（检查账户是否已启用 R2）
func (s *R2BucketService) EnableR2(cfAccountID uint) error {
	// 获取 CF 账号信息
	cfAccount, err := s.cfAccountService.GetCFAccount(cfAccountID)
	if err != nil {
		return err
	}

	// 获取 R2 API Token（优先使用 R2APIToken，如果没有则使用 APIToken）
	r2APIToken := s.cfAccountService.GetR2APIToken(cfAccount)
	if r2APIToken == "" {
		return fmt.Errorf("Cloudflare账号未配置 R2 API Token 或 API Token")
	}

	// 创建 R2 API 服务
	r2API := cloudflare.NewR2APIService(r2APIToken)

	// 获取账户 ID
	accountID, err := r2API.GetAccountID()
	if err != nil {
		return fmt.Errorf("获取账户ID失败: %w", err)
	}

	// 检查 R2 是否已启用
	return r2API.EnableR2(accountID)
}

// CreateR2Bucket 创建 R2 存储桶
func (s *R2BucketService) CreateR2Bucket(cfAccountID uint, bucketName, location, accountID, note string) (*models.R2Bucket, error) {
	// 获取 CF 账号信息
	cfAccount, err := s.cfAccountService.GetCFAccount(cfAccountID)
	if err != nil {
		return nil, fmt.Errorf("获取 CF 账号失败: %w", err)
	}

	// 获取 R2 API Token（优先使用 R2APIToken，如果没有则使用 APIToken）
	r2APIToken := s.cfAccountService.GetR2APIToken(cfAccount)
	if r2APIToken == "" {
		return nil, fmt.Errorf("Cloudflare账号未配置 R2 API Token 或 API Token，请在 CF 账号管理中配置")
	}

	// 创建 R2 API 服务
	r2API := cloudflare.NewR2APIService(r2APIToken)

	// 优先使用传入的 Account ID，其次使用 CF 账号的 Account ID，最后尝试通过 API Token 获取
	if accountID == "" {
		accountID = cfAccount.AccountID
	}
	if accountID == "" {
		var err error
		accountID, err = r2API.GetAccountID()
		if err != nil {
			return nil, fmt.Errorf("获取账户ID失败: %w。请在 CF 账号设置中配置 Account ID 或手动提供", err)
		}
	}

	// 创建存储桶（使用从 CF 账号获取的 API Token）
	if err := r2API.CreateBucket(accountID, bucketName, location); err != nil {
		return nil, fmt.Errorf("创建存储桶失败: %w", err)
	}

	// 保存到数据库
	bucket := &models.R2Bucket{
		CFAccountID: cfAccountID,
		BucketName:  bucketName,
		Location:    location,
		Note:        note,
	}

	if err := s.db.Create(bucket).Error; err != nil {
		return nil, fmt.Errorf("保存存储桶信息失败: %w", err)
	}

	return bucket, nil
}

// DeleteR2Bucket 删除 R2 存储桶
func (s *R2BucketService) DeleteR2Bucket(id uint) error {
	bucket, err := s.GetR2Bucket(id)
	if err != nil {
		return err
	}

	// 检查存储桶中是否有文件或目录
	// 从 CF 账号获取 R2 凭证
	cfAccount, err := s.cfAccountService.GetCFAccount(bucket.CFAccountID)
	if err != nil {
		return fmt.Errorf("获取 Cloudflare 账号失败: %w", err)
	}

	r2AccessKeyID := s.cfAccountService.GetR2AccessKeyID(cfAccount)
	r2SecretAccessKey := s.cfAccountService.GetR2SecretAccessKey(cfAccount)

	// 如果配置了 Access Key，尝试列出文件检查
	if r2AccessKeyID != "" && r2SecretAccessKey != "" {
		// 创建 R2 文件服务来检查文件
		fileService := NewR2FileService(s.db, s.cfAccountService)
		files, err := fileService.ListFiles(id, "")
		if err != nil {
			// 如果无法列出文件（可能是凭证问题），不允许删除
			return fmt.Errorf("无法检查存储桶中的文件，删除失败: %w。请确保存储桶凭证配置正确", err)
		}

		// 统计文件和目录
		fileCount := 0
		dirCount := 0
		for _, file := range files {
			if strings.HasSuffix(file, "/") {
				dirCount++
			} else {
				fileCount++
			}
		}

		if fileCount > 0 {
			return fmt.Errorf("存储桶中存在 %d 个文件，请先删除所有文件后再删除存储桶", fileCount)
		}

		if dirCount > 0 {
			return fmt.Errorf("存储桶中存在 %d 个目录，请先删除所有目录后再删除存储桶", dirCount)
		}
	} else {
		// 如果没有配置凭证，无法检查文件，给出提示但不阻止删除
		// 用户可能确定存储桶是空的，所以允许删除
		// 但会在日志中记录警告
	}

	// 注意：Cloudflare R2 API 不提供删除存储桶的接口，只能通过 Dashboard 删除
	// 这里只删除数据库记录
	if err := s.db.Delete(bucket).Error; err != nil {
		return fmt.Errorf("删除存储桶记录失败: %w", err)
	}

	return nil
}

// UpdateR2BucketNote 更新存储桶备注
func (s *R2BucketService) UpdateR2BucketNote(id uint, note string) error {
	bucket, err := s.GetR2Bucket(id)
	if err != nil {
		return err
	}

	bucket.Note = note
	if err := s.db.Save(bucket).Error; err != nil {
		return fmt.Errorf("更新存储桶备注失败: %w", err)
	}

	return nil
}

// UpdateR2BucketCredentials 已废弃：R2 凭证现在是账号维度的，请在 CF 账号管理中配置
// 保留此方法以保持向后兼容，但实际不做任何操作
func (s *R2BucketService) UpdateR2BucketCredentials(id uint, accessKeyID, secretAccessKey, accountID string) error {
	return fmt.Errorf("R2 Access Key 和 Secret Key 现在是账号维度的，请在 CF 账号管理中配置，而不是在存储桶中配置")
}

// ConfigureCORS 配置 CORS
func (s *R2BucketService) ConfigureCORS(id uint, corsConfig []map[string]interface{}) error {
	// 获取存储桶信息
	bucket, err := s.GetR2Bucket(id)
	if err != nil {
		return err
	}

	// 获取 CF 账号信息
	cfAccount, err := s.cfAccountService.GetCFAccount(bucket.CFAccountID)
	if err != nil {
		return err
	}

	// 获取 R2 API Token（优先使用 R2APIToken，如果没有则使用 APIToken）
	r2APIToken := s.cfAccountService.GetR2APIToken(cfAccount)
	if r2APIToken == "" {
		return fmt.Errorf("Cloudflare账号未配置 R2 API Token 或 API Token")
	}

	// 创建 R2 API 服务
	r2API := cloudflare.NewR2APIService(r2APIToken)

	// 获取账户 ID
	accountID, err := r2API.GetAccountID()
	if err != nil {
		return fmt.Errorf("获取账户ID失败: %w", err)
	}

	// 配置 CORS
	if err := r2API.ConfigureCORS(accountID, bucket.BucketName, corsConfig); err != nil {
		return fmt.Errorf("配置CORS失败: %w", err)
	}

	return nil
}
