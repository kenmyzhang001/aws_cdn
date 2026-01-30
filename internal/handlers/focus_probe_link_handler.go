package handlers

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/models"
	"aws_cdn/internal/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type FocusProbeLinkHandler struct {
	service *services.FocusProbeLinkService
}

func NewFocusProbeLinkHandler(service *services.FocusProbeLinkService) *FocusProbeLinkHandler {
	return &FocusProbeLinkHandler{service: service}
}

// GetFocusProbeLinks 获取重点探测链接列表
func (h *FocusProbeLinkHandler) GetFocusProbeLinks(c *gin.Context) {
	log := logger.GetLogger()

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	linkType := c.Query("link_type")
	search := c.Query("search")

	var enabled *bool
	if enabledStr := c.Query("enabled"); enabledStr != "" {
		e := enabledStr == "true"
		enabled = &e
	}

	links, total, err := h.service.GetFocusProbeLinks(page, pageSize, linkType, enabled, search)
	if err != nil {
		log.WithError(err).Error("获取重点探测链接列表失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  links,
		"total": total,
		"page":  page,
		"size":  pageSize,
	})
}

// GetFocusProbeLinkByID 获取重点探测链接详情
func (h *FocusProbeLinkHandler) GetFocusProbeLinkByID(c *gin.Context) {
	log := logger.GetLogger()

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}

	link, err := h.service.GetFocusProbeLinkByID(uint(id))
	if err != nil {
		log.WithError(err).Error("获取重点探测链接详情失败")
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, link)
}

// CreateFocusProbeLink 创建重点探测链接
func (h *FocusProbeLinkHandler) CreateFocusProbeLink(c *gin.Context) {
	log := logger.GetLogger()

	var req struct {
		LinkType             string  `json:"link_type" binding:"required"`
		LinkID               *uint   `json:"link_id"`
		URL                  string  `json:"url" binding:"required"`
		Name                 string  `json:"name"`
		Description          string  `json:"description"`
		ProbeIntervalMinutes int     `json:"probe_interval_minutes"`
		Enabled              bool    `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("创建重点探测链接失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证 link_type
	linkType := models.LinkType(req.LinkType)
	if linkType != models.LinkTypeDownloadPackage &&
		linkType != models.LinkTypeCustomDownloadLink &&
		linkType != models.LinkTypeR2File {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的链接类型"})
		return
	}

	// 设置默认值
	if req.ProbeIntervalMinutes == 0 {
		req.ProbeIntervalMinutes = 10
	}

	link := &models.FocusProbeLink{
		LinkType:             linkType,
		LinkID:               req.LinkID,
		URL:                  req.URL,
		Name:                 req.Name,
		Description:          req.Description,
		ProbeIntervalMinutes: req.ProbeIntervalMinutes,
		Enabled:              req.Enabled,
	}

	if err := h.service.CreateFocusProbeLink(link); err != nil {
		log.WithError(err).Error("创建重点探测链接失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"link_id": link.ID,
		"url":     link.URL,
	}).Info("创建重点探测链接成功")

	c.JSON(http.StatusOK, gin.H{
		"message": "创建成功",
		"data":    link,
	})
}

// UpdateFocusProbeLink 更新重点探测链接
func (h *FocusProbeLinkHandler) UpdateFocusProbeLink(c *gin.Context) {
	log := logger.GetLogger()

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("更新重点探测链接失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.UpdateFocusProbeLink(uint(id), req); err != nil {
		log.WithError(err).Error("更新重点探测链接失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"link_id": id,
	}).Info("更新重点探测链接成功")

	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

// DeleteFocusProbeLink 删除重点探测链接
func (h *FocusProbeLinkHandler) DeleteFocusProbeLink(c *gin.Context) {
	log := logger.GetLogger()

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}

	if err := h.service.DeleteFocusProbeLink(uint(id)); err != nil {
		log.WithError(err).Error("删除重点探测链接失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"link_id": id,
	}).Info("删除重点探测链接成功")

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// BatchDeleteFocusProbeLinks 批量删除重点探测链接
func (h *FocusProbeLinkHandler) BatchDeleteFocusProbeLinks(c *gin.Context) {
	log := logger.GetLogger()

	var req struct {
		IDs []uint `json:"ids" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("批量删除重点探测链接失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.BatchDeleteFocusProbeLinks(req.IDs); err != nil {
		log.WithError(err).Error("批量删除重点探测链接失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"count": len(req.IDs),
	}).Info("批量删除重点探测链接成功")

	c.JSON(http.StatusOK, gin.H{
		"message": "批量删除成功",
		"count":   len(req.IDs),
	})
}

// BatchUpdateProbeInterval 批量更新探测间隔
func (h *FocusProbeLinkHandler) BatchUpdateProbeInterval(c *gin.Context) {
	log := logger.GetLogger()

	var req struct {
		IDs              []uint `json:"ids"`
		IntervalMinutes  int    `json:"interval_minutes" binding:"required,gt=0"`
		UpdateAll        bool   `json:"update_all"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("批量更新探测间隔失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var err error
	if req.UpdateAll {
		err = h.service.UpdateAllProbeInterval(req.IntervalMinutes)
	} else {
		if len(req.IDs) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "未指定要更新的链接"})
			return
		}
		err = h.service.BatchUpdateProbeInterval(req.IDs, req.IntervalMinutes)
	}

	if err != nil {
		log.WithError(err).Error("批量更新探测间隔失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"interval_minutes": req.IntervalMinutes,
		"update_all":       req.UpdateAll,
		"count":            len(req.IDs),
	}).Info("批量更新探测间隔成功")

	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

// ToggleEnabled 切换启用状态
func (h *FocusProbeLinkHandler) ToggleEnabled(c *gin.Context) {
	log := logger.GetLogger()

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}

	if err := h.service.ToggleEnabled(uint(id)); err != nil {
		log.WithError(err).Error("切换启用状态失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"link_id": id,
	}).Info("切换启用状态成功")

	c.JSON(http.StatusOK, gin.H{"message": "操作成功"})
}

// GetStatistics 获取统计信息
func (h *FocusProbeLinkHandler) GetStatistics(c *gin.Context) {
	log := logger.GetLogger()

	stats, err := h.service.GetStatistics()
	if err != nil {
		log.WithError(err).Error("获取统计信息失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// AddFromDownloadPackage 从下载包添加到重点探测
func (h *FocusProbeLinkHandler) AddFromDownloadPackage(c *gin.Context) {
	log := logger.GetLogger()

	var req struct {
		PackageID uint   `json:"package_id" binding:"required"`
		URL       string `json:"url" binding:"required"`
		Name      string `json:"name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("添加重点探测链接失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.AddFromDownloadPackage(req.PackageID, req.URL, req.Name); err != nil {
		log.WithError(err).Error("添加重点探测链接失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"package_id": req.PackageID,
		"url":        req.URL,
	}).Info("从下载包添加重点探测链接成功")

	c.JSON(http.StatusOK, gin.H{"message": "添加成功"})
}

// AddFromCustomDownloadLink 从自定义下载链接添加到重点探测
func (h *FocusProbeLinkHandler) AddFromCustomDownloadLink(c *gin.Context) {
	log := logger.GetLogger()

	var req struct {
		LinkID uint   `json:"link_id" binding:"required"`
		URL    string `json:"url" binding:"required"`
		Name   string `json:"name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("添加重点探测链接失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.AddFromCustomDownloadLink(req.LinkID, req.URL, req.Name); err != nil {
		log.WithError(err).Error("添加重点探测链接失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"link_id": req.LinkID,
		"url":     req.URL,
	}).Info("从自定义下载链接添加重点探测链接成功")

	c.JSON(http.StatusOK, gin.H{"message": "添加成功"})
}

// AddFromR2File 从R2文件添加到重点探测
func (h *FocusProbeLinkHandler) AddFromR2File(c *gin.Context) {
	log := logger.GetLogger()

	var req struct {
		URL         string `json:"url" binding:"required"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("添加重点探测链接失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.AddFromR2File(req.URL, req.Name, req.Description); err != nil {
		log.WithError(err).Error("添加重点探测链接失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"url": req.URL,
	}).Info("从R2文件添加重点探测链接成功")

	c.JSON(http.StatusOK, gin.H{"message": "添加成功"})
}

// ExportLinks 导出链接列表
func (h *FocusProbeLinkHandler) ExportLinks(c *gin.Context) {
	log := logger.GetLogger()

	urls, err := h.service.ExportLinks()
	if err != nil {
		log.WithError(err).Error("导出链接列表失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  urls,
		"count": len(urls),
	})
}

// CheckIfURLExists 检查URL是否已存在
func (h *FocusProbeLinkHandler) CheckIfURLExists(c *gin.Context) {
	url := c.Query("url")
	if url == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "URL参数不能为空"})
		return
	}

	exists, err := h.service.CheckIfURLExists(url)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"exists": exists,
	})
}
