package handlers

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/services"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type DomainRedirectHandler struct {
	service *services.DomainRedirectService
}

func NewDomainRedirectHandler(service *services.DomainRedirectService) *DomainRedirectHandler {
	return &DomainRedirectHandler{service: service}
}

// List 列表，支持 ?cf_account_id= & domain= & page= & page_size=
func (h *DomainRedirectHandler) List(c *gin.Context) {
	log := logger.GetLogger()
	var cfAccountID *uint
	if idStr := c.Query("cf_account_id"); idStr != "" {
		id, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 cf_account_id"})
			return
		}
		u := uint(id)
		cfAccountID = &u
	}
	domain := strings.TrimSpace(c.Query("domain"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	list, total, err := h.service.List(cfAccountID, domain, page, pageSize)
	if err != nil {
		log.WithError(err).Error("获取域名重定向列表失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":       list,
		"pagination": gin.H{"total": total, "page": page, "page_size": pageSize},
	})
}

// CheckDomain 检查主域名（源）是否已被占用，用于创建前预检
func (h *DomainRedirectHandler) CheckDomain(c *gin.Context) {
	domain := strings.TrimSpace(c.Query("domain"))
	if domain == "" {
		c.JSON(http.StatusOK, gin.H{"available": true, "used_by": "", "ref_id": 0, "ref_name": ""})
		return
	}
	available, usedBy, refID, refName := h.service.CheckSourceDomainAvailable(domain)
	c.JSON(http.StatusOK, gin.H{
		"available": available,
		"used_by":   usedBy,
		"ref_id":    refID,
		"ref_name":  refName,
	})
}

// Get 获取单条
func (h *DomainRedirectHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}
	dr, err := h.service.Get(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dr)
}

// Create 创建
func (h *DomainRedirectHandler) Create(c *gin.Context) {
	log := logger.GetLogger()
	var req struct {
		CFAccountID  uint   `json:"cf_account_id" binding:"required"`
		ZoneID       string `json:"zone_id" binding:"required"`
		SourceDomain string `json:"source_domain" binding:"required"`
		TargetDomain string `json:"target_domain" binding:"required"`
		PreservePath *bool  `json:"preserve_path"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	preservePath := true
	if req.PreservePath != nil {
		preservePath = *req.PreservePath
	}
	dr, err := h.service.Create(req.CFAccountID, req.ZoneID, req.SourceDomain, req.TargetDomain, preservePath)
	if err != nil {
		log.WithError(err).Error("创建域名重定向失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dr)
}

// Update 更新
func (h *DomainRedirectHandler) Update(c *gin.Context) {
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}
	var req struct {
		TargetDomain string `json:"target_domain"`
		PreservePath *bool  `json:"preserve_path"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	dr, err := h.service.Update(uint(id), req.TargetDomain, req.PreservePath)
	if err != nil {
		log.WithError(err).Error("更新域名重定向失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dr)
}

// EnsureDNS 为源主机名创建/补建 DNS 记录（解决「无法解析主机」）
func (h *DomainRedirectHandler) EnsureDNS(c *gin.Context) {
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}
	if err := h.service.EnsureDNS(uint(id)); err != nil {
		log.WithError(err).WithField("id", id).Error("创建 DNS 记录失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "DNS 记录已创建"})
}

// Delete 删除
func (h *DomainRedirectHandler) Delete(c *gin.Context) {
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}
	if err := h.service.Delete(uint(id)); err != nil {
		log.WithError(err).Error("删除域名重定向失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已删除"})
}
