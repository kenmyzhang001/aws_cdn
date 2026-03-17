package handlers

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/services"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type CFWorkpageTemplateHandler struct {
	service *services.CFWorkpageTemplateService
}

func NewCFWorkpageTemplateHandler(service *services.CFWorkpageTemplateService) *CFWorkpageTemplateHandler {
	return &CFWorkpageTemplateHandler{service: service}
}

// List 列表，支持 ?keyword= & page= & page_size=
func (h *CFWorkpageTemplateHandler) List(c *gin.Context) {
	log := logger.GetLogger()
	keyword := strings.TrimSpace(c.Query("keyword"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	list, total, err := h.service.List(keyword, page, pageSize)
	if err != nil {
		log.WithError(err).Error("获取模版列表失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":       list,
		"pagination": gin.H{"total": total, "page": page, "page_size": pageSize},
	})
}

// Get 获取单条
func (h *CFWorkpageTemplateHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}
	t, err := h.service.Get(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, t)
}

// Create 创建
func (h *CFWorkpageTemplateHandler) Create(c *gin.Context) {
	log := logger.GetLogger()
	var req struct {
		NameZh      string `json:"name_zh"`
		NameMy      string `json:"name_my"`
		DefaultLang string `json:"default_lang"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	defaultLang := req.DefaultLang
	if defaultLang != "zh" && defaultLang != "my" {
		defaultLang = "zh"
	}
	t, err := h.service.Create(req.NameZh, req.NameMy, defaultLang)
	if err != nil {
		log.WithError(err).Error("创建模版失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, t)
}

// Update 更新
func (h *CFWorkpageTemplateHandler) Update(c *gin.Context) {
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}
	var req struct {
		NameZh      *string `json:"name_zh"`
		NameMy      *string `json:"name_my"`
		DefaultLang *string `json:"default_lang"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	t, err := h.service.Update(uint(id), req.NameZh, req.NameMy, req.DefaultLang)
	if err != nil {
		log.WithError(err).Error("更新模版失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, t)
}

// Delete 删除
func (h *CFWorkpageTemplateHandler) Delete(c *gin.Context) {
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}
	if err := h.service.Delete(uint(id)); err != nil {
		log.WithError(err).Error("删除模版失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已删除"})
}
