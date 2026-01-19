package services

import (
	"aws_cdn/internal/logger"
	"time"
)

// TaskFunc 定时任务函数类型
type TaskFunc func() error

// ScheduledTask 定时任务
type ScheduledTask struct {
	name     string
	taskFunc TaskFunc
	interval time.Duration
	stopChan chan struct{}
}

// SchedulerService 定时任务服务
type SchedulerService struct {
	tasks []*ScheduledTask
}

// NewSchedulerService 创建定时任务服务
func NewSchedulerService() *SchedulerService {
	return &SchedulerService{
		tasks: make([]*ScheduledTask, 0),
	}
}

// AddTask 添加定时任务
func (s *SchedulerService) AddTask(name string, taskFunc TaskFunc, interval time.Duration) {
	task := &ScheduledTask{
		name:     name,
		taskFunc: taskFunc,
		interval: interval,
		stopChan: make(chan struct{}),
	}
	s.tasks = append(s.tasks, task)
}

// Start 启动所有定时任务
func (s *SchedulerService) Start() {
	log := logger.GetLogger()
	for _, task := range s.tasks {
		log.WithFields(map[string]interface{}{
			"task_name": task.name,
			"interval":  task.interval,
		}).Info("启动定时任务")

		// 启动时立即执行一次
		go task.run()

		// 定时执行
		go task.start()
	}
}

// Stop 停止所有定时任务
func (s *SchedulerService) Stop() {
	log := logger.GetLogger()
	for _, task := range s.tasks {
		log.WithField("task_name", task.name).Info("停止定时任务")
		close(task.stopChan)
	}
}

// start 启动单个定时任务
func (t *ScheduledTask) start() {
	log := logger.GetLogger()
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			go t.run()
		case <-t.stopChan:
			log.WithField("task_name", t.name).Info("定时任务已停止")
			return
		}
	}
}

// run 执行任务
func (t *ScheduledTask) run() {
	log := logger.GetLogger()
	log.WithField("task_name", t.name).Info("执行定时任务")

	if err := t.taskFunc(); err != nil {
		log.WithError(err).WithField("task_name", t.name).Error("定时任务执行失败")
	}
}
