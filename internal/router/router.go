package router

import (
	"aws_cdn/internal/config"
	"aws_cdn/internal/handlers"
	"aws_cdn/internal/services"
	"aws_cdn/internal/services/aws"

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

	// 初始化服务
	domainService := services.NewDomainService(db, route53Svc, acmSvc, cloudFrontSvc, s3Svc)
	redirectService := services.NewRedirectService(db, cloudFrontSvc)

	// 初始化处理器
	domainHandler := handlers.NewDomainHandler(domainService)
	redirectHandler := handlers.NewRedirectHandler(redirectService)

	// API 路由
	api := r.Group("/api/v1")
	{
		// 域名管理
		domains := api.Group("/domains")
		{
			domains.POST("", domainHandler.TransferDomain)
			domains.GET("", domainHandler.ListDomains)
			domains.GET("/:id", domainHandler.GetDomain)
			domains.GET("/:id/ns-servers", domainHandler.GetNServers)
			domains.GET("/:id/status", domainHandler.GetDomainStatus)
			domains.POST("/:id/certificate", domainHandler.GenerateCertificate)
			domains.GET("/:id/certificate/status", domainHandler.GetCertificateStatus)
		}

		// 重定向管理
		redirects := api.Group("/redirects")
		{
			redirects.POST("", redirectHandler.CreateRedirectRule)
			redirects.GET("", redirectHandler.ListRedirectRules)
			redirects.GET("/:id", redirectHandler.GetRedirectRule)
			redirects.DELETE("/:id", redirectHandler.DeleteRule)
			redirects.POST("/:id/targets", redirectHandler.AddTarget)
			redirects.DELETE("/targets/:id", redirectHandler.RemoveTarget)
			redirects.POST("/:id/bind-cloudfront", redirectHandler.BindDomainToCloudFront)
		}
	}

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	return r
}

