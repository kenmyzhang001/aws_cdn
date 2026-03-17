package handlers

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/models"
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

// ListRows 获取模版下所有表格行（3列多行，每行对应一个下载链接）
func (h *CFWorkpageTemplateHandler) ListRows(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的模版 ID"})
		return
	}
	rows, err := h.service.ListRows(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rows)
}

// SaveRows 批量保存模版表格行（替换该模版下全部行；支持 auto_popup 指定访问页时自动弹出其中一个包）
func (h *CFWorkpageTemplateHandler) SaveRows(c *gin.Context) {
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的模版 ID"})
		return
	}
	var req struct {
		Rows []struct {
			Col1Zh      string `json:"col1_zh"`
			Col1My      string `json:"col1_my"`
			Col2Zh      string `json:"col2_zh"`
			Col2My      string `json:"col2_my"`
			Col3Zh      string `json:"col3_zh"`
			Col3My      string `json:"col3_my"`
			DownloadURL string `json:"download_url"`
			AutoPopup   bool   `json:"auto_popup"`
		} `json:"rows"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rows := make([]models.CFWorkpageTemplateRow, 0, len(req.Rows))
	for _, r := range req.Rows {
		rows = append(rows, models.CFWorkpageTemplateRow{
			Col1Zh:      r.Col1Zh,
			Col1My:      r.Col1My,
			Col2Zh:      r.Col2Zh,
			Col2My:      r.Col2My,
			Col3Zh:      r.Col3Zh,
			Col3My:      r.Col3My,
			DownloadURL: r.DownloadURL,
			AutoPopup:   r.AutoPopup,
		})
	}
	saved, err := h.service.SaveRows(uint(id), rows)
	if err != nil {
		log.WithError(err).Error("保存模版表格行失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, saved)
}
