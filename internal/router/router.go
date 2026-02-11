package router

import (
	"aws_cdn/internal/config"
	"aws_cdn/internal/handlers"
	"aws_cdn/internal/logger"
	"aws_cdn/internal/middleware"
	"aws_cdn/internal/models"
	"aws_cdn/internal/redis"
	"aws_cdn/internal/services"
	"aws_cdn/internal/services/aws"
	"aws_cdn/internal/services/cloudflare"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	redisv9 "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// SetupRouter 初始化路由。redisClient 与 fallbackRuleService 由 main 传入以便 main 可创建 FallbackRuleEngine 并注册定时任务。
func SetupRouter(db, db2, db3 *gorm.DB, cfg *config.Config, telegramService *services.TelegramService, redisClient *redisv9.Client, fallbackRuleService *services.FallbackRuleService) *gin.Engine {
	// 设置 Gin 模式
	gin.SetMode(cfg.Server.Mode)

	r := gin.Default()

	// 增加请求体大小限制（支持大文件上传，例如 10GB）
	r.MaxMultipartMemory = 10 << 30 // 10GB

	// CORS 配置
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	// 初始化 AWS 服务
	route53Svc, _ := aws.NewRoute53Service(&cfg.AWS)
	acmSvc, _ := aws.NewACMService(&cfg.AWS)
	cloudFrontSvc, _ := aws.NewCloudFrontService(&cfg.AWS)
	s3Svc, _ := aws.NewS3Service(&cfg.AWS)

	// 初始化 Cloudflare 服务
	cloudflareSvc, err := cloudflare.NewCloudflareService(&cfg.Cloudflare)
	if err != nil {
		log := logger.GetLogger()
		log.WithError(err).Warn("Cloudflare服务初始化失败，Cloudflare功能将不可用")
		cloudflareSvc = nil
	}

	var s3Origin string
	log := logger.GetLogger()
	if cfg.AWS.S3BucketName != "" {
		// 自动检查并创建存储桶（如果不存在）
		if err := s3Svc.EnsureBucketExists(cfg.AWS.S3BucketName); err != nil {
			// 记录错误但不阻止服务启动，因为可能是权限问题
			log.WithError(err).WithField("bucket_name", cfg.AWS.S3BucketName).Warn("无法确保S3存储桶存在")
		} else {
			log.WithField("bucket_name", cfg.AWS.S3BucketName).Info("S3存储桶已确认存在")
		}
		s3Origin = s3Svc.GetBucketDomain(cfg.AWS.S3BucketName)
	}

	// 初始化服务
	auditService := services.NewAuditService(db)
	groupService := services.NewGroupService(db)
	cfAccountService := services.NewCFAccountService(db)
	domainService := services.NewDomainService(db, route53Svc, acmSvc, cloudFrontSvc, s3Svc, cloudflareSvc, cfAccountService)
	redirectService := services.NewRedirectService(db, cloudFrontSvc, s3Svc, domainService, &cfg.AWS)
	authService := services.NewAuthService(db, &cfg.JWT)
	cloudFrontService := services.NewCloudFrontService(cloudFrontSvc, s3Origin)
	downloadPackageService := services.NewDownloadPackageService(db, db3, domainService, cloudFrontSvc, s3Svc, route53Svc, &cfg.AWS)

	// 初始化 R2 服务
	r2BucketService := services.NewR2BucketService(db, cfAccountService)
	r2CacheRuleService := services.NewR2CacheRuleService(db, cfAccountService, cloudflareSvc)
	r2CustomDomainService := services.NewR2CustomDomainService(db, cfAccountService, r2CacheRuleService)
	r2FileService := services.NewR2FileService(db, cfAccountService)

	// 初始化自定义下载链接服务
	customDownloadLinkService := services.NewCustomDownloadLinkService(db)

	// 初始化速度探测服务（速度阈值100KB/s，失败率阈值50%）
	speedProbeService := services.NewSpeedProbeServiceWithTwoDBs(db, db2, telegramService, models.ThresholdSpeedKbps, 0.5)

	// 初始化 Worker 服务
	cfWorkerService := services.NewCFWorkerService(db)

	// 初始化域名 302 重定向服务
	domainRedirectService := services.NewDomainRedirectService(db, cfAccountService)

	// 初始化重点探测链接服务
	focusProbeLinkService := services.NewFocusProbeLinkService(db)

	// 初始化处理器
	groupHandler := handlers.NewGroupHandler(groupService)
	domainHandler := handlers.NewDomainHandler(domainService)
	redirectHandler := handlers.NewRedirectHandler(redirectService)
	authHandler := handlers.NewAuthHandler(authService)
	cloudFrontHandler := handlers.NewCloudFrontHandler(cloudFrontService)
	downloadPackageHandler := handlers.NewDownloadPackageHandler(downloadPackageService)
	auditHandler := handlers.NewAuditHandler(auditService)
	cfAccountHandler := handlers.NewCFAccountHandler(cfAccountService)
	r2Handler := handlers.NewR2Handler(r2BucketService, r2CustomDomainService, r2CacheRuleService, r2FileService)
	customDownloadLinkHandler := handlers.NewCustomDownloadLinkHandler(customDownloadLinkService)
	allLinksHandler := handlers.NewAllLinksHandler(downloadPackageService, customDownloadLinkService, r2CustomDomainService, r2FileService, focusProbeLinkService, speedProbeService)
	speedProbeHandler := handlers.NewSpeedProbeHandler(speedProbeService)
	cfWorkerHandler := handlers.NewCFWorkerHandler(cfWorkerService)
	domainRedirectHandler := handlers.NewDomainRedirectHandler(domainRedirectService)
	focusProbeLinkHandler := handlers.NewFocusProbeLinkHandler(focusProbeLinkService)

	// Redis 与游戏统计（若 main 未传入 redisClient 则在此创建）
	if redisClient == nil {
		redisClient = redis.NewClient(&cfg.Redis)
	}
	gameStatsHandler := handlers.NewGameStatsHandler(redisClient)

	// 兜底规则（fallbackRuleService 由 main 传入）
	var fallbackRuleHandler *handlers.FallbackRuleHandler
	if fallbackRuleService != nil {
		fallbackRuleHandler = handlers.NewFallbackRuleHandler(fallbackRuleService)
	}

	// API 路由
	api := r.Group("/api/v1")

	// 公共路由（无需登录）
	api.POST("/auth/login", authHandler.Login)

	// 所有链接管理（统一查询接口）
	api.GET("/all-links", allLinksHandler.GetAllLinks)
	// 速度探测上报接口（公共接口，无需认证）
	api.POST("/speed-probe/report", speedProbeHandler.ReportProbeResult)
	api.POST("/speed-probe/report-batch", speedProbeHandler.BatchReportProbeResults)

	// 需要登录的受保护路由
	protected := api.Group("")
	protected.Use(middleware.JWTAuth(cfg.JWT.Secret))
	protected.Use(middleware.AuditMiddleware(auditService))
	{
		// 域名管理
		domains := protected.Group("/domains")
		{
			domains.POST("", domainHandler.TransferDomain)
			domains.GET("", domainHandler.ListDomains)
			domains.GET("/for-select", domainHandler.ListDomainsForSelect) // 轻量级接口，用于下拉选择框
			domains.GET("/:id", domainHandler.GetDomain)
			domains.DELETE("/:id", domainHandler.DeleteDomain)
			domains.GET("/:id/ns-servers", domainHandler.GetNServers)
			domains.GET("/:id/status", domainHandler.GetDomainStatus)
			domains.POST("/:id/certificate", domainHandler.GenerateCertificate)
			domains.GET("/:id/certificate/status", domainHandler.GetCertificateStatus)
			domains.GET("/:id/certificate/check", domainHandler.CheckCertificate)
			domains.POST("/:id/certificate/fix", domainHandler.FixCertificate)
			domains.PUT("/:id/note", domainHandler.UpdateDomainNote)
			domains.PUT("/:id/group", domainHandler.MoveDomainToGroup)
		}

		// 轮播管理
		redirects := protected.Group("/redirects")
		{
			redirects.POST("", redirectHandler.CreateRedirectRule)
			redirects.GET("", redirectHandler.ListRedirectRules)
			redirects.GET("/:id", redirectHandler.GetRedirectRule)
			redirects.DELETE("/:id", redirectHandler.DeleteRule)
			redirects.POST("/:id/targets", redirectHandler.AddTarget)
			redirects.DELETE("/targets/:id", redirectHandler.RemoveTarget)
			redirects.POST("/:id/bind-cloudfront", redirectHandler.BindDomainToCloudFront)
			redirects.GET("/:id/check", redirectHandler.CheckRedirectRule)
			redirects.POST("/:id/fix", redirectHandler.FixRedirectRule)
			redirects.PUT("/:id/note", redirectHandler.UpdateRedirectRuleNote)
		}

		// CloudFront 管理
		cloudfront := protected.Group("/cloudfront")
		{
			cloudfront.GET("/distributions", cloudFrontHandler.ListDistributions)
			cloudfront.GET("/distributions/:id", cloudFrontHandler.GetDistribution)
			cloudfront.POST("/distributions", cloudFrontHandler.CreateDistribution)
			cloudfront.PUT("/distributions/:id", cloudFrontHandler.UpdateDistribution)
			cloudfront.DELETE("/distributions/:id", cloudFrontHandler.DeleteDistribution)
		}

		// 下载包管理
		downloadPackages := protected.Group("/download-packages")
		{
			downloadPackages.POST("", downloadPackageHandler.CreateDownloadPackage)
			downloadPackages.GET("", downloadPackageHandler.ListDownloadPackages)
			downloadPackages.GET("/by-domain", downloadPackageHandler.ListDownloadPackagesByDomain)
			downloadPackages.GET("/:id", downloadPackageHandler.GetDownloadPackage)
			downloadPackages.DELETE("/:id", downloadPackageHandler.DeleteDownloadPackage)
			downloadPackages.GET("/:id/check", downloadPackageHandler.CheckDownloadPackage)
			downloadPackages.POST("/:id/fix", downloadPackageHandler.FixDownloadPackage)
			downloadPackages.PUT("/:id/note", downloadPackageHandler.UpdateDownloadPackageNote)
		}

		// 分组管理
		groups := protected.Group("/groups")
		{
			groups.GET("", groupHandler.ListGroups)
			groups.GET("/with-stats", groupHandler.ListGroupsWithStats) // 带统计信息的分组列表，用于优化页面加载
			groups.GET("/:id", groupHandler.GetGroup)
			groups.POST("", groupHandler.CreateGroup)
			groups.PUT("/:id", groupHandler.UpdateGroup)
			groups.DELETE("/:id", groupHandler.DeleteGroup)
		}

		// 审计日志管理
		audit := protected.Group("/audit-logs")
		{
			audit.GET("", auditHandler.ListAuditLogs)
		}

		// Cloudflare 账号管理
		cfAccounts := protected.Group("/cf-accounts")
		{
			cfAccounts.GET("", cfAccountHandler.ListCFAccounts)
			cfAccounts.POST("", cfAccountHandler.CreateCFAccount)
			cfAccounts.GET("/:id", cfAccountHandler.GetCFAccount)
			cfAccounts.PUT("/:id", cfAccountHandler.UpdateCFAccount)
			cfAccounts.DELETE("/:id", cfAccountHandler.DeleteCFAccount)
			cfAccounts.GET("/:id/zones", cfAccountHandler.GetCFAccountZones)
			cfAccounts.POST("/:id/zones/apk-security", cfAccountHandler.SetZoneAPKSecurityRule)
			cfAccounts.POST("/:id/enable-r2", r2Handler.EnableR2)
		}

		// R2 存储桶管理
		r2Buckets := protected.Group("/r2-buckets")
		{
			r2Buckets.GET("", r2Handler.ListR2Buckets)
			r2Buckets.GET("/:id", r2Handler.GetR2Bucket)
			r2Buckets.POST("", r2Handler.CreateR2Bucket)
			r2Buckets.DELETE("/:id", r2Handler.DeleteR2Bucket)
			r2Buckets.PUT("/:id/note", r2Handler.UpdateR2BucketNote)
			r2Buckets.PUT("/:id/cors", r2Handler.ConfigureCORS)
			r2Buckets.PUT("/:id/credentials", r2Handler.UpdateR2BucketCredentials)
		}

		// R2 自定义域名管理
		r2CustomDomains := protected.Group("/r2-custom-domains")
		{
			r2CustomDomains.GET("/buckets/:r2_bucket_id", r2Handler.ListR2CustomDomains)
			r2CustomDomains.POST("/buckets/:r2_bucket_id", r2Handler.AddR2CustomDomain)
			r2CustomDomains.GET("/:id", r2Handler.GetR2CustomDomain)
			r2CustomDomains.POST("/:id/retry", r2Handler.RetryR2CustomDomain)
			r2CustomDomains.DELETE("/:id", r2Handler.DeleteR2CustomDomain)
		}

		// R2 缓存规则管理
		r2CacheRules := protected.Group("/r2-cache-rules")
		{
			r2CacheRules.GET("/domains/:r2_custom_domain_id", r2Handler.ListR2CacheRules)
			r2CacheRules.POST("/domains/:r2_custom_domain_id", r2Handler.CreateR2CacheRule)
			r2CacheRules.DELETE("/:id", r2Handler.DeleteR2CacheRule)
		}

		// R2 文件管理
		r2Files := protected.Group("/r2-files")
		{
			r2Files.POST("/buckets/:r2_bucket_id/upload", r2Handler.UploadFile)
			r2Files.POST("/buckets/:r2_bucket_id/directories", r2Handler.CreateDirectory)
			r2Files.GET("/buckets/:r2_bucket_id", r2Handler.ListFiles)
			r2Files.DELETE("/buckets/:r2_bucket_id", r2Handler.DeleteFile)
			// APK 文件管理
			r2Files.GET("/buckets/:r2_bucket_id/apk-files", r2Handler.ListApkFiles)
			r2Files.GET("/buckets/:r2_bucket_id/apk-file-urls", r2Handler.GetApkFileUrls)
		}

		// 自定义下载链接管理
		customDownloadLinks := protected.Group("/custom-download-links")
		{
			customDownloadLinks.GET("", customDownloadLinkHandler.ListCustomDownloadLinks)
			customDownloadLinks.GET("/:id", customDownloadLinkHandler.GetCustomDownloadLink)
			customDownloadLinks.POST("", customDownloadLinkHandler.CreateCustomDownloadLink)
			customDownloadLinks.POST("/batch", customDownloadLinkHandler.BatchCreateCustomDownloadLinks)
			customDownloadLinks.PUT("/:id", customDownloadLinkHandler.UpdateCustomDownloadLink)
			customDownloadLinks.DELETE("/:id", customDownloadLinkHandler.DeleteCustomDownloadLink)
			customDownloadLinks.POST("/batch-delete", customDownloadLinkHandler.BatchDeleteCustomDownloadLinks)
			customDownloadLinks.POST("/:id/click", customDownloadLinkHandler.IncrementClickCount)
		}

		// 速度探测结果管理
		speedProbe := protected.Group("/speed-probe")
		{
			speedProbe.GET("/results", speedProbeHandler.ListProbeResults)        // 探测结果列表（带筛选）
			speedProbe.GET("/results/:ip", speedProbeHandler.GetProbeResultsByIP) // 按 IP 查询
			speedProbe.GET("/alerts", speedProbeHandler.ListAlertLogs)            // 告警记录列表（带筛选）
			speedProbe.POST("/check", speedProbeHandler.TriggerCheck)             // 手动触发检查
		}

		// Cloudflare Worker 管理
		cfWorkers := protected.Group("/cf-workers")
		{
			cfWorkers.GET("", cfWorkerHandler.GetWorkerList)
			cfWorkers.GET("/check-domain", cfWorkerHandler.CheckDomain)
			cfWorkers.GET("/:id", cfWorkerHandler.GetWorker)
			cfWorkers.POST("", cfWorkerHandler.CreateWorker)
			cfWorkers.PUT("/:id", cfWorkerHandler.UpdateWorker)
			cfWorkers.DELETE("/:id", cfWorkerHandler.DeleteWorker)
		}

		// 域名 302 重定向管理（CF Redirect Rules）
		domainRedirects := protected.Group("/domain-redirects")
		{
			domainRedirects.GET("", domainRedirectHandler.List)
			domainRedirects.GET("/check-domain", domainRedirectHandler.CheckDomain)
			domainRedirects.GET("/:id", domainRedirectHandler.Get)
			domainRedirects.POST("", domainRedirectHandler.Create)
			domainRedirects.PUT("/:id", domainRedirectHandler.Update)
			domainRedirects.DELETE("/:id", domainRedirectHandler.Delete)
			domainRedirects.POST("/:id/ensure-dns", domainRedirectHandler.EnsureDNS)
		}

		// 重点探测链接管理
		focusProbeLinks := protected.Group("/focus-probe-links")
		{
			focusProbeLinks.GET("", focusProbeLinkHandler.GetFocusProbeLinks)
			focusProbeLinks.GET("/statistics", focusProbeLinkHandler.GetStatistics)
			focusProbeLinks.GET("/export", focusProbeLinkHandler.ExportLinks)
			focusProbeLinks.GET("/check-url", focusProbeLinkHandler.CheckIfURLExists)
			focusProbeLinks.GET("/:id", focusProbeLinkHandler.GetFocusProbeLinkByID)
			focusProbeLinks.POST("", focusProbeLinkHandler.CreateFocusProbeLink)
			focusProbeLinks.PUT("/:id", focusProbeLinkHandler.UpdateFocusProbeLink)
			focusProbeLinks.DELETE("/:id", focusProbeLinkHandler.DeleteFocusProbeLink)
			focusProbeLinks.POST("/batch-delete", focusProbeLinkHandler.BatchDeleteFocusProbeLinks)
			focusProbeLinks.POST("/batch-update-interval", focusProbeLinkHandler.BatchUpdateProbeInterval)
			focusProbeLinks.POST("/:id/toggle-enabled", focusProbeLinkHandler.ToggleEnabled)
			focusProbeLinks.POST("/from-download-package", focusProbeLinkHandler.AddFromDownloadPackage)
			focusProbeLinks.POST("/from-custom-link", focusProbeLinkHandler.AddFromCustomDownloadLink)
			focusProbeLinks.POST("/from-r2-file", focusProbeLinkHandler.AddFromR2File)
		}

		// 游戏统计（Redis 渠道名称、站点日数据等）
		gameStats := protected.Group("/game-stats")
		{
			gameStats.GET("/full-channel-names", gameStatsHandler.ListFullChannelNames)
			gameStats.GET("/site-daily", gameStatsHandler.ListSiteDailyData)
		}

		// 兜底规则管理
		if fallbackRuleHandler != nil {
			fallbackRules := protected.Group("/fallback-rules")
			{
				fallbackRules.GET("", fallbackRuleHandler.ListFallbackRules)
				fallbackRules.GET("/:id", fallbackRuleHandler.GetFallbackRule)
				fallbackRules.POST("", fallbackRuleHandler.CreateFallbackRule)
				fallbackRules.PUT("/:id", fallbackRuleHandler.UpdateFallbackRule)
				fallbackRules.DELETE("/:id", fallbackRuleHandler.DeleteFallbackRule)
			}
		}
	}

	// Telegram webhook（公共路由，无需认证）
	if telegramService != nil {
		telegramHandler := handlers.NewTelegramHandler(telegramService)
		r.POST("/webhook/telegram", telegramHandler.HandleWebhook)
	}

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	return r
}
