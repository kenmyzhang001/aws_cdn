package router

import (
	"aws_cdn/internal/config"
	"aws_cdn/internal/handlers"
	"aws_cdn/internal/logger"
	"aws_cdn/internal/middleware"
	"aws_cdn/internal/services"
	"aws_cdn/internal/services/aws"
	"aws_cdn/internal/services/cloudflare"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func SetupRouter(db *gorm.DB, cfg *config.Config) *gin.Engine {
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
	domainService := services.NewDomainService(db, route53Svc, acmSvc, cloudFrontSvc, s3Svc, cloudflareSvc)
	redirectService := services.NewRedirectService(db, cloudFrontSvc, s3Svc, domainService, &cfg.AWS)
	authService := services.NewAuthService(db, &cfg.JWT)
	cloudFrontService := services.NewCloudFrontService(cloudFrontSvc, s3Origin)
	downloadPackageService := services.NewDownloadPackageService(db, domainService, cloudFrontSvc, s3Svc, route53Svc, &cfg.AWS)

	// 初始化处理器
	groupHandler := handlers.NewGroupHandler(groupService)
	domainHandler := handlers.NewDomainHandler(domainService)
	redirectHandler := handlers.NewRedirectHandler(redirectService)
	authHandler := handlers.NewAuthHandler(authService)
	cloudFrontHandler := handlers.NewCloudFrontHandler(cloudFrontService)
	downloadPackageHandler := handlers.NewDownloadPackageHandler(downloadPackageService)
	auditHandler := handlers.NewAuditHandler(auditService)

	// API 路由
	api := r.Group("/api/v1")

	// 公共路由（无需登录）
	api.POST("/auth/login", authHandler.Login)

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
			domains.GET("/:id", domainHandler.GetDomain)
			domains.DELETE("/:id", domainHandler.DeleteDomain)
			domains.GET("/:id/ns-servers", domainHandler.GetNServers)
			domains.GET("/:id/status", domainHandler.GetDomainStatus)
			domains.POST("/:id/certificate", domainHandler.GenerateCertificate)
			domains.GET("/:id/certificate/status", domainHandler.GetCertificateStatus)
			domains.GET("/:id/certificate/check", domainHandler.CheckCertificate)
			domains.POST("/:id/certificate/fix", domainHandler.FixCertificate)
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
		}

		// 分组管理
		groups := protected.Group("/groups")
		{
			groups.GET("", groupHandler.ListGroups)
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
	}

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	return r
}
