package handlers

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/services"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type RedirectHandler struct {
	service *services.RedirectService
}

func NewRedirectHandler(service *services.RedirectService) *RedirectHandler {
	return &RedirectHandler{service: service}
}

// CreateRedirectRule 创建重定向规则
func (h *RedirectHandler) CreateRedirectRule(c *gin.Context) {
	log := logger.GetLogger()
	var req struct {
		SourceDomain   string   `json:"source_domain" binding:"required"`
		TargetURLs     []string `json:"target_urls" binding:"required,min=1"`
		CertificateARN string   `json:"certificate_arn"` // 可选，用于创建CloudFront分发
		DNSProvider    string   `json:"dns_provider"`    // aws 或 cloudflare，默认为 aws
		GroupID        *uint    `json:"group_id"`        // 分组ID，可选
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"source_domain":   req.SourceDomain,
			"target_urls":     req.TargetURLs,
			"certificate_arn": req.CertificateARN,
			"dns_provider":    req.DNSProvider,
			"group_id":        req.GroupID,
		}).Error("创建重定向规则失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.service.CreateRedirectRule(req.SourceDomain, req.TargetURLs, req.CertificateARN, req.DNSProvider, req.GroupID)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"source_domain":   req.SourceDomain,
			"target_urls":     req.TargetURLs,
			"certificate_arn": req.CertificateARN,
			"dns_provider":    req.DNSProvider,
			"group_id":        req.GroupID,
		}).Error("创建重定向规则操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"rule_id":       result.Rule.ID,
		"source_domain": req.SourceDomain,
	}).Info("重定向规则创建成功")

	// 返回规则和警告信息
	response := gin.H{
		"rule":     result.Rule,
		"warnings": result.Warnings,
	}
	c.JSON(http.StatusOK, response)
}

// GetRedirectRule 获取重定向规则（包含状态检测）
func (h *RedirectHandler) GetRedirectRule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的规则 ID"})
		return
	}

	rule, err := h.service.GetRedirectRule(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// 构建响应并查询状态
	ruleMap := gin.H{
		"id":            rule.ID,
		"source_domain": rule.SourceDomain,
		"cloudfront_id": rule.CloudFrontID,
		"targets":       rule.Targets,
		"created_at":    rule.CreatedAt,
		"updated_at":    rule.UpdatedAt,
	}

	// 查询 CloudFront 状态和启用状态
	if rule.CloudFrontID != "" {
		status, err := h.service.GetCloudFrontStatus(rule.CloudFrontID)
		if err == nil {
			ruleMap["cloudfront_status"] = status
		}

		enabled, err := h.service.GetCloudFrontEnabled(rule.CloudFrontID)
		if err == nil {
			ruleMap["cloudfront_enabled"] = enabled
		}
	}

	// 获取 CloudFront OriginPath 信息
	currentPath, expectedPath, err := h.service.GetCloudFrontOriginPathInfo(rule)
	if err == nil {
		ruleMap["cloudfront_origin_path_current"] = currentPath
		ruleMap["cloudfront_origin_path_expected"] = expectedPath
	}

	// 查询域名状态和证书状态
	domainStatus, certStatus := h.service.GetDomainInfoByDomainName(rule.SourceDomain)
	if domainStatus != "" {
		ruleMap["domain_status"] = domainStatus
	}
	if certStatus != "" {
		ruleMap["certificate_status"] = certStatus
	}

	// 检查 S3 bucket policy 状态
	s3BucketPolicyStatus := h.service.CheckS3BucketPolicyStatus()
	if s3BucketPolicyStatus != "" {
		ruleMap["s3_bucket_policy_status"] = s3BucketPolicyStatus
	}

	// 查询 Route 53 DNS 记录状态
	if rule.CloudFrontID != "" {
		dnsStatus := h.service.CheckRoute53RecordStatus(rule.SourceDomain, rule.CloudFrontID)
		ruleMap["route53_dns_status"] = dnsStatus

		// 检查 www CNAME 记录状态（仅对根域名检查）
		if !strings.HasPrefix(rule.SourceDomain, "www.") {
			wwwCNAMEStatus := h.service.CheckWWWCNAMERecordStatus(rule.SourceDomain)
			ruleMap["www_cname_status"] = wwwCNAMEStatus
		}
	}

	// 检查目标URL状态
	targetsWithStatus := make([]gin.H, len(rule.Targets))
	for j, target := range rule.Targets {
		targetsWithStatus[j] = gin.H{
			"id":         target.ID,
			"target_url": target.TargetURL,
			"weight":     target.Weight,
			"is_active":  target.IsActive,
		}
		// 检查URL状态
		urlStatus := h.service.CheckURLStatus(target.TargetURL)
		targetsWithStatus[j]["url_status"] = urlStatus
	}
	ruleMap["targets"] = targetsWithStatus

	c.JSON(http.StatusOK, ruleMap)
}

// ListRedirectRules 列出所有重定向规则，支持按分组筛选
// 只返回基本域名列表信息，不进行状态检测以提升性能
func (h *RedirectHandler) ListRedirectRules(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	var groupID *uint
	if groupIDStr := c.Query("group_id"); groupIDStr != "" {
		if id, err := strconv.ParseUint(groupIDStr, 10, 32); err == nil {
			gid := uint(id)
			groupID = &gid
		}
	}

	var search *string
	if searchStr := c.Query("search"); searchStr != "" {
		search = &searchStr
	}

	rules, total, err := h.service.ListRedirectRules(page, pageSize, groupID, search)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 返回基本信息，包含 CloudFront 状态
	rulesWithStatus := make([]gin.H, len(rules))
	for i, rule := range rules {
		// 构建目标列表（不查询URL状态）
		targets := make([]gin.H, len(rule.Targets))
		for j, target := range rule.Targets {
			targets[j] = gin.H{
				"id":         target.ID,
				"target_url": target.TargetURL,
				"weight":     target.Weight,
				"is_active":  target.IsActive,
			}
		}

		rulesWithStatus[i] = gin.H{
			"id":            rule.ID,
			"source_domain": rule.SourceDomain,
			"cloudfront_id": rule.CloudFrontID,
			"status":        rule.Status,
			"targets":       targets,
			"created_at":    rule.CreatedAt,
			"updated_at":    rule.UpdatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  rulesWithStatus,
		"total": total,
		"page":  page,
		"size":  pageSize,
	})
}

// AddTarget 添加重定向目标
func (h *RedirectHandler) AddTarget(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的规则 ID"})
		return
	}

	var req struct {
		TargetURL string `json:"target_url" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.AddTarget(uint(id), req.TargetURL); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "目标添加成功"})
}

// RemoveTarget 删除重定向目标
func (h *RedirectHandler) RemoveTarget(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的目标 ID"})
		return
	}

	if err := h.service.RemoveTarget(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "目标删除成功"})
}

// BindDomainToCloudFront 绑定域名到 CloudFront
func (h *RedirectHandler) BindDomainToCloudFront(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的规则 ID"})
		return
	}

	var req struct {
		DistributionID string `json:"distribution_id" binding:"required"`
		DomainName     string `json:"domain_name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.BindDomainToCloudFront(uint(id), req.DistributionID, req.DomainName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "域名绑定成功"})
}

// DeleteRule 删除重定向规则
func (h *RedirectHandler) DeleteRule(c *gin.Context) {
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		log.WithError(err).WithField("id_param", c.Param("id")).Error("删除重定向规则失败：无效的规则ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的规则 ID"})
		return
	}

	log.WithField("rule_id", id).Info("开始删除重定向规则")
	if err := h.service.DeleteRule(uint(id)); err != nil {
		log.WithError(err).WithField("rule_id", id).Error("删除重定向规则操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithField("rule_id", id).Info("重定向规则删除成功")
	c.JSON(http.StatusOK, gin.H{"message": "规则删除成功"})
}

// CheckRedirectRule 检查重定向规则状态
func (h *RedirectHandler) CheckRedirectRule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的规则 ID"})
		return
	}

	status, err := h.service.CheckRedirectRule(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, status)
}

// FixRedirectRule 修复重定向规则
func (h *RedirectHandler) FixRedirectRule(c *gin.Context) {
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		log.WithError(err).WithField("id_param", c.Param("id")).Error("修复重定向规则失败：无效的规则ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的规则 ID"})
		return
	}

	log.WithField("rule_id", id).Info("开始修复重定向规则")
	result, err := h.service.FixRedirectRule(uint(id))
	if err != nil {
		log.WithError(err).WithField("rule_id", id).Error("修复重定向规则操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithField("rule_id", id).Info("重定向规则修复成功")

	// 返回规则和警告信息
	response := gin.H{
		"rule":     result.Rule,
		"warnings": result.Warnings,
	}
	c.JSON(http.StatusOK, response)
}

// UpdateRedirectRuleNote 更新重定向规则备注
func (h *RedirectHandler) UpdateRedirectRuleNote(c *gin.Context) {
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		log.WithError(err).WithField("id_param", c.Param("id")).Error("更新重定向规则备注失败：无效的规则ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的规则 ID"})
		return
	}

	var req struct {
		Note string `json:"note"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("更新重定向规则备注失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.UpdateRedirectRuleNote(uint(id), req.Note); err != nil {
		log.WithError(err).WithField("rule_id", id).Error("更新重定向规则备注操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithField("rule_id", id).Info("重定向规则备注更新成功")
	c.JSON(http.StatusOK, gin.H{"message": "备注更新成功"})
}
