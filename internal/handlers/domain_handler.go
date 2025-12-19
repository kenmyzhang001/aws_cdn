package handlers

import (
	"aws_cdn/internal/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type DomainHandler struct {
	service *services.DomainService
}

func NewDomainHandler(service *services.DomainService) *DomainHandler {
	return &DomainHandler{service: service}
}

// TransferDomain 转入域名
func (h *DomainHandler) TransferDomain(c *gin.Context) {
	var req struct {
		DomainName string `json:"domain_name" binding:"required"`
		Registrar  string `json:"registrar" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	domain, err := h.service.TransferDomain(req.DomainName, req.Registrar)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, domain)
}

// GetDomain 获取域名信息
func (h *DomainHandler) GetDomain(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的域名 ID"})
		return
	}

	domain, err := h.service.GetDomain(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, domain)
}

// ListDomains 列出所有域名
func (h *DomainHandler) ListDomains(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	domains, total, err := h.service.ListDomains(page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  domains,
		"total": total,
		"page":  page,
		"size":  pageSize,
	})
}

// GetNServers 获取 NS 服务器配置
func (h *DomainHandler) GetNServers(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的域名 ID"})
		return
	}

	nsServers, err := h.service.GetNServers(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"n_servers": nsServers})
}

// GenerateCertificate 生成证书
func (h *DomainHandler) GenerateCertificate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的域名 ID"})
		return
	}

	if err := h.service.GenerateCertificate(uint(id)); err != nil {
		// 检查是否是证书已存在的情况（状态为ISSUED等）
		errMsg := err.Error()
		if len(errMsg) >= 12 && errMsg[:12] == "证书已存在" {
			c.JSON(http.StatusOK, gin.H{"message": errMsg})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "证书生成请求已提交，验证记录已添加到 Route 53"})
}

// GetCertificateStatus 获取证书状态
func (h *DomainHandler) GetCertificateStatus(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的域名 ID"})
		return
	}

	status, err := h.service.GetCertificateStatus(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"certificate_status": status})
}

// GetDomainStatus 获取域名状态
func (h *DomainHandler) GetDomainStatus(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的域名 ID"})
		return
	}

	status, err := h.service.GetDomainStatus(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": status})
}

// DeleteDomain 删除域名
func (h *DomainHandler) DeleteDomain(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的域名 ID"})
		return
	}

	if err := h.service.DeleteDomain(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "域名删除成功"})
}

// CheckCertificate 检查证书配置和CNAME记录
func (h *DomainHandler) CheckCertificate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的域名 ID"})
		return
	}

	result, err := h.service.CheckCertificate(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// FixCertificate 修复证书配置和CNAME记录
func (h *DomainHandler) FixCertificate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的域名 ID"})
		return
	}

	if err := h.service.FixCertificate(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "证书修复请求已提交"})
}
