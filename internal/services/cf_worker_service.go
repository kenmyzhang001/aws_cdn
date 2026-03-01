package services

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/models"
	"aws_cdn/internal/services/cloudflare"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

// CFWorkerService Worker 服务
type CFWorkerService struct {
	db *gorm.DB
}

// NewCFWorkerService 创建 Worker 服务
func NewCFWorkerService(db *gorm.DB) *CFWorkerService {
	return &CFWorkerService{
		db: db,
	}
}

// CreateWorkerRequest 创建 Worker 请求
type CreateWorkerRequest struct {
	CFAccountID  uint     `json:"cf_account_id" binding:"required"`
	WorkerName   string   `json:"worker_name" binding:"required"`
	WorkerDomain string   `json:"worker_domain" binding:"required"`
	TargetDomain string   `json:"target_domain"` // 单链接时使用；与 targets 二选一（推广模式）
	Targets      []string `json:"targets"`       // 多目标链接（推广模式轮播/探针）
	FallbackURL  string   `json:"fallback_url"`  // 兜底链接（可选）
	Mode         string   `json:"mode"`          // single / time / random / probe
	BusinessMode string   `json:"business_mode"` // 业务模式：下载、推广
	RotateDays   int      `json:"rotate_days"`   // 时间轮播每 N 天
	BaseDate     string   `json:"base_date"`     // 时间轮播基准日期 ISO
	Description  string   `json:"description"`
	// 下载模式：R2 存储桶 ID，且域名→路径映射（一个 Worker 多域名，每域名对应一个文件）
	R2BucketID  uint              `json:"r2_bucket_id"`
	DomainPaths map[string]string `json:"domain_paths"` // 例：{"download1.example.com":"releases/app1.apk"}
}

// UpdateWorkerRequest 更新 Worker 请求
type UpdateWorkerRequest struct {
	TargetDomain string   `json:"target_domain"`
	Targets      []string `json:"targets"`
	FallbackURL  string   `json:"fallback_url"`
	Mode         string   `json:"mode"`
	BusinessMode string   `json:"business_mode"`
	RotateDays   int      `json:"rotate_days"`
	BaseDate     string   `json:"base_date"`
	Description  string   `json:"description"`
	Status       string   `json:"status"`
	// 下载模式：更新域名→路径映射（会同步到 KV）
	DomainPaths map[string]string `json:"domain_paths"`
}

// buildTargetsFromRequest 从请求中得到目标链接列表（至少一个）；下载模式不需要
func buildTargetsFromCreate(req *CreateWorkerRequest) ([]string, error) {
	if req.BusinessMode == "下载" {
		return nil, nil // 下载模式用 R2+KV，不校验 targets
	}
	if len(req.Targets) > 0 {
		return req.Targets, nil
	}
	if req.TargetDomain != "" {
		return []string{req.TargetDomain}, nil
	}
	return nil, fmt.Errorf("请提供 target_domain 或 targets")
}

func buildTargetsFromUpdate(req *UpdateWorkerRequest, currentTargets []string, currentTargetDomain string) []string {
	if len(req.Targets) > 0 {
		return req.Targets
	}
	if req.TargetDomain != "" {
		return []string{req.TargetDomain}
	}
	if len(currentTargets) > 0 {
		return currentTargets
	}
	if currentTargetDomain != "" {
		return []string{currentTargetDomain}
	}
	return nil
}

// generateScript 根据目标与模式生成脚本
func generateScript(targets []string, fallbackURL, mode string, rotateDays int, baseDate string) (string, error) {
	if len(targets) == 0 {
		return "", fmt.Errorf("目标链接不能为空")
	}
	useSingle := mode == "" || mode == "single" || (len(targets) == 1 && (mode == "" || mode == "single"))
	if useSingle {
		return cloudflare.GenerateWorkerScript(targets[0]), nil
	}
	adv, err := cloudflare.GenerateWorkerScriptAdvanced(cloudflare.WorkerScriptConfig{
		Targets:     targets,
		FallbackURL: fallbackURL,
		Mode:        mode,
		RotateDays:  rotateDays,
		BaseDate:    baseDate,
	})
	return adv, err
}

// escapeLike 转义 LIKE 中的 % 和 _
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)
	s = strings.ReplaceAll(s, "_", `\_`)
	return s
}

// CheckWorkerDomainAvailable 检查 Worker 域名是否已被占用。若被占用返回 used_by（domain_redirect/cf_worker）、ref_id、ref_name。
// excludeWorkerID 不为 0 时，若仅被该 Worker 占用则视为可用（同一 Worker 绑定/保留该域名）。
func (s *CFWorkerService) CheckWorkerDomainAvailable(domain string, excludeWorkerID uint) (available bool, usedBy string, refID uint, refName string) {
	if domain == "" {
		return true, "", 0, ""
	}
	domain = strings.TrimSpace(strings.ToLower(domain))
	likePattern := "%\"" + escapeLike(domain) + "\"%"
	var workers []models.CFWorker
	if err := s.db.Where("LOWER(worker_domain) = ? OR (worker_domains != '' AND worker_domains LIKE ?)", domain, likePattern).Find(&workers).Error; err != nil {
		return true, "", 0, ""
	}
	for i := range workers {
		w := &workers[i]
		for _, d := range w.DomainsList() {
			if strings.ToLower(d) == domain {
				if excludeWorkerID != 0 && w.ID == excludeWorkerID {
					return true, "", 0, ""
				}
				return false, "cf_worker", w.ID, w.WorkerName
			}
		}
	}
	var dr models.DomainRedirect
	if err := s.db.Where("LOWER(source_domain) = ?", domain).First(&dr).Error; err == nil {
		return false, "domain_redirect", dr.ID, dr.SourceDomain
	}
	return true, "", 0, ""
}

// CreateWorker 创建 Worker（推广模式：重定向逻辑；下载模式：R2+KV 多域名→文件）
func (s *CFWorkerService) CreateWorker(req *CreateWorkerRequest) (*models.CFWorker, error) {
	businessMode := req.BusinessMode
	if businessMode != "下载" && businessMode != "推广" {
		businessMode = "推广"
	}

	if businessMode == "下载" {
		return s.createWorkerDownloadMode(req)
	}
	return s.createWorkerPromotionMode(req)
}

// createWorkerDownloadMode 创建「下载模式」Worker：一个 R2 桶 + 一个 KV，多域名每域名对应一个文件路径，Worker 代理 R2 对象
func (s *CFWorkerService) createWorkerDownloadMode(req *CreateWorkerRequest) (*models.CFWorker, error) {
	log := logger.GetLogger()
	if req.R2BucketID == 0 {
		return nil, fmt.Errorf("下载模式必须指定 r2_bucket_id")
	}
	if len(req.DomainPaths) == 0 {
		return nil, fmt.Errorf("下载模式必须提供 domain_paths（至少一个域名→文件路径）")
	}
	// 域名规范化并检查占用
	domainPaths := make(map[string]string)
	for domain, path := range req.DomainPaths {
		domain = strings.TrimSpace(strings.ToLower(domain))
		path = strings.TrimSpace(path)
		if domain == "" || path == "" {
			continue
		}
		domainPaths[domain] = path
	}
	if len(domainPaths) == 0 {
		return nil, fmt.Errorf("domain_paths 中至少需要一对有效的 域名→路径")
	}
	for domain := range domainPaths {
		if ok, usedBy, refID, refName := s.CheckWorkerDomainAvailable(domain, 0); !ok {
			return nil, fmt.Errorf("域名 %s 已被占用（%s，ref: %s ID %d）", domain, usedBy, refName, refID)
		}
	}

	var bucket models.R2Bucket
	if err := s.db.First(&bucket, req.R2BucketID).Error; err != nil {
		return nil, fmt.Errorf("R2 存储桶不存在: %w", err)
	}
	var cfAccount models.CFAccount
	if err := s.db.First(&cfAccount, bucket.CFAccountID).Error; err != nil {
		return nil, fmt.Errorf("CF 账号不存在: %w", err)
	}
	apiToken := cfAccount.APIToken
	accountID := cfAccount.AccountID
	cfService := cloudflare.NewWorkerAPIService(apiToken, accountID)
	kvService := cloudflare.NewKVAPIService(apiToken, accountID)

	// 创建 KV 命名空间
	kvTitle := "r2-download-" + req.WorkerName
	kvNamespaceID, err := kvService.CreateKVNamespace(kvTitle)
	if err != nil {
		return nil, fmt.Errorf("创建 KV 命名空间失败: %w", err)
	}
	// 写入 域名→路径 到 KV
	for domain, path := range domainPaths {
		if err := kvService.WriteKVEntry(kvNamespaceID, domain, path); err != nil {
			_ = kvService.DeleteKVNamespace(kvNamespaceID)
			return nil, fmt.Errorf("写入 KV 条目 %s→%s 失败: %w", domain, path, err)
		}
	}

	script := cloudflare.GenerateDownloadModeWorkerScript()
	if err := cfService.CreateWorkerWithBindings(req.WorkerName, script, bucket.BucketName, kvNamespaceID); err != nil {
		_ = kvService.DeleteKVNamespace(kvNamespaceID)
		return nil, fmt.Errorf("上传 Worker（带 R2+KV 绑定）失败: %w", err)
	}

	// 为每个域名绑定路由/自定义域名（域名列表排序以保证顺序一致）
	domainsList := make([]string, 0, len(domainPaths))
	for d := range domainPaths {
		domainsList = append(domainsList, d)
	}
	sort.Strings(domainsList)
	worker := &models.CFWorker{
		CFAccountID:    bucket.CFAccountID,
		WorkerName:     req.WorkerName,
		WorkerDomain:   domainsList[0],
		TargetDomain:   "",
		Targets:        "",
		Mode:           "single",
		BusinessMode:   "下载",
		R2BucketID:     req.R2BucketID,
		KVNamespaceID:  kvNamespaceID,
		DomainPathsMap: domainPaths,
		Status:         "active",
		Description:    req.Description,
	}
	worker.WorkerDomainsArray = domainsList
	var bindings []models.WorkerDomainBinding
	for _, domain := range domainsList {
		rootDomain := extractRootDomain(domain)
		zoneID, _ := s.getZoneID(apiToken, rootDomain)
		var customDomainID, workerRoute string
		if zoneID != "" {
			if did, err := cfService.AddWorkerCustomDomain(req.WorkerName, domain, zoneID); err == nil {
				customDomainID = did
			}
			pattern := domain + "/*"
			if routeID, err := cfService.CreateWorkerRoute(zoneID, pattern, req.WorkerName); err == nil {
				workerRoute = routeID
			}
		}
		bindings = append(bindings, models.WorkerDomainBinding{
			Domain:         domain,
			ZoneID:         zoneID,
			WorkerRoute:    workerRoute,
			CustomDomainID: customDomainID,
		})
	}
	worker.DomainBindingsArray = bindings
	if err := s.db.Create(worker).Error; err != nil {
		_ = cfService.DeleteWorker(req.WorkerName)
		_ = kvService.DeleteKVNamespace(kvNamespaceID)
		for _, b := range bindings {
			if b.ZoneID != "" && b.WorkerRoute != "" {
				_ = cfService.DeleteWorkerRoute(b.ZoneID, b.WorkerRoute)
			}
			if b.CustomDomainID != "" {
				_ = cfService.DeleteWorkerCustomDomain(b.CustomDomainID)
			}
		}
		return nil, fmt.Errorf("保存 Worker 到数据库失败: %w", err)
	}
	log.WithFields(map[string]interface{}{
		"worker_id": worker.ID, "worker_name": worker.WorkerName, "business_mode": "下载",
	}).Info("Worker（下载模式）创建完成")
	return worker, nil
}

// createWorkerPromotionMode 创建「推广模式」Worker（原有重定向/轮播逻辑）
func (s *CFWorkerService) createWorkerPromotionMode(req *CreateWorkerRequest) (*models.CFWorker, error) {
	log := logger.GetLogger()
	targets, err := buildTargetsFromCreate(req)
	if err != nil {
		return nil, err
	}

	if ok, usedBy, refID, refName := s.CheckWorkerDomainAvailable(req.WorkerDomain, 0); !ok {
		switch usedBy {
		case "cf_worker":
			return nil, fmt.Errorf("域名 %s 已被「Cloudflare Worker」使用（%s，ID: %d），请先删除该 Worker 后再创建", req.WorkerDomain, refName, refID)
		case "domain_redirect":
			return nil, fmt.Errorf("域名 %s 已被「域名302重定向」使用（%s），请先删除该重定向后再创建", req.WorkerDomain, refName)
		default:
			return nil, fmt.Errorf("域名 %s 已被占用，请先删除占用项后再创建", req.WorkerDomain)
		}
	}

	var cfAccount models.CFAccount
	if err := s.db.First(&cfAccount, req.CFAccountID).Error; err != nil {
		return nil, fmt.Errorf("CF 账号不存在: %w", err)
	}
	apiToken := cfAccount.APIToken
	cfService := cloudflare.NewWorkerAPIService(apiToken, cfAccount.AccountID)

	rootDomain := extractRootDomain(req.WorkerDomain)
	zoneID, zoneErr := s.getZoneID(apiToken, rootDomain)
	if zoneErr != nil {
		log.WithError(zoneErr).WithFields(map[string]interface{}{
			"worker_domain": req.WorkerDomain,
			"root_domain":   rootDomain,
		}).Warn("获取 Zone ID 失败")
	}

	script, err := generateScript(targets, req.FallbackURL, req.Mode, req.RotateDays, req.BaseDate)
	if err != nil {
		return nil, err
	}

	if err := cfService.CreateWorker(req.WorkerName, script); err != nil {
		return nil, fmt.Errorf("创建 Worker 脚本失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"worker_name": req.WorkerName,
	}).Info("Worker 脚本创建成功")

	// 7. 添加自定义域名（优先使用，如果有 Zone ID）
	var customDomainID string
	var workerRoute string

	if zoneID != "" {
		// 优先尝试使用自定义域名（Custom Domain）
		domainID, err := cfService.AddWorkerCustomDomain(req.WorkerName, req.WorkerDomain, zoneID)
		if err != nil {
			log.WithError(err).WithFields(map[string]interface{}{
				"worker_name":   req.WorkerName,
				"worker_domain": req.WorkerDomain,
			}).Warn("添加 Worker 自定义域名失败，尝试使用路由模式")
		} else {
			customDomainID = domainID
			log.WithFields(map[string]interface{}{
				"worker_name":      req.WorkerName,
				"worker_domain":    req.WorkerDomain,
				"custom_domain_id": domainID,
			}).Info("Worker 自定义域名添加成功")
		}

		// 如果自定义域名失败，尝试使用路由模式作为备选
		pattern := fmt.Sprintf("%s/*", req.WorkerDomain)
		routeID, routeErr := cfService.CreateWorkerRoute(zoneID, pattern, req.WorkerName)
		if routeErr != nil {
			_ = cfService.DeleteWorker(req.WorkerName)
			return nil, fmt.Errorf("Worker 域名绑定失败: 路由错误 %v", routeErr)
		}
		workerRoute = routeID
		log.WithFields(map[string]interface{}{
			"worker_name": req.WorkerName,
			"pattern":     pattern,
			"route_id":    routeID,
		}).Info("Worker 路由创建成功")
	}

	// 9. 保存到数据库
	targetsJSON := ""
	if len(targets) > 0 {
		b, _ := json.Marshal(targets)
		targetsJSON = string(b)
	}
	firstTarget := ""
	if len(targets) > 0 {
		firstTarget = targets[0]
	}
	businessMode := req.BusinessMode
	if businessMode != "下载" && businessMode != "推广" {
		businessMode = "推广"
	}
	worker := &models.CFWorker{
		CFAccountID:    req.CFAccountID,
		WorkerName:     req.WorkerName,
		WorkerDomain:   req.WorkerDomain,
		TargetDomain:   firstTarget,
		Targets:        targetsJSON,
		FallbackURL:    req.FallbackURL,
		Mode:           req.Mode,
		BusinessMode:   businessMode,
		RotateDays:     req.RotateDays,
		BaseDate:       req.BaseDate,
		ZoneID:         zoneID,
		WorkerRoute:    workerRoute,
		CustomDomainID: customDomainID,
		Status:         "active",
		Description:    req.Description,
	}
	worker.WorkerDomainsArray = []string{req.WorkerDomain}
	worker.DomainBindingsArray = []models.WorkerDomainBinding{{
		Domain:         req.WorkerDomain,
		ZoneID:         zoneID,
		WorkerRoute:    workerRoute,
		CustomDomainID: customDomainID,
	}}

	if err := s.db.Create(worker).Error; err != nil {
		// 如果数据库保存失败，删除已创建的 Worker 和路由
		_ = cfService.DeleteWorker(req.WorkerName)
		if zoneID != "" && workerRoute != "" {
			_ = cfService.DeleteWorkerRoute(zoneID, workerRoute)
		}
		return nil, fmt.Errorf("保存 Worker 到数据库失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"worker_id":   worker.ID,
		"worker_name": worker.WorkerName,
	}).Info("Worker 创建完成")

	return worker, nil
}

// GetWorkerList 获取 Worker 列表，支持按域名关键词、业务模式筛选
func (s *CFWorkerService) GetWorkerList(page, pageSize int, cfAccountID uint, domain, businessMode string) ([]models.CFWorker, int64, error) {
	var workers []models.CFWorker
	var total int64

	query := s.db.Model(&models.CFWorker{}).Preload("CFAccount")

	if cfAccountID > 0 {
		query = query.Where("cf_account_id = ?", cfAccountID)
	}
	if domain != "" {
		like := "%" + domain + "%"
		query = query.Where("worker_domain LIKE ? OR target_domain LIKE ? OR worker_domains LIKE ?", like, like, like)
	}
	if businessMode != "" && (businessMode == "下载" || businessMode == "推广") {
		query = query.Where("business_mode = ?", businessMode)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("获取 Worker 总数失败: %w", err)
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&workers).Error; err != nil {
		return nil, 0, fmt.Errorf("查询 Worker 列表失败: %w", err)
	}

	return workers, total, nil
}

// GetWorkerByID 根据 ID 获取 Worker
func (s *CFWorkerService) GetWorkerByID(id uint) (*models.CFWorker, error) {
	var worker models.CFWorker
	if err := s.db.Preload("CFAccount").First(&worker, id).Error; err != nil {
		return nil, fmt.Errorf("Worker 不存在: %w", err)
	}
	return &worker, nil
}

// UpdateWorker 更新 Worker
func (s *CFWorkerService) UpdateWorker(id uint, req *UpdateWorkerRequest) (*models.CFWorker, error) {
	log := logger.GetLogger()

	worker, err := s.GetWorkerByID(id)
	if err != nil {
		return nil, err
	}

	var cfAccount models.CFAccount
	if err := s.db.First(&cfAccount, worker.CFAccountID).Error; err != nil {
		return nil, fmt.Errorf("CF 账号不存在: %w", err)
	}

	currentTargets := worker.TargetsList()
	needScriptUpdate := false
	newTargets := buildTargetsFromUpdate(req, currentTargets, worker.TargetDomain)
	if len(newTargets) > 0 {
		newFirst := newTargets[0]
		if newFirst != worker.TargetDomain {
			worker.TargetDomain = newFirst
			needScriptUpdate = true
		}
		targetsJSON, _ := json.Marshal(newTargets)
		if string(targetsJSON) != worker.Targets {
			worker.Targets = string(targetsJSON)
			needScriptUpdate = true
		}
	}
	if req.FallbackURL != worker.FallbackURL {
		worker.FallbackURL = req.FallbackURL
		needScriptUpdate = true
	}
	mode := req.Mode
	if mode == "" && worker.Mode != "" {
		mode = worker.Mode
	}
	if mode != worker.Mode {
		worker.Mode = mode
		needScriptUpdate = true
	}
	if req.RotateDays != 0 || worker.RotateDays != 0 {
		if req.RotateDays != worker.RotateDays {
			worker.RotateDays = req.RotateDays
			needScriptUpdate = true
		}
	}
	if req.BaseDate != worker.BaseDate {
		worker.BaseDate = req.BaseDate
		needScriptUpdate = true
	}
	if req.BusinessMode != "" && (req.BusinessMode == "下载" || req.BusinessMode == "推广") {
		worker.BusinessMode = req.BusinessMode
	}

	// 下载模式：同步 domain_paths 到 KV
	if worker.BusinessMode == "下载" && worker.KVNamespaceID != "" && len(req.DomainPaths) > 0 {
		kvService := cloudflare.NewKVAPIService(cfAccount.APIToken, cfAccount.AccountID)
		for domain, path := range req.DomainPaths {
			domain = strings.TrimSpace(strings.ToLower(domain))
			path = strings.TrimSpace(path)
			if domain == "" || path == "" {
				continue
			}
			if err := kvService.WriteKVEntry(worker.KVNamespaceID, domain, path); err != nil {
				log.WithError(err).WithFields(map[string]interface{}{"domain": domain, "path": path}).Warn("同步 KV 条目失败")
			}
		}
		worker.DomainPathsMap = req.DomainPaths
	}

	if needScriptUpdate && len(newTargets) > 0 {
		script, err := generateScript(newTargets, worker.FallbackURL, worker.Mode, worker.RotateDays, worker.BaseDate)
		if err != nil {
			return nil, err
		}
		cfService := cloudflare.NewWorkerAPIService(cfAccount.APIToken, cfAccount.AccountID)
		if err := cfService.CreateWorker(worker.WorkerName, script); err != nil {
			return nil, fmt.Errorf("更新 Worker 脚本失败: %w", err)
		}
		log.WithFields(map[string]interface{}{"worker_name": worker.WorkerName}).Info("Worker 脚本更新成功")
	}

	if req.Description != "" {
		worker.Description = req.Description
	}
	if req.Status != "" {
		worker.Status = req.Status
	}

	if err := s.db.Save(worker).Error; err != nil {
		return nil, fmt.Errorf("更新 Worker 失败: %w", err)
	}
	return worker, nil
}

// DeleteWorker 删除 Worker
func (s *CFWorkerService) DeleteWorker(id uint) error {
	log := logger.GetLogger()

	// 1. 获取 Worker 信息
	worker, err := s.GetWorkerByID(id)
	if err != nil {
		return err
	}

	// 2. 获取 CF 账号信息
	var cfAccount models.CFAccount
	if err := s.db.First(&cfAccount, worker.CFAccountID).Error; err != nil {
		return fmt.Errorf("CF 账号不存在: %w", err)
	}

	// 3. 使用 API Token（已明文存储）
	apiToken := cfAccount.APIToken

	// 4. 创建 Cloudflare Service 实例
	cfService := cloudflare.NewWorkerAPIService(apiToken, cfAccount.AccountID)

	// 5. 删除所有域名的 Worker 路由与自定义域名（优先使用 DomainBindingsArray，兼容旧数据）
	if len(worker.DomainBindingsArray) > 0 {
		for _, b := range worker.DomainBindingsArray {
			if b.ZoneID != "" && b.WorkerRoute != "" {
				if err := cfService.DeleteWorkerRoute(b.ZoneID, b.WorkerRoute); err != nil {
					log.WithError(err).WithFields(map[string]interface{}{
						"worker_id": worker.ID, "domain": b.Domain, "zone_id": b.ZoneID, "worker_route": b.WorkerRoute,
					}).Warn("删除 Worker 路由失败，继续")
				}
			}
			if b.CustomDomainID != "" {
				if err := cfService.DeleteWorkerCustomDomain(b.CustomDomainID); err != nil {
					log.WithError(err).WithFields(map[string]interface{}{
						"worker_id": worker.ID, "domain": b.Domain, "custom_domain_id": b.CustomDomainID,
					}).Warn("删除 Worker 自定义域名失败，继续")
				}
			}
		}
	} else {
		if worker.ZoneID != "" && worker.WorkerRoute != "" {
			if err := cfService.DeleteWorkerRoute(worker.ZoneID, worker.WorkerRoute); err != nil {
				log.WithError(err).WithFields(map[string]interface{}{
					"worker_id": worker.ID, "zone_id": worker.ZoneID, "worker_route": worker.WorkerRoute,
				}).Warn("删除 Worker 路由失败，继续删除 Worker 脚本")
			}
		}
		if worker.CustomDomainID != "" {
			if err := cfService.DeleteWorkerCustomDomain(worker.CustomDomainID); err != nil {
				log.WithError(err).WithFields(map[string]interface{}{
					"worker_id": worker.ID, "custom_domain_id": worker.CustomDomainID,
				}).Warn("删除 Worker 自定义域名失败，继续删除 Worker 脚本")
			}
		}
	}

	// 6. 下载模式：删除 KV 命名空间
	if worker.KVNamespaceID != "" {
		kvService := cloudflare.NewKVAPIService(apiToken, cfAccount.AccountID)
		if err := kvService.DeleteKVNamespace(worker.KVNamespaceID); err != nil {
			log.WithError(err).WithField("kv_namespace_id", worker.KVNamespaceID).Warn("删除 KV 命名空间失败，继续")
		}
	}

	// 7. 删除 Worker 脚本
	if err := cfService.DeleteWorker(worker.WorkerName); err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"worker_id":   worker.ID,
			"worker_name": worker.WorkerName,
		}).Warn("删除 Worker 脚本失败")
		// 继续删除数据库记录
	}

	// 8. 从数据库删除
	if err := s.db.Delete(worker).Error; err != nil {
		return fmt.Errorf("删除 Worker 数据库记录失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"worker_id":   worker.ID,
		"worker_name": worker.WorkerName,
	}).Info("Worker 删除成功")

	return nil
}

// BindWorkerDomain 为 Worker 绑定新域名（CF 添加路由/自定义域名 + 写入库）。下载模式时 filePath 必填，会写入 KV。
func (s *CFWorkerService) BindWorkerDomain(workerID uint, domain, filePath string) (*models.CFWorker, error) {
	log := logger.GetLogger()
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return nil, fmt.Errorf("域名不能为空")
	}

	worker, err := s.GetWorkerByID(workerID)
	if err != nil {
		return nil, err
	}
	for _, d := range worker.DomainsList() {
		if d == domain {
			// 已绑定：下载模式下若提供了 filePath 则更新 KV
			if worker.BusinessMode == "下载" && worker.KVNamespaceID != "" && strings.TrimSpace(filePath) != "" {
				var cfAccount models.CFAccount
				if s.db.First(&cfAccount, worker.CFAccountID).Error == nil {
					kvService := cloudflare.NewKVAPIService(cfAccount.APIToken, cfAccount.AccountID)
					_ = kvService.WriteKVEntry(worker.KVNamespaceID, domain, strings.TrimSpace(filePath))
				}
				if worker.DomainPathsMap == nil {
					worker.DomainPathsMap = make(map[string]string)
				}
				worker.DomainPathsMap[domain] = strings.TrimSpace(filePath)
				_ = s.db.Save(worker).Error
			}
			return worker, nil
		}
	}

	if worker.BusinessMode == "下载" && worker.KVNamespaceID != "" && strings.TrimSpace(filePath) == "" {
		return nil, fmt.Errorf("下载模式绑定域名时必须提供 file_path（该域名对应的 R2 文件路径）")
	}

	if ok, usedBy, refID, refName := s.CheckWorkerDomainAvailable(domain, workerID); !ok {
		switch usedBy {
		case "cf_worker":
			return nil, fmt.Errorf("域名 %s 已被「Cloudflare Worker」使用（%s，ID: %d）", domain, refName, refID)
		case "domain_redirect":
			return nil, fmt.Errorf("域名 %s 已被「域名302重定向」使用（%s）", domain, refName)
		default:
			return nil, fmt.Errorf("域名 %s 已被占用", domain)
		}
	}

	var cfAccount models.CFAccount
	if err := s.db.First(&cfAccount, worker.CFAccountID).Error; err != nil {
		return nil, fmt.Errorf("CF 账号不存在: %w", err)
	}
	cfService := cloudflare.NewWorkerAPIService(cfAccount.APIToken, cfAccount.AccountID)

	// 下载模式：写入 KV 域名→路径
	if worker.BusinessMode == "下载" && worker.KVNamespaceID != "" {
		path := strings.TrimSpace(filePath)
		if path == "" {
			return nil, fmt.Errorf("下载模式绑定域名时必须提供 file_path")
		}
		kvService := cloudflare.NewKVAPIService(cfAccount.APIToken, cfAccount.AccountID)
		if err := kvService.WriteKVEntry(worker.KVNamespaceID, domain, path); err != nil {
			return nil, fmt.Errorf("写入 KV 域名→路径失败: %w", err)
		}
		if worker.DomainPathsMap == nil {
			worker.DomainPathsMap = make(map[string]string)
		}
		worker.DomainPathsMap[domain] = path
	}

	rootDomain := extractRootDomain(domain)
	zoneID, zoneErr := s.getZoneID(cfAccount.APIToken, rootDomain)
	if zoneErr != nil {
		log.WithError(zoneErr).WithField("domain", domain).Warn("获取 Zone ID 失败")
	}

	var customDomainID, workerRoute string
	if zoneID != "" {
		if domainID, err := cfService.AddWorkerCustomDomain(worker.WorkerName, domain, zoneID); err != nil {
			log.WithError(err).WithField("domain", domain).Warn("添加 Worker 自定义域名失败")
		} else {
			customDomainID = domainID
		}
		pattern := fmt.Sprintf("%s/*", domain)
		routeID, routeErr := cfService.CreateWorkerRoute(zoneID, pattern, worker.WorkerName)
		if routeErr != nil {
			return nil, fmt.Errorf("Worker 域名绑定失败: %w", routeErr)
		}
		workerRoute = routeID
	}

	worker.BindDomain(domain)
	worker.SetBinding(models.WorkerDomainBinding{
		Domain:         domain,
		ZoneID:         zoneID,
		WorkerRoute:    workerRoute,
		CustomDomainID: customDomainID,
	})
	if err := s.db.Save(worker).Error; err != nil {
		if zoneID != "" && workerRoute != "" {
			_ = cfService.DeleteWorkerRoute(zoneID, workerRoute)
		}
		if customDomainID != "" {
			_ = cfService.DeleteWorkerCustomDomain(customDomainID)
		}
		return nil, fmt.Errorf("保存 Worker 失败: %w", err)
	}
	log.WithFields(map[string]interface{}{"worker_id": workerID, "domain": domain}).Info("Worker 域名绑定成功")
	return worker, nil
}

// UnbindWorkerDomain 解绑 Worker 的指定域名（CF 删除路由/自定义域名 + 更新库）
func (s *CFWorkerService) UnbindWorkerDomain(workerID uint, domain string) (*models.CFWorker, error) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return nil, fmt.Errorf("域名不能为空")
	}

	worker, err := s.GetWorkerByID(workerID)
	if err != nil {
		return nil, err
	}
	binding, ok := worker.GetBinding(domain)
	if !ok {
		// 可能只存在于 DomainsList 但无 binding（旧数据）
		for _, d := range worker.DomainsList() {
			if d == domain {
				worker.UnbindDomain(domain)
				_ = s.db.Save(worker).Error
				return worker, nil
			}
		}
		return nil, fmt.Errorf("该 Worker 未绑定域名 %s", domain)
	}

	cfAccount := new(models.CFAccount)
	if err := s.db.First(cfAccount, worker.CFAccountID).Error; err != nil {
		return nil, fmt.Errorf("CF 账号不存在: %w", err)
	}
	cfService := cloudflare.NewWorkerAPIService(cfAccount.APIToken, cfAccount.AccountID)

	log := logger.GetLogger()
	if binding.ZoneID != "" && binding.WorkerRoute != "" {
		if err := cfService.DeleteWorkerRoute(binding.ZoneID, binding.WorkerRoute); err != nil {
			log.WithError(err).WithFields(map[string]interface{}{"worker_id": workerID, "domain": domain}).Warn("删除 Worker 路由失败")
		}
	}
	if binding.CustomDomainID != "" {
		if err := cfService.DeleteWorkerCustomDomain(binding.CustomDomainID); err != nil {
			log.WithError(err).WithFields(map[string]interface{}{"worker_id": workerID, "domain": domain}).Warn("删除 Worker 自定义域名失败")
		}
	}

	worker.UnbindDomain(domain)
	if err := s.db.Save(worker).Error; err != nil {
		return nil, fmt.Errorf("保存 Worker 失败: %w", err)
	}
	log.WithFields(map[string]interface{}{"worker_id": workerID, "domain": domain}).Info("Worker 域名解绑成功")
	return worker, nil
}

// getZoneID 获取 Zone ID
func (s *CFWorkerService) getZoneID(apiToken, domainName string) (string, error) {
	log := logger.GetLogger()

	// 直接使用 HTTP 客户端来调用 Cloudflare API
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones?name=%s", domainName)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"domain":      domainName,
		"status_code": resp.StatusCode,
		"response":    string(body),
	}).Info("获取 Zone ID 响应")

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API请求失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Success bool `json:"success"`
		Result  []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if !result.Success {
		if len(result.Errors) > 0 {
			return "", fmt.Errorf("Cloudflare API错误: %s", result.Errors[0].Message)
		}
		return "", fmt.Errorf("获取Zone ID失败")
	}

	if len(result.Result) == 0 {
		return "", fmt.Errorf("未找到域名 %s 的Zone，请确保该域名已添加到 Cloudflare", domainName)
	}

	zoneID := result.Result[0].ID
	log.WithFields(map[string]interface{}{
		"domain":  domainName,
		"zone_id": zoneID,
	}).Info("成功获取 Zone ID")

	return zoneID, nil
}
