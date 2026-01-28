package services

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/models"
	"aws_cdn/internal/services/cloudflare"
	"fmt"
	"strings"

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
	CFAccountID  uint   `json:"cf_account_id" binding:"required"`
	WorkerName   string `json:"worker_name" binding:"required"`
	WorkerDomain string `json:"worker_domain" binding:"required"`
	TargetDomain string `json:"target_domain" binding:"required"`
	Description  string `json:"description"`
}

// UpdateWorkerRequest 更新 Worker 请求
type UpdateWorkerRequest struct {
	TargetDomain string `json:"target_domain" binding:"required"`
	Description  string `json:"description"`
	Status       string `json:"status"`
}

// CreateWorker 创建 Worker
func (s *CFWorkerService) CreateWorker(req *CreateWorkerRequest) (*models.CFWorker, error) {
	log := logger.GetLogger()

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
	// 从 worker_domain 中提取根域名
	rootDomain := extractRootDomain(req.WorkerDomain)
	
	// 尝试获取 Zone ID
	zoneID, zoneErr := s.getZoneID(apiToken, rootDomain)
	if zoneErr != nil {
		log.WithError(zoneErr).WithFields(map[string]interface{}{
			"worker_domain": req.WorkerDomain,
			"root_domain":   rootDomain,
		}).Warn("获取 Zone ID 失败")
		// 不阻止创建，继续执行
	}

	// 5. 生成 Worker 脚本
	script := cloudflare.GenerateWorkerScript(req.TargetDomain)

	// 6. 创建 Worker 脚本
	if err := cfService.CreateWorker(req.WorkerName, script); err != nil {
		return nil, fmt.Errorf("创建 Worker 脚本失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"worker_name": req.WorkerName,
	}).Info("Worker 脚本创建成功")

	// 7. 创建 Worker 路由（如果有 Zone ID）
	var workerRoute string
	if zoneID != "" {
		pattern := fmt.Sprintf("%s/*", req.WorkerDomain)
		routeID, err := cfService.CreateWorkerRoute(zoneID, pattern, req.WorkerName)
		if err != nil {
			// 如果路由创建失败，删除已创建的 Worker
			_ = cfService.DeleteWorker(req.WorkerName)
			return nil, fmt.Errorf("创建 Worker 路由失败: %w", err)
		}
		workerRoute = routeID

		log.WithFields(map[string]interface{}{
			"worker_name": req.WorkerName,
			"pattern":     pattern,
			"route_id":    routeID,
		}).Info("Worker 路由创建成功")
	}

	// 8. 保存到数据库
	worker := &models.CFWorker{
		CFAccountID:  req.CFAccountID,
		WorkerName:   req.WorkerName,
		WorkerDomain: req.WorkerDomain,
		TargetDomain: req.TargetDomain,
		ZoneID:       zoneID,
		WorkerRoute:  workerRoute,
		Status:       "active",
		Description:  req.Description,
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

// GetWorkerList 获取 Worker 列表
func (s *CFWorkerService) GetWorkerList(page, pageSize int, cfAccountID uint) ([]models.CFWorker, int64, error) {
	var workers []models.CFWorker
	var total int64

	query := s.db.Model(&models.CFWorker{}).Preload("CFAccount")
	
	if cfAccountID > 0 {
		query = query.Where("cf_account_id = ?", cfAccountID)
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

	// 1. 获取 Worker 信息
	worker, err := s.GetWorkerByID(id)
	if err != nil {
		return nil, err
	}

	// 2. 获取 CF 账号信息
	var cfAccount models.CFAccount
	if err := s.db.First(&cfAccount, worker.CFAccountID).Error; err != nil {
		return nil, fmt.Errorf("CF 账号不存在: %w", err)
	}

	// 3. 使用 API Token（已明文存储）
	apiToken := cfAccount.APIToken

	// 4. 如果目标域名变化，需要更新 Worker 脚本
	if req.TargetDomain != "" && req.TargetDomain != worker.TargetDomain {
		cfService := cloudflare.NewWorkerAPIService(apiToken, cfAccount.AccountID)
		script := cloudflare.GenerateWorkerScript(req.TargetDomain)

		if err := cfService.CreateWorker(worker.WorkerName, script); err != nil {
			return nil, fmt.Errorf("更新 Worker 脚本失败: %w", err)
		}

		log.WithFields(map[string]interface{}{
			"worker_name":   worker.WorkerName,
			"target_domain": req.TargetDomain,
		}).Info("Worker 脚本更新成功")

		worker.TargetDomain = req.TargetDomain
	}

	// 5. 更新其他字段
	if req.Description != "" {
		worker.Description = req.Description
	}
	if req.Status != "" {
		worker.Status = req.Status
	}

	// 6. 保存到数据库
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

	// 6. 删除 Worker 脚本
	if err := cfService.DeleteWorker(worker.WorkerName); err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"worker_id":   worker.ID,
			"worker_name": worker.WorkerName,
		}).Warn("删除 Worker 脚本失败")
		// 继续删除数据库记录
	}

	// 7. 从数据库删除
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
	// 为了简化实现，这里暂时返回空字符串
	// 在实际使用时，Worker 路由可以在 Cloudflare Dashboard 手动配置
	// 或者后续扩展实现完整的 Zone ID 查询逻辑
	return "", nil
}

// extractRootDomain 提取根域名
func extractRootDomain(domain string) string {
	// 移除协议前缀
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "https://")
	
	// 移除路径
	if idx := strings.Index(domain, "/"); idx != -1 {
		domain = domain[:idx]
	}
	
	// 移除端口
	if idx := strings.Index(domain, ":"); idx != -1 {
		domain = domain[:idx]
	}
	
	// 分割域名
	parts := strings.Split(domain, ".")
	if len(parts) <= 2 {
		return domain
	}
	
	// 返回最后两部分（根域名）
	return strings.Join(parts[len(parts)-2:], ".")
}
