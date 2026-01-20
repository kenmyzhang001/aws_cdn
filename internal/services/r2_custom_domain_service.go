package services

import (
	"aws_cdn/internal/config"
	"aws_cdn/internal/logger"
	"aws_cdn/internal/models"
	"aws_cdn/internal/services/cloudflare"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

type R2CustomDomainService struct {
	db               *gorm.DB
	cfAccountService *CFAccountService
}

func NewR2CustomDomainService(db *gorm.DB, cfAccountService *CFAccountService) *R2CustomDomainService {
	return &R2CustomDomainService{
		db:               db,
		cfAccountService: cfAccountService,
	}
}

// createCloudflareService 根据 CF 账号信息创建 CloudflareService
func (s *R2CustomDomainService) createCloudflareService(cfAccount *models.CFAccount) (*cloudflare.CloudflareService, error) {
	// 获取 API Token（优先使用 APIToken，如果没有则使用 R2APIToken）
	apiToken := s.cfAccountService.GetAPIToken(cfAccount)
	if apiToken == "" {
		apiToken = s.cfAccountService.GetR2APIToken(cfAccount)
	}

	if apiToken == "" {
		return nil, fmt.Errorf("Cloudflare账号未配置 API Token")
	}

	// 创建临时配置
	cfg := &config.CloudflareConfig{
		APIToken: apiToken,
	}

	// 创建 CloudflareService
	cloudflareSvc, err := cloudflare.NewCloudflareService(cfg)
	if err != nil {
		return nil, fmt.Errorf("创建 CloudflareService 失败: %w", err)
	}

	return cloudflareSvc, nil
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

	// 获取 R2 API Token（优先使用 R2APIToken，如果没有则使用 APIToken）
	r2APIToken := s.cfAccountService.GetR2APIToken(cfAccount)
	if r2APIToken == "" {
		return nil, fmt.Errorf("Cloudflare账号未配置 R2 API Token 或 API Token")
	}

	// 创建 R2 API 服务
	accountID := cfAccount.AccountID

	// 根据 CF 账号信息创建 CloudflareService
	cloudflareSvc, err := s.createCloudflareService(cfAccount)
	if err != nil {
		return nil, fmt.Errorf("创建 CloudflareService 失败: %w", err)
	}

	// 获取 Zone ID（用于添加自定义域名）
	// 注意：如果 domain 是子域名（如 assets.example.com），需要先提取根域名（example.com）
	// 因为 Cloudflare Zone 是基于根域名创建的
	rootDomain := s.ExtractRootDomain(domain)
	log := logger.GetLogger()

	// 如果子域名和根域名不同，记录日志
	if rootDomain != domain {
		log.WithFields(map[string]interface{}{
			"domain":      domain,
			"root_domain": rootDomain,
		}).Info("检测到子域名，使用根域名获取 Zone ID")
	}

	zoneID, err := cloudflareSvc.GetZoneID(rootDomain)
	if err != nil {
		// Zone ID 获取失败不影响域名添加，Cloudflare 会自动查找
		zoneID = ""
		log.WithError(err).WithFields(map[string]interface{}{
			"domain":      domain,
			"root_domain": rootDomain,
		}).Warn("无法获取 Zone ID，将尝试自动查找")
	} else {
		log.WithFields(map[string]interface{}{
			"domain":      domain,
			"root_domain": rootDomain,
			"zone_id":     zoneID,
		}).Info("成功获取 Zone ID")
	}

	// 添加自定义域名（enabled 默认为 true）
	domainID, err := cloudflareSvc.AddCustomDomain(accountID, bucket.BucketName, domain, zoneID, true)
	if err != nil {
		return nil, fmt.Errorf("添加自定义域名失败: %w", err)
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
