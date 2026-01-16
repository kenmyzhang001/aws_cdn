package services

import (
	"aws_cdn/internal/logger"
	"time"
)

// SchedulerService 定时任务服务
type SchedulerService struct {
	urlChecker *URLCheckerService
	interval   time.Duration
	stopChan   chan struct{}
}

// NewSchedulerService 创建定时任务服务
func NewSchedulerService(urlChecker *URLCheckerService, interval time.Duration) *SchedulerService {
	return &SchedulerService{
		urlChecker: urlChecker,
		interval:   interval,
		stopChan:   make(chan struct{}),
	}
}

// Start 启动定时任务
func (s *SchedulerService) Start() {
	log := logger.GetLogger()
	log.WithField("interval", s.interval).Info("启动 URL 检查定时任务")

	// 启动时立即执行一次
	go s.runCheck()

	// 定时执行
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			go s.runCheck()
		case <-s.stopChan:
			log.Info("停止 URL 检查定时任务")
			return
		}
	}
}

// Stop 停止定时任务
func (s *SchedulerService) Stop() {
	close(s.stopChan)
}

// runCheck 执行检查
func (s *SchedulerService) runCheck() {
	log := logger.GetLogger()
	log.Info("执行 URL 检查任务")

	if err := s.urlChecker.CheckDownloadURLs(); err != nil {
		log.WithError(err).Error("URL 检查任务执行失败")
	}
}
