package handlers

import (
	"aws_cdn/internal/services"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type AuditHandler struct {
	auditService *services.AuditService
}

func NewAuditHandler(auditService *services.AuditService) *AuditHandler {
	return &AuditHandler{auditService: auditService}
}

// ListAuditLogs 查询审计日志列表
func (h *AuditHandler) ListAuditLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	// 构建过滤条件
	filters := make(map[string]interface{})

	// 用户ID过滤
	if userIDStr := c.Query("user_id"); userIDStr != "" {
		if userID, err := strconv.ParseUint(userIDStr, 10, 32); err == nil {
			filters["user_id"] = uint(userID)
		}
	}

	// 用户名过滤
	if username := c.Query("username"); username != "" {
		filters["username"] = username
	}

	// 操作类型过滤
	if action := c.Query("action"); action != "" {
		filters["action"] = action
	}

	// 资源类型过滤
	if resource := c.Query("resource"); resource != "" {
		filters["resource"] = resource
	}

	// 资源ID过滤
	if resourceID := c.Query("resource_id"); resourceID != "" {
		filters["resource_id"] = resourceID
	}

	// IP过滤
	if ip := c.Query("ip"); ip != "" {
		filters["ip"] = ip
	}

	// 时间范围过滤
	if startTimeStr := c.Query("start_time"); startTimeStr != "" {
		if startTime, err := time.Parse("2006-01-02 15:04:05", startTimeStr); err == nil {
			filters["start_time"] = startTime
		}
	}
	if endTimeStr := c.Query("end_time"); endTimeStr != "" {
		if endTime, err := time.Parse("2006-01-02 15:04:05", endTimeStr); err == nil {
			filters["end_time"] = endTime
		}
	}

	logs, total, err := h.auditService.QueryAuditLogs(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  logs,
		"total": total,
		"page":  page,
		"size":  pageSize,
	})
}





