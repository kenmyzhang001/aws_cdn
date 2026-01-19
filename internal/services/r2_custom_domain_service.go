package services

import (
	"aws_cdn/internal/models"
	"aws_cdn/internal/services/cloudflare"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

type R2CustomDomainService struct {
	db              *gorm.DB
	cfAccountService *CFAccountService
	cloudflareService *cloudflare.CloudflareService
}

func NewR2CustomDomainService(db *gorm.DB, cfAccountService *CFAccountService, cloudflareService *cloudflare.CloudflareService) *R2CustomDomainService {
	return &R2CustomDomainService{
		db:               db,
		cfAccountService: cfAccountService,
		cloudflareService: cloudflareService,
	}
}

// ListR2CustomDomains 列出所有自定义域名
func (s *R2CustomDomainService) ListR2CustomDomains(r2BucketID uint) ([]models.R2CustomDomain, error) {
	var domains []models.R2CustomDomain
	if err := s.db.Preload("R2Bucket").Where("r2_bucket_id = ?", r2BucketID).Order("id DESC").Find(&domains).Error; err != nil {
		return nil, fmt.Errorf("获取自定义域名列表失败: %w", err)
	}
	return domains, nil
}

// GetR2CustomDomain 获取自定义域名信息
func (s *R2CustomDomainService) GetR2CustomDomain(id uint) (*models.R2CustomDomain, error) {
	var domain models.R2CustomDomain
	if err := s.db.Preload("R2Bucket").First(&domain, id).Error; err != nil {
		return nil, fmt.Errorf("自定义域名不存在: %w", err)
	}
	return &domain, nil
}

// AddCustomDomain 添加自定义域名
func (s *R2CustomDomainService) AddCustomDomain(r2BucketID uint, domain, note string) (*models.R2CustomDomain, error) {
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

	// 创建 R2 API 服务
	r2API := cloudflare.NewR2APIService(apiToken)

	// 获取账户 ID
	accountID, err := r2API.GetAccountID()
	if err != nil {
		return nil, fmt.Errorf("获取账户ID失败: %w", err)
	}

	// 添加自定义域名
	domainID, err := r2API.AddCustomDomain(accountID, bucket.BucketName, domain)
	if err != nil {
		return nil, fmt.Errorf("添加自定义域名失败: %w", err)
	}

	// 获取 Zone ID（用于后续创建缓存规则）
	zoneID, err := s.cloudflareService.GetZoneID(domain)
	if err != nil {
		// Zone ID 获取失败不影响域名添加，后续可以手动设置
		zoneID = ""
	}

	// 保存到数据库
	customDomain := &models.R2CustomDomain{
		R2BucketID: r2BucketID,
		Domain:     domain,
		ZoneID:     zoneID,
		Status:     "active",
		Note:       note,
	}

	if err := s.db.Create(customDomain).Error; err != nil {
		return nil, fmt.Errorf("保存自定义域名信息失败: %w", err)
	}

	// 更新 domainID（如果 API 返回了）
	if domainID != "" {
		// 注意：这里 domainID 可能不是我们需要的字段，先保留
		_ = domainID
	}

	return customDomain, nil
}

// DeleteR2CustomDomain 删除自定义域名
func (s *R2CustomDomainService) DeleteR2CustomDomain(id uint) error {
	domain, err := s.GetR2CustomDomain(id)
	if err != nil {
		return err
	}

	// 注意：Cloudflare R2 API 不提供删除自定义域名的接口，只能通过 Dashboard 删除
	// 这里只删除数据库记录
	if err := s.db.Delete(domain).Error; err != nil {
		return fmt.Errorf("删除自定义域名记录失败: %w", err)
	}

	return nil
}

// UpdateR2CustomDomainNote 更新自定义域名备注
func (s *R2CustomDomainService) UpdateR2CustomDomainNote(id uint, note string) error {
	domain, err := s.GetR2CustomDomain(id)
	if err != nil {
		return err
	}

	domain.Note = note
	if err := s.db.Save(domain).Error; err != nil {
		return fmt.Errorf("更新自定义域名备注失败: %w", err)
	}

	return nil
}

// ExtractRootDomain 提取根域名
func (s *R2CustomDomainService) ExtractRootDomain(domain string) string {
	parts := strings.Split(domain, ".")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], ".")
	}
	return domain
}
