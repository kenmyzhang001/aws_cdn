package handlers

import (
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
	var req struct {
		SourceDomain  string   `json:"source_domain" binding:"required"`
		TargetURLs    []string `json:"target_urls" binding:"required,min=1"`
		CertificateARN string  `json:"certificate_arn"` // 可选，用于创建CloudFront分发
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.service.CreateRedirectRule(req.SourceDomain, req.TargetURLs, req.CertificateARN)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 返回规则和警告信息
	response := gin.H{
		"rule":     result.Rule,
		"warnings": result.Warnings,
	}
	c.JSON(http.StatusOK, response)
}

// GetRedirectRule 获取重定向规则
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

	c.JSON(http.StatusOK, rule)
}

// ListRedirectRules 列出所有重定向规则
func (h *RedirectHandler) ListRedirectRules(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	rules, total, err := h.service.ListRedirectRules(page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 为每个规则添加 CloudFront 状态、域名状态、证书状态和目标URL状态
	rulesWithStatus := make([]gin.H, len(rules))
	for i, rule := range rules {
		ruleMap := gin.H{
			"id":             rule.ID,
			"source_domain":  rule.SourceDomain,
			"cloudfront_id":  rule.CloudFrontID,
			"targets":        rule.Targets,
			"created_at":     rule.CreatedAt,
			"updated_at":     rule.UpdatedAt,
		}
		
		// 查询 CloudFront 状态
		if rule.CloudFrontID != "" {
			status, err := h.service.GetCloudFrontStatus(rule.CloudFrontID)
			if err == nil {
				ruleMap["cloudfront_status"] = status
			}
		}
		
		// 查询域名状态和证书状态
		domainStatus, certStatus := h.service.GetDomainInfoByDomainName(rule.SourceDomain)
		if domainStatus != "" {
			ruleMap["domain_status"] = domainStatus
		}
		if certStatus != "" {
			ruleMap["certificate_status"] = certStatus
		}
		
		// 查询 Route 53 DNS 记录状态（验证是否指向正确的 CloudFront）
		if rule.CloudFrontID != "" {
			dnsStatus := h.service.CheckRoute53RecordStatus(rule.SourceDomain, rule.CloudFrontID)
			ruleMap["route53_dns_status"] = dnsStatus
			
			// 检查 www CNAME 记录状态（仅对根域名检查，不包括 www 子域名）
			if !strings.HasPrefix(rule.SourceDomain, "www.") {
				wwwCNAMEStatus := h.service.CheckWWWCNAMERecordStatus(rule.SourceDomain)
				ruleMap["www_cname_status"] = wwwCNAMEStatus
			}
		}
		
		// 检查目标URL状态
		targetsWithStatus := make([]gin.H, len(rule.Targets))
		for j, target := range rule.Targets {
			targetMap := gin.H{
				"id":         target.ID,
				"target_url": target.TargetURL,
				"weight":     target.Weight,
				"is_active":  target.IsActive,
			}
			// 检查URL状态
			urlStatus := h.service.CheckURLStatus(target.TargetURL)
			targetMap["url_status"] = urlStatus
			targetsWithStatus[j] = targetMap
		}
		ruleMap["targets"] = targetsWithStatus
		
		rulesWithStatus[i] = ruleMap
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
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的规则 ID"})
		return
	}

	if err := h.service.DeleteRule(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

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
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的规则 ID"})
		return
	}

	result, err := h.service.FixRedirectRule(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 返回规则和警告信息
	response := gin.H{
		"rule":     result.Rule,
		"warnings": result.Warnings,
	}
	c.JSON(http.StatusOK, response)
}


