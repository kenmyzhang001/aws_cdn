package services

import (
	"aws_cdn/internal/config"
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
	// 验证域名是否存在
	domain, err := s.domainService.GetDomain(domainID)
	if err != nil {
		return nil, fmt.Errorf("域名不存在: %w", err)
	}

	// 检查域名证书状态
	if domain.CertificateStatus != "issued" {
		return nil, fmt.Errorf("域名证书未签发，当前状态: %s", domain.CertificateStatus)
	}

	// 使用域名的domain_name作为下载域名
	domainName := domain.DomainName

	// 检查域名是否已被重定向规则使用
	isUsed, err := s.CheckDomainUsedByRedirect(domainName)
	if err != nil {
		return nil, fmt.Errorf("检查域名使用状态失败: %w", err)
	}
	if isUsed {
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

	// 确保 S3 bucket policy 允许公开访问 downloads/* 路径
	if s.config.S3BucketName != "" {
		if err := s.s3Svc.EnsureBucketPolicyForDownloads(s.config.S3BucketName); err != nil {
			s.db.Model(pkg).Updates(map[string]interface{}{
				"status":        models.DownloadPackageStatusFailed,
				"error_message": fmt.Sprintf("配置 S3 bucket policy 失败: %v", err),
			})
			return
		}
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

	// 上传文件到S3（使用public-read ACL以便CloudFront访问）
	if err := s.s3Svc.UploadFileWithACL(s.config.S3BucketName, pkg.S3Key, fileReader, contentType, "public-read"); err != nil {
		s.db.Model(pkg).Updates(map[string]interface{}{
			"status":        models.DownloadPackageStatusFailed,
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
			"status":        models.DownloadPackageStatusFailed,
			"error_message": fmt.Sprintf("创建CloudFront分发失败: %v", err),
		})
		return
	}

	// 获取CloudFront域名
	cloudFrontDomain, err := s.cloudFrontSvc.GetDistributionDomain(cloudFrontID)
	if err != nil {
		s.db.Model(pkg).Updates(map[string]interface{}{
			"status":        models.DownloadPackageStatusFailed,
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
					"status":        models.DownloadPackageStatusFailed,
					"error_message": fmt.Sprintf("创建Route53记录失败: %v", err),
				})
				return
			}
		}
	}

	// 5. 构建下载URL
	downloadURL := fmt.Sprintf("https://%s/%s", pkg.DomainName, pkg.FileName)

	// 确保 CloudFront 分发已启用
	enabled := true
	if err := s.cloudFrontSvc.UpdateDistribution(cloudFrontID, nil, "", &enabled); err != nil {
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
			// 计算期望的 originPath
			expectedOriginPath := ""
			if strings.Contains(pkg.S3Key, "/") {
				parts := strings.Split(pkg.S3Key, "/")
				if len(parts) > 1 {
					expectedOriginPath = "/" + strings.Join(parts[:len(parts)-1], "/")
				}
			}
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
				exists, err := s.domainService.CheckCloudFrontAliasRecord(domain.HostedZoneID, pkg.DomainName, pkg.CloudFrontDomain)
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
			// 计算期望的 originPath
			expectedOriginPath := ""
			if strings.Contains(pkg.S3Key, "/") {
				parts := strings.Split(pkg.S3Key, "/")
				if len(parts) > 1 {
					expectedOriginPath = "/" + strings.Join(parts[:len(parts)-1], "/")
				}
			}

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

		// 计算originPath
		originPath := ""
		if strings.Contains(pkg.S3Key, "/") {
			parts := strings.Split(pkg.S3Key, "/")
			if len(parts) > 1 {
				originPath = "/" + strings.Join(parts[:len(parts)-1], "/")
			}
		}

		// 创建CloudFront分发
		cloudFrontID, err := s.cloudFrontSvc.CreateDistributionForLargeFileDownload(
			pkg.DomainName,
			domain.CertificateARN,
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
		exists, err := s.domainService.CheckCloudFrontAliasRecord(domain.HostedZoneID, pkg.DomainName, pkg.CloudFrontDomain)
		if err != nil {
			// 检查失败，尝试创建
			if err := s.domainService.CreateCloudFrontAliasRecord(domain.HostedZoneID, pkg.DomainName, pkg.CloudFrontDomain); err != nil {
				return fmt.Errorf("创建Route 53 DNS记录失败: %w", err)
			}
		} else if !exists {
			// 记录不存在，创建它
			if err := s.domainService.CreateCloudFrontAliasRecord(domain.HostedZoneID, pkg.DomainName, pkg.CloudFrontDomain); err != nil {
				return fmt.Errorf("创建Route 53 DNS记录失败: %w", err)
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
