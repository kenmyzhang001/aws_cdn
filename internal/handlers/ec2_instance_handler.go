package handlers

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Ec2InstanceHandler struct {
	svc *services.Ec2InstanceService
}

func NewEc2InstanceHandler(svc *services.Ec2InstanceService) *Ec2InstanceHandler {
	return &Ec2InstanceHandler{svc: svc}
}

func (h *Ec2InstanceHandler) GetRegionConfig(c *gin.Context) {
	configs := h.svc.GetRegionConfig()
	c.JSON(http.StatusOK, gin.H{"data": configs})
}

// List 仅从数据库查询未删除的实例列表，不调用 AWS 接口
func (h *Ec2InstanceHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	region := c.Query("region")
	list, total, err := h.svc.List(page, pageSize, region)
	if err != nil {
		logger.GetLogger().WithError(err).Error("EC2 实例列表失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":       list,
		"pagination": gin.H{"total": total, "page": page, "page_size": pageSize},
	})
}

func (h *Ec2InstanceHandler) ListDeleted(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	list, total, err := h.svc.ListDeleted(page, pageSize)
	if err != nil {
		logger.GetLogger().WithError(err).Error("EC2 回收站列表失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":       list,
		"pagination": gin.H{"total": total, "page": page, "page_size": pageSize},
	})
}

func (h *Ec2InstanceHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的实例 ID"})
		return
	}
	inst, err := h.svc.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "实例不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": inst})
}

func (h *Ec2InstanceHandler) Create(c *gin.Context) {
	var req services.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}
	inst, err := h.svc.Create(&req)
	if err != nil {
		logger.GetLogger().WithError(err).Error("创建 EC2 实例失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "创建成功", "data": inst})
}

func (h *Ec2InstanceHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的实例 ID"})
		return
	}
	var req services.UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}
	inst, err := h.svc.Update(uint(id), &req)
	if err != nil {
		logger.GetLogger().WithError(err).Error("更新 EC2 实例失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "更新成功", "data": inst})
}

func (h *Ec2InstanceHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的实例 ID"})
		return
	}
	if err := h.svc.Delete(uint(id)); err != nil {
		logger.GetLogger().WithError(err).Error("删除 EC2 实例失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已软删除并终止实例"})
}
