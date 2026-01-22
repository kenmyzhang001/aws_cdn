package handlers

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/models"
	"aws_cdn/internal/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type SpeedProbeHandler struct {
	service *services.SpeedProbeService
}

func NewSpeedProbeHandler(service *services.SpeedProbeService) *SpeedProbeHandler {
	return &SpeedProbeHandler{service: service}
}

// ReportProbeResult 上报单个探测结果
func (h *SpeedProbeHandler) ReportProbeResult(c *gin.Context) {
	log := logger.GetLogger()

	var req struct {
		URL            string  `json:"url" binding:"required"`
		SpeedKbps      float64 `json:"speed_kbps" binding:"required"`
		FileSize       *int64  `json:"file_size"`
		DownloadTimeMs *int64  `json:"download_time_ms"`
		Status         string  `json:"status"`
		ErrorMessage   string  `json:"error_message"`
		UserAgent      string  `json:"user_agent"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("上报探测结果失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取客户端IP
	clientIP := c.ClientIP()

	// 如果没有指定状态，根据速度判断
	status := models.SpeedProbeStatusSuccess
	if req.Status != "" {
		status = models.SpeedProbeStatus(req.Status)
	}

	// 获取User-Agent
	userAgent := req.UserAgent
	if userAgent == "" {
		userAgent = c.Request.UserAgent()
	}

	// 创建探测结果
	result := &models.SpeedProbeResult{
		URL:            req.URL,
		ClientIP:       clientIP,
		SpeedKbps:      req.SpeedKbps,
		FileSize:       req.FileSize,
		DownloadTimeMs: req.DownloadTimeMs,
		Status:         status,
		ErrorMessage:   req.ErrorMessage,
		UserAgent:      userAgent,
	}

	if err := h.service.ReportProbeResult(result); err != nil {
		log.WithError(err).Error("上报探测结果操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"result_id": result.ID,
		"client_ip": clientIP,
		"url":       req.URL,
		"speed":     req.SpeedKbps,
	}).Info("探测结果上报成功")

	c.JSON(http.StatusOK, gin.H{
		"message": "探测结果上报成功",
		"id":      result.ID,
	})
}

// BatchReportProbeResults 批量上报探测结果
func (h *SpeedProbeHandler) BatchReportProbeResults(c *gin.Context) {
	log := logger.GetLogger()

	var req struct {
		Results []struct {
			URL            string  `json:"url" binding:"required"`
			SpeedKbps      float64 `json:"speed_kbps" binding:"required"`
			FileSize       *int64  `json:"file_size"`
			DownloadTimeMs *int64  `json:"download_time_ms"`
			Status         string  `json:"status"`
			ErrorMessage   string  `json:"error_message"`
			UserAgent      string  `json:"user_agent"`
		} `json:"results" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("批量上报探测结果失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取客户端IP
	clientIP := c.ClientIP()

	// 转换为模型
	results := make([]models.SpeedProbeResult, len(req.Results))
	for i, r := range req.Results {
		status := models.SpeedProbeStatusSuccess
		if r.Status != "" {
			status = models.SpeedProbeStatus(r.Status)
		}

		userAgent := r.UserAgent
		if userAgent == "" {
			userAgent = c.Request.UserAgent()
		}

		results[i] = models.SpeedProbeResult{
			URL:            r.URL,
			ClientIP:       clientIP,
			SpeedKbps:      r.SpeedKbps,
			FileSize:       r.FileSize,
			DownloadTimeMs: r.DownloadTimeMs,
			Status:         status,
			ErrorMessage:   r.ErrorMessage,
			UserAgent:      userAgent,
		}
	}

	if err := h.service.BatchReportProbeResults(results); err != nil {
		log.WithError(err).Error("批量上报探测结果操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"count":     len(results),
		"client_ip": clientIP,
	}).Info("批量探测结果上报成功")

	c.JSON(http.StatusOK, gin.H{
		"message": "批量探测结果上报成功",
		"count":   len(results),
	})
}

// GetProbeResultsByIP 获取指定IP的探测结果
func (h *SpeedProbeHandler) GetProbeResultsByIP(c *gin.Context) {
	clientIP := c.Param("ip")
	if clientIP == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "IP地址不能为空"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	results, total, err := h.service.GetProbeResultsByIP(clientIP, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  results,
		"total": total,
		"page":  page,
		"size":  pageSize,
	})
}

// GetAlertLogs 获取告警记录
func (h *SpeedProbeHandler) GetAlertLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	logs, total, err := h.service.GetAlertLogs(page, pageSize)
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

// TriggerCheck 手动触发检查（用于测试）
func (h *SpeedProbeHandler) TriggerCheck(c *gin.Context) {
	log := logger.GetLogger()

	windowMinutes, _ := strconv.Atoi(c.DefaultQuery("window_minutes", "30"))

	if err := h.service.CheckAndAlertAll(windowMinutes); err != nil {
		log.WithError(err).Error("触发检查失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "检查已触发",
		"window_minutes": windowMinutes,
	})
}
