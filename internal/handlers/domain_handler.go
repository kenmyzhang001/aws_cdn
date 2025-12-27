package handlers

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/models"
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
	log := logger.GetLogger()
	var req struct {
		DomainName  string `json:"domain_name" binding:"required"`
		Registrar   string `json:"registrar" binding:"required"`
		DNSProvider string `json:"dns_provider"` // aws 或 cloudflare，默认为 aws
		GroupID     *uint  `json:"group_id"`     // 分组ID，可选
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"domain_name":  req.DomainName,
			"registrar":    req.Registrar,
			"dns_provider": req.DNSProvider,
			"group_id":     req.GroupID,
		}).Error("转入域名请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 默认使用AWS
	dnsProvider := models.DNSProviderAWS
	if req.DNSProvider == "cloudflare" {
		dnsProvider = models.DNSProviderCloudflare
	} else if req.DNSProvider != "" && req.DNSProvider != "aws" {
		log.WithFields(map[string]interface{}{
			"domain_name":  req.DomainName,
			"dns_provider": req.DNSProvider,
		}).Error("转入域名失败：无效的DNS提供商")
		c.JSON(http.StatusBadRequest, gin.H{"error": "dns_provider 必须是 'aws' 或 'cloudflare'"})
		return
	}

	domain, err := h.service.TransferDomain(req.DomainName, req.Registrar, dnsProvider, req.GroupID)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"domain_name":  req.DomainName,
			"registrar":    req.Registrar,
			"dns_provider": dnsProvider,
			"group_id":     req.GroupID,
		}).Error("转入域名操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"domain_id":   domain.ID,
		"domain_name": domain.DomainName,
	}).Info("转入域名操作成功")
	c.JSON(http.StatusOK, domain)
}

// GetDomain 获取域名信息
func (h *DomainHandler) GetDomain(c *gin.Context) {
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		log.WithError(err).WithField("id_param", c.Param("id")).Error("获取域名信息失败：无效的域名ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的域名 ID"})
		return
	}

	domain, err := h.service.GetDomain(uint(id))
	if err != nil {
		log.WithError(err).WithField("domain_id", id).Error("获取域名信息失败：域名不存在或查询失败")
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, domain)
}

// ListDomains 列出所有域名，支持按分组筛选
func (h *DomainHandler) ListDomains(c *gin.Context) {
	log := logger.GetLogger()
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	var groupID *uint
	if groupIDStr := c.Query("group_id"); groupIDStr != "" {
		if id, err := strconv.ParseUint(groupIDStr, 10, 32); err == nil {
			gid := uint(id)
			groupID = &gid
		}
	}

	domains, total, err := h.service.ListDomains(page, pageSize, groupID)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"page":      page,
			"page_size": pageSize,
			"group_id":  groupID,
		}).Error("列出域名失败")
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
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		log.WithError(err).WithField("id_param", c.Param("id")).Error("生成证书失败：无效的域名ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的域名 ID"})
		return
	}

	log.WithField("domain_id", id).Info("开始生成证书")
	if err := h.service.GenerateCertificate(uint(id)); err != nil {
		// 检查是否是证书已存在的情况（状态为ISSUED等）
		errMsg := err.Error()
		if len(errMsg) >= 12 && errMsg[:12] == "证书已存在" {
			log.WithField("domain_id", id).Info("证书已存在")
			c.JSON(http.StatusOK, gin.H{"message": errMsg})
			return
		}
		log.WithError(err).WithField("domain_id", id).Error("生成证书操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
		return
	}

	log.WithField("domain_id", id).Info("证书生成请求已提交")
	c.JSON(http.StatusOK, gin.H{"message": "证书生成请求已提交，验证记录已添加到 DNS 提供商"})
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
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		log.WithError(err).WithField("id_param", c.Param("id")).Error("删除域名失败：无效的域名ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的域名 ID"})
		return
	}

	log.WithField("domain_id", id).Info("开始删除域名")
	if err := h.service.DeleteDomain(uint(id)); err != nil {
		log.WithError(err).WithField("domain_id", id).Error("删除域名操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithField("domain_id", id).Info("域名删除成功")
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
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		log.WithError(err).WithField("id_param", c.Param("id")).Error("修复证书失败：无效的域名ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的域名 ID"})
		return
	}

	log.WithField("domain_id", id).Info("开始修复证书")
	if err := h.service.FixCertificate(uint(id)); err != nil {
		log.WithError(err).WithField("domain_id", id).Error("修复证书操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithField("domain_id", id).Info("证书修复请求已提交")
	c.JSON(http.StatusOK, gin.H{"message": "证书修复请求已提交"})
}
