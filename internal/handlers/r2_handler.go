package handlers

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type R2Handler struct {
	bucketService    *services.R2BucketService
	domainService    *services.R2CustomDomainService
	cacheRuleService *services.R2CacheRuleService
	fileService      *services.R2FileService
}

func NewR2Handler(
	bucketService *services.R2BucketService,
	domainService *services.R2CustomDomainService,
	cacheRuleService *services.R2CacheRuleService,
	fileService *services.R2FileService,
) *R2Handler {
	return &R2Handler{
		bucketService:    bucketService,
		domainService:    domainService,
		cacheRuleService: cacheRuleService,
		fileService:      fileService,
	}
}

// EnableR2 启用 R2
func (h *R2Handler) EnableR2(c *gin.Context) {
	log := logger.GetLogger()
	cfAccountID, err := strconv.ParseUint(c.Param("cf_account_id"), 10, 32)
	if err != nil {
		log.WithError(err).Error("启用R2失败：无效的账号ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的账号 ID"})
		return
	}

	if err := h.bucketService.EnableR2(uint(cfAccountID)); err != nil {
		log.WithError(err).Error("启用R2操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "R2已启用"})
}

// ListR2Buckets 列出所有 R2 存储桶
func (h *R2Handler) ListR2Buckets(c *gin.Context) {
	log := logger.GetLogger()
	buckets, err := h.bucketService.ListR2Buckets()
	if err != nil {
		log.WithError(err).Error("列出R2存储桶失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, buckets)
}

// GetR2Bucket 获取 R2 存储桶信息
func (h *R2Handler) GetR2Bucket(c *gin.Context) {
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		log.WithError(err).Error("获取R2存储桶失败：无效的ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}

	bucket, err := h.bucketService.GetR2Bucket(uint(id))
	if err != nil {
		log.WithError(err).Error("获取R2存储桶操作失败")
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, bucket)
}

// CreateR2Bucket 创建 R2 存储桶
func (h *R2Handler) CreateR2Bucket(c *gin.Context) {
	log := logger.GetLogger()
	var req struct {
		CFAccountID uint   `json:"cf_account_id" binding:"required"`
		BucketName  string `json:"bucket_name" binding:"required"`
		Location    string `json:"location"`
		AccountID   string `json:"account_id"` // 可选，如果不提供会尝试自动获取
		Note        string `json:"note"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("创建R2存储桶失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	bucket, err := h.bucketService.CreateR2Bucket(req.CFAccountID, req.BucketName, req.Location, req.AccountID, req.Note)
	if err != nil {
		log.WithError(err).Error("创建R2存储桶操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"bucket_id":   bucket.ID,
		"bucket_name": bucket.BucketName,
	}).Info("R2存储桶创建成功")
	c.JSON(http.StatusOK, bucket)
}

// DeleteR2Bucket 删除 R2 存储桶
func (h *R2Handler) DeleteR2Bucket(c *gin.Context) {
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		log.WithError(err).Error("删除R2存储桶失败：无效的ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}

	if err := h.bucketService.DeleteR2Bucket(uint(id)); err != nil {
		log.WithError(err).Error("删除R2存储桶操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithField("bucket_id", id).Info("R2存储桶删除成功")
	c.JSON(http.StatusOK, gin.H{"message": "R2存储桶删除成功"})
}

// UpdateR2BucketNote 更新存储桶备注
func (h *R2Handler) UpdateR2BucketNote(c *gin.Context) {
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		log.WithError(err).Error("更新R2存储桶备注失败：无效的ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}

	var req struct {
		Note string `json:"note" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("更新R2存储桶备注失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.bucketService.UpdateR2BucketNote(uint(id), req.Note); err != nil {
		log.WithError(err).Error("更新R2存储桶备注操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "备注更新成功"})
}

// UpdateR2BucketCredentials 已废弃：R2 凭证现在是账号维度的
func (h *R2Handler) UpdateR2BucketCredentials(c *gin.Context) {
	c.JSON(http.StatusBadRequest, gin.H{"error": "R2 Access Key 和 Secret Key 现在是账号维度的，请在 CF 账号管理中配置"})
}

// ConfigureCORS 配置 CORS
func (h *R2Handler) ConfigureCORS(c *gin.Context) {
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		log.WithError(err).Error("配置CORS失败：无效的ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}

	var req struct {
		CORSConfig []map[string]interface{} `json:"cors_config" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("配置CORS失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取存储桶信息
	bucket, err := h.bucketService.GetR2Bucket(uint(id))
	if err != nil {
		log.WithError(err).Error("获取R2存储桶失败")
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// 调用服务层配置 CORS
	if err := h.bucketService.ConfigureCORS(uint(id), req.CORSConfig); err != nil {
		log.WithError(err).Error("配置CORS操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"bucket_id":   bucket.ID,
		"bucket_name": bucket.BucketName,
	}).Info("CORS配置成功")
	c.JSON(http.StatusOK, gin.H{"message": "CORS配置成功"})
}

// ListR2CustomDomains 列出自定义域名
func (h *R2Handler) ListR2CustomDomains(c *gin.Context) {
	log := logger.GetLogger()
	r2BucketID, err := strconv.ParseUint(c.Param("r2_bucket_id"), 10, 32)
	if err != nil {
		log.WithError(err).Error("列出自定义域名失败：无效的存储桶ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的存储桶 ID"})
		return
	}

	domains, err := h.domainService.ListR2CustomDomains(uint(r2BucketID))
	if err != nil {
		log.WithError(err).Error("列出自定义域名操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, domains)
}

// AddR2CustomDomain 添加自定义域名
func (h *R2Handler) AddR2CustomDomain(c *gin.Context) {
	log := logger.GetLogger()
	r2BucketID, err := strconv.ParseUint(c.Param("r2_bucket_id"), 10, 32)
	if err != nil {
		log.WithError(err).Error("添加自定义域名失败：无效的存储桶ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的存储桶 ID"})
		return
	}

	var req struct {
		Domain string `json:"domain" binding:"required"`
		Note   string `json:"note"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("添加自定义域名失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	domain, err := h.domainService.AddCustomDomain(uint(r2BucketID), req.Domain, req.Note)
	if err != nil {
		log.WithError(err).Error("添加自定义域名操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"domain_id": domain.ID,
		"domain":    domain.Domain,
	}).Info("自定义域名添加成功")

	// 自动创建默认缓存规则
	// Edge Cache TTL: 1个月 (30天) = 2592000秒
	// Browser Cache TTL: 1年 = 31536000秒
	// Cache Status: Eligible (启用缓存)
	_, err = h.cacheRuleService.CreateCacheRule(
		domain.ID,
		"默认缓存规则",
		"true", // 匹配所有请求
		"Eligible",
		"1 month",
		"1 year",
		"自动创建的默认缓存规则",
	)
	if err != nil {
		log.WithError(err).Warn("自动创建默认缓存规则失败")
		// 不影响域名添加成功的响应，只记录警告
	} else {
		log.WithField("domain_id", domain.ID).Info("默认缓存规则创建成功")
	}

	c.JSON(http.StatusOK, domain)
}

// DeleteR2CustomDomain 删除自定义域名
func (h *R2Handler) DeleteR2CustomDomain(c *gin.Context) {
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		log.WithError(err).Error("删除自定义域名失败：无效的ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}

	if err := h.domainService.DeleteR2CustomDomain(uint(id)); err != nil {
		log.WithError(err).Error("删除自定义域名操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithField("domain_id", id).Info("自定义域名删除成功")
	c.JSON(http.StatusOK, gin.H{"message": "自定义域名删除成功"})
}

// ListR2CacheRules 列出缓存规则
func (h *R2Handler) ListR2CacheRules(c *gin.Context) {
	log := logger.GetLogger()
	r2CustomDomainID, err := strconv.ParseUint(c.Param("r2_custom_domain_id"), 10, 32)
	if err != nil {
		log.WithError(err).Error("列出缓存规则失败：无效的自定义域名ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的自定义域名 ID"})
		return
	}

	rules, err := h.cacheRuleService.ListR2CacheRules(uint(r2CustomDomainID))
	if err != nil {
		log.WithError(err).Error("列出缓存规则操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rules)
}

// CreateR2CacheRule 创建缓存规则
func (h *R2Handler) CreateR2CacheRule(c *gin.Context) {
	log := logger.GetLogger()
	r2CustomDomainID, err := strconv.ParseUint(c.Param("r2_custom_domain_id"), 10, 32)
	if err != nil {
		log.WithError(err).Error("创建缓存规则失败：无效的自定义域名ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的自定义域名 ID"})
		return
	}

	var req struct {
		RuleName    string `json:"rule_name" binding:"required"`
		Expression  string `json:"expression" binding:"required"`
		CacheStatus string `json:"cache_status" binding:"required"` // Eligible, Bypass
		EdgeTTL     string `json:"edge_ttl" binding:"required"`     // 如：1 month
		BrowserTTL  string `json:"browser_ttl" binding:"required"`  // 如：1 month
		Note        string `json:"note"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("创建缓存规则失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rule, err := h.cacheRuleService.CreateCacheRule(
		uint(r2CustomDomainID),
		req.RuleName,
		req.Expression,
		req.CacheStatus,
		req.EdgeTTL,
		req.BrowserTTL,
		req.Note,
	)
	if err != nil {
		log.WithError(err).Error("创建缓存规则操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"rule_id":   rule.ID,
		"rule_name": rule.RuleName,
	}).Info("缓存规则创建成功")
	c.JSON(http.StatusOK, rule)
}

// DeleteR2CacheRule 删除缓存规则
func (h *R2Handler) DeleteR2CacheRule(c *gin.Context) {
	log := logger.GetLogger()
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		log.WithError(err).Error("删除缓存规则失败：无效的ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}

	if err := h.cacheRuleService.DeleteR2CacheRule(uint(id)); err != nil {
		log.WithError(err).Error("删除缓存规则操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithField("rule_id", id).Info("缓存规则删除成功")
	c.JSON(http.StatusOK, gin.H{"message": "缓存规则删除成功"})
}

// UploadFile 上传文件
func (h *R2Handler) UploadFile(c *gin.Context) {
	log := logger.GetLogger()
	r2BucketID, err := strconv.ParseUint(c.Param("r2_bucket_id"), 10, 32)
	if err != nil {
		log.WithError(err).Error("上传文件失败：无效的存储桶ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的存储桶 ID"})
		return
	}

	// 获取上传的文件
	file, err := c.FormFile("file")
	if err != nil {
		log.WithError(err).Error("上传文件失败：无法获取文件")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法获取文件"})
		return
	}

	// 获取文件路径（key）
	key := c.PostForm("key")
	if key == "" {
		key = file.Filename
	}

	// 打开文件
	src, err := file.Open()
	if err != nil {
		log.WithError(err).Error("上传文件失败：无法打开文件")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法打开文件"})
		return
	}
	defer src.Close()

	// 上传文件
	if err := h.fileService.UploadFile(uint(r2BucketID), key, src, file.Header.Get("Content-Type")); err != nil {
		log.WithError(err).Error("上传文件操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 同步文件记录到数据库
	fileSize := file.Size
	contentType := file.Header.Get("Content-Type")
	if err := h.fileService.SyncFileRecord(uint(r2BucketID), key, file.Filename, &fileSize, &contentType, nil); err != nil {
		log.WithError(err).Warn("同步文件记录到数据库失败")
		// 不影响上传成功的响应
	}

	log.WithFields(map[string]interface{}{
		"bucket_id": r2BucketID,
		"key":       key,
	}).Info("文件上传成功")
	c.JSON(http.StatusOK, gin.H{"message": "文件上传成功", "key": key})
}

// CreateDirectory 创建目录
func (h *R2Handler) CreateDirectory(c *gin.Context) {
	log := logger.GetLogger()
	r2BucketID, err := strconv.ParseUint(c.Param("r2_bucket_id"), 10, 32)
	if err != nil {
		log.WithError(err).Error("创建目录失败：无效的存储桶ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的存储桶 ID"})
		return
	}

	var req struct {
		Prefix string `json:"prefix" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("创建目录失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.fileService.CreateDirectory(uint(r2BucketID), req.Prefix); err != nil {
		log.WithError(err).Error("创建目录操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.WithFields(map[string]interface{}{
		"bucket_id": r2BucketID,
		"prefix":    req.Prefix,
	}).Info("目录创建成功")
	c.JSON(http.StatusOK, gin.H{"message": "目录创建成功"})
}

// ListFiles 列出文件
func (h *R2Handler) ListFiles(c *gin.Context) {
	log := logger.GetLogger()
	r2BucketID, err := strconv.ParseUint(c.Param("r2_bucket_id"), 10, 32)
	if err != nil {
		log.WithError(err).Error("列出文件失败：无效的存储桶ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的存储桶 ID"})
		return
	}

	prefix := c.Query("prefix")

	files, err := h.fileService.ListFiles(uint(r2BucketID), prefix)
	if err != nil {
		log.WithError(err).Error("列出文件操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"files": files})
}

// DeleteFile 删除文件
func (h *R2Handler) DeleteFile(c *gin.Context) {
	log := logger.GetLogger()
	r2BucketID, err := strconv.ParseUint(c.Param("r2_bucket_id"), 10, 32)
	if err != nil {
		log.WithError(err).Error("删除文件失败：无效的存储桶ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的存储桶 ID"})
		return
	}

	var req struct {
		Key string `json:"key" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("删除文件失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.fileService.DeleteFile(uint(r2BucketID), req.Key); err != nil {
		log.WithError(err).Error("删除文件操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 更新数据库记录（标记为deleted）
	if err := h.fileService.DeleteR2FileRecord(uint(r2BucketID), req.Key); err != nil {
		log.WithError(err).Warn("更新数据库文件记录失败")
		// 不影响删除成功的响应
	}

	log.WithFields(map[string]interface{}{
		"bucket_id": r2BucketID,
		"key":       req.Key,
	}).Info("文件删除成功")
	c.JSON(http.StatusOK, gin.H{"message": "文件删除成功"})
}

// ListApkFiles 列出所有APK文件（不包含域名信息）
func (h *R2Handler) ListApkFiles(c *gin.Context) {
	log := logger.GetLogger()
	r2BucketID, err := strconv.ParseUint(c.Param("r2_bucket_id"), 10, 32)
	if err != nil {
		log.WithError(err).Error("列出APK文件失败：无效的存储桶ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的存储桶 ID"})
		return
	}

	prefix := c.Query("prefix")

	// 获取所有文件
	files, err := h.fileService.ListFiles(uint(r2BucketID), prefix)
	if err != nil {
		log.WithError(err).Error("列出APK文件操作失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 过滤出APK文件
	apkFiles := make([]map[string]interface{}, 0)
	for _, file := range files {
		// 跳过目录
		if len(file) > 0 && file[len(file)-1:] == "/" {
			continue
		}
		// 检查是否是APK文件
		if len(file) > 4 && file[len(file)-4:] == ".apk" {
			// 提取文件名
			fileName := file
			if idx := len(file) - 1; idx >= 0 {
				for i := idx; i >= 0; i-- {
					if file[i:i+1] == "/" {
						fileName = file[i+1:]
						break
					}
				}
			}

			apkFiles = append(apkFiles, map[string]interface{}{
				"file_name": fileName,
				"file_path": file,
			})
		}
	}

	c.JSON(http.StatusOK, apkFiles)
}

// GetApkFileUrls 获取指定APK文件的所有自定义域名访问链接
func (h *R2Handler) GetApkFileUrls(c *gin.Context) {
	log := logger.GetLogger()
	r2BucketID, err := strconv.ParseUint(c.Param("r2_bucket_id"), 10, 32)
	if err != nil {
		log.WithError(err).Error("获取APK文件链接失败：无效的存储桶ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的存储桶 ID"})
		return
	}

	filePath := c.Query("file_path")
	if filePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 file_path 参数"})
		return
	}

	// 获取该存储桶的所有自定义域名
	domains, err := h.domainService.ListR2CustomDomains(uint(r2BucketID))
	if err != nil {
		log.WithError(err).Error("获取自定义域名列表失败")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 为每个域名生成访问链接
	urls := make([]map[string]interface{}, 0, len(domains))
	for _, domain := range domains {
		urls = append(urls, map[string]interface{}{
			"domain": domain.Domain,
			"url":    "https://" + domain.Domain + "/" + filePath,
		})
	}

	c.JSON(http.StatusOK, urls)
}
