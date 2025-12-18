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
	config        *config.AWSConfig
}

func NewRedirectService(db *gorm.DB, cloudFrontSvc *aws.CloudFrontService, s3Svc *aws.S3Service, cfg *config.AWSConfig) *RedirectService {
	return &RedirectService{
		db:            db,
		cloudFrontSvc: cloudFrontSvc,
		s3Svc:         s3Svc,
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
	// 先上传HTML文件
	if err := s.uploadHTMLOnly(rule); err != nil {
		return err
	}

	// 获取S3域名
	s3Origin := s.s3Svc.GetBucketDomain(s.config.S3BucketName)

	// S3目录路径
	s3Path := fmt.Sprintf("redirects/%s/", rule.SourceDomain)

	// 创建CloudFront分发，指向S3目录
	distributionID, err := s.cloudFrontSvc.CreateDistributionWithPath(
		rule.SourceDomain,
		certificateARN,
		s3Origin,
		"/"+s3Path, // OriginPath需要以/开头
	)
	if err != nil {
		return fmt.Errorf("创建CloudFront分发失败: %w", err)
	}

	// 更新规则中的CloudFront ID
	rule.CloudFrontID = distributionID
	if err := s.db.Save(rule).Error; err != nil {
		return fmt.Errorf("更新规则失败: %w", err)
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

// CreateRedirectRule 创建重定向规则并自动部署
func (s *RedirectService) CreateRedirectRule(sourceDomain string, targetURLs []string, certificateARN string) (*models.RedirectRule, error) {
	// 检查源域名是否已存在（排除软删除的记录）
	var existingRule models.RedirectRule
	if err := s.db.Where("source_domain = ?", sourceDomain).First(&existingRule).Error; err == nil {
		return nil, fmt.Errorf("源域名 %s 的重定向规则已存在", sourceDomain)
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

	// 自动部署到S3和CloudFront
	// 如果找到了证书ARN，创建CloudFront分发
	if certificateARN != "" {
		if err := s.deployRedirectRule(rule, certificateARN); err != nil {
			// 如果部署失败，记录错误但不阻止规则创建
			// 可以后续通过更新接口重新部署
			fmt.Printf("警告: 部署重定向规则失败: %v\n", err)
		}
	} else {
		// 如果没有找到证书，先上传HTML文件到S3
		// 系统会自动尝试从domain表中查找证书，如果找到会自动创建CloudFront
		// 如果确实没有证书，可以后续通过BindDomainToCloudFront接口手动绑定CloudFront
		if err := s.uploadHTMLOnly(rule); err != nil {
			fmt.Printf("警告: 上传HTML文件失败: %v\n", err)
		}
	}

	return rule, nil
}

// GetRedirectRule 获取重定向规则
func (s *RedirectService) GetRedirectRule(id uint) (*models.RedirectRule, error) {
	var rule models.RedirectRule
	if err := s.db.Preload("Targets").First(&rule, id).Error; err != nil {
		return nil, fmt.Errorf("重定向规则不存在: %w", err)
	}
	return &rule, nil
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
