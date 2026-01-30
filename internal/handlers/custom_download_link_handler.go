package handlers

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/models"
	"aws_cdn/internal/services"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type CustomDownloadLinkHandler struct {
	service *services.CustomDownloadLinkService
}

func NewCustomDownloadLinkHandler(service *services.CustomDownloadLinkService) *CustomDownloadLinkHandler {
	return &CustomDownloadLinkHandler{service: service}
}

// CreateCustomDownloadLink 创建单个自定义下载链接
func (h *CustomDownloadLinkHandler) CreateCustomDownloadLink(c *gin.Context) {
	log := logger.GetLogger()

	var req struct {
		URL         string `json:"url" binding:"required"`
		Name        string `json:"name"`
		Description string `json:"description"`
		GroupID     *uint  `json:"group_id"`
		Status      string `json:"status"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("创建自定义下载链接失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	status := models.CustomDownloadLinkStatusActive
	if req.Status != "" {
		status = models.CustomDownloadLinkStatus(req.Status)
	}

	link := &models.CustomDownloadLink{
		URL:         req.URL,
		Name:        req.Name,
		Description: req.Description,
		Status:      status,
	}

	if err := h.service.CreateCustomDownloadLink(link); err != nil {
		log.WithError(err).Error("创建自定义下载链接操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithField("link_id", link.ID).Info("自定义下载链接创建成功")
	c.JSON(http.StatusOK, link)
}

// BatchCreateCustomDownloadLinks 批量创建自定义下载链接
func (h *CustomDownloadLinkHandler) BatchCreateCustomDownloadLinks(c *gin.Context) {
	log := logger.GetLogger()

	var req struct {
		URLs    string `json:"urls" binding:"required"` // 支持换行符或逗号分隔
		GroupID *uint  `json:"group_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("批量创建自定义下载链接失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	links, err := h.service.BatchCreateCustomDownloadLinks(req.URLs)
	if err != nil {
		log.WithError(err).Error("批量创建自定义下载链接操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithField("count", len(links)).Info("批量创建自定义下载链接成功")
	c.JSON(http.StatusOK, gin.H{
		"message": "批量创建成功",
		"count":   len(links),
		"data":    links,
	})
}

// GetCustomDownloadLink 获取自定义下载链接
func (h *CustomDownloadLinkHandler) GetCustomDownloadLink(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的链接 ID"})
		return
	}

	link, err := h.service.GetCustomDownloadLink(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, h.toResponse(link))
}

// ListCustomDownloadLinks 列出所有自定义下载链接
func (h *CustomDownloadLinkHandler) ListCustomDownloadLinks(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	var groupID *uint
	if groupIDStr := c.Query("group_id"); groupIDStr != "" {
		if id, err := strconv.ParseUint(groupIDStr, 10, 32); err == nil {
			gid := uint(id)
			groupID = &gid
		}
	}

	var search *string
	if searchStr := c.Query("search"); searchStr != "" {
		search = &searchStr
	}

	var status *models.CustomDownloadLinkStatus
	if statusStr := c.Query("status"); statusStr != "" {
		s := models.CustomDownloadLinkStatus(statusStr)
		status = &s
	}

	links, total, err := h.service.ListCustomDownloadLinks(page, pageSize, groupID, search, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	responses := make([]CustomDownloadLinkResponse, len(links))
	for i, link := range links {
		responses[i] = h.toResponse(&link)
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  responses,
		"total": total,
		"page":  page,
		"size":  pageSize,
	})
}

// UpdateCustomDownloadLink 更新自定义下载链接
func (h *CustomDownloadLinkHandler) UpdateCustomDownloadLink(c *gin.Context) {
	log := logger.GetLogger()

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		log.WithError(err).WithField("id_param", c.Param("id")).Error("更新自定义下载链接失败：无效的链接ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的链接 ID"})
		return
	}

	var req struct {
		URL         *string `json:"url"`
		Name        *string `json:"name"`
		Description *string `json:"description"`
		GroupID     *uint   `json:"group_id"`
		Status      *string `json:"status"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("更新自定义下载链接失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := make(map[string]interface{})
	if req.URL != nil {
		updates["url"] = *req.URL
	}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.GroupID != nil {
		updates["group_id"] = *req.GroupID
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有需要更新的字段"})
		return
	}

	if err := h.service.UpdateCustomDownloadLink(uint(id), updates); err != nil {
		log.WithError(err).WithField("link_id", id).Error("更新自定义下载链接操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithField("link_id", id).Info("自定义下载链接更新成功")
	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

// DeleteCustomDownloadLink 删除自定义下载链接
func (h *CustomDownloadLinkHandler) DeleteCustomDownloadLink(c *gin.Context) {
	log := logger.GetLogger()

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		log.WithError(err).WithField("id_param", c.Param("id")).Error("删除自定义下载链接失败：无效的链接ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的链接 ID"})
		return
	}

	if err := h.service.DeleteCustomDownloadLink(uint(id)); err != nil {
		log.WithError(err).WithField("link_id", id).Error("删除自定义下载链接操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithField("link_id", id).Info("自定义下载链接删除成功")
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// BatchDeleteCustomDownloadLinks 批量删除自定义下载链接
func (h *CustomDownloadLinkHandler) BatchDeleteCustomDownloadLinks(c *gin.Context) {
	log := logger.GetLogger()

	var req struct {
		IDs []uint `json:"ids" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("批量删除自定义下载链接失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "IDs 不能为空"})
		return
	}

	if err := h.service.BatchDeleteCustomDownloadLinks(req.IDs); err != nil {
		log.WithError(err).Error("批量删除自定义下载链接操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithField("count", len(req.IDs)).Info("批量删除自定义下载链接成功")
	c.JSON(http.StatusOK, gin.H{
		"message": "批量删除成功",
		"count":   len(req.IDs),
	})
}

// IncrementClickCount 增加点击次数
func (h *CustomDownloadLinkHandler) IncrementClickCount(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的链接 ID"})
		return
	}

	if err := h.service.IncrementClickCount(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "点击次数已更新"})
}

// CustomDownloadLinkResponse 自定义下载链接响应结构
type CustomDownloadLinkResponse struct {
	ID          uint   `json:"id"`
	URL         string `json:"url"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	ClickCount  uint   `json:"click_count"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// toResponse 转换为响应结构
func (h *CustomDownloadLinkHandler) toResponse(link *models.CustomDownloadLink) CustomDownloadLinkResponse {
	resp := CustomDownloadLinkResponse{
		ID:          link.ID,
		URL:         link.URL,
		Name:        link.Name,
		Description: link.Description,
		Status:      string(link.Status),
		ClickCount:  link.ClickCount,
		CreatedAt:   link.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   link.UpdatedAt.Format(time.RFC3339),
	}

	return resp
}
