package services

import (
	"aws_cdn/internal/config"
	"aws_cdn/internal/logger"
	"aws_cdn/internal/models"
	"aws_cdn/internal/services/aws"
	"fmt"
	"io"
	"mime"
	"net/http"
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

// CheckDomainUsedByRedirect 检查域名是否被重定向规则使用（排除软删除的记录）
func (s *DownloadPackageService) CheckDomainUsedByRedirect(domainName string) (bool, error) {
	var count int64
	if err := s.db.Model(&models.RedirectRule{}).
		Where("source_domain = ?", domainName).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// CreateDownloadPackage 创建下载包
// 1. 上传文件到S3
// 2. 创建CloudFront分发
// 3. 将域名绑定到CloudFront
func (s *DownloadPackageService) CreateDownloadPackage(domainID uint, fileName string, fileReader io.ReadSeeker, fileSize int64) (*models.DownloadPackage, error) {
	log := logger.GetLogger()
	log.WithFields(map[string]interface{}{
		"domain_id": domainID,
		"file_name": fileName,
		"file_size": fileSize,
	}).Info("开始创建下载包")

	// 验证域名是否存在
	domain, err := s.domainService.GetDomain(domainID)
	if err != nil {
		log.WithError(err).WithField("domain_id", domainID).Error("域名不存在")
		return nil, fmt.Errorf("域名不存在: %w", err)
	}

	// 检查域名证书状态（仅对 AWS 托管的域名检查）
	// Cloudflare 托管的域名使用 CloudFront 默认证书，不需要证书
	if domain.DNSProvider == models.DNSProviderAWS && domain.CertificateStatus != "issued" {
		log.WithFields(map[string]interface{}{
			"domain_id":          domainID,
			"domain_name":        domain.DomainName,
			"certificate_status": domain.CertificateStatus,
		}).Error("域名证书未签发")
		return nil, fmt.Errorf("域名证书未签发，当前状态: %s", domain.CertificateStatus)
	}

	// 使用域名的domain_name作为下载域名
	domainName := domain.DomainName

	// 检查域名是否已被重定向规则使用
	isUsed, err := s.CheckDomainUsedByRedirect(domainName)
	if err != nil {
		log.WithError(err).WithField("domain_name", domainName).Error("检查域名使用状态失败")
		return nil, fmt.Errorf("检查域名使用状态失败: %w", err)
	}
	if isUsed {
		log.WithField("domain_name", domainName).Error("域名已被重定向规则使用")
		return nil, fmt.Errorf("域名 %s 已被重定向规则使用，请先删除重定向规则后再使用", domainName)
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
		log.WithError(err).WithFields(map[string]interface{}{
			"domain_id": domainID,
			"file_name": fileName,
			"s3_key":    s3Key,
		}).Error("创建下载包记录失败")
		return nil, fmt.Errorf("创建下载包记录失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"package_id": downloadPackage.ID,
		"domain_id":  domainID,
		"file_name":  fileName,
		"s3_key":     s3Key,
	}).Info("下载包记录创建成功，开始异步处理")

	// 异步处理上传和配置
	go s.processDownloadPackageAsync(downloadPackage, fileReader, domain)

	return downloadPackage, nil
}

// processDownloadPackageAsync 异步处理下载包
func (s *DownloadPackageService) processDownloadPackageAsync(pkg *models.DownloadPackage, fileReader io.ReadSeeker, domain *models.Domain) {
	log := logger.GetLogger()
	log.WithFields(map[string]interface{}{
		"package_id": pkg.ID,
		"domain_id":  pkg.DomainID,
		"file_name":  pkg.FileName,
		"s3_key":     pkg.S3Key,
		"file_size":  pkg.FileSize,
	}).Info("开始异步处理下载包")

	// 更新状态为上传中
	s.db.Model(pkg).Update("status", models.DownloadPackageStatusUploading)

	// 确保 S3 bucket policy 允许公开访问 downloads/* 路径
	if s.config.S3BucketName != "" {
		log.WithField("bucket_name", s.config.S3BucketName).Info("检查S3 bucket policy配置")
		if err := s.s3Svc.EnsureBucketPolicyForDownloads(s.config.S3BucketName); err != nil {
			log.WithError(err).WithFields(map[string]interface{}{
				"package_id":  pkg.ID,
				"bucket_name": s.config.S3BucketName,
			}).Error("配置 S3 bucket policy 失败")
			s.db.Model(pkg).Updates(map[string]interface{}{
				"status":        models.DownloadPackageStatusFailed,
				"error_message": fmt.Sprintf("配置 S3 bucket policy 失败: %v", err),
			})
			return
		}
		log.WithField("bucket_name", s.config.S3BucketName).Info("S3 bucket policy配置成功")
	}

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

	log.WithFields(map[string]interface{}{
		"package_id":   pkg.ID,
		"s3_key":       pkg.S3Key,
		"bucket_name":  s.config.S3BucketName,
		"content_type": contentType,
		"file_size":    pkg.FileSize,
	}).Info("开始上传文件到S3")

	// 上传文件到S3（使用public-read ACL以便CloudFront访问）
	if err := s.s3Svc.UploadFileWithACL(s.config.S3BucketName, pkg.S3Key, fileReader, contentType, "public-read"); err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"package_id":  pkg.ID,
			"s3_key":      pkg.S3Key,
			"bucket_name": s.config.S3BucketName,
		}).Error("上传文件到S3失败")
		s.db.Model(pkg).Updates(map[string]interface{}{
			"status":        models.DownloadPackageStatusFailed,
			"error_message": fmt.Sprintf("上传文件到S3失败: %v", err),
		})
		return
	}

	log.WithFields(map[string]interface{}{
		"package_id":  pkg.ID,
		"s3_key":      pkg.S3Key,
		"bucket_name": s.config.S3BucketName,
	}).Info("文件上传到S3成功，开始验证文件存在性")

	// 验证文件是否真的存在于S3中（使用重试机制，因为S3可能有最终一致性延迟）
	maxRetries := 5
	retryInterval := 2 * time.Second
	var exists bool
	var err error

	for i := 0; i < maxRetries; i++ {
		// 等待一段时间让S3完成写入
		if i > 0 {
			log.WithFields(map[string]interface{}{
				"package_id":     pkg.ID,
				"s3_key":         pkg.S3Key,
				"retry_attempt":  i,
				"max_retries":    maxRetries,
				"retry_interval": retryInterval,
			}).Info("等待后重试验证S3文件存在性")
			time.Sleep(retryInterval)
		}

		exists, err = s.s3Svc.ObjectExists(s.config.S3BucketName, pkg.S3Key)
		if err != nil {
			log.WithError(err).WithFields(map[string]interface{}{
				"package_id":    pkg.ID,
				"s3_key":        pkg.S3Key,
				"retry_attempt": i + 1,
			}).Warn("检查S3文件存在性时出错")
			// 如果是权限错误，立即返回
			if strings.Contains(err.Error(), "AccessDenied") || strings.Contains(err.Error(), "Access Denied") || strings.Contains(err.Error(), "403") {
				log.WithError(err).WithFields(map[string]interface{}{
					"package_id": pkg.ID,
					"s3_key":     pkg.S3Key,
				}).Error("验证S3文件存在性失败（权限错误）")
				s.db.Model(pkg).Updates(map[string]interface{}{
					"status":        models.DownloadPackageStatusFailed,
					"error_message": fmt.Sprintf("验证S3文件存在性失败（权限错误）: %v", err),
				})
				return
			}
			// 其他错误，继续重试
			continue
		}

		if exists {
			log.WithFields(map[string]interface{}{
				"package_id":    pkg.ID,
				"s3_key":        pkg.S3Key,
				"retry_attempt": i + 1,
			}).Info("S3文件验证成功")
			// 文件存在，验证成功
			break
		}

		log.WithFields(map[string]interface{}{
			"package_id":    pkg.ID,
			"s3_key":        pkg.S3Key,
			"retry_attempt": i + 1,
			"max_retries":   maxRetries,
		}).Warn("S3文件不存在，将继续重试")
		// 文件不存在，继续重试
	}

	// 检查最终结果
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"package_id":  pkg.ID,
			"s3_key":      pkg.S3Key,
			"max_retries": maxRetries,
		}).Error("验证S3文件存在性失败（重试后）")
		s.db.Model(pkg).Updates(map[string]interface{}{
			"status":        models.DownloadPackageStatusFailed,
			"error_message": fmt.Sprintf("验证S3文件存在性失败（重试%d次后）: %v", maxRetries, err),
		})
		return
	}
	if !exists {
		log.WithFields(map[string]interface{}{
			"package_id":  pkg.ID,
			"s3_key":      pkg.S3Key,
			"max_retries": maxRetries,
		}).Error("文件上传后验证失败：S3中不存在文件")
		s.db.Model(pkg).Updates(map[string]interface{}{
			"status":        models.DownloadPackageStatusFailed,
			"error_message": fmt.Sprintf("文件上传后验证失败：S3中不存在文件 %s（已重试%d次）", pkg.S3Key, maxRetries),
		})
		return
	}

	// 更新状态为处理中
	s.db.Model(pkg).Update("status", models.DownloadPackageStatusProcessing)
	log.WithField("package_id", pkg.ID).Info("文件验证成功，开始处理CloudFront配置")

	// 2. 获取S3域名
	s3Origin := s.s3Svc.GetBucketDomain(s.config.S3BucketName)
	log.WithFields(map[string]interface{}{
		"package_id": pkg.ID,
		"s3_origin":  s3Origin,
	}).Info("获取S3域名")

	// 3. 检查该域名是否已有CloudFront分发（用于支持同一域名下多个文件）
	var cloudFrontID string
	var cloudFrontDomain string

	// 查找该域名下是否已有其他下载包（已完成状态）
	var existingPackage models.DownloadPackage
	err = s.db.Where("domain_name = ? AND cloudfront_id != '' AND status = ?", pkg.DomainName, models.DownloadPackageStatusCompleted).
		First(&existingPackage).Error

	if err == nil && existingPackage.CloudFrontID != "" {
		// 已存在该域名的CloudFront分发，复用它
		cloudFrontID = existingPackage.CloudFrontID
		cloudFrontDomain = existingPackage.CloudFrontDomain
		log.WithFields(map[string]interface{}{
			"package_id":        pkg.ID,
			"domain_name":       pkg.DomainName,
			"cloudfront_id":     cloudFrontID,
			"cloudfront_domain": cloudFrontDomain,
		}).Info("复用已存在的CloudFront分发")
	} else {
		// 不存在，创建新的CloudFront分发
		// 计算originPath：同一域名下的所有文件都使用相同的目录路径 downloads/{domain_name}/
		originPath := fmt.Sprintf("/downloads/%s", pkg.DomainName)
		log.WithFields(map[string]interface{}{
			"package_id":   pkg.ID,
			"domain_name":  pkg.DomainName,
			"origin_path":  originPath,
			"s3_origin":    s3Origin,
			"dns_provider": domain.DNSProvider,
		}).Info("开始创建新的CloudFront分发")

		// 对于 Cloudflare 托管的域名，使用空证书 ARN（将使用 CloudFront 默认证书）
		certificateARN := domain.CertificateARN
		if domain.DNSProvider == models.DNSProviderCloudflare {
			certificateARN = "" // Cloudflare 域名使用 CloudFront 默认证书
			log.WithFields(map[string]interface{}{
				"package_id":  pkg.ID,
				"domain_name": pkg.DomainName,
			}).Info("Cloudflare 托管域名，使用 CloudFront 默认证书")
		}

		cloudFrontID, err = s.cloudFrontSvc.CreateDistributionForLargeFileDownload(
			pkg.DomainName,
			certificateARN,
			s3Origin,
			originPath,
		)
		if err != nil {
			log.WithError(err).WithFields(map[string]interface{}{
				"package_id":  pkg.ID,
				"domain_name": pkg.DomainName,
				"origin_path": originPath,
			}).Error("创建CloudFront分发失败")
			s.db.Model(pkg).Updates(map[string]interface{}{
				"status":        models.DownloadPackageStatusFailed,
				"error_message": fmt.Sprintf("创建CloudFront分发失败: %v", err),
			})
			return
		}

		log.WithFields(map[string]interface{}{
			"package_id":    pkg.ID,
			"cloudfront_id": cloudFrontID,
		}).Info("CloudFront分发创建成功，获取域名")

		// 获取CloudFront域名
		cloudFrontDomain, err = s.cloudFrontSvc.GetDistributionDomain(cloudFrontID)
		if err != nil {
			log.WithError(err).WithFields(map[string]interface{}{
				"package_id":    pkg.ID,
				"cloudfront_id": cloudFrontID,
			}).Error("获取CloudFront域名失败")
			s.db.Model(pkg).Updates(map[string]interface{}{
				"status":        models.DownloadPackageStatusFailed,
				"error_message": fmt.Sprintf("获取CloudFront域名失败: %v", err),
			})
			return
		}

		log.WithFields(map[string]interface{}{
			"package_id":        pkg.ID,
			"cloudfront_id":     cloudFrontID,
			"cloudfront_domain": cloudFrontDomain,
		}).Info("CloudFront域名获取成功")
	}
	// 4. 将域名绑定到CloudFront（创建Route53 A记录）
	if domain.HostedZoneID != "" {
		log.WithFields(map[string]interface{}{
			"package_id":        pkg.ID,
			"domain_name":       pkg.DomainName,
			"hosted_zone_id":    domain.HostedZoneID,
			"cloudfront_domain": cloudFrontDomain,
		}).Info("开始配置Route53 DNS记录")
		// 等待一下让CloudFront分发完全部署
		time.Sleep(5 * time.Second)

		// 检查是否已存在记录
		exists, err := s.domainService.CheckCloudFrontCNAMERecord(domain, cloudFrontDomain)
		if err == nil && !exists {
			// 创建DNS记录指向CloudFront
			log.WithFields(map[string]interface{}{
				"package_id":        pkg.ID,
				"domain_name":       pkg.DomainName,
				"cloudfront_domain": cloudFrontDomain,
				"dns_provider":      domain.DNSProvider,
			}).Info("创建DNS记录")
			if err := s.domainService.CreateCloudFrontCNAMERecord(domain, cloudFrontDomain); err != nil {
				log.WithError(err).WithFields(map[string]interface{}{
					"package_id":        pkg.ID,
					"domain_name":       pkg.DomainName,
					"cloudfront_domain": cloudFrontDomain,
				}).Error("创建DNS记录失败")
				s.db.Model(pkg).Updates(map[string]interface{}{
					"status":        models.DownloadPackageStatusFailed,
					"error_message": fmt.Sprintf("创建DNS记录失败: %v", err),
				})
				return
			}
			log.WithFields(map[string]interface{}{
				"package_id":  pkg.ID,
				"domain_name": pkg.DomainName,
			}).Info("DNS记录创建成功")
		} else if err != nil {
			log.WithError(err).WithFields(map[string]interface{}{
				"package_id":  pkg.ID,
				"domain_name": pkg.DomainName,
			}).Warn("检查DNS记录时出错")
		} else {
			log.WithFields(map[string]interface{}{
				"package_id":  pkg.ID,
				"domain_name": pkg.DomainName,
			}).Info("DNS记录已存在，跳过创建")
		}
	} else {
		log.WithFields(map[string]interface{}{
			"package_id":  pkg.ID,
			"domain_name": pkg.DomainName,
		}).Warn("域名未配置HostedZoneID，跳过Route53 DNS配置")
	}

	// 5. 构建下载URL
	downloadURL := fmt.Sprintf("https://%s/%s", pkg.DomainName, pkg.FileName)

	// 确保 CloudFront 分发已启用
	log.WithFields(map[string]interface{}{
		"package_id":    pkg.ID,
		"cloudfront_id": cloudFrontID,
	}).Info("启用CloudFront分发")
	enabled := true
	if err := s.cloudFrontSvc.UpdateDistribution(cloudFrontID, nil, "", &enabled); err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"package_id":    pkg.ID,
			"cloudfront_id": cloudFrontID,
		}).Error("启用CloudFront分发失败")
		s.db.Model(pkg).Updates(map[string]interface{}{
			"status":        models.DownloadPackageStatusFailed,
			"error_message": fmt.Sprintf("启用CloudFront分发失败: %v", err),
		})
		return
	}

	// 更新下载包信息
	s.db.Model(pkg).Updates(map[string]interface{}{
		"cloudfront_id":     cloudFrontID,
		"cloudfront_domain": cloudFrontDomain,
		"download_url":      downloadURL,
		"status":            models.DownloadPackageStatusCompleted,
	})

	log.WithFields(map[string]interface{}{
		"package_id":        pkg.ID,
		"domain_name":       pkg.DomainName,
		"file_name":         pkg.FileName,
		"cloudfront_id":     cloudFrontID,
		"cloudfront_domain": cloudFrontDomain,
		"download_url":      downloadURL,
		"s3_key":            pkg.S3Key,
	}).Info("下载包处理完成")
}

// GetDownloadPackage 获取下载包信息
func (s *DownloadPackageService) GetDownloadPackage(id uint) (*models.DownloadPackage, error) {
	var pkg models.DownloadPackage
	if err := s.db.Preload("Domain").First(&pkg, id).Error; err != nil {
		return nil, fmt.Errorf("下载包不存在: %w", err)
	}
	return &pkg, nil
}

// GetCloudFrontStatus 获取CloudFront分发状态
func (s *DownloadPackageService) GetCloudFrontStatus(cloudFrontID string) (string, error) {
	if cloudFrontID == "" {
		return "", nil
	}

	dist, err := s.cloudFrontSvc.GetDistribution(cloudFrontID)
	if err != nil {
		return "", err
	}

	if dist == nil || dist.Status == nil {
		return "", fmt.Errorf("无法获取分发状态")
	}

	return *dist.Status, nil
}

// GetCloudFrontEnabled 获取CloudFront分发启用状态
func (s *DownloadPackageService) GetCloudFrontEnabled(cloudFrontID string) (bool, error) {
	if cloudFrontID == "" {
		return false, nil
	}

	dist, err := s.cloudFrontSvc.GetDistribution(cloudFrontID)
	if err != nil {
		return false, err
	}

	if dist == nil || dist.DistributionConfig == nil {
		return false, fmt.Errorf("无法获取分发配置")
	}

	if dist.DistributionConfig.Enabled == nil {
		return false, nil
	}

	return *dist.DistributionConfig.Enabled, nil
}

// GetCloudFrontOriginPathInfo 获取CloudFront OriginPath信息（当前路径和期望路径）
func (s *DownloadPackageService) GetCloudFrontOriginPathInfo(pkg *models.DownloadPackage) (currentPath, expectedPath string, err error) {
	// 计算期望的 originPath：同一域名下的所有文件都使用相同的目录路径 downloads/{domain_name}/
	expectedPath = fmt.Sprintf("/downloads/%s", pkg.DomainName)

	// 获取当前的 OriginPath
	if pkg.CloudFrontID != "" {
		currentPath, err = s.cloudFrontSvc.GetDistributionOriginPath(pkg.CloudFrontID)
		if err != nil {
			return "", expectedPath, err
		}
	}

	return currentPath, expectedPath, nil
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

// ListDownloadPackagesByDomain 列出指定域名下的所有下载包
func (s *DownloadPackageService) ListDownloadPackagesByDomain(domainID uint) ([]models.DownloadPackage, error) {
	var packages []models.DownloadPackage

	if err := s.db.Preload("Domain").
		Where("domain_id = ?", domainID).
		Order("created_at DESC").
		Find(&packages).Error; err != nil {
		return nil, err
	}

	return packages, nil
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

	// 检查该CloudFront分发下是否还有其他文件
	// 如果这是该域名下最后一个文件，可以选择保留CloudFront分发（因为可能还会添加新文件）
	// 或者删除CloudFront分发（这里选择保留，因为删除CloudFront分发需要先禁用，然后等待，比较复杂）
	var otherPackagesCount int64
	s.db.Model(&models.DownloadPackage{}).
		Where("cloudfront_id = ? AND id != ? AND deleted_at IS NULL", pkg.CloudFrontID, pkg.ID).
		Count(&otherPackagesCount)

	// 注意：CloudFront分发可能被其他下载包使用，所以不删除
	// 即使这是最后一个文件，也保留CloudFront分发，方便后续添加新文件

	// 从数据库删除（软删除）
	if err := s.db.Delete(pkg).Error; err != nil {
		return fmt.Errorf("删除下载包记录失败: %w", err)
	}

	return nil
}

// DownloadPackageStatus 下载包检查状态
type DownloadPackageStatus struct {
	PackageExists                bool     `json:"package_exists"`
	S3FileExists                 bool     `json:"s3_file_exists"`
	S3FileError                  string   `json:"s3_file_error,omitempty"`
	CloudFrontExists             bool     `json:"cloudfront_exists"`
	CloudFrontError              string   `json:"cloudfront_error,omitempty"`
	CloudFrontEnabled            bool     `json:"cloudfront_enabled"`
	CloudFrontEnabledError       string   `json:"cloudfront_enabled_error,omitempty"`
	CloudFrontOriginPathMatch    bool     `json:"cloudfront_origin_path_match"`
	CloudFrontOriginPathError    string   `json:"cloudfront_origin_path_error,omitempty"`
	CloudFrontOriginPathCurrent  string   `json:"cloudfront_origin_path_current,omitempty"`
	CloudFrontOriginPathExpected string   `json:"cloudfront_origin_path_expected,omitempty"`
	Route53DNSConfigured         bool     `json:"route53_dns_configured"`
	Route53DNSError              string   `json:"route53_dns_error,omitempty"`
	DownloadURLAccessible        bool     `json:"download_url_accessible"`
	DownloadURLError             string   `json:"download_url_error,omitempty"`
	CertificateFound             bool     `json:"certificate_found"`
	CertificateARN               string   `json:"certificate_arn,omitempty"`
	Issues                       []string `json:"issues"`
	CanFix                       bool     `json:"can_fix"`
}

// CheckDownloadPackage 检查下载包状态
func (s *DownloadPackageService) CheckDownloadPackage(id uint) (*DownloadPackageStatus, error) {
	status := &DownloadPackageStatus{
		Issues: []string{},
	}

	// 获取下载包
	pkg, err := s.GetDownloadPackage(id)
	if err != nil {
		return nil, fmt.Errorf("获取下载包失败: %w", err)
	}
	status.PackageExists = true

	// 获取域名信息
	domain, err := s.domainService.GetDomain(pkg.DomainID)
	if err != nil {
		status.Issues = append(status.Issues, fmt.Sprintf("获取域名信息失败: %v", err))
	} else {
		status.CertificateFound = domain.CertificateARN != ""
		status.CertificateARN = domain.CertificateARN
	}

	// 检查S3文件是否存在
	if s.config.S3BucketName != "" && pkg.S3Key != "" {
		exists, err := s.s3Svc.ObjectExists(s.config.S3BucketName, pkg.S3Key)
		if err != nil {
			status.S3FileError = err.Error()
			status.Issues = append(status.Issues, fmt.Sprintf("检查S3文件失败: %v", err))
		} else if !exists {
			status.Issues = append(status.Issues, "S3文件不存在")
		} else {
			status.S3FileExists = true
		}
	} else {
		if pkg.S3Key == "" {
			status.Issues = append(status.Issues, "S3键未配置")
		}
		if s.config.S3BucketName == "" {
			status.Issues = append(status.Issues, "S3存储桶名称未配置")
		}
	}

	// 检查CloudFront分发是否存在
	if pkg.CloudFrontID != "" {
		_, err := s.cloudFrontSvc.GetDistribution(pkg.CloudFrontID)
		if err != nil {
			status.CloudFrontError = err.Error()
			status.Issues = append(status.Issues, fmt.Sprintf("CloudFront分发不存在或无法访问: %v", err))
		} else {
			status.CloudFrontExists = true

			// 检查 CloudFront 是否已启用
			enabled, err := s.GetCloudFrontEnabled(pkg.CloudFrontID)
			if err != nil {
				status.CloudFrontEnabledError = fmt.Sprintf("检查CloudFront启用状态失败: %v", err)
				status.Issues = append(status.Issues, "检查CloudFront启用状态失败")
			} else if !enabled {
				status.CloudFrontEnabledError = "CloudFront分发已禁用"
				status.Issues = append(status.Issues, "CloudFront分发已禁用，需要启用后才能正常使用")
			} else {
				status.CloudFrontEnabled = true
			}

			// 检查 CloudFront OriginPath 是否匹配
			// 计算期望的 originPath：同一域名下的所有文件都使用相同的目录路径 downloads/{domain_name}/
			expectedOriginPath := fmt.Sprintf("/downloads/%s", pkg.DomainName)
			status.CloudFrontOriginPathExpected = expectedOriginPath

			// 获取当前的 OriginPath
			currentOriginPath, err := s.cloudFrontSvc.GetDistributionOriginPath(pkg.CloudFrontID)
			if err != nil {
				status.CloudFrontOriginPathError = fmt.Sprintf("获取 CloudFront OriginPath 失败: %v", err)
				status.Issues = append(status.Issues, "检查 CloudFront OriginPath 失败")
			} else {
				status.CloudFrontOriginPathCurrent = currentOriginPath
				if currentOriginPath != expectedOriginPath {
					status.CloudFrontOriginPathError = fmt.Sprintf("OriginPath 不匹配: 当前=%s, 期望=%s", currentOriginPath, expectedOriginPath)
					status.Issues = append(status.Issues, fmt.Sprintf("CloudFront OriginPath 不匹配，当前指向 %s，期望指向 %s", currentOriginPath, expectedOriginPath))
				} else {
					status.CloudFrontOriginPathMatch = true
				}
			}

			// 检查 Route 53 DNS 记录是否指向正确的 CloudFront
			if domain.HostedZoneID != "" && pkg.CloudFrontDomain != "" {
				exists, err := s.domainService.CheckCloudFrontCNAMERecord(domain, pkg.CloudFrontDomain)
				if err != nil {
					status.Route53DNSError = fmt.Sprintf("检查Route 53 DNS记录失败: %v", err)
					status.Issues = append(status.Issues, "检查Route 53 DNS记录失败")
				} else if !exists {
					status.Route53DNSError = "未配置Route 53 DNS记录或指向错误的CloudFront分发"
					status.Issues = append(status.Issues, "Route 53 DNS记录未配置或指向错误")
				} else {
					status.Route53DNSConfigured = true
				}
			} else {
				if domain.HostedZoneID == "" {
					status.Issues = append(status.Issues, "域名未配置Route 53托管区域")
				}
				if pkg.CloudFrontDomain == "" {
					status.Issues = append(status.Issues, "CloudFront域名未配置")
				}
			}
		}
	} else {
		status.Issues = append(status.Issues, "未创建CloudFront分发")
	}

	// 检查下载URL是否可访问（如果已配置）
	if pkg.DownloadURL != "" {
		// 实际测试下载 URL 的可访问性
		client := &http.Client{
			Timeout: 10 * time.Second,
		}

		resp, err := client.Head(pkg.DownloadURL)
		if err != nil {
			status.DownloadURLError = fmt.Sprintf("无法访问下载URL: %v", err)
			status.Issues = append(status.Issues, fmt.Sprintf("下载URL无法访问: %v", err))
		} else {
			defer resp.Body.Close()
			if resp.StatusCode == 200 || resp.StatusCode == 206 {
				status.DownloadURLAccessible = true
			} else if resp.StatusCode == 403 {
				status.DownloadURLError = fmt.Sprintf("访问被拒绝 (HTTP %d)，可能是S3 bucket policy配置问题", resp.StatusCode)
				status.Issues = append(status.Issues, "下载URL返回403错误，可能是S3 bucket policy配置问题")
			} else {
				status.DownloadURLError = fmt.Sprintf("返回错误状态码: HTTP %d", resp.StatusCode)
				status.Issues = append(status.Issues, fmt.Sprintf("下载URL返回错误状态码: HTTP %d", resp.StatusCode))
			}
		}
	} else {
		status.Issues = append(status.Issues, "下载URL未配置")
	}

	// 判断是否可以修复
	status.CanFix = len(status.Issues) > 0 && s.config.S3BucketName != ""

	return status, nil
}

// FixDownloadPackage 修复下载包
func (s *DownloadPackageService) FixDownloadPackage(id uint) error {
	// 获取下载包
	pkg, err := s.GetDownloadPackage(id)
	if err != nil {
		return fmt.Errorf("获取下载包失败: %w", err)
	}

	// 获取域名信息
	domain, err := s.domainService.GetDomain(pkg.DomainID)
	if err != nil {
		return fmt.Errorf("获取域名信息失败: %w", err)
	}

	// 检查证书状态
	if domain.CertificateStatus != "issued" {
		return fmt.Errorf("域名证书未签发，当前状态: %s", domain.CertificateStatus)
	}

	// 如果已有CloudFront ID，先检查是否存在和是否已启用
	if pkg.CloudFrontID != "" {
		dist, err := s.cloudFrontSvc.GetDistribution(pkg.CloudFrontID)
		if err != nil {
			// CloudFront不存在，清除ID，重新创建
			s.db.Model(pkg).Update("cloudfront_id", "")
			pkg.CloudFrontID = ""
		} else {
			// 检查是否已启用，如果未启用则启用它
			if dist != nil && dist.DistributionConfig != nil {
				enabled := dist.DistributionConfig.Enabled
				if enabled == nil || !*enabled {
					enabledValue := true
					if err := s.cloudFrontSvc.UpdateDistribution(pkg.CloudFrontID, nil, "", &enabledValue); err != nil {
						return fmt.Errorf("启用CloudFront分发失败: %w", err)
					}
				}
			}

			// 检查并更新 OriginPath
			// 计算期望的 originPath：同一域名下的所有文件都使用相同的目录路径 downloads/{domain_name}/
			expectedOriginPath := fmt.Sprintf("/downloads/%s", pkg.DomainName)

			// 获取当前的 OriginPath
			currentOriginPath, err := s.cloudFrontSvc.GetDistributionOriginPath(pkg.CloudFrontID)
			if err != nil {
				return fmt.Errorf("获取 CloudFront OriginPath 失败: %w", err)
			}

			// 如果路径不匹配，更新它
			if currentOriginPath != expectedOriginPath {
				if err := s.cloudFrontSvc.UpdateDistributionOriginPath(pkg.CloudFrontID, expectedOriginPath); err != nil {
					return fmt.Errorf("更新 CloudFront OriginPath 失败: %w", err)
				}
			}
		}
	}

	// 确保 S3 bucket policy 允许公开访问 downloads/* 路径
	if s.config.S3BucketName != "" {
		if err := s.s3Svc.EnsureBucketPolicyForDownloads(s.config.S3BucketName); err != nil {
			return fmt.Errorf("配置 S3 bucket policy 失败: %w", err)
		}
	}

	// 检查S3文件是否存在
	if pkg.S3Key != "" && s.config.S3BucketName != "" {
		exists, err := s.s3Svc.ObjectExists(s.config.S3BucketName, pkg.S3Key)
		if err != nil {
			return fmt.Errorf("检查S3文件失败: %w", err)
		}
		if !exists {
			return fmt.Errorf("S3文件不存在，无法修复。请重新上传文件")
		}
	}

	// 如果CloudFront分发不存在，重新创建
	if pkg.CloudFrontID == "" {
		// 获取S3域名
		s3Origin := s.s3Svc.GetBucketDomain(s.config.S3BucketName)

		// 计算originPath：同一域名下的所有文件都使用相同的目录路径 downloads/{domain_name}/
		originPath := fmt.Sprintf("/downloads/%s", pkg.DomainName)

		// 对于 Cloudflare 托管的域名，使用空证书 ARN（将使用 CloudFront 默认证书）
		certificateARN := domain.CertificateARN
		if domain.DNSProvider == models.DNSProviderCloudflare {
			certificateARN = "" // Cloudflare 域名使用 CloudFront 默认证书
		}

		// 创建CloudFront分发
		cloudFrontID, err := s.cloudFrontSvc.CreateDistributionForLargeFileDownload(
			pkg.DomainName,
			certificateARN,
			s3Origin,
			originPath,
		)
		if err != nil {
			return fmt.Errorf("创建CloudFront分发失败: %w", err)
		}

		// 获取CloudFront域名
		cloudFrontDomain, err := s.cloudFrontSvc.GetDistributionDomain(cloudFrontID)
		if err != nil {
			return fmt.Errorf("获取CloudFront域名失败: %w", err)
		}

		// 更新下载包信息
		s.db.Model(pkg).Updates(map[string]interface{}{
			"cloudfront_id":     cloudFrontID,
			"cloudfront_domain": cloudFrontDomain,
		})
		pkg.CloudFrontID = cloudFrontID
		pkg.CloudFrontDomain = cloudFrontDomain
	}

	// 检查并创建 Route 53 DNS 记录（如果不存在）
	if domain.HostedZoneID != "" && pkg.CloudFrontDomain != "" {
		exists, err := s.domainService.CheckCloudFrontCNAMERecord(domain, pkg.CloudFrontDomain)
		if err != nil {
			// 检查失败，尝试创建
			if err := s.domainService.CreateCloudFrontCNAMERecord(domain, pkg.CloudFrontDomain); err != nil {
				return fmt.Errorf("创建DNS记录失败: %w", err)
			}
		} else if !exists {
			// 记录不存在，创建它
			if err := s.domainService.CreateCloudFrontCNAMERecord(domain, pkg.CloudFrontDomain); err != nil {
				return fmt.Errorf("创建DNS记录失败: %w", err)
			}
		}
	}

	// 构建下载URL
	downloadURL := fmt.Sprintf("https://%s/%s", pkg.DomainName, pkg.FileName)

	// 确保 CloudFront 分发已启用
	enabled := true
	if err := s.cloudFrontSvc.UpdateDistribution(pkg.CloudFrontID, nil, "", &enabled); err != nil {
		return fmt.Errorf("启用CloudFront分发失败: %w", err)
	}

	// 更新下载包状态和信息
	s.db.Model(pkg).Updates(map[string]interface{}{
		"download_url": downloadURL,
		"status":       models.DownloadPackageStatusCompleted,
	})

	return nil
}
