package handlers

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type ChannelGroupHandler struct {
	service *services.ChannelGroupService
}

func NewChannelGroupHandler(service *services.ChannelGroupService) *ChannelGroupHandler {
	return &ChannelGroupHandler{service: service}
}

// ListChannelGroups 列出所有渠道分组
// GET /api/v1/game-stats/channel-groups
func (h *ChannelGroupHandler) ListChannelGroups(c *gin.Context) {
	list, err := h.service.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

// GetChannelGroup 获取单个渠道分组
// GET /api/v1/game-stats/channel-groups/:id
func (h *ChannelGroupHandler) GetChannelGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}
	g, err := h.service.Get(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, g)
}

// CreateChannelGroup 创建渠道分组
// POST /api/v1/game-stats/channel-groups
func (h *ChannelGroupHandler) CreateChannelGroup(c *gin.Context) {
	log := logger.GetLogger()
	var req struct {
		Name         string   `json:"name" binding:"required"`
		ChannelCodes []string `json:"channel_codes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}
	g, err := h.service.Create(req.Name, req.ChannelCodes)
	if err != nil {
		log.WithError(err).WithField("name", req.Name).Error("创建渠道分组失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, g)
}

// UpdateChannelGroup 更新渠道分组
// PUT /api/v1/game-stats/channel-groups/:id
func (h *ChannelGroupHandler) UpdateChannelGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}
	var req struct {
		Name         string   `json:"name" binding:"required"`
		ChannelCodes []string `json:"channel_codes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}
	g, err := h.service.Update(uint(id), req.Name, req.ChannelCodes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, g)
}

// DeleteChannelGroup 删除渠道分组
// DELETE /api/v1/game-stats/channel-groups/:id
func (h *ChannelGroupHandler) DeleteChannelGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}
	if err := h.service.Delete(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}
