package handlers

import (
	"aws_cdn/internal/config"
	"aws_cdn/internal/logger"
	"aws_cdn/internal/services"
	"aws_cdn/internal/services/cloudflare"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type CFAccountHandler struct {
	service *services.CFAccountService
}

func NewCFAccountHandler(service *services.CFAccountService) *CFAccountHandler {
	return &CFAccountHandler{service: service}
}

// ListCFAccounts 列出所有 Cloudflare 账号
func (h *CFAccountHandler) ListCFAccounts(c *gin.Context) {
	log := logger.GetLogger()
	accounts, err := h.service.ListCFAccounts()
	if err != nil {
		log.WithError(err).Error("列出Cloudflare账号失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, accounts)
}

// GetCFAccount 获取 Cloudflare 账号信息
func (h *CFAccountHandler) GetCFAccount(c *gin.Context) {
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		log.WithError(err).WithField("id_param", c.Param("id")).Error("获取Cloudflare账号失败：无效的账号ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的账号 ID"})
		return
	}

	account, err := h.service.GetCFAccount(uint(id))
	if err != nil {
		log.WithError(err).WithField("account_id", id).Error("获取Cloudflare账号失败：账号不存在或查询失败")
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, account)
}

// CreateCFAccount 创建 Cloudflare 账号
func (h *CFAccountHandler) CreateCFAccount(c *gin.Context) {
	log := logger.GetLogger()
	var req struct {
		Email             string `json:"email" binding:"required,email"`
		Password          string `json:"password" binding:"required"`
		APIToken          string `json:"api_token"`
		R2APIToken        string `json:"r2_api_token"`
		AccountID         string `json:"account_id"`
		R2AccessKeyID     string `json:"r2_access_key_id"`
		R2SecretAccessKey string `json:"r2_secret_access_key"`
		Note              string `json:"note"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"email": req.Email,
		}).Error("创建Cloudflare账号失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	account, err := h.service.CreateCFAccount(req.Email, req.Password, req.APIToken, req.R2APIToken, req.AccountID, req.R2AccessKeyID, req.R2SecretAccessKey, req.Note)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"email": req.Email,
		}).Error("创建Cloudflare账号操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"account_id": account.ID,
		"email":      account.Email,
	}).Info("Cloudflare账号创建成功")
	c.JSON(http.StatusOK, account)
}

// UpdateCFAccount 更新 Cloudflare 账号
func (h *CFAccountHandler) UpdateCFAccount(c *gin.Context) {
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		log.WithError(err).WithField("id_param", c.Param("id")).Error("更新Cloudflare账号失败：无效的账号ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的账号 ID"})
		return
	}

	var req struct {
		Email             *string `json:"email"`
		Password          *string `json:"password"`
		APIToken          *string `json:"api_token"`
		R2APIToken        *string `json:"r2_api_token"`
		AccountID         *string `json:"account_id"`
		R2AccessKeyID     *string `json:"r2_access_key_id"`
		R2SecretAccessKey *string `json:"r2_secret_access_key"`
		Note              *string `json:"note"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).WithField("account_id", id).Error("更新Cloudflare账号失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证邮箱格式（如果提供了邮箱）
	if req.Email != nil {
		if *req.Email == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "邮箱不能为空"})
			return
		}
		// 简单的邮箱格式验证
		if !strings.Contains(*req.Email, "@") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "邮箱格式不正确"})
			return
		}
	}

	account, err := h.service.UpdateCFAccount(uint(id), req.Email, req.Password, req.APIToken, req.R2APIToken, req.AccountID, req.R2AccessKeyID, req.R2SecretAccessKey, req.Note)
	if err != nil {
		log.WithError(err).WithField("account_id", id).Error("更新Cloudflare账号操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"account_id": account.ID,
		"email":      account.Email,
	}).Info("Cloudflare账号更新成功")
	c.JSON(http.StatusOK, account)
}

// DeleteCFAccount 删除 Cloudflare 账号
func (h *CFAccountHandler) DeleteCFAccount(c *gin.Context) {
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		log.WithError(err).WithField("id_param", c.Param("id")).Error("删除Cloudflare账号失败：无效的账号ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的账号 ID"})
		return
	}

	log.WithField("account_id", id).Info("开始删除Cloudflare账号")
	if err := h.service.DeleteCFAccount(uint(id)); err != nil {
		log.WithError(err).WithField("account_id", id).Error("删除Cloudflare账号操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithField("account_id", id).Info("Cloudflare账号删除成功")
	c.JSON(http.StatusOK, gin.H{"message": "Cloudflare账号删除成功"})
}

// GetCFAccountZones 获取 CF 账号下的所有域名（Zones）
func (h *CFAccountHandler) GetCFAccountZones(c *gin.Context) {
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		log.WithError(err).WithField("id_param", c.Param("id")).Error("获取域名列表失败：无效的账号ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的账号 ID"})
		return
	}

	// 获取 CF 账号信息
	account, err := h.service.GetCFAccount(uint(id))
	if err != nil {
		log.WithError(err).WithField("account_id", id).Error("获取CF账号失败")
		c.JSON(http.StatusNotFound, gin.H{"error": "CF 账号不存在"})
		return
	}

	// 使用 API Token 创建 Cloudflare 服务
	cfg := &config.CloudflareConfig{
		APIToken: account.APIToken,
	}

	cfService, err := cloudflare.NewCloudflareService(cfg)
	if err != nil {
		log.WithError(err).Error("创建 Cloudflare 服务失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建 Cloudflare 服务失败"})
		return
	}

	// 获取分页参数
	page := 1
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	perPage := 20 // 默认每页 20 条
	if perPageStr := c.Query("per_page"); perPageStr != "" {
		if pp, err := strconv.Atoi(perPageStr); err == nil && pp > 0 && pp <= 50 {
			perPage = pp
		}
	}

	// 获取可选的域名搜索参数
	name := c.Query("name")

	// 获取域名列表（支持分页和搜索）
	result, err := cfService.ListZones(page, perPage, name)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"account_id": id,
			"page":       page,
			"per_page":   perPage,
			"name":       name,
		}).Error("获取域名列表失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"account_id":  id,
		"page":        page,
		"per_page":    perPage,
		"name":        name,
		"count":       len(result.Zones),
		"total_count": result.TotalCount,
	}).Info("获取域名列表成功")

	c.JSON(http.StatusOK, result)
}
