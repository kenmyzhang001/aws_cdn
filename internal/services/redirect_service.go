package services

import (
	"aws_cdn/internal/models"
	"aws_cdn/internal/services/aws"
	"fmt"
	"net/http"
	"time"

	"gorm.io/gorm"
)

type RedirectService struct {
	db           *gorm.DB
	cloudFrontSvc *aws.CloudFrontService
}

func NewRedirectService(db *gorm.DB, cloudFrontSvc *aws.CloudFrontService) *RedirectService {
	return &RedirectService{
		db:            db,
		cloudFrontSvc: cloudFrontSvc,
	}
}

// CreateRedirectRule 创建重定向规则
func (s *RedirectService) CreateRedirectRule(sourceDomain string, targetURLs []string) (*models.RedirectRule, error) {
	// 检查源域名是否已存在
	var existingRule models.RedirectRule
	if err := s.db.Where("source_domain = ?", sourceDomain).First(&existingRule).Error; err == nil {
		return nil, fmt.Errorf("源域名 %s 的重定向规则已存在", sourceDomain)
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

	// 创建 CloudFront 分发（需要先有证书）
	// 这里简化处理，实际应该先检查域名证书
	// distributionID, err := s.cloudFrontSvc.CreateDistribution(...)
	// if err == nil {
	// 	rule.CloudFrontID = distributionID
	// 	s.db.Save(rule)
	// }

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

// AddTarget 添加重定向目标
func (s *RedirectService) AddTarget(ruleID uint, targetURL string) error {
	// 验证规则是否存在
	_, err := s.GetRedirectRule(ruleID)
	if err != nil {
		return err
	}

	target := &models.RedirectTarget{
		RuleID:    ruleID,
		TargetURL: targetURL,
		Weight:    1,
		IsActive:  true,
	}

	return s.db.Create(target).Error
}

// RemoveTarget 删除重定向目标
func (s *RedirectService) RemoveTarget(targetID uint) error {
	return s.db.Delete(&models.RedirectTarget{}, targetID).Error
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

// DeleteRule 删除重定向规则
func (s *RedirectService) DeleteRule(id uint) error {
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

