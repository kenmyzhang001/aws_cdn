package services

import (
	"aws_cdn/internal/models"
	"aws_cdn/internal/services/cloudflare"
	"fmt"

	"gorm.io/gorm"
)

type R2CacheRuleService struct {
	db                *gorm.DB
	cfAccountService  *CFAccountService
	cloudflareService *cloudflare.CloudflareService
}

func NewR2CacheRuleService(db *gorm.DB, cfAccountService *CFAccountService, cloudflareService *cloudflare.CloudflareService) *R2CacheRuleService {
	return &R2CacheRuleService{
		db:                db,
		cfAccountService:  cfAccountService,
		cloudflareService: cloudflareService,
	}
}

// ListR2CacheRules 列出所有缓存规则
func (s *R2CacheRuleService) ListR2CacheRules(r2CustomDomainID uint) ([]models.R2CacheRule, error) {
	var rules []models.R2CacheRule
	if err := s.db.Preload("R2CustomDomain.R2Bucket").Where("r2_custom_domain_id = ?", r2CustomDomainID).Order("id DESC").Find(&rules).Error; err != nil {
		return nil, fmt.Errorf("获取缓存规则列表失败: %w", err)
	}
	return rules, nil
}

// GetR2CacheRule 获取缓存规则信息
func (s *R2CacheRuleService) GetR2CacheRule(id uint) (*models.R2CacheRule, error) {
	var rule models.R2CacheRule
	if err := s.db.Preload("R2CustomDomain.R2Bucket").First(&rule, id).Error; err != nil {
		return nil, fmt.Errorf("缓存规则不存在: %w", err)
	}
	return &rule, nil
}

// CreateCacheRule 创建缓存规则
func (s *R2CacheRuleService) CreateCacheRule(r2CustomDomainID uint, ruleName, expression, cacheStatus, edgeTTL, browserTTL, note string) (*models.R2CacheRule, error) {
	// 获取自定义域名信息
	var customDomain models.R2CustomDomain
	if err := s.db.Preload("R2Bucket.CFAccount").First(&customDomain, r2CustomDomainID).Error; err != nil {
		return nil, fmt.Errorf("自定义域名不存在: %w", err)
	}

	// 检查 Zone ID
	if customDomain.ZoneID == "" {
		// 尝试获取 Zone ID
		zoneID, err := s.cloudflareService.GetZoneID(customDomain.Domain)
		if err != nil {
			return nil, fmt.Errorf("无法获取Zone ID，请确保域名已在Cloudflare托管: %w", err)
		}
		customDomain.ZoneID = zoneID
		if err := s.db.Save(&customDomain).Error; err != nil {
			return nil, fmt.Errorf("更新Zone ID失败: %w", err)
		}
	}

	// 获取 CF 账号信息
	cfAccount, err := s.cfAccountService.GetCFAccount(customDomain.R2Bucket.CFAccountID)
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

	// 创建缓存规则
	cloudflareRuleID, err := r2API.CreateCacheRule(customDomain.ZoneID, ruleName, expression, cacheStatus, edgeTTL, browserTTL)
	if err != nil {
		return nil, fmt.Errorf("创建缓存规则失败: %w", err)
	}

	// 保存到数据库
	rule := &models.R2CacheRule{
		R2CustomDomainID: r2CustomDomainID,
		RuleName:         ruleName,
		Expression:       expression,
		CacheStatus:      cacheStatus,
		EdgeTTL:          edgeTTL,
		BrowserTTL:       browserTTL,
		CloudflareRuleID: cloudflareRuleID,
		Status:           "active",
		Note:             note,
	}

	if err := s.db.Create(rule).Error; err != nil {
		return nil, fmt.Errorf("保存缓存规则信息失败: %w", err)
	}

	return rule, nil
}

// DeleteR2CacheRule 删除缓存规则
func (s *R2CacheRuleService) DeleteR2CacheRule(id uint) error {
	rule, err := s.GetR2CacheRule(id)
	if err != nil {
		return err
	}

	// 注意：Cloudflare API 删除缓存规则需要额外的实现
	// 这里先删除数据库记录
	if err := s.db.Delete(rule).Error; err != nil {
		return fmt.Errorf("删除缓存规则记录失败: %w", err)
	}

	return nil
}

// UpdateR2CacheRuleNote 更新缓存规则备注
func (s *R2CacheRuleService) UpdateR2CacheRuleNote(id uint, note string) error {
	rule, err := s.GetR2CacheRule(id)
	if err != nil {
		return err
	}

	rule.Note = note
	if err := s.db.Save(rule).Error; err != nil {
		return fmt.Errorf("更新缓存规则备注失败: %w", err)
	}

	return nil
}
