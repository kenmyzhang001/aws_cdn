package main

import (
	"aws_cdn/internal/config"
	"aws_cdn/internal/database"
	"aws_cdn/internal/logger"
	"aws_cdn/internal/models"
	"aws_cdn/internal/router"
	"aws_cdn/internal/services"
	"os"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	// 初始化日志系统（20MB，保留10个文件）
	logger.InitLogger("./logs", "app", 20, 10, 0)
	log := logger.GetLogger()

	// 加载环境变量
	if err := godotenv.Load(); err != nil {
		log.Info("未找到 .env 文件，使用环境变量")
	}

	// 初始化配置
	cfg := config.Load()

	// 初始化数据库
	db, err := database.Initialize(database.DatabaseConfig{
		Host:     cfg.Database.Host,
		Port:     cfg.Database.Port,
		User:     cfg.Database.User,
		Password: cfg.Database.Password,
		DBName:   cfg.Database.DBName,
		SSLMode:  cfg.Database.SSLMode,
	})
	if err != nil {
		log.WithError(err).Fatal("数据库初始化失败")
	}

	db2, err := database.Initialize(database.DatabaseConfig{
		Host:     cfg.Database2.Host,
		Port:     cfg.Database2.Port,
		User:     cfg.Database2.User,
		Password: cfg.Database2.Password,
		DBName:   cfg.Database2.DBName,
		SSLMode:  cfg.Database2.SSLMode,
	})
	if err != nil {
		log.Infof("数据库初始化失败: %v", err)
	}

	db3, err := database.Initialize(database.DatabaseConfig{
		Host:     cfg.Database3.Host,
		Port:     cfg.Database3.Port,
		User:     cfg.Database3.User,
		Password: cfg.Database3.Password,
		DBName:   cfg.Database3.DBName,
		SSLMode:  cfg.Database3.SSLMode,
	})
	if err != nil {
		log.Infof("数据库初始化失败: %v", err)
	}

	//// 自动迁移数据库
	//if err := database.AutoMigrate(db); err != nil {
	//	log.WithError(err).Fatal("数据库迁移失败")
	//}

	// 初始化 Telegram 服务
	botToken := "7366631415:AAGQm8flfcjfrYDv5ZawwebczZqNSg_nbqo"
	chatID := int64(-1003333638037)

	//botToken := "7651821486:AAE9pJTRYr_jR5ulvp5ms6jCXhozS7RejAY"
	//chatID := int64(1408352847)
	telegramService := services.NewTelegramService(botToken, chatID, cfg.Server.Sitename)

	// 初始化速度探测告警服务（速度阈值100KB/s，失败率阈值50%）
	speedProbeService := services.NewSpeedProbeServiceWithTwoDBs(db, db2, telegramService, models.ThresholdSpeedKbps, 0.5)

	// 初始化自定义下载链接服务
	customDownloadLinkService := services.NewCustomDownloadLinkService(db)

	// 初始化并启动定时任务服务
	schedulerService := services.NewSchedulerService()

	// 速度探测告警检查任务（根据配置决定是否启用）
	if cfg.ScheduledTask.EnableSpeedProbeAlert {
		go speedProbeService.CheckAndAlertAll(services.TimeWindowMinutes)
		// 添加速度探测告警检查任务（每30分钟检查一次，检查最近30分钟的数据）
		schedulerService.AddTask("速度探测告警检查", func() error {
			return speedProbeService.CheckAndAlertAll(services.TimeWindowMinutes)
		}, 10*time.Minute)
		log.Info("定时任务已启用：速度探测告警检查（每30分钟执行一次）")
	} else {
		log.Info("定时任务已禁用：速度探测告警检查")
	}

	// 清理旧探测结果任务（根据配置决定是否启用）
	if cfg.ScheduledTask.EnableCleanOldResults {
		go speedProbeService.CleanOldResults(services.TimeWindowMinutes)
		// 添加清理旧探测结果任务（每天执行一次，保留30天数据）
		schedulerService.AddTask("清理旧探测结果", func() error {
			return speedProbeService.CleanOldResults(services.TimeWindowMinutes)
		}, 24*time.Hour)
		log.Info("定时任务已启用：清理旧探测结果（每24小时执行一次）")
	} else {
		log.Info("定时任务已禁用：清理旧探测结果")
	}

	// 更新自定义下载链接实际URL任务（根据配置决定是否启用）
	if cfg.ScheduledTask.EnableUpdateCustomDownloadLinks {
		go customDownloadLinkService.UpdateActualURLsForAllLinks()
		// 添加更新自定义下载链接 actual_url 任务（每30分钟执行一次）
		schedulerService.AddTask("更新自定义下载链接实际URL", func() error {
			return customDownloadLinkService.UpdateActualURLsForAllLinks()
		}, 30*time.Minute)
		log.Info("定时任务已启用：更新自定义下载链接实际URL（每30分钟执行一次）")
	} else {
		log.Info("定时任务已禁用：更新自定义下载链接实际URL")
	}

	// 启动所有定时任务
	go schedulerService.Start()

	log.Info("定时任务服务已启动")
	log.Info("  - 注意：链接探测由独立的 agent 进程执行")

	// 初始化路由（传入 Telegram 服务以支持 webhook）
	r := router.SetupRouter(db, db2, db3, cfg, telegramService)

	// 启动服务器
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}

	log.WithField("port", port).Info("服务器启动")
	if err := r.Run(":" + port); err != nil {
		log.WithError(err).Fatal("服务器启动失败")
	}
}
