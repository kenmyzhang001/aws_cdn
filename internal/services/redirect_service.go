package services

import (
	"aws_cdn/internal/config"
	"aws_cdn/internal/models"
	"aws_cdn/internal/services/aws"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"
)

type RedirectService struct {
	db            *gorm.DB
	cloudFrontSvc *aws.CloudFrontService
	s3Svc         *aws.S3Service
	domainSvc     *DomainService // 域名服务，用于查询域名和证书状态
	config        *config.AWSConfig
}

func NewRedirectService(db *gorm.DB, cloudFrontSvc *aws.CloudFrontService, s3Svc *aws.S3Service, domainSvc *DomainService, cfg *config.AWSConfig) *RedirectService {
	return &RedirectService{
		db:            db,
		cloudFrontSvc: cloudFrontSvc,
		s3Svc:         s3Svc,
		domainSvc:     domainSvc,
		config:        cfg,
	}
}

// generateRedirectHTML 生成包含轮播逻辑的HTML文件
func (s *RedirectService) generateRedirectHTML(targetURLs []string) (string, error) {
	// 将目标URL列表转换为JSON字符串，用于嵌入到HTML中
	targetsJSON, err := json.Marshal(targetURLs)
	if err != nil {
		return "", fmt.Errorf("序列化目标URL失败: %w", err)
	}

	htmlTemplate := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Redirecting...</title>
    <style>
        body {
            margin: 0;
            padding: 0;
            background: #000;
            overflow: hidden;
        }
    </style>
</head>
<body>
    <script>
        (function() {
            // 目标URL列表（嵌入在HTML中）
            const targets = {{.TargetsJSON}};
            
            if (!targets || targets.length === 0) {
                console.error('No target URLs available');
                return;
            }
            
            // 从localStorage获取访问计数器
            const storageKey = 'redirect_counter';
            let counter = parseInt(localStorage.getItem(storageKey) || '0', 10);
            
            // 选择目标URL（轮询）
            const targetIndex = counter % targets.length;
            const targetURL = targets[targetIndex];
            
            // 增加计数器并保存
            counter++;
            localStorage.setItem(storageKey, counter.toString());
            
            // 立即跳转，无感知
            window.location.replace(targetURL);
        })();
    </script>
</body>
</html>`

	tmpl, err := template.New("redirect").Parse(htmlTemplate)
	if err != nil {
		return "", fmt.Errorf("解析HTML模板失败: %w", err)
	}

	var buf strings.Builder
	data := map[string]interface{}{
		"TargetsJSON": template.JS(string(targetsJSON)),
	}
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("生成HTML失败: %w", err)
	}

	return buf.String(), nil
}

// uploadHTMLOnly 仅上传HTML文件到S3（不创建CloudFront分发）
func (s *RedirectService) uploadHTMLOnly(rule *models.RedirectRule) error {
	if s.config.S3BucketName == "" {
		return fmt.Errorf("S3存储桶名称未配置")
	}

	// 收集活跃的目标URL
	var activeTargets []string
	for _, target := range rule.Targets {
		if target.IsActive {
			activeTargets = append(activeTargets, target.TargetURL)
		}
	}

	if len(activeTargets) == 0 {
		return fmt.Errorf("没有可用的重定向目标")
	}

	// 生成HTML文件
	htmlContent, err := s.generateRedirectHTML(activeTargets)
	if err != nil {
		return fmt.Errorf("生成HTML文件失败: %w", err)
	}

	// S3目录路径：redirects/{domain}/
	s3Path := fmt.Sprintf("redirects/%s/", rule.SourceDomain)
	s3Key := s3Path + "index.html"

	// 上传HTML文件到S3
	return s.s3Svc.UploadHTML(s.config.S3BucketName, s3Key, htmlContent)
}

// deployRedirectRule 部署重定向规则到S3和CloudFront
func (s *RedirectService) deployRedirectRule(rule *models.RedirectRule, certificateARN string) error {
	// 确保 S3 bucket policy 允许公开访问
	if s.config.S3BucketName != "" {
		if err := s.s3Svc.EnsureBucketPolicyForPublicAccess(s.config.S3BucketName); err != nil {
			// 记录警告但不阻止流程，因为可能已有其他策略
			fmt.Printf("警告: 配置 S3 bucket policy 失败: %v\n", err)
		}
	}

	// 验证 S3 bucket 配置
	if s.config.S3BucketName == "" {
		return fmt.Errorf("S3存储桶名称未配置")
	}

	// 确保 S3 bucket 存在（如果不存在则创建）
	if err := s.s3Svc.EnsureBucketExists(s.config.S3BucketName); err != nil {
		return fmt.Errorf("确保S3存储桶存在失败: %w", err)
	}

	// 先上传HTML文件
	if err := s.uploadHTMLOnly(rule); err != nil {
		return err
	}

	// 获取S3域名
	s3Origin := s.s3Svc.GetBucketDomain(s.config.S3BucketName)
	if s3Origin == "" {
		return fmt.Errorf("无法获取S3存储桶域名")
	}

	// 验证 S3 bucket 是否可访问（通过检查 bucket 是否存在）
	exists, err := s.s3Svc.BucketExists(s.config.S3BucketName)
	if err != nil {
		return fmt.Errorf("验证S3存储桶可访问性失败: %w", err)
	}
	if !exists {
		return fmt.Errorf("S3存储桶 %s 不存在或不可访问", s.config.S3BucketName)
	}

	// 输出调试信息
	fmt.Printf("创建CloudFront分发 - S3 Bucket: %s, S3 Origin: %s, Region: %s\n",
		s.config.S3BucketName, s3Origin, s.config.Region)

	// S3目录路径（OriginPath会在CreateDistributionWithPath中自动格式化）
	s3Path := fmt.Sprintf("redirects/%s", rule.SourceDomain)

	// 创建CloudFront分发，指向S3目录
	distributionID, err := s.cloudFrontSvc.CreateDistributionWithPath(
		rule.SourceDomain,
		certificateARN,
		s3Origin,
		s3Path, // OriginPath会自动格式化为 /redirects/example.com
	)
	if err != nil {
		return fmt.Errorf("创建CloudFront分发失败: %w", err)
	}

	// 更新规则中的CloudFront ID
	rule.CloudFrontID = distributionID
	if err := s.db.Save(rule).Error; err != nil {
		return fmt.Errorf("更新规则失败: %w", err)
	}

	// 创建 Route 53 DNS 记录指向 CloudFront
	if err := s.createRoute53RecordForCloudFront(rule.SourceDomain, distributionID); err != nil {
		// 记录错误但不阻止流程，因为 DNS 记录可以稍后手动创建
		fmt.Printf("警告: 创建 Route 53 DNS 记录失败: %v\n", err)
	}

	return nil
}

// findCertificateARN 查找域名的证书ARN（从domain表中查找）
func (s *RedirectService) findCertificateARN(domainName string) string {
	var domain models.Domain
	if err := s.db.Where("domain_name = ?", domainName).First(&domain).Error; err == nil {
		if domain.CertificateARN != "" {
			return domain.CertificateARN
		}
	}
	return ""
}

// CreateRedirectRuleResult 创建重定向规则的结果
type CreateRedirectRuleResult struct {
	Rule     *models.RedirectRule
	Warnings []string
}

// CheckDomainUsedByDownloadPackage 检查域名是否被下载包使用（排除软删除的记录）
func (s *RedirectService) CheckDomainUsedByDownloadPackage(domainName string) (bool, error) {
	var count int64
	if err := s.db.Model(&models.DownloadPackage{}).
		Where("domain_name = ?", domainName).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// CreateRedirectRule 创建重定向规则并自动部署
func (s *RedirectService) CreateRedirectRule(sourceDomain string, targetURLs []string, certificateARN string) (*CreateRedirectRuleResult, error) {
	// 检查源域名是否已存在（排除软删除的记录）
	var existingRule models.RedirectRule
	if err := s.db.Where("source_domain = ?", sourceDomain).First(&existingRule).Error; err == nil {
		return nil, fmt.Errorf("源域名 %s 的重定向规则已存在", sourceDomain)
	}

	// 检查域名是否已被下载包使用
	isUsed, err := s.CheckDomainUsedByDownloadPackage(sourceDomain)
	if err != nil {
		return nil, fmt.Errorf("检查域名使用状态失败: %w", err)
	}
	if isUsed {
		return nil, fmt.Errorf("域名 %s 已被下载包使用，请先删除下载包后再使用", sourceDomain)
	}

	// 检查是否存在软删除的记录，如果存在则先真正删除它
	var softDeletedRule models.RedirectRule
	if err := s.db.Unscoped().Where("source_domain = ?", sourceDomain).First(&softDeletedRule).Error; err == nil {
		// 如果存在软删除的记录，先真正删除它（硬删除）
		// 同时删除相关的CloudFront和S3资源
		if softDeletedRule.CloudFrontID != "" {
			// 先禁用分发
			enabled := false
			if err := s.cloudFrontSvc.UpdateDistribution(softDeletedRule.CloudFrontID, nil, "", &enabled); err != nil {
				fmt.Printf("警告: 禁用CloudFront分发失败: %v\n", err)
			}
			time.Sleep(2 * time.Second)
			// 删除分发
			if err := s.cloudFrontSvc.DeleteDistribution(softDeletedRule.CloudFrontID); err != nil {
				fmt.Printf("警告: 删除CloudFront分发失败: %v\n", err)
			}
		}
		// 删除S3目录
		if s.config.S3BucketName != "" {
			s3Path := fmt.Sprintf("redirects/%s/", softDeletedRule.SourceDomain)
			if err := s.s3Svc.DeleteObjectsWithPrefix(s.config.S3BucketName, s3Path); err != nil {
				fmt.Printf("警告: 删除S3目录失败: %v\n", err)
			}
		}
		// 真正删除数据库记录（硬删除）
		if err := s.db.Unscoped().Delete(&softDeletedRule).Error; err != nil {
			return nil, fmt.Errorf("清理已删除的记录失败: %w", err)
		}
	}

	// 创建重定向规则
	rule := &models.RedirectRule{
		SourceDomain: sourceDomain,
	}

	if err := s.db.Create(rule).Error; err != nil {
		return nil, fmt.Errorf("创建重定向规则失败: %w", err)
	}

	// 创建目标 URL
	for _, targetURL := range targetURLs {
		target := &models.RedirectTarget{
			RuleID:    rule.ID,
			TargetURL: targetURL,
			Weight:    1,
			IsActive:  true,
		}
		if err := s.db.Create(target).Error; err != nil {
			return nil, fmt.Errorf("创建重定向目标失败: %w", err)
		}
		rule.Targets = append(rule.Targets, *target)
	}

	// 重新加载规则以获取所有目标
	if err := s.db.Preload("Targets").First(rule, rule.ID).Error; err != nil {
		return nil, fmt.Errorf("加载重定向规则失败: %w", err)
	}

	// 如果没有提供证书ARN，尝试从domain表中查找
	if certificateARN == "" {
		certificateARN = s.findCertificateARN(sourceDomain)
	}

	// 收集部署警告信息
	var deploymentWarnings []string

	// 自动部署到S3和CloudFront
	// 如果找到了证书ARN，创建CloudFront分发
	if certificateARN != "" {
		if err := s.deployRedirectRule(rule, certificateARN); err != nil {
			// 如果部署失败，记录错误但不阻止规则创建
			// 可以后续通过更新接口重新部署
			warningMsg := fmt.Sprintf("部署重定向规则失败: %v", err)
			deploymentWarnings = append(deploymentWarnings, warningMsg)
			fmt.Printf("警告: %s\n", warningMsg)
		}
	} else {
		// 如果没有找到证书，先上传HTML文件到S3
		// 系统会自动尝试从domain表中查找证书，如果找到会自动创建CloudFront
		// 如果确实没有证书，可以后续通过BindDomainToCloudFront接口手动绑定CloudFront
		if err := s.uploadHTMLOnly(rule); err != nil {
			warningMsg := fmt.Sprintf("上传HTML文件失败: %v", err)
			deploymentWarnings = append(deploymentWarnings, warningMsg)
			fmt.Printf("警告: %s\n", warningMsg)
		} else {
			// 上传成功但没有证书，提示用户
			deploymentWarnings = append(deploymentWarnings, "未找到证书，已上传HTML文件到S3，但未创建CloudFront分发")
		}
	}

	// 返回规则和警告信息
	return &CreateRedirectRuleResult{
		Rule:     rule,
		Warnings: deploymentWarnings,
	}, nil
}

// GetRedirectRule 获取重定向规则
func (s *RedirectService) GetRedirectRule(id uint) (*models.RedirectRule, error) {
	var rule models.RedirectRule
	if err := s.db.Preload("Targets").First(&rule, id).Error; err != nil {
		return nil, fmt.Errorf("重定向规则不存在: %w", err)
	}
	return &rule, nil
}

// CheckURLStatus 检查URL是否可访问
func (s *RedirectService) CheckURLStatus(targetURL string) string {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// 确保URL有协议
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "https://" + targetURL
	}

	resp, err := client.Head(targetURL)
	if err != nil {
		return "unreachable" // 不可访问
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return "active" // 可访问
	}
	return "error" // 错误状态码
}

// GetDomainInfoByDomainName 根据域名名称获取域名信息
// 返回域名状态和证书状态
func (s *RedirectService) GetDomainInfoByDomainName(domainName string) (string, string) {
	if s.domainSvc == nil {
		return "", ""
	}

	var domain models.Domain
	if err := s.db.Where("domain_name = ?", domainName).First(&domain).Error; err != nil {
		return "", "" // 域名不存在
	}

	// 查询最新的证书状态
	certStatus := domain.CertificateStatus
	if domain.CertificateARN != "" {
		if status, err := s.domainSvc.GetCertificateStatus(domain.ID); err == nil {
			certStatus = status
		}
	}

	return string(domain.Status), certStatus
}

// CheckRoute53RecordStatus 检查 Route 53 DNS 记录状态
// domainName: 域名
// cloudFrontID: CloudFront 分发 ID（可选，如果提供则验证是否指向正确的分发）
// 返回 "configured"（已配置且正确）、"not_configured"（未配置）、"mismatched"（指向错误的分发）或 "error"（错误）
func (s *RedirectService) CheckRoute53RecordStatus(domainName, cloudFrontID string) string {
	if s.domainSvc == nil {
		return "error"
	}

	// 查找域名对应的 Route 53 Hosted Zone ID
	var domain models.Domain
	if err := s.db.Where("domain_name = ?", domainName).First(&domain).Error; err != nil {
		return "not_configured" // 域名不存在，认为未配置
	}

	if domain.HostedZoneID == "" {
		return "not_configured" // 没有托管区域
	}

	// 如果提供了 CloudFront ID，获取对应的域名进行验证
	var cloudFrontDomainName string
	if cloudFrontID != "" {
		dist, err := s.cloudFrontSvc.GetDistribution(cloudFrontID)
		if err == nil && dist != nil && dist.DomainName != nil {
			cloudFrontDomainName = *dist.DomainName
		}
	}

	// 检查是否存在指向 CloudFront 的 A 记录，并验证是否指向正确的分发
	exists, err := s.domainSvc.CheckCloudFrontAliasRecord(domain.HostedZoneID, domainName, cloudFrontDomainName)
	if err != nil {
		return "error"
	}

	if exists {
		return "configured"
	}

	// 如果指定了 CloudFront 域名但检查失败，可能是指向了错误的分发
	if cloudFrontDomainName != "" {
		// 再次检查是否指向了其他 CloudFront（不指定域名）
		existsAny, err := s.domainSvc.CheckCloudFrontAliasRecord(domain.HostedZoneID, domainName, "")
		if err == nil && existsAny {
			return "mismatched" // 指向了 CloudFront 但不是正确的分发
		}
	}

	return "not_configured"
}

// CheckWWWCNAMERecordStatus 检查 www CNAME 记录状态
// domainName: 根域名（例如：example.com）
// 返回 "configured"（已配置）、"not_configured"（未配置）或 "error"（错误）
func (s *RedirectService) CheckWWWCNAMERecordStatus(domainName string) string {
	if s.domainSvc == nil {
		return "error"
	}

	// 如果域名是 www 子域名，不需要检查
	if strings.HasPrefix(domainName, "www.") {
		return "not_configured" // www 子域名不需要 www CNAME
	}

	// 查找域名对应的 Route 53 Hosted Zone ID
	var domain models.Domain
	if err := s.db.Where("domain_name = ?", domainName).First(&domain).Error; err != nil {
		return "not_configured" // 域名不存在，认为未配置
	}

	if domain.HostedZoneID == "" {
		return "not_configured" // 没有托管区域
	}

	// 检查是否存在 www CNAME 记录
	exists, err := s.domainSvc.CheckWWWCNAMERecord(domain.HostedZoneID, domainName)
	if err != nil {
		return "error"
	}

	if exists {
		return "configured"
	}

	return "not_configured"
}

// CheckS3BucketPolicyStatus 检查 S3 bucket policy 状态
// 返回 "configured"（已配置）、"not_configured"（未配置）或 "error"（错误）
func (s *RedirectService) CheckS3BucketPolicyStatus() string {
	if s.config.S3BucketName == "" {
		return "not_configured"
	}

	policyConfigured, err := s.s3Svc.CheckBucketPolicyForPublicAccess(s.config.S3BucketName)
	if err != nil {
		return "error"
	}

	if policyConfigured {
		return "configured"
	}

	return "not_configured"
}

// createRoute53RecordForCloudFront 为 CloudFront 分发创建 Route 53 DNS 记录
// 如果记录已存在，会更新为指向正确的 CloudFront 分发
func (s *RedirectService) createRoute53RecordForCloudFront(domainName, distributionID string) error {
	if s.domainSvc == nil {
		return fmt.Errorf("域名服务未初始化")
	}

	// 查找域名对应的 Route 53 Hosted Zone ID
	var domain models.Domain
	if err := s.db.Where("domain_name = ?", domainName).First(&domain).Error; err != nil {
		return fmt.Errorf("未找到域名 %s 的 Route 53 托管区域", domainName)
	}

	if domain.HostedZoneID == "" {
		return fmt.Errorf("域名 %s 没有配置 Route 53 托管区域", domainName)
	}

	// 获取 CloudFront 分发的域名
	dist, err := s.cloudFrontSvc.GetDistribution(distributionID)
	if err != nil {
		return fmt.Errorf("获取 CloudFront 分发信息失败: %w", err)
	}

	if dist == nil || dist.DomainName == nil {
		return fmt.Errorf("CloudFront 分发没有域名信息")
	}

	cloudFrontDomainName := *dist.DomainName

	// 先检查是否已存在正确的 DNS 记录
	exists, err := s.domainSvc.CheckCloudFrontAliasRecord(domain.HostedZoneID, domainName, cloudFrontDomainName)
	if err != nil {
		// 检查失败，继续尝试创建
	} else if !exists {
		// 如果不存在，创建或更新 Alias 记录（使用 UPSERT，如果存在则更新）
		if err := s.domainSvc.CreateCloudFrontAliasRecord(domain.HostedZoneID, domainName, cloudFrontDomainName); err != nil {
			return err
		}
	}

	// 如果域名不是 www 子域名，为 www 子域名创建 CNAME 记录指向根域名
	// 即使根域名的 A 记录已存在，也要确保 www CNAME 记录存在
	if !strings.HasPrefix(domainName, "www.") {
		if err := s.domainSvc.CreateWWWCNAMERecord(domain.HostedZoneID, domainName); err != nil {
			// 记录错误但不阻止流程，因为 www CNAME 不是必需的
			fmt.Printf("警告: 创建 www CNAME 记录失败: %v\n", err)
		}
	}

	return nil
}

// ListRedirectRules 列出所有重定向规则
func (s *RedirectService) ListRedirectRules(page, pageSize int) ([]models.RedirectRule, int64, error) {
	var rules []models.RedirectRule
	var total int64

	offset := (page - 1) * pageSize

	if err := s.db.Model(&models.RedirectRule{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := s.db.Preload("Targets").Offset(offset).Limit(pageSize).Find(&rules).Error; err != nil {
		return nil, 0, err
	}

	return rules, total, nil
}

// GetCloudFrontStatus 获取 CloudFront 分发状态
func (s *RedirectService) GetCloudFrontStatus(cloudFrontID string) (string, error) {
	if cloudFrontID == "" {
		return "", nil
	}

	dist, err := s.cloudFrontSvc.GetDistribution(cloudFrontID)
	if err != nil {
		return "", err
	}

	if dist == nil || dist.Status == nil {
		return "", nil
	}

	return *dist.Status, nil
}

// GetCloudFrontEnabled 获取CloudFront分发启用状态
func (s *RedirectService) GetCloudFrontEnabled(cloudFrontID string) (bool, error) {
	if cloudFrontID == "" {
		return false, nil
	}

	dist, err := s.cloudFrontSvc.GetDistribution(cloudFrontID)
	if err != nil {
		return false, err
	}

	if dist == nil || dist.DistributionConfig == nil {
		return false, fmt.Errorf("无法获取分发配置")
	}

	if dist.DistributionConfig.Enabled == nil {
		return false, nil
	}

	return *dist.DistributionConfig.Enabled, nil
}

// AddTarget 添加重定向目标并重新部署
func (s *RedirectService) AddTarget(ruleID uint, targetURL string) error {
	target := &models.RedirectTarget{
		RuleID:    ruleID,
		TargetURL: targetURL,
		Weight:    1,
		IsActive:  true,
	}

	if err := s.db.Create(target).Error; err != nil {
		return err
	}

	// 重新加载规则
	rule, err := s.GetRedirectRule(ruleID)
	if err != nil {
		return err
	}

	// 如果已有CloudFront分发，重新部署HTML文件
	if rule.CloudFrontID != "" {
		if err := s.redeployHTML(rule); err != nil {
			fmt.Printf("警告: 重新部署HTML文件失败: %v\n", err)
		}
	}

	return nil
}

// redeployHTML 重新部署HTML文件（不重新创建CloudFront分发）
func (s *RedirectService) redeployHTML(rule *models.RedirectRule) error {
	// 收集活跃的目标URL
	var activeTargets []string
	for _, target := range rule.Targets {
		if target.IsActive {
			activeTargets = append(activeTargets, target.TargetURL)
		}
	}

	if len(activeTargets) == 0 {
		return fmt.Errorf("没有可用的重定向目标")
	}

	// 生成HTML文件
	htmlContent, err := s.generateRedirectHTML(activeTargets)
	if err != nil {
		return fmt.Errorf("生成HTML文件失败: %w", err)
	}

	// S3目录路径
	s3Path := fmt.Sprintf("redirects/%s/", rule.SourceDomain)
	s3Key := s3Path + "index.html"

	// 上传HTML文件到S3
	return s.s3Svc.UploadHTML(s.config.S3BucketName, s3Key, htmlContent)
}

// RemoveTarget 删除重定向目标并重新部署
func (s *RedirectService) RemoveTarget(targetID uint) error {
	// 先获取目标信息，以便找到对应的规则
	var target models.RedirectTarget
	if err := s.db.First(&target, targetID).Error; err != nil {
		return err
	}

	ruleID := target.RuleID

	// 删除目标
	if err := s.db.Delete(&models.RedirectTarget{}, targetID).Error; err != nil {
		return err
	}

	// 重新加载规则
	rule, err := s.GetRedirectRule(ruleID)
	if err != nil {
		return err
	}

	// 如果已有CloudFront分发，重新部署HTML文件
	if rule.CloudFrontID != "" {
		if err := s.redeployHTML(rule); err != nil {
			fmt.Printf("警告: 重新部署HTML文件失败: %v\n", err)
		}
	}

	return nil
}

// SelectTarget 选择目标 URL（轮询算法，基于浏览器缓存）
func (s *RedirectService) SelectTarget(ruleID uint, clientIP string) (string, error) {
	redirectRule, err := s.GetRedirectRule(ruleID)
	if err != nil {
		return "", err
	}

	// 过滤活跃的目标
	var activeTargets []models.RedirectTarget
	for _, target := range redirectRule.Targets {
		if target.IsActive {
			activeTargets = append(activeTargets, target)
		}
	}

	if len(activeTargets) == 0 {
		return "", fmt.Errorf("没有可用的重定向目标")
	}

	// 基于客户端 IP 和时间的哈希来选择目标（模拟轮询）
	// 实际生产环境应该在浏览器端使用 localStorage 或 cookie 来记录访问次数
	hash := s.hashClient(clientIP)
	index := hash % len(activeTargets)

	return activeTargets[index].TargetURL, nil
}

// hashClient 基于客户端信息生成哈希
func (s *RedirectService) hashClient(clientIP string) int {
	hash := 0
	for _, char := range clientIP {
		hash = hash*31 + int(char)
	}
	hash += int(time.Now().Unix() / 60) // 每分钟轮换
	if hash < 0 {
		hash = -hash
	}
	return hash
}

// HandleRedirect HTTP 重定向处理器
func (s *RedirectService) HandleRedirect(w http.ResponseWriter, r *http.Request, sourceDomain string) {
	// 查找重定向规则
	var rule models.RedirectRule
	if err := s.db.Where("source_domain = ?", sourceDomain).Preload("Targets").First(&rule).Error; err != nil {
		http.NotFound(w, r)
		return
	}

	// 选择目标 URL
	targetURL, err := s.SelectTarget(rule.ID, r.RemoteAddr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 设置缓存头，让浏览器缓存选择结果
	w.Header().Set("Cache-Control", "private, max-age=3600") // 缓存 1 小时
	w.Header().Set("X-Target-URL", targetURL)

	// 重定向
	http.Redirect(w, r, targetURL, http.StatusFound)
}

// UpdateRule 更新重定向规则
func (s *RedirectService) UpdateRule(id uint, sourceDomain string) error {
	return s.db.Model(&models.RedirectRule{}).Where("id = ?", id).Update("source_domain", sourceDomain).Error
}

// DeleteRule 删除重定向规则（同时删除CloudFront分发和S3目录）
func (s *RedirectService) DeleteRule(id uint) error {
	// 获取规则信息
	rule, err := s.GetRedirectRule(id)
	if err != nil {
		return err
	}

	// 删除CloudFront分发（如果存在）
	if rule.CloudFrontID != "" {
		// 先禁用分发
		enabled := false
		if err := s.cloudFrontSvc.UpdateDistribution(rule.CloudFrontID, nil, "", &enabled); err != nil {
			fmt.Printf("警告: 禁用CloudFront分发失败: %v\n", err)
		}

		// 等待一段时间确保禁用生效（CloudFront需要时间）
		// 注意：实际生产环境应该使用轮询检查状态
		time.Sleep(2 * time.Second)

		// 删除分发
		if err := s.cloudFrontSvc.DeleteDistribution(rule.CloudFrontID); err != nil {
			fmt.Printf("警告: 删除CloudFront分发失败: %v\n", err)
		}
	}

	// 删除S3目录中的文件
	if s.config.S3BucketName != "" {
		s3Path := fmt.Sprintf("redirects/%s/", rule.SourceDomain)
		if err := s.s3Svc.DeleteObjectsWithPrefix(s.config.S3BucketName, s3Path); err != nil {
			fmt.Printf("警告: 删除S3目录失败: %v\n", err)
		}
	}

	// 从数据库删除规则（软删除）
	return s.db.Delete(&models.RedirectRule{}, id).Error
}

// BindDomainToCloudFront 将域名绑定到 CloudFront
func (s *RedirectService) BindDomainToCloudFront(ruleID uint, distributionID string, domainName string) error {
	rule, err := s.GetRedirectRule(ruleID)
	if err != nil {
		return err
	}

	// 更新 CloudFront 分发的别名
	if err := s.cloudFrontSvc.UpdateDistributionAliases(distributionID, []string{domainName}); err != nil {
		return fmt.Errorf("更新 CloudFront 别名失败: %w", err)
	}

	// 更新规则中的 CloudFront ID
	rule.CloudFrontID = distributionID
	return s.db.Save(rule).Error
}

// CheckRedirectRuleStatus 检查重定向规则的状态
type RedirectRuleStatus struct {
	RuleExists               bool     `json:"rule_exists"`
	HTMLUploaded             bool     `json:"html_uploaded"`
	HTMLUploadError          string   `json:"html_upload_error,omitempty"`
	S3BucketPolicyConfigured bool     `json:"s3_bucket_policy_configured"`
	S3BucketPolicyError      string   `json:"s3_bucket_policy_error,omitempty"`
	CloudFrontExists         bool     `json:"cloudfront_exists"`
	CloudFrontError          string   `json:"cloudfront_error,omitempty"`
	CloudFrontEnabled        bool     `json:"cloudfront_enabled"`
	CloudFrontEnabledError   string   `json:"cloudfront_enabled_error,omitempty"`
	Route53DNSConfigured     bool     `json:"route53_dns_configured"`
	Route53DNSError          string   `json:"route53_dns_error,omitempty"`
	WWWCNAMEConfigured       bool     `json:"www_cname_configured"`
	WWWCNAMEError            string   `json:"www_cname_error,omitempty"`
	CertificateFound         bool     `json:"certificate_found"`
	CertificateARN           string   `json:"certificate_arn,omitempty"`
	RedirectURLAccessible    bool     `json:"redirect_url_accessible"`
	RedirectURLError         string   `json:"redirect_url_error,omitempty"`
	Issues                   []string `json:"issues"`
	CanFix                   bool     `json:"can_fix"`
}

// CheckRedirectRule 检查重定向规则的状态
func (s *RedirectService) CheckRedirectRule(ruleID uint) (*RedirectRuleStatus, error) {
	status := &RedirectRuleStatus{
		Issues: []string{},
	}

	// 获取规则
	rule, err := s.GetRedirectRule(ruleID)
	if err != nil {
		return nil, fmt.Errorf("获取重定向规则失败: %w", err)
	}
	status.RuleExists = true

	// 检查S3 HTML文件是否存在
	if s.config.S3BucketName != "" {
		// 检查 S3 bucket policy 是否已配置
		policyConfigured, err := s.s3Svc.CheckBucketPolicyForPublicAccess(s.config.S3BucketName)
		if err != nil {
			status.S3BucketPolicyError = fmt.Sprintf("检查bucket policy失败: %v", err)
			status.Issues = append(status.Issues, "检查S3 bucket policy失败")
		} else if !policyConfigured {
			status.S3BucketPolicyError = "未配置S3 bucket policy"
			status.Issues = append(status.Issues, "S3 bucket policy未配置，CloudFront可能无法访问S3对象")
		} else {
			status.S3BucketPolicyConfigured = true
		}

		s3Path := fmt.Sprintf("redirects/%s/", rule.SourceDomain)
		s3Key := s3Path + "index.html"
		exists, err := s.s3Svc.ObjectExists(s.config.S3BucketName, s3Key)
		if err != nil {
			status.HTMLUploadError = err.Error()
			status.Issues = append(status.Issues, fmt.Sprintf("检查S3文件失败: %v", err))
		} else if !exists {
			status.Issues = append(status.Issues, "S3 HTML文件不存在")
		} else {
			status.HTMLUploaded = true
		}
	} else {
		status.HTMLUploadError = "S3存储桶名称未配置"
		status.Issues = append(status.Issues, "S3存储桶名称未配置")
	}

	// 检查CloudFront分发是否存在
	if rule.CloudFrontID != "" {
		_, err := s.cloudFrontSvc.GetDistribution(rule.CloudFrontID)
		if err != nil {
			status.CloudFrontError = err.Error()
			status.Issues = append(status.Issues, fmt.Sprintf("CloudFront分发不存在或无法访问: %v", err))
		} else {
			status.CloudFrontExists = true

			// 检查 CloudFront 是否已启用
			enabled, err := s.GetCloudFrontEnabled(rule.CloudFrontID)
			if err != nil {
				status.CloudFrontEnabledError = fmt.Sprintf("检查CloudFront启用状态失败: %v", err)
				status.Issues = append(status.Issues, "检查CloudFront启用状态失败")
			} else if !enabled {
				status.CloudFrontEnabledError = "CloudFront分发已禁用"
				status.Issues = append(status.Issues, "CloudFront分发已禁用，需要启用后才能正常使用")
			} else {
				status.CloudFrontEnabled = true
			}

			// 检查 Route 53 DNS 记录是否指向正确的 CloudFront
			dnsStatus := s.CheckRoute53RecordStatus(rule.SourceDomain, rule.CloudFrontID)
			if dnsStatus == "configured" {
				status.Route53DNSConfigured = true
			} else if dnsStatus == "mismatched" {
				status.Route53DNSError = "DNS记录指向了错误的CloudFront分发"
				status.Issues = append(status.Issues, "Route 53 DNS记录指向了错误的CloudFront分发")
			} else if dnsStatus == "not_configured" {
				status.Route53DNSError = "未配置Route 53 DNS记录"
				status.Issues = append(status.Issues, "Route 53 DNS记录未配置")
			} else if dnsStatus == "error" {
				status.Route53DNSError = "检查Route 53 DNS记录失败"
				status.Issues = append(status.Issues, "检查Route 53 DNS记录失败")
			}

			// 检查 www CNAME 记录（仅对根域名检查，不包括 www 子域名）
			if !strings.HasPrefix(rule.SourceDomain, "www.") {
				// 查找域名对应的 Route 53 Hosted Zone ID
				var domain models.Domain
				if err := s.db.Where("domain_name = ?", rule.SourceDomain).First(&domain).Error; err == nil && domain.HostedZoneID != "" {
					exists, err := s.domainSvc.CheckWWWCNAMERecord(domain.HostedZoneID, rule.SourceDomain)
					if err != nil {
						status.WWWCNAMEError = fmt.Sprintf("检查www CNAME记录失败: %v", err)
						status.Issues = append(status.Issues, "检查www CNAME记录失败")
					} else if !exists {
						status.WWWCNAMEError = "未配置www CNAME记录"
						status.Issues = append(status.Issues, "www CNAME记录未配置")
					} else {
						status.WWWCNAMEConfigured = true
					}
				}
			}
		}
	} else {
		status.Issues = append(status.Issues, "未创建CloudFront分发")
	}

	// 检查证书
	certificateARN := s.findCertificateARN(rule.SourceDomain)
	if certificateARN != "" {
		status.CertificateFound = true
		status.CertificateARN = certificateARN
	} else {
		status.Issues = append(status.Issues, "未找到证书")
	}

	// 检查重定向 URL 是否可以访问（如果 CloudFront 已配置）
	if rule.CloudFrontID != "" && status.CloudFrontExists && status.CloudFrontEnabled {
		redirectURL := fmt.Sprintf("https://%s", rule.SourceDomain)
		client := &http.Client{
			Timeout: 10 * time.Second,
		}

		resp, err := client.Get(redirectURL)
		if err != nil {
			status.RedirectURLError = fmt.Sprintf("无法访问重定向URL: %v", err)
			status.Issues = append(status.Issues, fmt.Sprintf("重定向URL无法访问: %v", err))
		} else {
			defer resp.Body.Close()
			if resp.StatusCode == 200 || resp.StatusCode == 302 || resp.StatusCode == 301 {
				status.RedirectURLAccessible = true
			} else if resp.StatusCode == 403 {
				status.RedirectURLError = fmt.Sprintf("访问被拒绝 (HTTP %d)，可能是S3 bucket policy配置问题", resp.StatusCode)
				status.Issues = append(status.Issues, "重定向URL返回403错误，可能是S3 bucket policy配置问题")
			} else {
				status.RedirectURLError = fmt.Sprintf("返回错误状态码: HTTP %d", resp.StatusCode)
				status.Issues = append(status.Issues, fmt.Sprintf("重定向URL返回错误状态码: HTTP %d", resp.StatusCode))
			}
		}
	}

	// 判断是否可以修复
	status.CanFix = len(status.Issues) > 0 && s.config.S3BucketName != ""

	return status, nil
}

// FixRedirectRule 修复重定向规则
func (s *RedirectService) FixRedirectRule(ruleID uint) (*CreateRedirectRuleResult, error) {
	// 获取规则
	rule, err := s.GetRedirectRule(ruleID)
	if err != nil {
		return nil, fmt.Errorf("获取重定向规则失败: %w", err)
	}

	// 重新加载规则以获取所有目标
	if err := s.db.Preload("Targets").First(rule, rule.ID).Error; err != nil {
		return nil, fmt.Errorf("加载重定向规则失败: %w", err)
	}

	// 收集部署警告信息
	var deploymentWarnings []string

	// 查找证书
	certificateARN := s.findCertificateARN(rule.SourceDomain)

	// 如果已有CloudFront ID，先检查是否存在和是否已启用
	if rule.CloudFrontID != "" {
		dist, err := s.cloudFrontSvc.GetDistribution(rule.CloudFrontID)
		if err != nil {
			// CloudFront不存在，清除ID，重新创建
			rule.CloudFrontID = ""
			s.db.Save(rule)
		} else {
			// 检查是否已启用，如果未启用则启用它
			if dist != nil && dist.DistributionConfig != nil {
				enabled := dist.DistributionConfig.Enabled
				if enabled == nil || !*enabled {
					enabledValue := true
					if err := s.cloudFrontSvc.UpdateDistribution(rule.CloudFrontID, nil, "", &enabledValue); err != nil {
						return nil, fmt.Errorf("启用CloudFront分发失败: %w", err)
					}
				}
			}

			// 检查并更新 OriginPath
			// 计算期望的 originPath
			expectedOriginPath := fmt.Sprintf("/redirects/%s", rule.SourceDomain)

			// 获取当前的 OriginPath
			currentOriginPath, err := s.cloudFrontSvc.GetDistributionOriginPath(rule.CloudFrontID)
			if err != nil {
				warningMsg := fmt.Sprintf("获取 CloudFront OriginPath 失败: %v", err)
				deploymentWarnings = append(deploymentWarnings, warningMsg)
			} else if currentOriginPath != expectedOriginPath {
				// 如果路径不匹配，更新它
				if err := s.cloudFrontSvc.UpdateDistributionOriginPath(rule.CloudFrontID, expectedOriginPath); err != nil {
					warningMsg := fmt.Sprintf("更新 CloudFront OriginPath 失败: %v", err)
					deploymentWarnings = append(deploymentWarnings, warningMsg)
				}
			}
		}
	}

	// 确保 S3 bucket policy 允许公开访问（修复时自动配置）
	if s.config.S3BucketName != "" {
		if err := s.s3Svc.EnsureBucketPolicyForPublicAccess(s.config.S3BucketName); err != nil {
			warningMsg := fmt.Sprintf("配置 S3 bucket policy 失败: %v", err)
			deploymentWarnings = append(deploymentWarnings, warningMsg)
		}
	}

	// 自动部署到S3和CloudFront
	if certificateARN != "" {
		// 如果已有CloudFront ID，只重新上传HTML
		if rule.CloudFrontID != "" {
			// 确保 CloudFront 分发已启用
			enabled := true
			if err := s.cloudFrontSvc.UpdateDistribution(rule.CloudFrontID, nil, "", &enabled); err != nil {
				warningMsg := fmt.Sprintf("启用CloudFront分发失败: %v", err)
				deploymentWarnings = append(deploymentWarnings, warningMsg)
			}

			if err := s.redeployHTML(rule); err != nil {
				warningMsg := fmt.Sprintf("重新部署HTML文件失败: %v", err)
				deploymentWarnings = append(deploymentWarnings, warningMsg)
			}

			// 检查并创建 Route 53 DNS 记录（如果不存在）
			if err := s.createRoute53RecordForCloudFront(rule.SourceDomain, rule.CloudFrontID); err != nil {
				warningMsg := fmt.Sprintf("创建 Route 53 DNS 记录失败: %v", err)
				deploymentWarnings = append(deploymentWarnings, warningMsg)
			}
		} else {
			// 创建新的CloudFront分发
			if err := s.deployRedirectRule(rule, certificateARN); err != nil {
				warningMsg := fmt.Sprintf("部署重定向规则失败: %v", err)
				deploymentWarnings = append(deploymentWarnings, warningMsg)
			}
			// deployRedirectRule 内部已经会创建 DNS 记录，这里不需要重复创建
			// 确保新创建的分发已启用（deployRedirectRule 中已设置 Enabled: true，但这里再次确认）
			if rule.CloudFrontID != "" {
				enabled := true
				if err := s.cloudFrontSvc.UpdateDistribution(rule.CloudFrontID, nil, "", &enabled); err != nil {
					warningMsg := fmt.Sprintf("启用CloudFront分发失败: %v", err)
					deploymentWarnings = append(deploymentWarnings, warningMsg)
				}
			}
		}
	} else {
		// 没有证书，只上传HTML文件
		if err := s.uploadHTMLOnly(rule); err != nil {
			warningMsg := fmt.Sprintf("上传HTML文件失败: %v", err)
			deploymentWarnings = append(deploymentWarnings, warningMsg)
		} else {
			deploymentWarnings = append(deploymentWarnings, "未找到证书，已上传HTML文件到S3，但未创建CloudFront分发")
		}
	}

	return &CreateRedirectRuleResult{
		Rule:     rule,
		Warnings: deploymentWarnings,
	}, nil
}
