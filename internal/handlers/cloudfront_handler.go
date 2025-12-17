package handlers

import (
	"aws_cdn/internal/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

// CloudFrontHandler CloudFront 分发管理接口
type CloudFrontHandler struct {
	service *services.CloudFrontService
}

func NewCloudFrontHandler(service *services.CloudFrontService) *CloudFrontHandler {
	return &CloudFrontHandler{service: service}
}

// ListDistributions 列出 CloudFront 分发
func (h *CloudFrontHandler) ListDistributions(c *gin.Context) {
	dists, err := h.service.ListDistributions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": dists})
}

// GetDistribution 获取分发详情
func (h *CloudFrontHandler) GetDistribution(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少分发 ID"})
		return
	}

	detail, err := h.service.GetDistribution(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, detail)
}

// CreateDistribution 创建分发并绑定域名和证书
func (h *CloudFrontHandler) CreateDistribution(c *gin.Context) {
	var req struct {
		DomainName     string `json:"domain_name" binding:"required"`
		CertificateARN string `json:"certificate_arn" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	id, err := h.service.CreateDistribution(req.DomainName, req.CertificateARN)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": id})
}

// UpdateDistribution 更新分发（别名、证书、启用状态）
func (h *CloudFrontHandler) UpdateDistribution(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少分发 ID"})
		return
	}

	var req struct {
		Aliases        []string `json:"aliases"`
		CertificateARN string   `json:"certificate_arn"`
		Enabled        *bool    `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.UpdateDistribution(id, req.Aliases, req.CertificateARN, req.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

// DeleteDistribution 删除分发
func (h *CloudFrontHandler) DeleteDistribution(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少分发 ID"})
		return
	}

	if err := h.service.DeleteDistribution(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}


