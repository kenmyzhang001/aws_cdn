package main

import (
	"aws_cdn/internal/config"
	"aws_cdn/internal/database"
	"aws_cdn/internal/logger"
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

	// 自动迁移数据库
	if err := database.AutoMigrate(db); err != nil {
		log.WithError(err).Fatal("数据库迁移失败")
	}

	// 初始化路由
	r := router.SetupRouter(db, cfg)

	// 初始化 Telegram 服务
	botToken := "7366631415:AAGQm8flfcjfrYDv5ZawwebczZqNSg_nbqo"
	chatID := int64(-1003333638037)
	telegramService := services.NewTelegramService(botToken, chatID, cfg.Server.Sitename)

	// 初始化 URL 检查服务
	urlCheckerService := services.NewURLCheckerService(db, telegramService)

	// 初始化并启动定时任务（每10分钟检查一次）
	schedulerService := services.NewSchedulerService(urlCheckerService, 10*time.Minute)
	go schedulerService.Start()

	log.Info("URL 检查定时任务已启动（每10分钟检查一次）")

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
