package handlers

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/services"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// CFWorkerHandler Worker Handler
type CFWorkerHandler struct {
	workerService *services.CFWorkerService
}

// NewCFWorkerHandler 创建 Worker Handler
func NewCFWorkerHandler(workerService *services.CFWorkerService) *CFWorkerHandler {
	return &CFWorkerHandler{
		workerService: workerService,
	}
}

// CreateWorker 创建 Worker
// @Summary 创建 Worker
// @Tags Worker
// @Accept json
// @Produce json
// @Param request body services.CreateWorkerRequest true "创建 Worker 请求"
// @Success 200 {object} map[string]interface{}
// @Router /api/workers [post]
func (h *CFWorkerHandler) CreateWorker(c *gin.Context) {
	log := logger.GetLogger()

	var req services.CreateWorkerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("参数绑定失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"cf_account_id": req.CFAccountID,
		"worker_name":   req.WorkerName,
		"worker_domain": req.WorkerDomain,
		"mode":          req.Mode,
	}).Info("开始创建 Worker")

	worker, err := h.workerService.CreateWorker(&req)
	if err != nil {
		log.WithError(err).Error("创建 Worker 失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"worker_id":   worker.ID,
		"worker_name": worker.WorkerName,
	}).Info("Worker 创建成功")

	c.JSON(http.StatusOK, gin.H{
		"message": "Worker 创建成功",
		"data":    worker,
	})
}

// GetWorkerList 获取 Worker 列表
// @Summary 获取 Worker 列表
// @Tags Worker
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Param cf_account_id query int false "CF 账号 ID"
// @Param domain query string false "域名关键词（Worker 域名或目标域名）"
// @Success 200 {object} map[string]interface{}
// @Router /api/workers [get]
func (h *CFWorkerHandler) GetWorkerList(c *gin.Context) {
	log := logger.GetLogger()

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	cfAccountID, _ := strconv.ParseUint(c.Query("cf_account_id"), 10, 32)
	domain := strings.TrimSpace(c.Query("domain"))

	log.WithFields(map[string]interface{}{
		"page":          page,
		"page_size":     pageSize,
		"cf_account_id": cfAccountID,
		"domain":        domain,
	}).Info("查询 Worker 列表")

	workers, total, err := h.workerService.GetWorkerList(page, pageSize, uint(cfAccountID), domain)
	if err != nil {
		log.WithError(err).Error("查询 Worker 列表失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": workers,
		"pagination": gin.H{
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// GetWorker 获取 Worker 详情
// @Summary 获取 Worker 详情
// @Tags Worker
// @Produce json
// @Param id path int true "Worker ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/workers/{id} [get]
func (h *CFWorkerHandler) GetWorker(c *gin.Context) {
	log := logger.GetLogger()

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 Worker ID"})
		return
	}

	worker, err := h.workerService.GetWorkerByID(uint(id))
	if err != nil {
		log.WithError(err).Error("查询 Worker 失败")
		c.JSON(http.StatusNotFound, gin.H{"error": "Worker 不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": worker})
}

// UpdateWorker 更新 Worker
// @Summary 更新 Worker
// @Tags Worker
// @Accept json
// @Produce json
// @Param id path int true "Worker ID"
// @Param request body services.UpdateWorkerRequest true "更新 Worker 请求"
// @Success 200 {object} map[string]interface{}
// @Router /api/workers/{id} [put]
func (h *CFWorkerHandler) UpdateWorker(c *gin.Context) {
	log := logger.GetLogger()

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 Worker ID"})
		return
	}

	var req services.UpdateWorkerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("参数绑定失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"worker_id": id,
		"mode":     req.Mode,
	}).Info("开始更新 Worker")

	worker, err := h.workerService.UpdateWorker(uint(id), &req)
	if err != nil {
		log.WithError(err).Error("更新 Worker 失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"worker_id": id,
	}).Info("Worker 更新成功")

	c.JSON(http.StatusOK, gin.H{
		"message": "Worker 更新成功",
		"data":    worker,
	})
}

// DeleteWorker 删除 Worker
// @Summary 删除 Worker
// @Tags Worker
// @Produce json
// @Param id path int true "Worker ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/workers/{id} [delete]
func (h *CFWorkerHandler) DeleteWorker(c *gin.Context) {
	log := logger.GetLogger()

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 Worker ID"})
		return
	}

	log.WithFields(map[string]interface{}{
		"worker_id": id,
	}).Info("开始删除 Worker")

	if err := h.workerService.DeleteWorker(uint(id)); err != nil {
		log.WithError(err).Error("删除 Worker 失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"worker_id": id,
	}).Info("Worker 删除成功")

	c.JSON(http.StatusOK, gin.H{"message": "Worker 删除成功"})
}
