package services

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/models"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type SpeedProbeService struct {
	db                   *gorm.DB
	telegram             *TelegramService
	speedThreshold       float64 // 速度阈值（KB/s）
	failureRateThreshold float64 // 失败率阈值（0-1）
}

// IPDetail IP探测详情
type IPDetail struct {
	IP         string  `json:"ip"`
	Probes     int     `json:"probes"`
	FailedRate float64 `json:"failed_rate"`
	AvgSpeed   float64 `json:"avg_speed"`
	Status     string  `json:"status"` // "达标" or "未达标"
}

func NewSpeedProbeService(db *gorm.DB, telegram *TelegramService, speedThreshold, failureRateThreshold float64) *SpeedProbeService {
	// 设置默认值
	if speedThreshold <= 0 {
		speedThreshold = 100.0 // 默认100 KB/s
	}
	if failureRateThreshold <= 0 || failureRateThreshold > 1 {
		failureRateThreshold = 0.5 // 默认50%失败率
	}

	return &SpeedProbeService{
		db:                   db,
		telegram:             telegram,
		speedThreshold:       speedThreshold,
		failureRateThreshold: failureRateThreshold,
	}
}

// ReportProbeResult 上报探测结果
func (s *SpeedProbeService) ReportProbeResult(result *models.SpeedProbeResult) error {
	log := logger.GetLogger()

	// 验证必填字段
	if result.URL == "" {
		return fmt.Errorf("URL不能为空")
	}
	if result.ClientIP == "" {
		return fmt.Errorf("客户端IP不能为空")
	}

	// 保存到数据库
	if err := s.db.Create(result).Error; err != nil {
		log.WithError(err).Error("保存探测结果失败")
		return fmt.Errorf("保存探测结果失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"result_id": result.ID,
		"client_ip": result.ClientIP,
		"url":       result.URL,
		"speed":     result.SpeedKbps,
		"status":    result.Status,
	}).Info("探测结果已保存")

	return nil
}

// BatchReportProbeResults 批量上报探测结果
func (s *SpeedProbeService) BatchReportProbeResults(results []models.SpeedProbeResult) error {
	log := logger.GetLogger()

	if len(results) == 0 {
		return fmt.Errorf("没有探测结果")
	}

	// 验证所有结果
	for i, result := range results {
		if result.URL == "" {
			return fmt.Errorf("第%d个结果的URL不能为空", i+1)
		}
		if result.ClientIP == "" {
			return fmt.Errorf("第%d个结果的客户端IP不能为空", i+1)
		}
	}

	// 批量保存
	if err := s.db.Create(&results).Error; err != nil {
		log.WithError(err).Error("批量保存探测结果失败")
		return fmt.Errorf("批量保存探测结果失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"count": len(results),
	}).Info("批量探测结果已保存")

	return nil
}

// CheckAndPrepareAlertForURL 检查指定URL的探测结果并准备告警（如果需要）
// 返回需要发送的告警记录，如果不需要告警则返回nil
func (s *SpeedProbeService) CheckAndPrepareAlertForURL(url string, timeWindowMinutes int) (*models.SpeedAlertLog, error) {
	log := logger.GetLogger()

	// 计算时间窗口
	windowEnd := time.Now()
	windowStart := windowEnd.Add(-time.Duration(timeWindowMinutes) * time.Minute)

	// 查询该URL在时间窗口内的所有探测结果，按IP分组
	var results []models.SpeedProbeResult
	if err := s.db.Where("url = ? AND created_at >= ? AND created_at <= ?",
		url, windowStart, windowEnd).
		Find(&results).Error; err != nil {
		log.WithError(err).Error("查询探测结果失败")
		return nil, fmt.Errorf("查询探测结果失败: %w", err)
	}

	// 如果没有探测结果，直接返回
	if len(results) == 0 {
		log.WithFields(map[string]interface{}{
			"url":            url,
			"window_minutes": timeWindowMinutes,
		}).Debug("该URL在时间窗口内没有探测结果")
		return nil, nil
	}

	// 按IP分组统计
	type IPStats struct {
		IP           string
		TotalProbes  int
		FailedProbes int
		TotalSpeed   float64
		SuccessCount int
		IsFailed     bool // 该IP对该URL的探测是否被判定为失败
	}

	ipStatsMap := make(map[string]*IPStats)
	for _, result := range results {
		if _, exists := ipStatsMap[result.ClientIP]; !exists {
			ipStatsMap[result.ClientIP] = &IPStats{
				IP: result.ClientIP,
			}
		}

		stats := ipStatsMap[result.ClientIP]
		stats.TotalProbes++

		// 判断该次探测是否失败
		isFailed := result.Status == models.SpeedProbeStatusFailed ||
			result.Status == models.SpeedProbeStatusTimeout ||
			result.SpeedKbps < s.speedThreshold

		if isFailed {
			stats.FailedProbes++
		} else {
			stats.TotalSpeed += result.SpeedKbps
			stats.SuccessCount++
		}
	}

	// 判断每个IP是否未达标（失败率 >= failureRateThreshold）
	totalIPs := len(ipStatsMap)
	failedIPs := 0
	successIPs := 0
	var totalAvgSpeed float64
	ipDetailsCount := 0

	// 收集IP详情用于告警消息
	var ipDetails []IPDetail

	for _, stats := range ipStatsMap {
		failRate := float64(stats.FailedProbes) / float64(stats.TotalProbes)
		var avgSpeed float64
		if stats.SuccessCount > 0 {
			avgSpeed = stats.TotalSpeed / float64(stats.SuccessCount)
			totalAvgSpeed += avgSpeed
			ipDetailsCount++
		}

		detail := IPDetail{
			IP:         stats.IP,
			Probes:     stats.TotalProbes,
			FailedRate: failRate * 100,
			AvgSpeed:   avgSpeed,
		}

		// 判断该IP是否未达标
		if failRate >= s.failureRateThreshold {
			stats.IsFailed = true
			failedIPs++
			detail.Status = "未达标"
		} else {
			successIPs++
			detail.Status = "达标"
		}

		ipDetails = append(ipDetails, detail)
	}

	// 计算未达标IP的比例
	failedIPRate := float64(failedIPs) / float64(totalIPs)

	// 计算全局平均速度
	var globalAvgSpeed *float64
	if ipDetailsCount > 0 {
		avg := totalAvgSpeed / float64(ipDetailsCount)
		globalAvgSpeed = &avg
	}

	log.WithFields(map[string]interface{}{
		"url":         url,
		"total_ips":   totalIPs,
		"failed_ips":  failedIPs,
		"success_ips": successIPs,
		"failed_rate": failedIPRate,
		"avg_speed":   globalAvgSpeed,
		"threshold":   s.failureRateThreshold,
	}).Info("URL探测结果分析完成")

	// 如果未达标IP超过一半，准备告警
	if failedIPRate > 0.5 { // 超过50%
		log.WithFields(map[string]interface{}{
			"url":         url,
			"failed_rate": failedIPRate,
		}).Warn("检测到超过一半的IP未达标，准备生成告警")

		// 检查是否已经发送过告警（避免重复告警）
		var existingAlert models.SpeedAlertLog
		err := s.db.Where("url = ? AND time_window_start = ? AND time_window_end = ? AND alert_sent = ?",
			url, windowStart, windowEnd, true).
			First(&existingAlert).Error

		if err == nil {
			log.WithField("url", url).Debug("该时间窗口已发送过告警，跳过")
			return nil, nil
		}

		// 序列化IP详情为JSON
		ipDetailsJSON, _ := json.Marshal(ipDetails)

		// 创建告警记录
		alertLog := &models.SpeedAlertLog{
			URL:             url,
			TimeWindowStart: windowStart,
			TimeWindowEnd:   windowEnd,
			TotalIPs:        totalIPs,
			FailedIPs:       failedIPs,
			SuccessIPs:      successIPs,
			FailedRate:      failedIPRate * 100, // 转换为百分比
			AvgSpeedKbps:    globalAvgSpeed,
			AlertSent:       false,
			IPDetails:       string(ipDetailsJSON),
		}

		// 构建告警消息
		message := s.buildAlertMessageForURL(alertLog, ipDetails, timeWindowMinutes)
		alertLog.AlertMessage = message

		// 返回告警记录，由调用方批量发送
		return alertLog, nil
	}

	return nil, nil
}

// CheckAndAlertForURL 检查指定URL的探测结果并发送告警（如果需要）
// 已废弃：现在使用 CheckAndPrepareAlertForURL 和批量发送
func (s *SpeedProbeService) CheckAndAlertForURL(url string, timeWindowMinutes int) error {
	alert, err := s.CheckAndPrepareAlertForURL(url, timeWindowMinutes)
	if err != nil {
		return err
	}

	if alert == nil {
		return nil
	}

	// 单独发送告警
	if s.telegram != nil {
		if err := s.telegram.SendMessage(alert.AlertMessage); err != nil {
			log := logger.GetLogger()
			log.WithError(err).Error("发送Telegram告警失败")
			// 继续保存记录，但标记未发送
		} else {
			alert.AlertSent = true
		}
	}

	// 保存告警记录
	if err := s.db.Create(alert).Error; err != nil {
		return fmt.Errorf("保存告警记录失败: %w", err)
	}

	return nil
}

// CheckAndAlertForIP 检查指定IP的探测结果并发送告警（如果需要）
// 已废弃：现在使用 CheckAndAlertForURL 按URL维度检查
// 该方法保留仅为了向后兼容，但不再执行任何操作
func (s *SpeedProbeService) CheckAndAlertForIP(clientIP string, timeWindowMinutes int) error {
	log := logger.GetLogger()
	log.WithField("client_ip", clientIP).Warn("CheckAndAlertForIP 方法已废弃，请使用 CheckAndAlertForURL")
	return fmt.Errorf("CheckAndAlertForIP 方法已废弃，现在按URL维度进行告警检查")
}

// CheckAndAlertAll 检查所有URL的探测结果并发送告警
// 新逻辑：针对每个URL，如果探测它的多个IP中超过一半都未达标，才发送告警
// 优化：批量发送告警，每5条发送一次，每次发送后sleep 2秒
func (s *SpeedProbeService) CheckAndAlertAll(timeWindowMinutes int) error {
	log := logger.GetLogger()

	// 计算时间窗口
	windowEnd := time.Now()
	windowStart := windowEnd.Add(-time.Duration(timeWindowMinutes) * time.Minute)

	// 获取时间窗口内所有不同的URL
	var urls []string
	if err := s.db.Model(&models.SpeedProbeResult{}).
		Where("created_at >= ? AND created_at <= ?", windowStart, windowEnd).
		Distinct("url").
		Pluck("url", &urls).Error; err != nil {
		log.WithError(err).Error("查询URL列表失败")
		return fmt.Errorf("查询URL列表失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"url_count":      len(urls),
		"window_minutes": timeWindowMinutes,
	}).Info("开始检查所有URL的探测结果")

	// 收集所有需要发送的告警
	var alertsToSend []*models.SpeedAlertLog
	errorCount := 0

	for _, url := range urls {
		alert, err := s.CheckAndPrepareAlertForURL(url, timeWindowMinutes)
		if err != nil {
			log.WithError(err).WithField("url", url).Error("检查URL失败")
			errorCount++
			continue
		}

		// 如果有告警需要发送，添加到列表
		if alert != nil {
			alertsToSend = append(alertsToSend, alert)
		}
	}

	if errorCount > 0 {
		log.WithField("error_count", errorCount).Warn("部分URL检查失败")
	}

	// 批量发送告警
	if len(alertsToSend) > 0 {
		log.WithField("alert_count", len(alertsToSend)).Info("开始批量发送告警")
		if err := s.sendAlertsBatch(alertsToSend); err != nil {
			log.WithError(err).Error("批量发送告警失败")
			return err
		}
	} else {
		log.Info("所有URL检查完成，无需发送告警")
	}

	return nil
}

// sendAlertsBatch 批量发送告警，每5条发送一次，每次发送后sleep 2秒
func (s *SpeedProbeService) sendAlertsBatch(alerts []*models.SpeedAlertLog) error {
	log := logger.GetLogger()

	batchSize := 5
	totalAlerts := len(alerts)
	sentCount := 0
	failedCount := 0

	for i := 0; i < totalAlerts; i += batchSize {
		end := i + batchSize
		if end > totalAlerts {
			end = totalAlerts
		}

		batch := alerts[i:end]
		batchNum := (i / batchSize) + 1
		totalBatches := (totalAlerts + batchSize - 1) / batchSize

		log.WithFields(map[string]interface{}{
			"batch":         batchNum,
			"total_batches": totalBatches,
			"batch_size":    len(batch),
		}).Info("发送告警批次")

		// 发送当前批次的所有告警
		for _, alert := range batch {
			if s.telegram != nil {
				if err := s.telegram.SendMessage(alert.AlertMessage); err != nil {
					log.WithError(err).WithField("url", alert.URL).Error("发送Telegram告警失败")
					failedCount++
					// 继续发送其他告警
				} else {
					alert.AlertSent = true
					sentCount++
					log.WithField("url", alert.URL).Info("Telegram告警发送成功")
				}
			}

			// 保存告警记录
			if err := s.db.Create(alert).Error; err != nil {
				log.WithError(err).WithField("url", alert.URL).Error("保存告警记录失败")
				// 继续处理其他告警
			}
		}

		// 如果不是最后一批，且后面还有告警，则sleep 2秒
		if end < totalAlerts {
			log.WithField("sleep_seconds", 2).Debug("批次发送完成，等待后再发送下一批")
			time.Sleep(2 * time.Second)
		}
	}

	log.WithFields(map[string]interface{}{
		"total":  totalAlerts,
		"sent":   sentCount,
		"failed": failedCount,
	}).Info("批量告警发送完成")

	if failedCount > 0 {
		return fmt.Errorf("部分告警发送失败: %d/%d", failedCount, totalAlerts)
	}

	return nil
}

// buildAlertMessageForURL 构建URL维度的告警消息
func (s *SpeedProbeService) buildAlertMessageForURL(alert *models.SpeedAlertLog, ipDetails []IPDetail, windowMinutes int) string {
	message := "🚨 下载速度告警（URL维度）\n\n"
	if s.telegram != nil && s.telegram.GetSitename() != "" {
		message = fmt.Sprintf("[%s] 🚨 下载速度告警\n\n", s.telegram.GetSitename())
	}

	// URL信息（截断显示）
	displayURL := alert.URL
	if len(displayURL) > 80 {
		displayURL = displayURL[:77] + "..."
	}
	message += fmt.Sprintf("链接地址: %s\n", displayURL)
	message += fmt.Sprintf("时间窗口: %d 分钟\n", windowMinutes)
	message += fmt.Sprintf("窗口时间: %s 至 %s\n\n",
		alert.TimeWindowStart.Format("2006-01-02 15:04:05"),
		alert.TimeWindowEnd.Format("2006-01-02 15:04:05"))

	message += fmt.Sprintf("探测该链接的IP总数: %d\n", alert.TotalIPs)
	message += fmt.Sprintf("未达标IP数量: %d\n", alert.FailedIPs)
	message += fmt.Sprintf("达标IP数量: %d\n", alert.SuccessIPs)
	message += fmt.Sprintf("未达标比例: %.2f%%\n", alert.FailedRate)

	if alert.AvgSpeedKbps != nil {
		message += fmt.Sprintf("平均速度: %.2f KB/s\n", *alert.AvgSpeedKbps)
	} else {
		message += "平均速度: 无可用数据\n"
	}

	message += fmt.Sprintf("\n速度阈值: %.2f KB/s\n", s.speedThreshold)
	message += fmt.Sprintf("失败率阈值: %.0f%%\n\n", s.failureRateThreshold*100)

	// IP详情（最多显示前10个）
	message += "IP探测详情:\n"
	displayCount := len(ipDetails)
	if displayCount > 10 {
		displayCount = 10
	}

	for i := 0; i < displayCount; i++ {
		detail := ipDetails[i]
		status := "✅"
		if detail.Status == "未达标" {
			status = "❌"
		}
		message += fmt.Sprintf("%s IP: %s | 探测%d次 | 失败率%.1f%% | 平均%.1fKB/s\n",
			status, detail.IP, detail.Probes, detail.FailedRate, detail.AvgSpeed)
	}

	if len(ipDetails) > 10 {
		message += fmt.Sprintf("... 还有 %d 个IP未显示\n", len(ipDetails)-10)
	}

	message += "\n⚠️ 该链接被超过一半的IP探测时未达标，可能存在访问问题！"

	return message
}

// buildAlertMessage 构建告警消息（旧版本，按IP维度）
// 已废弃：现在使用 buildAlertMessageForURL
func (s *SpeedProbeService) buildAlertMessage(alert *models.SpeedAlertLog, windowMinutes int) string {
	message := "🚨 下载速度告警\n\n"
	if s.telegram != nil && s.telegram.GetSitename() != "" {
		message = fmt.Sprintf("[%s] 🚨 下载速度告警\n\n", s.telegram.GetSitename())
	}

	message += fmt.Sprintf("时间窗口: %d 分钟\n", windowMinutes)
	message += fmt.Sprintf("窗口时间: %s 至 %s\n\n",
		alert.TimeWindowStart.Format("2006-01-02 15:04:05"),
		alert.TimeWindowEnd.Format("2006-01-02 15:04:05"))

	if alert.AvgSpeedKbps != nil {
		message += fmt.Sprintf("平均速度: %.2f KB/s\n", *alert.AvgSpeedKbps)
	} else {
		message += "平均速度: 无可用数据\n"
	}

	message += fmt.Sprintf("速度阈值: %.2f KB/s\n", s.speedThreshold)
	message += fmt.Sprintf("失败率阈值: %.0f%%\n", s.failureRateThreshold*100)

	message += "\n⚠️ 下载速度已低于预期标准，请检查网络情况。"

	return message
}

// GetProbeResultsByIP 获取指定IP的探测结果
func (s *SpeedProbeService) GetProbeResultsByIP(clientIP string, page, pageSize int) ([]models.SpeedProbeResult, int64, error) {
	var results []models.SpeedProbeResult
	var total int64

	offset := (page - 1) * pageSize

	query := s.db.Model(&models.SpeedProbeResult{}).Where("client_ip = ?", clientIP)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&results).Error; err != nil {
		return nil, 0, err
	}

	return results, total, nil
}

// GetAlertLogs 获取告警记录
func (s *SpeedProbeService) GetAlertLogs(page, pageSize int) ([]models.SpeedAlertLog, int64, error) {
	var logs []models.SpeedAlertLog
	var total int64

	offset := (page - 1) * pageSize

	query := s.db.Model(&models.SpeedAlertLog{})

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// CleanOldResults 清理旧的探测结果（保留指定天数）
func (s *SpeedProbeService) CleanOldResults(keepDays int) error {
	log := logger.GetLogger()

	cutoffTime := time.Now().AddDate(0, 0, -keepDays)

	result := s.db.Where("created_at < ?", cutoffTime).Delete(&models.SpeedProbeResult{})
	if result.Error != nil {
		log.WithError(result.Error).Error("清理旧探测结果失败")
		return fmt.Errorf("清理旧探测结果失败: %w", result.Error)
	}

	log.WithFields(map[string]interface{}{
		"deleted_count": result.RowsAffected,
		"keep_days":     keepDays,
	}).Info("旧探测结果清理完成")

	return nil
}
