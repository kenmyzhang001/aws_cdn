package router

import (
	"aws_cdn/internal/config"
	"aws_cdn/internal/handlers"
	"aws_cdn/internal/middleware"
	"aws_cdn/internal/services"
	"aws_cdn/internal/services/aws"
	"log"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func SetupRouter(db *gorm.DB, cfg *config.Config) *gin.Engine {
	// 设置 Gin 模式
	gin.SetMode(cfg.Server.Mode)

	r := gin.Default()

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

	var s3Origin string
	if cfg.AWS.S3BucketName != "" {
		// 自动检查并创建存储桶（如果不存在）
		if err := s3Svc.EnsureBucketExists(cfg.AWS.S3BucketName); err != nil {
			// 记录错误但不阻止服务启动，因为可能是权限问题
			log.Printf("警告: 无法确保S3存储桶 '%s' 存在: %v", cfg.AWS.S3BucketName, err)
		} else {
			log.Printf("S3存储桶 '%s' 已确认存在", cfg.AWS.S3BucketName)
		}
		s3Origin = s3Svc.GetBucketDomain(cfg.AWS.S3BucketName)
	}

	// 初始化服务
	domainService := services.NewDomainService(db, route53Svc, acmSvc, cloudFrontSvc, s3Svc)
	redirectService := services.NewRedirectService(db, cloudFrontSvc, s3Svc, domainService, &cfg.AWS)
	authService := services.NewAuthService(db, &cfg.JWT)
	cloudFrontService := services.NewCloudFrontService(cloudFrontSvc, s3Origin)
	downloadPackageService := services.NewDownloadPackageService(db, domainService, cloudFrontSvc, s3Svc, route53Svc, &cfg.AWS)

	// 初始化处理器
	domainHandler := handlers.NewDomainHandler(domainService)
	redirectHandler := handlers.NewRedirectHandler(redirectService)
	authHandler := handlers.NewAuthHandler(authService)
	cloudFrontHandler := handlers.NewCloudFrontHandler(cloudFrontService)
	downloadPackageHandler := handlers.NewDownloadPackageHandler(downloadPackageService)

	// API 路由
	api := r.Group("/api/v1")

	// 公共路由（无需登录）
	api.POST("/auth/login", authHandler.Login)

	// 需要登录的受保护路由
	protected := api.Group("")
	protected.Use(middleware.JWTAuth(cfg.JWT.Secret))
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

		// 重定向管理
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
			downloadPackages.GET("/:id", downloadPackageHandler.GetDownloadPackage)
			downloadPackages.DELETE("/:id", downloadPackageHandler.DeleteDownloadPackage)
		}
	}

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	return r
}
