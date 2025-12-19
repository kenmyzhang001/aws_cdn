package services

import (
	"aws_cdn/internal/config"
	"aws_cdn/internal/models"
	"aws_cdn/internal/services/aws"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"
)

type DownloadPackageService struct {
	db            *gorm.DB
	domainService *DomainService
	cloudFrontSvc *aws.CloudFrontService
	s3Svc         *aws.S3Service
	route53Svc    *aws.Route53Service
	config        *config.AWSConfig
}

func NewDownloadPackageService(
	db *gorm.DB,
	domainService *DomainService,
	cloudFrontSvc *aws.CloudFrontService,
	s3Svc *aws.S3Service,
	route53Svc *aws.Route53Service,
	cfg *config.AWSConfig,
) *DownloadPackageService {
	return &DownloadPackageService{
		db:            db,
		domainService: domainService,
		cloudFrontSvc: cloudFrontSvc,
		s3Svc:         s3Svc,
		route53Svc:    route53Svc,
		config:        cfg,
	}
}

// CreateDownloadPackage 创建下载包
// 1. 上传文件到S3
// 2. 创建CloudFront分发
// 3. 将域名绑定到CloudFront
func (s *DownloadPackageService) CreateDownloadPackage(domainID uint, domainName string, fileName string, fileReader io.ReadSeeker, fileSize int64) (*models.DownloadPackage, error) {
	// 验证域名是否存在
	domain, err := s.domainService.GetDomain(domainID)
	if err != nil {
		return nil, fmt.Errorf("域名不存在: %w", err)
	}

	// 检查域名证书状态
	if domain.CertificateStatus != "issued" {
		return nil, fmt.Errorf("域名证书未签发，当前状态: %s", domain.CertificateStatus)
	}

	// 生成S3键（使用downloads/前缀）
	s3Key := fmt.Sprintf("downloads/%s/%s", domainName, fileName)

	// 创建下载包记录
	downloadPackage := &models.DownloadPackage{
		DomainID:   domainID,
		DomainName: domainName,
		FileName:   fileName,
		FileSize:   fileSize,
		FileType:   mime.TypeByExtension(filepath.Ext(fileName)),
		S3Key:      s3Key,
		Status:     models.DownloadPackageStatusUploading,
	}

	if err := s.db.Create(downloadPackage).Error; err != nil {
		return nil, fmt.Errorf("创建下载包记录失败: %w", err)
	}

	// 异步处理上传和配置
	go s.processDownloadPackageAsync(downloadPackage, fileReader, domain)

	return downloadPackage, nil
}

// processDownloadPackageAsync 异步处理下载包
func (s *DownloadPackageService) processDownloadPackageAsync(pkg *models.DownloadPackage, fileReader io.ReadSeeker, domain *models.Domain) {
	// 更新状态为上传中
	s.db.Model(pkg).Update("status", models.DownloadPackageStatusUploading)

	// 1. 上传文件到S3
	// 确保文件读取器位置在开始
	if seeker, ok := fileReader.(io.Seeker); ok {
		seeker.Seek(0, io.SeekStart)
	}

	// 确定Content-Type
	contentType := pkg.FileType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// 上传文件到S3（使用public-read ACL以便CloudFront访问）
	if err := s.s3Svc.UploadFileWithACL(s.config.S3BucketName, pkg.S3Key, fileReader, contentType, "public-read"); err != nil {
		s.db.Model(pkg).Updates(map[string]interface{}{
			"status":       models.DownloadPackageStatusFailed,
			"error_message": fmt.Sprintf("上传文件到S3失败: %v", err),
		})
		return
	}

	// 更新状态为处理中
	s.db.Model(pkg).Update("status", models.DownloadPackageStatusProcessing)

	// 2. 获取S3域名
	s3Origin := s.s3Svc.GetBucketDomain(s.config.S3BucketName)

	// 3. 创建CloudFront分发（使用大文件下载优化配置）
	// 计算originPath（去掉文件名，只保留目录路径）
	originPath := ""
	if strings.Contains(pkg.S3Key, "/") {
		parts := strings.Split(pkg.S3Key, "/")
		if len(parts) > 1 {
			originPath = "/" + strings.Join(parts[:len(parts)-1], "/")
		}
	}

	cloudFrontID, err := s.cloudFrontSvc.CreateDistributionForLargeFileDownload(
		pkg.DomainName,
		domain.CertificateARN,
		s3Origin,
		originPath,
	)
	if err != nil {
		s.db.Model(pkg).Updates(map[string]interface{}{
			"status":       models.DownloadPackageStatusFailed,
			"error_message": fmt.Sprintf("创建CloudFront分发失败: %v", err),
		})
		return
	}

	// 获取CloudFront域名
	cloudFrontDomain, err := s.cloudFrontSvc.GetDistributionDomain(cloudFrontID)
	if err != nil {
		s.db.Model(pkg).Updates(map[string]interface{}{
			"status":       models.DownloadPackageStatusFailed,
			"error_message": fmt.Sprintf("获取CloudFront域名失败: %v", err),
		})
		return
	}

	// 4. 将域名绑定到CloudFront（创建Route53 A记录）
	if domain.HostedZoneID != "" {
		// 等待一下让CloudFront分发完全部署
		time.Sleep(5 * time.Second)

		// 检查是否已存在记录
		exists, err := s.domainService.CheckCloudFrontAliasRecord(domain.HostedZoneID, pkg.DomainName, cloudFrontDomain)
		if err == nil && !exists {
			// 创建A记录指向CloudFront
			if err := s.domainService.CreateCloudFrontAliasRecord(domain.HostedZoneID, pkg.DomainName, cloudFrontDomain); err != nil {
				s.db.Model(pkg).Updates(map[string]interface{}{
					"status":       models.DownloadPackageStatusFailed,
					"error_message": fmt.Sprintf("创建Route53记录失败: %v", err),
				})
				return
			}
		}
	}

	// 5. 构建下载URL
	downloadURL := fmt.Sprintf("https://%s/%s", pkg.DomainName, pkg.FileName)

	// 更新下载包信息
	s.db.Model(pkg).Updates(map[string]interface{}{
		"cloudfront_id":     cloudFrontID,
		"cloudfront_domain": cloudFrontDomain,
		"download_url":      downloadURL,
		"status":            models.DownloadPackageStatusCompleted,
	})
}

// GetDownloadPackage 获取下载包信息
func (s *DownloadPackageService) GetDownloadPackage(id uint) (*models.DownloadPackage, error) {
	var pkg models.DownloadPackage
	if err := s.db.Preload("Domain").First(&pkg, id).Error; err != nil {
		return nil, fmt.Errorf("下载包不存在: %w", err)
	}
	return &pkg, nil
}

// ListDownloadPackages 列出所有下载包
func (s *DownloadPackageService) ListDownloadPackages(page, pageSize int) ([]models.DownloadPackage, int64, error) {
	var packages []models.DownloadPackage
	var total int64

	offset := (page - 1) * pageSize

	if err := s.db.Model(&models.DownloadPackage{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := s.db.Preload("Domain").Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&packages).Error; err != nil {
		return nil, 0, err
	}

	return packages, total, nil
}

// DeleteDownloadPackage 删除下载包
func (s *DownloadPackageService) DeleteDownloadPackage(id uint) error {
	pkg, err := s.GetDownloadPackage(id)
	if err != nil {
		return err
	}

	// 删除S3文件
	if pkg.S3Key != "" {
		if err := s.s3Svc.DeleteObject(s.config.S3BucketName, pkg.S3Key); err != nil {
			// 记录错误但不阻止删除，因为文件可能已经被删除
			// 可以记录日志，这里简化处理
		}
	}

	// 注意：CloudFront分发可能被其他下载包使用，所以不删除
	// 如果需要删除，应该检查是否有其他下载包使用同一个分发

	// 从数据库删除（软删除）
	if err := s.db.Delete(pkg).Error; err != nil {
		return fmt.Errorf("删除下载包记录失败: %w", err)
	}

	return nil
}

