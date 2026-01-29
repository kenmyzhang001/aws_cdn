package services

import (
	"aws_cdn/internal/config"
	"aws_cdn/internal/logger"
	"aws_cdn/internal/models"
	"aws_cdn/internal/services/cloudflare"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type R2CustomDomainService struct {
	db               *gorm.DB
	cfAccountService *CFAccountService
	cacheRuleService *R2CacheRuleService
}

func NewR2CustomDomainService(db *gorm.DB, cfAccountService *CFAccountService, cacheRuleService *R2CacheRuleService) *R2CustomDomainService {
	return &R2CustomDomainService{
		db:               db,
		cfAccountService: cfAccountService,
		cacheRuleService: cacheRuleService,
	}
}

// ConfigLog 配置日志条目
type ConfigLog struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"` // info, warning, error
	Action    string `json:"action"`
	Message   string `json:"message"`
	Details   string `json:"details,omitempty"`
}

// configLogger 配置日志收集器
type configLogger struct {
	logs []ConfigLog
}

func newConfigLogger() *configLogger {
	return &configLogger{
		logs: make([]ConfigLog, 0),
	}
}

func (cl *configLogger) info(action, message, details string) {
	cl.logs = append(cl.logs, ConfigLog{
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		Level:     "info",
		Action:    action,
		Message:   message,
		Details:   details,
	})
}

func (cl *configLogger) warning(action, message, details string) {
	cl.logs = append(cl.logs, ConfigLog{
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		Level:     "warning",
		Action:    action,
		Message:   message,
		Details:   details,
	})
}

func (cl *configLogger) error(action, message, details string) {
	cl.logs = append(cl.logs, ConfigLog{
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		Level:     "error",
		Action:    action,
		Message:   message,
		Details:   details,
	})
}

func (cl *configLogger) toJSON() string {
	data, _ := json.Marshal(cl.logs)
	return string(data)
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

// ListAllR2CustomDomains 列出所有R2自定义域名（不分页，不按存储桶筛选）
func (s *R2CustomDomainService) ListAllR2CustomDomains() ([]models.R2CustomDomain, error) {
	var domains []models.R2CustomDomain
	if err := s.db.Preload("R2Bucket").
		Where("deleted_at IS NULL").
		Order("id DESC").
		Find(&domains).Error; err != nil {
		return nil, fmt.Errorf("获取所有自定义域名列表失败: %w", err)
	}
	return domains, nil
}

// ListR2CustomDomains 列出所有自定义域名
func (s *R2CustomDomainService) ListR2CustomDomains(r2BucketID uint) ([]models.R2CustomDomain, error) {
	var domains []models.R2CustomDomain
	if err := s.db.Preload("R2Bucket").Where("r2_bucket_id = ? AND deleted_at IS NULL", r2BucketID).Order("id DESC").Find(&domains).Error; err != nil {
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

// CreatePendingDomain 创建一个 pending 状态的域名记录（用于异步创建）
func (s *R2CustomDomainService) CreatePendingDomain(r2BucketID uint, domain, note, defaultFilePath string) (*models.R2CustomDomain, error) {
	log := logger.GetLogger()

	// 检查存储桶是否存在
	var bucket models.R2Bucket
	if err := s.db.First(&bucket, r2BucketID).Error; err != nil {
		return nil, fmt.Errorf("R2存储桶不存在: %w", err)
	}

	// 创建 pending 状态的域名记录
	customDomain := &models.R2CustomDomain{
		R2BucketID:      r2BucketID,
		Domain:          domain,
		Status:          "pending",
		Note:            note,
		DefaultFilePath: defaultFilePath,
	}

	if err := s.db.Create(customDomain).Error; err != nil {
		return nil, fmt.Errorf("保存自定义域名信息失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"domain_id": customDomain.ID,
		"domain":    customDomain.Domain,
		"status":    "pending",
	}).Info("域名记录已创建，状态为 pending")

	return customDomain, nil
}

// ConfigureCustomDomainAsync 异步配置自定义域名（执行实际的 Cloudflare API 调用）
func (s *R2CustomDomainService) ConfigureCustomDomainAsync(domainID uint) error {
	log := logger.GetLogger()
	configLog := newConfigLogger()

	// 获取域名记录
	var customDomain models.R2CustomDomain
	if err := s.db.Preload("R2Bucket.CFAccount").First(&customDomain, domainID).Error; err != nil {
		return fmt.Errorf("域名记录不存在: %w", err)
	}

	configLog.info("开始配置", fmt.Sprintf("开始配置域名 %s", customDomain.Domain), "")

	// 更新状态为 processing
	customDomain.Status = "processing"
	if err := s.db.Save(&customDomain).Error; err != nil {
		log.WithError(err).Error("更新域名状态为 processing 失败")
		configLog.error("状态更新", "更新域名状态为 processing 失败", err.Error())
		s.saveConfigLogs(domainID, configLog)
		return fmt.Errorf("更新域名状态为 processing 失败: %w", err)
	}

	configLog.info("状态更新", "域名状态已更新为 processing", "")

	log.WithFields(map[string]interface{}{
		"domain_id": customDomain.ID,
		"domain":    customDomain.Domain,
	}).Info("开始配置自定义域名")

	// 获取 CF 账号信息
	cfAccount, err := s.cfAccountService.GetCFAccount(customDomain.R2Bucket.CFAccountID)
	if err != nil {
		configLog.error("获取账号", "获取 Cloudflare 账号信息失败", err.Error())
		s.saveConfigLogs(domainID, configLog)
		if updateErr := s.updateDomainStatus(domainID, "failed", fmt.Sprintf("获取CF账号失败: %v", err)); updateErr != nil {
			log.WithError(updateErr).Error("更新域名状态失败")
		}
		return err
	}
	configLog.info("获取账号", "成功获取 Cloudflare 账号信息", fmt.Sprintf("Account ID: %s", cfAccount.AccountID))

	// 获取 R2 API Token
	r2APIToken := s.cfAccountService.GetR2APIToken(cfAccount)
	if r2APIToken == "" {
		err := fmt.Errorf("Cloudflare账号未配置 R2 API Token 或 API Token")
		configLog.error("验证Token", "API Token 未配置", err.Error())
		s.saveConfigLogs(domainID, configLog)
		if updateErr := s.updateDomainStatus(domainID, "failed", err.Error()); updateErr != nil {
			log.WithError(updateErr).Error("更新域名状态失败")
		}
		return err
	}
	configLog.info("验证Token", "API Token 验证成功", "")

	// 创建 R2 API 服务
	accountID := cfAccount.AccountID

	// 根据 CF 账号信息创建 CloudflareService
	cloudflareSvc, err := s.createCloudflareService(cfAccount)
	if err != nil {
		configLog.error("创建服务", "创建 Cloudflare Service 失败", err.Error())
		s.saveConfigLogs(domainID, configLog)
		if updateErr := s.updateDomainStatus(domainID, "failed", fmt.Sprintf("创建 CloudflareService 失败: %v", err)); updateErr != nil {
			log.WithError(updateErr).Error("更新域名状态失败")
		}
		return fmt.Errorf("创建 CloudflareService 失败: %w", err)
	}
	configLog.info("创建服务", "Cloudflare Service 创建成功", "")

	// 获取 Zone ID
	rootDomain := s.ExtractRootDomain(customDomain.Domain)
	if rootDomain != customDomain.Domain {
		configLog.info("域名解析", fmt.Sprintf("检测到子域名，使用根域名获取 Zone ID: %s", rootDomain), "")
		log.WithFields(map[string]interface{}{
			"domain":      customDomain.Domain,
			"root_domain": rootDomain,
		}).Info("检测到子域名，使用根域名获取 Zone ID")
	}

	zoneID, err := cloudflareSvc.GetZoneID(rootDomain)
	if err != nil {
		zoneID = ""
		configLog.warning("获取Zone ID", "无法获取 Zone ID，将尝试自动查找", err.Error())
		log.WithError(err).WithFields(map[string]interface{}{
			"domain":      customDomain.Domain,
			"root_domain": rootDomain,
		}).Warn("无法获取 Zone ID，将尝试自动查找")
	} else {
		configLog.info("获取Zone ID", fmt.Sprintf("成功获取 Zone ID: %s", zoneID), "")
		log.WithFields(map[string]interface{}{
			"domain":      customDomain.Domain,
			"root_domain": rootDomain,
			"zone_id":     zoneID,
		}).Info("成功获取 Zone ID")
	}

	// 添加自定义域名
	domainIDStr, err := cloudflareSvc.AddCustomDomain(accountID, customDomain.R2Bucket.BucketName, customDomain.Domain, zoneID, true)
	if err != nil {
		configLog.error("添加域名", "添加自定义域名到 R2 失败", err.Error())
		s.saveConfigLogs(domainID, configLog)
		if updateErr := s.updateDomainStatus(domainID, "failed", fmt.Sprintf("添加自定义域名失败: %v", err)); updateErr != nil {
			log.WithError(updateErr).Error("更新域名状态失败")
		}
		return fmt.Errorf("添加自定义域名失败: %w", err)
	}
	configLog.info("添加域名", fmt.Sprintf("自定义域名已成功添加到 R2 (ID: %s)", domainIDStr), "")

	// 更新 ZoneID
	if zoneID != "" {
		customDomain.ZoneID = zoneID
		if err := s.db.Save(&customDomain).Error; err != nil {
			log.WithError(err).Error("更新 ZoneID 失败")
			configLog.error("更新ZoneID", "保存 Zone ID 到数据库失败", err.Error())
			s.saveConfigLogs(domainID, configLog)
			if updateErr := s.updateDomainStatus(domainID, "failed", fmt.Sprintf("更新 ZoneID 失败: %v", err)); updateErr != nil {
				log.WithError(updateErr).Error("更新域名状态失败")
			}
			return fmt.Errorf("更新 ZoneID 失败: %w", err)
		}
		configLog.info("更新ZoneID", "Zone ID 已保存到数据库", "")
	}

	// 自动创建各种规则和优化配置（使用根域名，这样规则会覆盖所有子域名）
	s.configureCloudflareOptimizations(cloudflareSvc, zoneID, rootDomain, customDomain.DefaultFilePath, configLog)

	// 保存配置日志
	s.saveConfigLogs(domainID, configLog)

	// 更新状态为 active
	if err := s.updateDomainStatus(domainID, "active", ""); err != nil {
		log.WithError(err).Error("更新域名状态为 active 失败")
		return fmt.Errorf("更新域名状态为 active 失败: %w", err)
	}

	configLog.info("配置完成", "域名配置已全部完成", "")
	s.saveConfigLogs(domainID, configLog)

	log.WithFields(map[string]interface{}{
		"domain_id":            customDomain.ID,
		"domain":               customDomain.Domain,
		"cloudflare_domain_id": domainIDStr,
	}).Info("自定义域名配置完成")

	return nil
}

// saveConfigLogs 保存配置日志到数据库
func (s *R2CustomDomainService) saveConfigLogs(domainID uint, configLog *configLogger) {
	log := logger.GetLogger()
	logsJSON := configLog.toJSON()

	if err := s.db.Model(&models.R2CustomDomain{}).Where("id = ?", domainID).Update("config_logs", logsJSON).Error; err != nil {
		log.WithError(err).WithField("domain_id", domainID).Error("保存配置日志失败")
	}
}

// updateDomainStatus 更新域名状态
func (s *R2CustomDomainService) updateDomainStatus(domainID uint, status string, errorMsg string) error {
	log := logger.GetLogger()

	updates := map[string]interface{}{
		"status": status,
	}

	if errorMsg != "" {
		// 将错误信息追加到 note 字段
		var domain models.R2CustomDomain
		if err := s.db.First(&domain, domainID).Error; err != nil {
			log.WithError(err).WithField("domain_id", domainID).Error("获取域名记录失败")
			return fmt.Errorf("获取域名记录失败: %w", err)
		}

		if domain.Note != "" {
			updates["note"] = domain.Note + "\n错误: " + errorMsg
		} else {
			updates["note"] = "错误: " + errorMsg
		}
	}

	if err := s.db.Model(&models.R2CustomDomain{}).Where("id = ?", domainID).Updates(updates).Error; err != nil {
		log.WithError(err).WithField("domain_id", domainID).Error("更新域名状态失败")
		return fmt.Errorf("更新域名状态失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"domain_id": domainID,
		"status":    status,
	}).Info("域名状态已更新")

	return nil
}

// configureCloudflareOptimizations 配置 Cloudflare 优化规则
func (s *R2CustomDomainService) configureCloudflareOptimizations(cloudflareSvc *cloudflare.CloudflareService, zoneID, domain, defaultFilePath string, configLog *configLogger) {
	log := logger.GetLogger()

	if zoneID == "" {
		configLog.warning("优化配置", "Zone ID 为空，跳过配置优化规则", "")
		log.WithField("domain", domain).Warn("Zone ID 为空，跳过配置优化规则")
		return
	}

	configLog.info("优化配置", "开始配置 Cloudflare 优化规则", "")

	// 自动创建 CORS Transform Rule
	corsRuleID, corsErr := cloudflareSvc.CreateCORSTransformRule(zoneID, domain, "*")
	if corsErr != nil {
		configLog.warning("CORS规则", "创建 CORS Transform Rule 失败", corsErr.Error())
		log.WithError(corsErr).WithFields(map[string]interface{}{
			"domain":  domain,
			"zone_id": zoneID,
		}).Warn("自动创建 CORS Transform Rule 失败")
	} else if corsRuleID != "" {
		configLog.info("CORS规则", fmt.Sprintf("CORS Transform Rule 已创建 (ID: %s)", corsRuleID), "")
		log.WithFields(map[string]interface{}{
			"domain":  domain,
			"zone_id": zoneID,
			"rule_id": corsRuleID,
		}).Info("CORS Transform Rule 已自动创建")
	}

	// 自动创建 WAF "免检金牌" VIP 下载规则
	vipRuleID, vipErr := cloudflareSvc.CreateWAFVIPDownloadRule(zoneID, domain)
	if vipErr != nil {
		configLog.warning("WAF VIP规则", "创建 WAF VIP 下载规则失败", vipErr.Error())
		log.WithError(vipErr).WithFields(map[string]interface{}{
			"domain":  domain,
			"zone_id": zoneID,
		}).Warn("自动创建 WAF VIP 下载规则失败")
	} else if vipRuleID != "" {
		configLog.info("WAF VIP规则", fmt.Sprintf("WAF VIP 下载规则已创建 (ID: %s)", vipRuleID), "")
		log.WithFields(map[string]interface{}{
			"domain":  domain,
			"zone_id": zoneID,
			"rule_id": vipRuleID,
		}).Info("🎉 WAF VIP 下载规则已自动创建")
	}

	// 自动创建 WAF 安全规则
	wafRuleID, wafErr := cloudflareSvc.CreateWAFSecurityRule(zoneID, domain, []string{"apk"})
	if wafErr != nil {
		configLog.warning("WAF安全规则", "创建 WAF 安全规则失败", wafErr.Error())
		log.WithError(wafErr).WithFields(map[string]interface{}{
			"domain":  domain,
			"zone_id": zoneID,
		}).Warn("自动创建 WAF 安全规则失败")
	} else if wafRuleID != "" {
		configLog.info("WAF安全规则", fmt.Sprintf("WAF 安全规则已创建 (ID: %s)", wafRuleID), "")
		log.WithFields(map[string]interface{}{
			"domain":  domain,
			"zone_id": zoneID,
			"rule_id": wafRuleID,
		}).Info("WAF 安全规则已自动创建")
	}

	// 自动创建 Page Rule
	pageRuleID, pageErr := cloudflareSvc.CreatePageRule(zoneID, domain, true)
	if pageErr != nil {
		configLog.warning("Page Rule", "创建 Page Rule (缓存优化) 失败", pageErr.Error())
		log.WithError(pageErr).WithFields(map[string]interface{}{
			"domain":  domain,
			"zone_id": zoneID,
		}).Warn("自动创建 Page Rule 失败")
	} else if pageRuleID != "" {
		configLog.info("Page Rule", fmt.Sprintf("Page Rule 已创建 (ID: %s, Edge TTL: 30天, Browser TTL: 1年)", pageRuleID), "")
		log.WithFields(map[string]interface{}{
			"domain":     domain,
			"zone_id":    zoneID,
			"rule_id":    pageRuleID,
			"cache_ttl":  "Edge: 30天, Browser: 1年",
			"cache_mode": "Cache Everything",
		}).Info("Page Rule 已自动创建")
	}

	// 启用各种优化功能
	if err := cloudflareSvc.EnableSmartTieredCache(zoneID); err == nil {
		configLog.info("智能分层缓存", "智能分层缓存已启用", "")
	} else {
		configLog.warning("智能分层缓存", "启用智能分层缓存失败", err.Error())
	}

	if err := cloudflareSvc.EnableHTTP3(zoneID); err == nil {
		configLog.info("HTTP/3", "HTTP/3 (QUIC) 已启用", "")
	} else {
		configLog.warning("HTTP/3", "启用 HTTP/3 失败", err.Error())
	}

	if err := cloudflareSvc.Enable0RTT(zoneID); err == nil {
		configLog.info("0-RTT", "0-RTT 连接恢复已启用", "")
	} else {
		configLog.warning("0-RTT", "启用 0-RTT 失败", err.Error())
	}

	if err := cloudflareSvc.EnableIPv6(zoneID); err == nil {
		configLog.info("IPv6", "IPv6 已启用", "")
	} else {
		configLog.warning("IPv6", "启用 IPv6 失败", err.Error())
	}

	if err := cloudflareSvc.EnableMinTLS13(zoneID); err == nil {
		configLog.info("TLS 1.3", "TLS 1.3 最低版本已设置", "")
	} else {
		configLog.warning("TLS 1.3", "设置 TLS 1.3 失败", err.Error())
	}

	if err := cloudflareSvc.EnableBrotli(zoneID); err == nil {
		configLog.info("Brotli压缩", "Brotli 压缩已启用", "")
	} else {
		configLog.warning("Brotli压缩", "启用 Brotli 失败", err.Error())
	}

	if err := cloudflareSvc.EnableAlwaysUseHTTPS(zoneID); err == nil {
		configLog.info("Always HTTPS", "Always Use HTTPS 已启用", "")
	} else {
		configLog.warning("Always HTTPS", "启用 Always HTTPS 失败", err.Error())
	}

	if err := cloudflareSvc.DisableRocketLoader(zoneID); err == nil {
		configLog.info("Rocket Loader", "Rocket Loader 已禁用", "")
	} else {
		configLog.warning("Rocket Loader", "禁用 Rocket Loader 失败", err.Error())
	}

	if err := cloudflareSvc.DisableAutoMinify(zoneID); err == nil {
		configLog.info("Auto Minify", "Auto Minify 已禁用", "")
	} else {
		configLog.warning("Auto Minify", "禁用 Auto Minify 失败", err.Error())
	}

	// 如果设置了默认文件路径，创建重定向规则
	if defaultFilePath != "" {
		redirectRuleID, redirectErr := cloudflareSvc.CreateDefaultFileRedirect(zoneID, domain, defaultFilePath)
		if redirectErr != nil {
			configLog.warning("默认文件重定向", fmt.Sprintf("创建默认文件重定向规则失败 (%s)", defaultFilePath), redirectErr.Error())
			log.WithError(redirectErr).WithFields(map[string]interface{}{
				"domain":            domain,
				"zone_id":           zoneID,
				"default_file_path": defaultFilePath,
			}).Warn("创建默认文件重定向规则失败")
		} else if redirectRuleID != "" {
			configLog.info("默认文件重定向", fmt.Sprintf("默认文件重定向规则已创建 (ID: %s, 文件: %s)", redirectRuleID, defaultFilePath), "")
			log.WithFields(map[string]interface{}{
				"domain":            domain,
				"zone_id":           zoneID,
				"rule_id":           redirectRuleID,
				"default_file_path": defaultFilePath,
			}).Info("🎉 默认文件重定向规则已创建")
		}
	}

	configLog.info("优化配置", "Cloudflare 优化配置完成", "")
	log.WithField("domain", domain).Info("Cloudflare 优化配置完成")
}

// AddCustomDomain 添加自定义域名
func (s *R2CustomDomainService) AddCustomDomain(r2BucketID uint, domain, note, defaultFilePath string) (*models.R2CustomDomain, error) {
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

	// 自动创建 CORS Transform Rule
	if zoneID != "" {
		// 尝试自动创建 CORS 规则（如果失败只记录警告，不阻止域名添加）
		corsRuleID, corsErr := cloudflareSvc.CreateCORSTransformRule(zoneID, domain, "*")
		if corsErr != nil {
			log.WithError(corsErr).WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
			}).Warn("自动创建 CORS Transform Rule 失败，请手动在 Cloudflare Dashboard 配置")
		} else if corsRuleID != "" {
			log.WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
				"rule_id": corsRuleID,
			}).Info("CORS Transform Rule 已自动创建")
		}

		// 自动创建 WAF "免检金牌" VIP 下载规则（00_Allow_APK_Download_VIP）
		// 这是最重要的规则，优先级最高，跳过所有防火墙检查
		// 匹配：.apk 或 .obb 或 /download/ 路径
		vipRuleID, vipErr := cloudflareSvc.CreateWAFVIPDownloadRule(zoneID, domain)
		if vipErr != nil {
			log.WithError(vipErr).WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
			}).Warn("自动创建 WAF VIP 下载规则失败，请手动在 Cloudflare Dashboard 配置")
		} else if vipRuleID != "" {
			log.WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
				"rule_id": vipRuleID,
			}).Info("🎉 WAF VIP 下载规则已自动创建（00_Allow_APK_Download_VIP - 免检金牌）")
		}

		// 自动创建 WAF 安全规则（VPN 白名单 + IDM 高频下载豁免）
		// 注意：这是备用规则，VIP 规则优先级更高
		wafRuleID, wafErr := cloudflareSvc.CreateWAFSecurityRule(zoneID, domain, []string{"apk"})
		if wafErr != nil {
			log.WithError(wafErr).WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
			}).Warn("自动创建 WAF 安全规则失败，请手动在 Cloudflare Dashboard 配置")
		} else if wafRuleID != "" {
			log.WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
				"rule_id": wafRuleID,
			}).Info("WAF 安全规则已自动创建（VPN白名单+IDM高频下载豁免）")
		}

		// 自动创建 Page Rule（缓存优化规则）
		// Cache Everything + Edge TTL 30天 + Browser TTL 1年
		pageRuleID, pageErr := cloudflareSvc.CreatePageRule(zoneID, domain, true)
		if pageErr != nil {
			log.WithError(pageErr).WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
			}).Warn("自动创建 Page Rule（缓存优化）失败，请手动在 Cloudflare Dashboard 配置")
		} else if pageRuleID != "" {
			log.WithFields(map[string]interface{}{
				"domain":     domain,
				"zone_id":    zoneID,
				"rule_id":    pageRuleID,
				"cache_ttl":  "Edge: 30天, Browser: 1年",
				"cache_mode": "Cache Everything",
			}).Info("Page Rule（缓存优化）已自动创建，节省源站流量费用")
		} else {
			log.WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
			}).Info("Page Rule 可能已存在，跳过创建")
		}

		// 自动启用智能分层缓存 (Smart Tiered Cache)
		if smartCacheErr := cloudflareSvc.EnableSmartTieredCache(zoneID); smartCacheErr != nil {
			log.WithError(smartCacheErr).WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
			}).Warn("启用智能分层缓存失败，请手动在 Cloudflare Dashboard 配置")
		} else {
			log.WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
			}).Info("智能分层缓存已启用，节点接力优化完成")
		}

		// 自动启用 HTTP/3 (QUIC)
		if http3Err := cloudflareSvc.EnableHTTP3(zoneID); http3Err != nil {
			log.WithError(http3Err).WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
			}).Warn("启用 HTTP/3 (QUIC) 失败，请手动在 Cloudflare Dashboard 配置")
		} else {
			log.WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
			}).Info("HTTP/3 (QUIC) 已启用，抗丢包优化完成")
		}

		// 自动启用 0-RTT 连接恢复
		if rttErr := cloudflareSvc.Enable0RTT(zoneID); rttErr != nil {
			log.WithError(rttErr).WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
			}).Warn("启用 0-RTT 连接恢复失败，请手动在 Cloudflare Dashboard 配置")
		} else {
			log.WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
			}).Info("0-RTT 连接恢复已启用，秒连优化完成")
		}

		// 自动启用 IPv6
		if ipv6Err := cloudflareSvc.EnableIPv6(zoneID); ipv6Err != nil {
			log.WithError(ipv6Err).WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
			}).Warn("启用 IPv6 失败，请手动在 Cloudflare Dashboard 配置")
		} else {
			log.WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
			}).Info("IPv6 已启用，直连东南亚移动网")
		}

		// 自动启用 TLS 1.3 最低版本
		if tlsErr := cloudflareSvc.EnableMinTLS13(zoneID); tlsErr != nil {
			log.WithError(tlsErr).WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
			}).Warn("设置 TLS 1.3 失败，请手动在 Cloudflare Dashboard 配置")
		} else {
			log.WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
			}).Info("TLS 1.3 最低版本已设置，新手机极速握手")
		}

		// 自动启用 Brotli 压缩
		if brotliErr := cloudflareSvc.EnableBrotli(zoneID); brotliErr != nil {
			log.WithError(brotliErr).WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
			}).Warn("启用 Brotli 失败，请手动在 Cloudflare Dashboard 配置")
		} else {
			log.WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
			}).Info("Brotli 压缩已启用，加速推广页白屏加载")
		}

		// 自动启用 Always Use HTTPS
		if httpsErr := cloudflareSvc.EnableAlwaysUseHTTPS(zoneID); httpsErr != nil {
			log.WithError(httpsErr).WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
			}).Warn("启用 Always Use HTTPS 失败，请手动在 Cloudflare Dashboard 配置")
		} else {
			log.WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
			}).Info("Always Use HTTPS 已启用，全站强制 HTTPS，防劫持")
		}

		// 自动禁用 Rocket Loader（保护 APK）
		if rocketErr := cloudflareSvc.DisableRocketLoader(zoneID); rocketErr != nil {
			log.WithError(rocketErr).WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
			}).Warn("禁用 Rocket Loader 失败，请手动在 Cloudflare Dashboard 配置")
		} else {
			log.WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
			}).Info("Rocket Loader 已禁用，保护 APK 不被处理")
		}

		// 自动禁用 Auto Minify（节省处理时间）
		if minifyErr := cloudflareSvc.DisableAutoMinify(zoneID); minifyErr != nil {
			log.WithError(minifyErr).WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
			}).Warn("禁用 Auto Minify 失败，请手动在 Cloudflare Dashboard 配置")
		} else {
			log.WithFields(map[string]interface{}{
				"domain":  domain,
				"zone_id": zoneID,
			}).Info("Auto Minify 已全部禁用，节省处理时间，纯净传输")
		}
	} else {
		log.WithFields(map[string]interface{}{
			"domain": domain,
		}).Warn("Zone ID 为空，跳过自动创建 CORS Transform Rule、WAF 安全规则、Page Rule 和所有网络优化规则，请手动在 Cloudflare Dashboard 配置")
	}

	// 如果设置了默认文件路径，创建重定向规则
	if defaultFilePath != "" && zoneID != "" {
		log.WithFields(map[string]interface{}{
			"domain":            domain,
			"zone_id":           zoneID,
			"default_file_path": defaultFilePath,
		}).Info("开始创建默认文件重定向规则")

		redirectRuleID, redirectErr := cloudflareSvc.CreateDefaultFileRedirect(zoneID, domain, defaultFilePath)
		if redirectErr != nil {
			log.WithError(redirectErr).WithFields(map[string]interface{}{
				"domain":            domain,
				"zone_id":           zoneID,
				"default_file_path": defaultFilePath,
			}).Warn("创建默认文件重定向规则失败，请手动在 Cloudflare Dashboard 配置")
		} else if redirectRuleID != "" {
			log.WithFields(map[string]interface{}{
				"domain":            domain,
				"zone_id":           zoneID,
				"rule_id":           redirectRuleID,
				"default_file_path": defaultFilePath,
			}).Info("🎉 默认文件重定向规则已创建，访问根路径将自动跳转到默认文件")
		}
	}

	// 保存到数据库
	customDomain := &models.R2CustomDomain{
		R2BucketID:      r2BucketID,
		Domain:          domain,
		ZoneID:          zoneID,
		Status:          "active",
		Note:            note,
		DefaultFilePath: defaultFilePath,
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
