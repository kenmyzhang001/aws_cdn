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

// List 列表，可选按 CF 账号、域名关键词筛选，支持分页
func (s *DomainRedirectService) List(cfAccountID *uint, domain string, page, pageSize int) ([]models.DomainRedirect, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	var list []models.DomainRedirect
	var total int64
	query := s.db.Model(&models.DomainRedirect{})
	if cfAccountID != nil {
		query = query.Where("cf_account_id = ?", *cfAccountID)
	}
	if domain != "" {
		like := "%" + domain + "%"
		query = query.Where("source_domain LIKE ? OR target_domain LIKE ?", like, like)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("查询总数失败: %w", err)
	}
	offset := (page - 1) * pageSize
	if err := query.Preload("CFAccount").Order("id DESC").Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("查询列表失败: %w", err)
	}
	return list, total, nil
}

// Get 获取单条
func (s *DomainRedirectService) Get(id uint) (*models.DomainRedirect, error) {
	var dr models.DomainRedirect
	if err := s.db.Preload("CFAccount").First(&dr, id).Error; err != nil {
		return nil, fmt.Errorf("重定向记录不存在: %w", err)
	}
	return &dr, nil
}

// CheckSourceDomainAvailable 检查主域名（源）是否已被占用。若被占用返回 used_by（domain_redirect/cf_worker）、ref_id、ref_name。
func (s *DomainRedirectService) CheckSourceDomainAvailable(domain string) (available bool, usedBy string, refID uint, refName string) {
	if domain == "" {
		return true, "", 0, ""
	}
	domain = strings.TrimSpace(strings.ToLower(domain))
	var dr models.DomainRedirect
	if err := s.db.Where("LOWER(source_domain) = ?", domain).First(&dr).Error; err == nil {
		return false, "domain_redirect", dr.ID, dr.SourceDomain
	}
	likePattern := "%\"" + escapeLike(domain) + "\"%"
	var workers []models.CFWorker
	if err := s.db.Where("LOWER(worker_domain) = ? OR (worker_domains != '' AND worker_domains LIKE ?)", domain, likePattern).Limit(1).Find(&workers).Error; err == nil && len(workers) > 0 {
		return false, "cf_worker", workers[0].ID, workers[0].WorkerName
	}
	return true, "", 0, ""
}

// Create 创建：在 CF 创建 302 规则并落库，并尝试为源主机名创建 DNS 记录
func (s *DomainRedirectService) Create(cfAccountID uint, zoneID, sourceDomain, targetDomain string, preservePath bool) (*models.DomainRedirect, error) {
	log := logger.GetLogger()
	// 预先检查域名是否已被占用（302 或 Worker）
	if ok, usedBy, refID, refName := s.CheckSourceDomainAvailable(sourceDomain); !ok {
		switch usedBy {
		case "domain_redirect":
			return nil, fmt.Errorf("主域名 %s 已被「域名302重定向」使用（%s），请先删除该重定向后再创建", sourceDomain, refName)
		case "cf_worker":
			return nil, fmt.Errorf("主域名 %s 已被「Cloudflare Worker」使用（%s，ID: %d），请先删除该 Worker 后再创建", sourceDomain, refName, refID)
		default:
			return nil, fmt.Errorf("主域名 %s 已被占用，请先删除占用项后再创建", sourceDomain)
		}
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
