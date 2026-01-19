package services

import (
	"aws_cdn/internal/models"
	"aws_cdn/internal/services/cloudflare"
	"fmt"

	"gorm.io/gorm"
)

type R2BucketService struct {
	db              *gorm.DB
	cfAccountService *CFAccountService
}

func NewR2BucketService(db *gorm.DB, cfAccountService *CFAccountService) *R2BucketService {
	return &R2BucketService{
		db:              db,
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

	// 获取 API Token
	apiToken := s.cfAccountService.GetAPIToken(cfAccount)
	if apiToken == "" {
		return fmt.Errorf("Cloudflare账号未配置API Token")
	}

	// 创建 R2 API 服务
	r2API := cloudflare.NewR2APIService(apiToken)

	// 获取账户 ID
	accountID, err := r2API.GetAccountID()
	if err != nil {
		return fmt.Errorf("获取账户ID失败: %w", err)
	}

	// 检查 R2 是否已启用
	return r2API.EnableR2(accountID)
}

// CreateR2Bucket 创建 R2 存储桶
func (s *R2BucketService) CreateR2Bucket(cfAccountID uint, bucketName, location, note string) (*models.R2Bucket, error) {
	// 获取 CF 账号信息
	cfAccount, err := s.cfAccountService.GetCFAccount(cfAccountID)
	if err != nil {
		return nil, err
	}

	// 获取 API Token
	apiToken := s.cfAccountService.GetAPIToken(cfAccount)
	if apiToken == "" {
		return nil, fmt.Errorf("Cloudflare账号未配置API Token")
	}

	// 创建 R2 API 服务
	r2API := cloudflare.NewR2APIService(apiToken)

	// 获取账户 ID
	accountID, err := r2API.GetAccountID()
	if err != nil {
		return nil, fmt.Errorf("获取账户ID失败: %w", err)
	}

	// 检查 R2 是否已启用
	if err := r2API.EnableR2(accountID); err != nil {
		return nil, fmt.Errorf("R2未启用: %w", err)
	}

	// 创建存储桶
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

	// 获取 API Token
	apiToken := s.cfAccountService.GetAPIToken(cfAccount)
	if apiToken == "" {
		return fmt.Errorf("Cloudflare账号未配置API Token")
	}

	// 创建 R2 API 服务
	r2API := cloudflare.NewR2APIService(apiToken)

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
