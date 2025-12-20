package services

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/models"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

type AuditService struct {
	db *gorm.DB
}

func NewAuditService(db *gorm.DB) *AuditService {
	return &AuditService{db: db}
}

// LogAudit 记录审计日志
func (s *AuditService) LogAudit(logEntry *models.AuditLog) error {
	log := logger.GetLogger()

	// 记录到数据库
	if err := s.db.Create(logEntry).Error; err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"action":      logEntry.Action,
			"resource":    logEntry.Resource,
			"resource_id": logEntry.ResourceID,
			"user_id":     logEntry.UserID,
			"username":    logEntry.Username,
		}).Error("保存审计日志失败")
		return err
	}

	// 同时记录到日志文件
	log.WithFields(map[string]interface{}{
		"type":        "audit",
		"action":      logEntry.Action,
		"resource":    logEntry.Resource,
		"resource_id": logEntry.ResourceID,
		"user_id":     logEntry.UserID,
		"username":    logEntry.Username,
		"method":      logEntry.Method,
		"path":        logEntry.Path,
		"ip":          logEntry.IP,
		"status":      logEntry.Status,
		"duration_ms": logEntry.Duration,
		"message":     logEntry.Message,
	}).Info("审计日志")

	return nil
}

// CreateAuditLog 创建审计日志条目
func (s *AuditService) CreateAuditLog(userID uint, username, action, resource, resourceID, method, path, ip, userAgent string, request, response interface{}, status int, message string, err error, duration time.Duration) *models.AuditLog {
	logEntry := &models.AuditLog{
		UserID:     userID,
		Username:   username,
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Method:     method,
		Path:       path,
		IP:         ip,
		UserAgent:  userAgent,
		Status:     status,
		Message:    message,
		Duration:   duration.Milliseconds(),
		CreatedAt:  time.Now(),
	}

	// 序列化请求数据
	if request != nil {
		if reqBytes, err := json.Marshal(request); err == nil {
			logEntry.Request = string(reqBytes)
		}
	}

	// 序列化响应数据（只记录关键信息，避免记录大量数据）
	if response != nil {
		if respBytes, err := json.Marshal(response); err == nil {
			// 限制响应数据长度，避免记录过大的数据
			respStr := string(respBytes)
			if len(respStr) > 10000 {
				respStr = respStr[:10000] + "...(truncated)"
			}
			logEntry.Response = respStr
		}
	}

	// 记录错误信息
	if err != nil {
		logEntry.Error = err.Error()
	}

	return logEntry
}

// QueryAuditLogs 查询审计日志
func (s *AuditService) QueryAuditLogs(page, pageSize int, filters map[string]interface{}) ([]models.AuditLog, int64, error) {
	var logs []models.AuditLog
	var total int64

	query := s.db.Model(&models.AuditLog{})

	// 应用过滤条件
	if userID, ok := filters["user_id"].(uint); ok && userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if username, ok := filters["username"].(string); ok && username != "" {
		query = query.Where("username LIKE ?", "%"+username+"%")
	}
	if action, ok := filters["action"].(string); ok && action != "" {
		query = query.Where("action = ?", action)
	}
	if resource, ok := filters["resource"].(string); ok && resource != "" {
		query = query.Where("resource = ?", resource)
	}
	if resourceID, ok := filters["resource_id"].(string); ok && resourceID != "" {
		query = query.Where("resource_id = ?", resourceID)
	}
	if ip, ok := filters["ip"].(string); ok && ip != "" {
		query = query.Where("ip = ?", ip)
	}
	if startTime, ok := filters["start_time"].(time.Time); ok {
		query = query.Where("created_at >= ?", startTime)
	}
	if endTime, ok := filters["end_time"].(time.Time); ok {
		query = query.Where("created_at <= ?", endTime)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}
