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

// AddZones 批量添加域名到指定 CF 账号
func (h *CFAccountHandler) AddZones(c *gin.Context) {
	log := logger.GetLogger()

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		log.WithError(err).WithField("id_param", c.Param("id")).Error("添加域名失败：无效的账号ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的账号 ID"})
		return
	}

	var req struct {
		Domains []string `json:"domains" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).WithField("account_id", id).Error("添加域名失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供 domains 数组，如 [\"example.com\"]"})
		return
	}

	// 去重、去空
	seen := make(map[string]bool)
	var domains []string
	for _, d := range req.Domains {
		d = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(d), "."))
		if d != "" && !seen[d] {
			seen[d] = true
			domains = append(domains, d)
		}
	}
	if len(domains) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请至少提供一个有效域名"})
		return
	}

	account, err := h.service.GetCFAccount(uint(id))
	if err != nil {
		log.WithError(err).WithField("account_id", id).Error("获取CF账号失败")
		c.JSON(http.StatusNotFound, gin.H{"error": "CF 账号不存在"})
		return
	}

	cfg := &config.CloudflareConfig{APIToken: account.APIToken}
	cfService, err := cloudflare.NewCloudflareService(cfg)
	if err != nil {
		log.WithError(err).Error("创建 Cloudflare 服务失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建 Cloudflare 服务失败"})
		return
	}

	cfAccountID := account.AccountID
	if cfAccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该 CF 账号未配置 account_id，无法添加域名"})
		return
	}

	type zoneResult struct {
		Domain      string   `json:"domain"`
		Status      string   `json:"status"`
		ZoneID      string   `json:"zone_id,omitempty"`
		NameServers []string `json:"name_servers,omitempty"`
		Message     string   `json:"message,omitempty"`
	}

	var results []zoneResult
	var successCount, failedCount int

	for _, domain := range domains {
		created, err := cfService.CreateZone(cfAccountID, domain)
		if err != nil {
			log.WithError(err).WithField("domain", domain).Warn("添加域名失败")
			results = append(results, zoneResult{
				Domain:  domain,
				Status:  "failed",
				Message: err.Error(),
			})
			failedCount++
			continue
		}
		results = append(results, zoneResult{
			Domain:      domain,
			Status:      "success",
			ZoneID:      created.ID,
			NameServers: created.NameServers,
		})
		successCount++
	}

	message := "批量添加完成"
	if failedCount > 0 && successCount == 0 {
		message = "全部添加失败"
	} else if failedCount > 0 {
		message = "部分添加成功"
	}

	c.JSON(http.StatusOK, gin.H{
		"message": message,
		"results": results,
		"stats": gin.H{
			"success_count": successCount,
			"failed_count":  failedCount,
			"total":         len(domains),
		},
	})
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

	// 获取可选的 Cloudflare 账户ID过滤参数
	accountID := account.AccountID

	// 获取域名列表（支持分页和搜索）
	result, err := cfService.ListZones(page, perPage, name, accountID)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"account_id":    id,
			"cf_account_id": accountID,
			"page":          page,
			"per_page":      perPage,
			"name":          name,
		}).Error("获取域名列表失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"account_id":    id,
		"cf_account_id": accountID,
		"page":          page,
		"per_page":      perPage,
		"name":          name,
		"count":         len(result.Zones),
		"total_count":   result.TotalCount,
	}).Info("获取域名列表成功")

	c.JSON(http.StatusOK, result)
}

// SetZoneAPKSecurityRule 为域名设置 APK 放行规则（根域名及所有子域名）
func (h *CFAccountHandler) SetZoneAPKSecurityRule(c *gin.Context) {
	log := logger.GetLogger()

	// 获取 CF 账号 ID
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		log.WithError(err).WithField("id_param", c.Param("id")).Error("设置APK安全规则失败：无效的账号ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的账号 ID"})
		return
	}

	// 获取请求参数
	var req struct {
		ZoneID     string `json:"zone_id" binding:"required"`
		DomainName string `json:"domain_name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).WithField("account_id", accountID).Error("设置APK安全规则失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取 CF 账号信息
	account, err := h.service.GetCFAccount(uint(accountID))
	if err != nil {
		log.WithError(err).WithField("account_id", accountID).Error("获取CF账号失败")
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

	log.WithFields(map[string]interface{}{
		"account_id": accountID,
		"zone_id":    req.ZoneID,
		"domain":     req.DomainName,
	}).Info("开始为域名设置 APK 放行规则")

	// 存储规则创建结果
	type RuleResult struct {
		RuleName string `json:"rule_name"`
		RuleID   string `json:"rule_id"`
		Status   string `json:"status"`
		Message  string `json:"message"`
	}

	results := []RuleResult{}

	// 1. 创建 WAF VIP 下载规则（最高优先级，免检金牌）
	vipRuleID, vipErr := cfService.CreateWAFVIPDownloadRule(req.ZoneID, req.DomainName)
	if vipErr != nil {
		log.WithError(vipErr).WithFields(map[string]interface{}{
			"domain":  req.DomainName,
			"zone_id": req.ZoneID,
		}).Warn("创建 WAF VIP 下载规则失败")
		results = append(results, RuleResult{
			RuleName: "WAF VIP下载规则",
			Status:   "failed",
			Message:  vipErr.Error(),
		})
	} else if vipRuleID != "" {
		log.WithFields(map[string]interface{}{
			"domain":  req.DomainName,
			"zone_id": req.ZoneID,
			"rule_id": vipRuleID,
		}).Info("WAF VIP 下载规则创建成功")
		results = append(results, RuleResult{
			RuleName: "WAF VIP下载规则",
			RuleID:   vipRuleID,
			Status:   "success",
			Message:  "规则创建成功，APK/OBB下载将直接放行",
		})
	}

	// 2. 创建 WAF 安全规则（威胁评分豁免）
	wafRuleID, wafErr := cfService.CreateWAFSecurityRule(req.ZoneID, req.DomainName, []string{"apk"})
	if wafErr != nil {
		log.WithError(wafErr).WithFields(map[string]interface{}{
			"domain":  req.DomainName,
			"zone_id": req.ZoneID,
		}).Warn("创建 WAF 安全规则失败")
		results = append(results, RuleResult{
			RuleName: "WAF安全规则",
			Status:   "failed",
			Message:  wafErr.Error(),
		})
	} else if wafRuleID != "" {
		log.WithFields(map[string]interface{}{
			"domain":  req.DomainName,
			"zone_id": req.ZoneID,
			"rule_id": wafRuleID,
		}).Info("WAF 安全规则创建成功")
		results = append(results, RuleResult{
			RuleName: "WAF安全规则",
			RuleID:   wafRuleID,
			Status:   "success",
			Message:  "规则创建成功，VPN和高频下载已豁免",
		})
	}

	// 判断整体状态
	successCount := 0
	failedCount := 0
	for _, r := range results {
		if r.Status == "success" {
			successCount++
		} else {
			failedCount++
		}
	}

	var responseStatus int
	var message string
	if failedCount == 0 {
		responseStatus = http.StatusOK
		message = "所有安全规则创建成功"
	} else if successCount > 0 {
		responseStatus = http.StatusOK
		message = "部分安全规则创建成功"
	} else {
		responseStatus = http.StatusInternalServerError
		message = "所有安全规则创建失败"
	}

	log.WithFields(map[string]interface{}{
		"account_id":    accountID,
		"zone_id":       req.ZoneID,
		"domain":        req.DomainName,
		"success_count": successCount,
		"failed_count":  failedCount,
	}).Info("APK安全规则设置完成")

	c.JSON(responseStatus, gin.H{
		"message": message,
		"results": results,
		"stats": gin.H{
			"success_count": successCount,
			"failed_count":  failedCount,
		},
	})
}
