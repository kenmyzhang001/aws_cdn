package handlers

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type GroupHandler struct {
	service *services.GroupService
}

func NewGroupHandler(service *services.GroupService) *GroupHandler {
	return &GroupHandler{service: service}
}

// ListGroups 列出所有分组
func (h *GroupHandler) ListGroups(c *gin.Context) {
	groups, err := h.service.ListGroups()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, groups)
}

// ListGroupsWithStats 列出所有分组及其统计信息（用于优化页面加载）
func (h *GroupHandler) ListGroupsWithStats(c *gin.Context) {
	groups, err := h.service.ListGroupsWithStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, groups)
}

// GetGroup 获取分组信息
func (h *GroupHandler) GetGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的分组 ID"})
		return
	}

	group, err := h.service.GetGroup(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, group)
}

// CreateGroup 创建分组
func (h *GroupHandler) CreateGroup(c *gin.Context) {
	log := logger.GetLogger()
	var req struct {
		Name string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).WithField("name", req.Name).Error("创建分组失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	group, err := h.service.CreateGroup(req.Name)
	if err != nil {
		log.WithError(err).WithField("name", req.Name).Error("创建分组操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"group_id": group.ID,
		"name":    req.Name,
	}).Info("分组创建成功")
	c.JSON(http.StatusOK, group)
}

// UpdateGroup 更新分组
func (h *GroupHandler) UpdateGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的分组 ID"})
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	group, err := h.service.UpdateGroup(uint(id), req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, group)
}

// DeleteGroup 删除分组
func (h *GroupHandler) DeleteGroup(c *gin.Context) {
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		log.WithError(err).WithField("id_param", c.Param("id")).Error("删除分组失败：无效的分组ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的分组 ID"})
		return
	}

	log.WithField("group_id", id).Info("开始删除分组")
	if err := h.service.DeleteGroup(uint(id)); err != nil {
		log.WithError(err).WithField("group_id", id).Error("删除分组操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithField("group_id", id).Info("分组删除成功")
	c.JSON(http.StatusOK, gin.H{"message": "分组删除成功"})
}

