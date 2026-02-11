package services

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/models"
	"aws_cdn/internal/services/cloudflare"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	TargetDomain string   `json:"target_domain"`   // 单链接时使用；与 targets 二选一
	Targets      []string `json:"targets"`          // 多目标链接（轮播/探针时使用）
	FallbackURL  string   `json:"fallback_url"`    // 兜底链接（可选）
	Mode         string   `json:"mode"`            // single / time / random / probe
	RotateDays   int      `json:"rotate_days"`    // 时间轮播每 N 天
	BaseDate     string   `json:"base_date"`       // 时间轮播基准日期 ISO
	Description  string   `json:"description"`
}

// UpdateWorkerRequest 更新 Worker 请求
type UpdateWorkerRequest struct {
	TargetDomain string   `json:"target_domain"`
	Targets      []string `json:"targets"`
	FallbackURL  string   `json:"fallback_url"`
	Mode         string   `json:"mode"`
	RotateDays   int      `json:"rotate_days"`
	BaseDate     string   `json:"base_date"`
	Description  string   `json:"description"`
	Status       string   `json:"status"`
}

// buildTargetsFromRequest 从请求中得到目标链接列表（至少一个）
func buildTargetsFromCreate(req *CreateWorkerRequest) ([]string, error) {
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

// CheckWorkerDomainAvailable 检查 Worker 域名是否已被占用。若被占用返回 used_by（domain_redirect/cf_worker）、ref_id、ref_name。
func (s *CFWorkerService) CheckWorkerDomainAvailable(domain string) (available bool, usedBy string, refID uint, refName string) {
	if domain == "" {
		return true, "", 0, ""
	}
	var w models.CFWorker
	if err := s.db.Where("LOWER(worker_domain) = LOWER(?)", domain).First(&w).Error; err == nil {
		return false, "cf_worker", w.ID, w.WorkerName
	}
	var dr models.DomainRedirect
	if err := s.db.Where("LOWER(source_domain) = LOWER(?)", domain).First(&dr).Error; err == nil {
		return false, "domain_redirect", dr.ID, dr.SourceDomain
	}
	return true, "", 0, ""
}

// CreateWorker 创建 Worker
func (s *CFWorkerService) CreateWorker(req *CreateWorkerRequest) (*models.CFWorker, error) {
	log := logger.GetLogger()

	targets, err := buildTargetsFromCreate(req)
	if err != nil {
		return nil, err
	}

	// 预先检查 Worker 域名是否已被占用（Worker 或 302 重定向）
	if ok, usedBy, refID, refName := s.CheckWorkerDomainAvailable(req.WorkerDomain); !ok {
		switch usedBy {
		case "cf_worker":
			return nil, fmt.Errorf("域名 %s 已被「Cloudflare Worker」使用（%s，ID: %d），请先删除该 Worker 后再创建", req.WorkerDomain, refName, refID)
		case "domain_redirect":
			return nil, fmt.Errorf("域名 %s 已被「域名302重定向」使用（%s），请先删除该重定向后再创建", req.WorkerDomain, refName)
		default:
			return nil, fmt.Errorf("域名 %s 已被占用，请先删除占用项后再创建", req.WorkerDomain)
		}
	}

	// 1. 获取 CF 账号信息
	var cfAccount models.CFAccount
	if err := s.db.First(&cfAccount, req.CFAccountID).Error; err != nil {
		return nil, fmt.Errorf("CF 账号不存在: %w", err)
	}

	// 2. 使用 API Token（已明文存储）
	apiToken := cfAccount.APIToken

	// 3. 创建 Cloudflare Service 实例
	cfService := cloudflare.NewWorkerAPIService(apiToken, cfAccount.AccountID)

	// 4. 获取 Worker 域名的 Zone ID
	var zoneID string
	rootDomain := extractRootDomain(req.WorkerDomain)
	zoneID, zoneErr := s.getZoneID(apiToken, rootDomain)
	if zoneErr != nil {
		log.WithError(zoneErr).WithFields(map[string]interface{}{
			"worker_domain": req.WorkerDomain,
			"root_domain":   rootDomain,
		}).Warn("获取 Zone ID 失败")
	}

	// 5. 生成 Worker 脚本
	script, err := generateScript(targets, req.FallbackURL, req.Mode, req.RotateDays, req.BaseDate)
	if err != nil {
		return nil, err
	}

	// 6. 创建 Worker 脚本
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
	worker := &models.CFWorker{
		CFAccountID:    req.CFAccountID,
		WorkerName:     req.WorkerName,
		WorkerDomain:   req.WorkerDomain,
		TargetDomain:   firstTarget,
		Targets:        targetsJSON,
		FallbackURL:    req.FallbackURL,
		Mode:           req.Mode,
		RotateDays:     req.RotateDays,
		BaseDate:       req.BaseDate,
		ZoneID:         zoneID,
		WorkerRoute:    workerRoute,
		CustomDomainID: customDomainID,
		Status:         "active",
		Description:    req.Description,
	}

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

// GetWorkerList 获取 Worker 列表，支持按域名关键词筛选（Worker 域名或目标域名）
func (s *CFWorkerService) GetWorkerList(page, pageSize int, cfAccountID uint, domain string) ([]models.CFWorker, int64, error) {
	var workers []models.CFWorker
	var total int64

	query := s.db.Model(&models.CFWorker{}).Preload("CFAccount")

	if cfAccountID > 0 {
		query = query.Where("cf_account_id = ?", cfAccountID)
	}
	if domain != "" {
		like := "%" + domain + "%"
		query = query.Where("worker_domain LIKE ? OR target_domain LIKE ?", like, like)
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

	// 5. 删除 Worker 路由（如果存在）
	if worker.ZoneID != "" && worker.WorkerRoute != "" {
		if err := cfService.DeleteWorkerRoute(worker.ZoneID, worker.WorkerRoute); err != nil {
			log.WithError(err).WithFields(map[string]interface{}{
				"worker_id":    worker.ID,
				"zone_id":      worker.ZoneID,
				"worker_route": worker.WorkerRoute,
			}).Warn("删除 Worker 路由失败，继续删除 Worker 脚本")
		}
	}

	// 6. 删除自定义域名（如果存在）
	if worker.CustomDomainID != "" {
		if err := cfService.DeleteWorkerCustomDomain(worker.CustomDomainID); err != nil {
			log.WithError(err).WithFields(map[string]interface{}{
				"worker_id":        worker.ID,
				"custom_domain_id": worker.CustomDomainID,
			}).Warn("删除 Worker 自定义域名失败，继续删除 Worker 脚本")
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
