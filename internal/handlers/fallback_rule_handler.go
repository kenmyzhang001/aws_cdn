package handlers

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/models"
	"aws_cdn/internal/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type FallbackRuleHandler struct {
	service *services.FallbackRuleService
}

func NewFallbackRuleHandler(service *services.FallbackRuleService) *FallbackRuleHandler {
	return &FallbackRuleHandler{service: service}
}

// CreateFallbackRule 创建兜底规则
func (h *FallbackRuleHandler) CreateFallbackRule(c *gin.Context) {
	log := logger.GetLogger()

	var req struct {
		ChannelCode string `json:"channel_code" binding:"required"`
		Name        string `json:"name" binding:"required"`
		RuleType    string `json:"rule_type" binding:"required"`
		ParamsJSON  string `json:"params_json" binding:"required"`
		Enabled     *bool  `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("创建兜底规则失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	rule := &models.FallbackRule{
		ChannelCode: req.ChannelCode,
		Name:        req.Name,
		RuleType:    models.FallbackRuleType(req.RuleType),
		ParamsJSON:  req.ParamsJSON,
		Enabled:     enabled,
	}

	if err := h.service.Create(rule); err != nil {
		log.WithError(err).Error("创建兜底规则失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.WithField("rule_id", rule.ID).Info("兜底规则创建成功")
	c.JSON(http.StatusOK, rule)
}

// GetFallbackRule 获取单条规则
func (h *FallbackRuleHandler) GetFallbackRule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的规则 ID"})
		return
	}

	rule, err := h.service.Get(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rule)
}

// ListFallbackRules 分页列表
func (h *FallbackRuleHandler) ListFallbackRules(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	var channelCode *string
	if v := c.Query("channel_code"); v != "" {
		channelCode = &v
	}
	var ruleType *models.FallbackRuleType
	if v := c.Query("rule_type"); v != "" {
		t := models.FallbackRuleType(v)
		ruleType = &t
	}
	var enabled *bool
	if v := c.Query("enabled"); v != "" {
		b := v == "true" || v == "1"
		enabled = &b
	}

	list, total, err := h.service.List(page, pageSize, channelCode, ruleType, enabled)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  list,
		"total": total,
		"page":  page,
		"size":  pageSize,
	})
}

// UpdateFallbackRule 更新规则
func (h *FallbackRuleHandler) UpdateFallbackRule(c *gin.Context) {
	log := logger.GetLogger()

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的规则 ID"})
		return
	}

	var req struct {
		ChannelCode *string `json:"channel_code"`
		Name        *string `json:"name"`
		RuleType    *string `json:"rule_type"`
		ParamsJSON  *string `json:"params_json"`
		Enabled     *bool   `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := make(map[string]interface{})
	if req.ChannelCode != nil {
		updates["channel_code"] = *req.ChannelCode
	}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.RuleType != nil {
		updates["rule_type"] = *req.RuleType
	}
	if req.ParamsJSON != nil {
		updates["params_json"] = *req.ParamsJSON
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有需要更新的字段"})
		return
	}

	if err := h.service.Update(uint(id), updates); err != nil {
		log.WithError(err).Error("更新兜底规则失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.WithField("rule_id", id).Info("兜底规则更新成功")
	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

// DeleteFallbackRule 删除规则
func (h *FallbackRuleHandler) DeleteFallbackRule(c *gin.Context) {
	log := logger.GetLogger()

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的规则 ID"})
		return
	}

	if err := h.service.Delete(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithField("rule_id", id).Info("兜底规则删除成功")
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}
