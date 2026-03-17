package handlers

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type CFWorkpageSiteHandler struct {
	service *services.CFWorkpageSiteService
}

func NewCFWorkpageSiteHandler(service *services.CFWorkpageSiteService) *CFWorkpageSiteHandler {
	return &CFWorkpageSiteHandler{service: service}
}

// List 列表，支持 ?cf_account_id= & template_id= & page= & page_size=
func (h *CFWorkpageSiteHandler) List(c *gin.Context) {
	log := logger.GetLogger()
	var cfAccountID, templateID *uint
	if idStr := c.Query("cf_account_id"); idStr != "" {
		id, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 cf_account_id"})
			return
		}
		u := uint(id)
		cfAccountID = &u
	}
	if idStr := c.Query("template_id"); idStr != "" {
		id, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 template_id"})
			return
		}
		u := uint(id)
		templateID = &u
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	list, total, err := h.service.List(cfAccountID, templateID, page, pageSize)
	if err != nil {
		log.WithError(err).Error("获取站点列表失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":       list,
		"pagination": gin.H{"total": total, "page": page, "page_size": pageSize},
	})
}

// Get 获取单条
func (h *CFWorkpageSiteHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}
	site, err := h.service.Get(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, site)
}

// Create 创建
func (h *CFWorkpageSiteHandler) Create(c *gin.Context) {
	log := logger.GetLogger()
	var req struct {
		CFAccountID uint   `json:"cf_account_id" binding:"required"`
		TemplateID  uint   `json:"template_id" binding:"required"`
		ZoneID      string `json:"zone_id" binding:"required"`
		MainDomain  string `json:"main_domain" binding:"required"`
		Subdomain   string `json:"subdomain"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	site, err := h.service.Create(req.CFAccountID, req.TemplateID, req.ZoneID, req.MainDomain, req.Subdomain)
	if err != nil {
		log.WithError(err).Error("创建站点失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, site)
}

// Update 更新
func (h *CFWorkpageSiteHandler) Update(c *gin.Context) {
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}
	var req struct {
		Subdomain *string `json:"subdomain"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	site, err := h.service.Update(uint(id), req.Subdomain)
	if err != nil {
		log.WithError(err).Error("更新站点失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, site)
}

// Delete 删除
func (h *CFWorkpageSiteHandler) Delete(c *gin.Context) {
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}
	if err := h.service.Delete(uint(id)); err != nil {
		log.WithError(err).Error("删除站点失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已删除"})
}
