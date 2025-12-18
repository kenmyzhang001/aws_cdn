package handlers

import (
	"aws_cdn/internal/services"
	"net/http"
	"strconv"

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

	rule, err := h.service.CreateRedirectRule(req.SourceDomain, req.TargetURLs, req.CertificateARN)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rule)
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

	c.JSON(http.StatusOK, gin.H{
		"data":  rules,
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


