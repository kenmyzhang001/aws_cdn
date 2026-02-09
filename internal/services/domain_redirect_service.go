package services

import (
	"aws_cdn/internal/config"
	"aws_cdn/internal/logger"
	"aws_cdn/internal/models"
	"aws_cdn/internal/services/cloudflare"
	"fmt"

	"gorm.io/gorm"
)

type DomainRedirectService struct {
	db               *gorm.DB
	cfAccountService *CFAccountService
}

func NewDomainRedirectService(db *gorm.DB, cfAccountService *CFAccountService) *DomainRedirectService {
	return &DomainRedirectService{db: db, cfAccountService: cfAccountService}
}

// getCFService 根据 CF 账号 ID 创建 Cloudflare 服务实例
func (s *DomainRedirectService) getCFService(cfAccountID uint) (*cloudflare.CloudflareService, error) {
	account, err := s.cfAccountService.GetCFAccount(cfAccountID)
	if err != nil {
		return nil, err
	}
	token := s.cfAccountService.GetAPIToken(account)
	if token == "" {
		return nil, fmt.Errorf("该 CF 账号未配置 API Token")
	}
	cfg := &config.CloudflareConfig{APIToken: token}
	return cloudflare.NewCloudflareService(cfg)
}

// List 列表，可选按 CF 账号筛选
func (s *DomainRedirectService) List(cfAccountID *uint) ([]models.DomainRedirect, error) {
	var list []models.DomainRedirect
	query := s.db.Model(&models.DomainRedirect{}).Order("id DESC")
	if cfAccountID != nil {
		query = query.Where("cf_account_id = ?", *cfAccountID)
	}
	if err := query.Preload("CFAccount").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("查询列表失败: %w", err)
	}
	return list, nil
}

// Get 获取单条
func (s *DomainRedirectService) Get(id uint) (*models.DomainRedirect, error) {
	var dr models.DomainRedirect
	if err := s.db.Preload("CFAccount").First(&dr, id).Error; err != nil {
		return nil, fmt.Errorf("重定向记录不存在: %w", err)
	}
	return &dr, nil
}

// Create 创建：在 CF 创建 302 规则并落库，并尝试为源主机名创建 DNS 记录
func (s *DomainRedirectService) Create(cfAccountID uint, zoneID, sourceDomain, targetDomain string, preservePath bool) (*models.DomainRedirect, error) {
	log := logger.GetLogger()
	// 唯一性：同一主域名只能有一条
	var exist models.DomainRedirect
	if err := s.db.Where("source_domain = ?", sourceDomain).First(&exist).Error; err == nil {
		return nil, fmt.Errorf("主域名 %s 已存在重定向配置", sourceDomain)
	}

	cfSvc, err := s.getCFService(cfAccountID)
	if err != nil {
		return nil, err
	}

	ruleID, err := cfSvc.CreateDomainRedirectRule(zoneID, sourceDomain, targetDomain, preservePath)
	if err != nil {
		return nil, fmt.Errorf("在 Cloudflare 创建重定向规则失败: %w", err)
	}

	// 确保源主机名有 DNS 记录，否则无法解析、302 不会触发
	zoneName, zoneErr := cfSvc.GetZoneByID(zoneID)
	if zoneErr == nil {
		if dnsErr := cfSvc.EnsureRedirectSourceDNS(zoneID, zoneName, sourceDomain); dnsErr != nil {
			log.WithError(dnsErr).WithFields(map[string]interface{}{
				"zone_id":       zoneID,
				"source_domain": sourceDomain,
			}).Warn("为源主机名创建 DNS 记录失败，请到 Cloudflare 手动添加 A/CNAME 记录")
		}
	} else {
		log.WithError(zoneErr).WithField("zone_id", zoneID).Warn("获取 Zone 名称失败，跳过自动创建 DNS")
	}

	dr := &models.DomainRedirect{
		CFAccountID:  cfAccountID,
		ZoneID:       zoneID,
		SourceDomain: sourceDomain,
		TargetDomain: targetDomain,
		PreservePath: preservePath,
		CFRuleID:     ruleID,
		Status:       "active",
	}
	// 显式 Select 包含 PreservePath，否则 GORM 会跳过零值(false)，导致 DB 使用 default 1
	if err := s.db.Select("CFAccountID", "ZoneID", "SourceDomain", "TargetDomain", "PreservePath", "CFRuleID", "Status").Create(dr).Error; err != nil {
		return nil, fmt.Errorf("保存记录失败: %w", err)
	}
	log.WithFields(map[string]interface{}{
		"id":      dr.ID,
		"source":  sourceDomain,
		"target":  targetDomain,
		"rule_id": ruleID,
	}).Info("域名302重定向已创建")
	return dr, nil
}

// Update 更新目标域名或是否保留路径，并同步更新 CF 规则
func (s *DomainRedirectService) Update(id uint, targetDomain string, preservePath *bool) (*models.DomainRedirect, error) {
	dr, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if targetDomain != "" {
		dr.TargetDomain = targetDomain
	}
	if preservePath != nil {
		dr.PreservePath = *preservePath
	}

	cfSvc, err := s.getCFService(dr.CFAccountID)
	if err != nil {
		return nil, err
	}
	if dr.CFRuleID != "" {
		if err := cfSvc.UpdateDomainRedirectRule(dr.ZoneID, dr.CFRuleID, dr.SourceDomain, dr.TargetDomain, dr.PreservePath); err != nil {
			return nil, fmt.Errorf("更新 Cloudflare 规则失败: %w", err)
		}
	}
	// 显式 Select 包含 PreservePath，否则 GORM 会跳过零值(false)，更新不生效
	if err := s.db.Model(dr).Select("TargetDomain", "PreservePath").Updates(map[string]interface{}{
		"target_domain": dr.TargetDomain,
		"preserve_path": dr.PreservePath,
	}).Error; err != nil {
		return nil, fmt.Errorf("保存失败: %w", err)
	}
	return dr, nil
}

// EnsureDNS 为已有重定向的源主机名创建/补建 DNS 记录（A 或 CNAME），解决「无法解析主机」。
func (s *DomainRedirectService) EnsureDNS(id uint) error {
	dr, err := s.Get(id)
	if err != nil {
		return err
	}
	cfSvc, err := s.getCFService(dr.CFAccountID)
	if err != nil {
		return err
	}
	zoneName, err := cfSvc.GetZoneByID(dr.ZoneID)
	if err != nil {
		return fmt.Errorf("获取 Zone 名称失败: %w", err)
	}
	return cfSvc.EnsureRedirectSourceDNS(dr.ZoneID, zoneName, dr.SourceDomain)
}

// Delete 删除：删除 CF 规则并软删除记录
func (s *DomainRedirectService) Delete(id uint) error {
	dr, err := s.Get(id)
	if err != nil {
		return err
	}
	if dr.CFRuleID != "" {
		cfSvc, err := s.getCFService(dr.CFAccountID)
		if err == nil {
			rulesetID, _ := cfSvc.GetURLRedirectRulesetID(dr.ZoneID)
			if rulesetID != "" {
				_ = cfSvc.DeleteRedirectRule(dr.ZoneID, rulesetID, dr.CFRuleID)
			}
		}
	}
	return s.db.Delete(dr).Error
}
