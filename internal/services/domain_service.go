package services

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/models"
	"aws_cdn/internal/services/aws"
	"aws_cdn/internal/services/cloudflare"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type DomainService struct {
	db            *gorm.DB
	route53Svc    *aws.Route53Service
	acmSvc        *aws.ACMService
	cloudFrontSvc *aws.CloudFrontService
	s3Svc         *aws.S3Service
	cloudflareSvc *cloudflare.CloudflareService
}

func NewDomainService(db *gorm.DB, route53Svc *aws.Route53Service, acmSvc *aws.ACMService, cloudFrontSvc *aws.CloudFrontService, s3Svc *aws.S3Service, cloudflareSvc *cloudflare.CloudflareService) *DomainService {
	return &DomainService{
		db:            db,
		route53Svc:    route53Svc,
		acmSvc:        acmSvc,
		cloudFrontSvc: cloudFrontSvc,
		s3Svc:         s3Svc,
		cloudflareSvc: cloudflareSvc,
	}
}

// TransferDomain 转入域名到 AWS 或 Cloudflare
func (s *DomainService) TransferDomain(domainName, registrar string, dnsProvider models.DNSProvider, groupID *uint) (*models.Domain, error) {
	log := logger.GetLogger()
	log.WithFields(map[string]interface{}{
		"domain_name":  domainName,
		"registrar":    registrar,
		"dns_provider": dnsProvider,
	}).Info("开始转入域名")

	// 检查域名是否已存在
	var existingDomain models.Domain
	if err := s.db.Where("domain_name = ?", domainName).First(&existingDomain).Error; err == nil {
		log.WithFields(map[string]interface{}{
			"domain_name": domainName,
			"existing_id": existingDomain.ID,
		}).Warn("域名已存在")
		return nil, fmt.Errorf("域名 %s 已存在", domainName)
	}

	var hostedZoneID string
	var nsServers []string
	var nsServersJSON string

	if dnsProvider == models.DNSProviderCloudflare {
		// Cloudflare: 获取Zone ID
		log.WithField("domain_name", domainName).Info("开始获取Cloudflare Zone ID")
		zoneID, err := s.cloudflareSvc.GetZoneID(domainName)
		if err != nil {
			log.WithError(err).WithField("domain_name", domainName).Error("获取Cloudflare Zone ID失败")
			return nil, fmt.Errorf("获取Cloudflare Zone ID失败: %w", err)
		}
		hostedZoneID = zoneID
		nsServers = []string{} // Cloudflare不需要NS服务器配置
		nsServersJSON = "[]"
		log.WithFields(map[string]interface{}{
			"domain_name":    domainName,
			"hosted_zone_id": hostedZoneID,
		}).Info("Cloudflare Zone ID获取成功")
	} else {
		// AWS Route53: 创建托管区域
		log.WithField("domain_name", domainName).Info("开始创建Route53托管区域")
		var err error
		hostedZoneID, nsServers, err = s.route53Svc.CreateHostedZone(domainName)
		if err != nil {
			log.WithError(err).WithField("domain_name", domainName).Error("创建托管区域失败")
			return nil, fmt.Errorf("创建托管区域失败: %w", err)
		}
		log.WithFields(map[string]interface{}{
			"domain_name":    domainName,
			"hosted_zone_id": hostedZoneID,
			"ns_servers":     nsServers,
		}).Info("Route53托管区域创建成功")

		// 格式化 NS 服务器为 JSON
		nsServersJSON, err = aws.FormatNServersJSON(nsServers)
		if err != nil {
			log.WithError(err).WithField("domain_name", domainName).Error("格式化NS服务器失败")
			return nil, fmt.Errorf("格式化 NS 服务器失败: %w", err)
		}
	}

	// 如果没有指定分组，使用默认分组
	var finalGroupID *uint
	if groupID == nil {
		groupService := NewGroupService(s.db)
		defaultGroup, err := groupService.GetOrCreateDefaultGroup()
		if err != nil {
			log.WithError(err).Warn("获取默认分组失败，将不设置分组")
		} else {
			finalGroupID = &defaultGroup.ID
		}
	} else {
		finalGroupID = groupID
	}

	// 创建域名记录
	domain := &models.Domain{
		DomainName:   domainName,
		Registrar:    registrar,
		GroupID:      finalGroupID,
		DNSProvider:  dnsProvider,
		Status:       models.DomainStatusCompleted,
		NServers:     nsServersJSON,
		HostedZoneID: hostedZoneID,
	}

	if err := s.db.Create(domain).Error; err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"domain_name":    domainName,
			"hosted_zone_id": hostedZoneID,
		}).Error("创建域名记录失败")
		return nil, fmt.Errorf("创建域名记录失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"domain_id":      domain.ID,
		"domain_name":    domainName,
		"hosted_zone_id": hostedZoneID,
		"dns_provider":   dnsProvider,
	}).Info("域名记录创建成功")

	// 根据DNS提供商处理证书
	if dnsProvider == models.DNSProviderAWS {
		log.WithFields(map[string]interface{}{
			"domain_id":   domain.ID,
			"domain_name": domainName,
		}).Info("开始异步请求证书")
		// 异步请求证书（证书验证不影响域名转入状态）
		go s.requestCertificateAsync(domain)
	} else if dnsProvider == models.DNSProviderCloudflare {
		log.WithFields(map[string]interface{}{
			"domain_id":    domain.ID,
			"domain_name":  domainName,
			"dns_provider": dnsProvider,
		}).Info("Cloudflare 托管域名，开始生成泛域名证书并导入ACM")
		// 异步生成Cloudflare证书并导入ACM
		go s.generateAndImportCloudflareCertificateAsync(domain)
	}

	return domain, nil
}

// requestCertificateAsync 异步请求证书
// 注意：证书验证失败不应该影响域名转入状态，域名转入成功与否基于 Route53 Hosted Zone 是否创建成功
func (s *DomainService) requestCertificateAsync(domain *models.Domain) {
	log := logger.GetLogger()
	log.WithFields(map[string]interface{}{
		"domain_id":   domain.ID,
		"domain_name": domain.DomainName,
	}).Info("开始异步请求证书")

	certificateARN, err := s.acmSvc.RequestCertificate(domain.DomainName)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"domain_id":   domain.ID,
			"domain_name": domain.DomainName,
		}).Error("证书请求失败")
		// 证书请求失败，只更新证书状态，不影响域名转入状态
		s.db.Model(domain).Update("certificate_status", "failed")
		return
	}

	log.WithFields(map[string]interface{}{
		"domain_id":       domain.ID,
		"domain_name":     domain.DomainName,
		"certificate_arn": certificateARN,
	}).Info("证书请求成功，等待验证")

	s.db.Model(domain).Updates(map[string]interface{}{
		"certificate_arn":    certificateARN,
		"certificate_status": "pending",
	})

	// 等待证书验证（最多等待 1 小时）
	log.WithFields(map[string]interface{}{
		"domain_id":       domain.ID,
		"certificate_arn": certificateARN,
		"timeout":         "1小时",
	}).Info("开始等待证书验证")
	if err := s.acmSvc.WaitForCertificateValidation(certificateARN, 1*time.Hour); err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"domain_id":       domain.ID,
			"certificate_arn": certificateARN,
		}).Error("证书验证失败")
		// 证书验证失败，只更新证书状态，不影响域名转入状态
		s.db.Model(domain).Update("certificate_status", "failed")
		return
	}

	log.WithFields(map[string]interface{}{
		"domain_id":       domain.ID,
		"domain_name":     domain.DomainName,
		"certificate_arn": certificateARN,
	}).Info("证书验证成功")
	// 证书验证成功，只更新证书状态
	// 域名转入状态已经在创建 Hosted Zone 时设置为 completed，这里不需要再更新
	s.db.Model(domain).Update("certificate_status", "issued")
}

// GetDomain 获取域名信息
func (s *DomainService) GetDomain(id uint) (*models.Domain, error) {
	var domain models.Domain
	if err := s.db.First(&domain, id).Error; err != nil {
		return nil, fmt.Errorf("域名不存在: %w", err)
	}
	return &domain, nil
}

// DomainWithUsage 带使用状态的域名信息
type DomainWithUsage struct {
	models.Domain
	UsedByRedirect        bool `json:"used_by_redirect"`         // 是否被重定向使用
	UsedByDownloadPackage bool `json:"used_by_download_package"` // 是否被下载包使用
}

// CheckDomainUsage 检查域名的使用情况
func (s *DomainService) CheckDomainUsage(domainName string) (usedByRedirect bool, usedByDownloadPackage bool, err error) {
	// 检查是否被重定向规则使用（排除软删除的记录）
	var redirectCount int64
	if err := s.db.Model(&models.RedirectRule{}).
		Where("source_domain = ? AND deleted_at IS NULL", domainName).
		Count(&redirectCount).Error; err != nil {
		return false, false, err
	}
	usedByRedirect = redirectCount > 0

	// 检查是否被下载包使用（排除软删除的记录）
	var downloadPackageCount int64
	if err := s.db.Model(&models.DownloadPackage{}).
		Where("domain_name = ? AND deleted_at IS NULL", domainName).
		Count(&downloadPackageCount).Error; err != nil {
		return false, false, err
	}
	usedByDownloadPackage = downloadPackageCount > 0

	return usedByRedirect, usedByDownloadPackage, nil
}

// ListDomains 列出所有域名，支持按分组筛选
func (s *DomainService) ListDomains(page, pageSize int, groupID *uint) ([]DomainWithUsage, int64, error) {
	var domains []models.Domain
	var total int64

	offset := (page - 1) * pageSize

	query := s.db.Model(&models.Domain{}).Where("deleted_at IS NULL")
	if groupID != nil {
		query = query.Where("group_id = ?", *groupID)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("id DESC").Offset(offset).Limit(pageSize).Find(&domains).Error; err != nil {
		return nil, 0, err
	}

	// 转换为带使用状态的域名列表
	result := make([]DomainWithUsage, len(domains))
	for i := range domains {
		// 为每个有证书的域名查询最新的证书状态
		if domains[i].CertificateARN != "" {
			status, err := s.acmSvc.GetCertificateStatus(domains[i].CertificateARN)
			if err == nil {
				// 更新证书状态（如果状态有变化，也更新数据库）
				if domains[i].CertificateStatus != status {
					domains[i].CertificateStatus = status
					// 异步更新数据库，不阻塞列表返回
					go func(domainID uint, certStatus string) {
						s.db.Model(&models.Domain{}).Where("id = ?", domainID).Update("certificate_status", certStatus)
					}(domains[i].ID, status)
				} else {
					domains[i].CertificateStatus = status
				}
			}
		}

		// 检查域名使用情况
		usedByRedirect, usedByDownloadPackage, err := s.CheckDomainUsage(domains[i].DomainName)
		if err != nil {
			// 如果检查失败，记录错误但不阻止返回
			fmt.Printf("检查域名 %s 使用状态失败: %v\n", domains[i].DomainName, err)
		}

		result[i] = DomainWithUsage{
			Domain:                domains[i],
			UsedByRedirect:        usedByRedirect,
			UsedByDownloadPackage: usedByDownloadPackage,
		}
	}

	return result, total, nil
}

// GetNServers 获取域名的 NS 服务器配置
func (s *DomainService) GetNServers(id uint) ([]string, error) {
	domain, err := s.GetDomain(id)
	if err != nil {
		return nil, err
	}

	// Cloudflare 托管的域名不需要 NS 服务器配置
	if domain.DNSProvider == models.DNSProviderCloudflare {
		return []string{}, nil
	}

	if domain.HostedZoneID == "" {
		return nil, fmt.Errorf("域名尚未创建托管区域")
	}

	nsServers, err := s.route53Svc.GetNameServers(domain.HostedZoneID)
	if err != nil {
		// 如果获取失败，尝试从数据库解析
		return aws.ParseNServersJSON(domain.NServers)
	}

	return nsServers, nil
}

// GenerateCertificate 生成域名证书
func (s *DomainService) GenerateCertificate(id uint) error {
	log := logger.GetLogger()
	log.WithField("domain_id", id).Info("开始生成证书")

	domain, err := s.GetDomain(id)
	if err != nil {
		log.WithError(err).WithField("domain_id", id).Error("获取域名信息失败")
		return err
	}

	// Cloudflare 托管的域名：生成 Origin 证书并导入 ACM
	if domain.DNSProvider == models.DNSProviderCloudflare {
		log.WithFields(map[string]interface{}{
			"domain_id":    id,
			"domain_name":  domain.DomainName,
			"dns_provider": domain.DNSProvider,
		}).Info("Cloudflare 托管域名，开始生成 Origin 证书并导入 ACM")

		// 检查证书状态，如果证书已存在且状态正常，则不需要重新生成
		if domain.CertificateARN != "" {
			status, err := s.acmSvc.GetCertificateStatus(domain.CertificateARN)
			if err == nil && status == "issued" {
				log.WithFields(map[string]interface{}{
					"domain_id":       id,
					"certificate_arn": domain.CertificateARN,
					"status":          status,
				}).Info("证书已存在且状态正常，无需重新生成")
				return fmt.Errorf("证书已存在且状态正常: %s (状态: %s)", domain.CertificateARN, status)
			}
			// 如果证书状态不正常，继续生成新证书（会覆盖旧的）
			log.WithFields(map[string]interface{}{
				"domain_id":       id,
				"certificate_arn": domain.CertificateARN,
				"status":          status,
			}).Info("证书状态不正常，将重新生成")
		}

		// 异步生成 Cloudflare Origin 证书并导入 ACM
		go s.generateAndImportCloudflareCertificateAsync(domain)
		return nil
	}

	var certificateARN string
	if domain.CertificateARN != "" {
		// 证书已存在，检查状态
		certificateARN = domain.CertificateARN
		log.WithFields(map[string]interface{}{
			"domain_id":       id,
			"domain_name":     domain.DomainName,
			"certificate_arn": certificateARN,
		}).Info("证书已存在，检查状态")
		status, err := s.acmSvc.GetCertificateStatus(certificateARN)
		if err != nil {
			log.WithError(err).WithFields(map[string]interface{}{
				"domain_id":       id,
				"certificate_arn": certificateARN,
			}).Error("获取证书状态失败")
			return fmt.Errorf("获取证书状态失败: %w", err)
		}

		// 如果证书状态是 PENDING_VALIDATION，需要添加验证记录
		if status == "PENDING_VALIDATION" {
			if domain.HostedZoneID == "" {
				log.WithFields(map[string]interface{}{
					"domain_id":       id,
					"certificate_arn": certificateARN,
				}).Error("证书已存在但缺少托管区域ID")
				return fmt.Errorf("证书已存在但缺少托管区域ID，无法添加验证记录")
			}

			// 获取证书验证记录
			validationRecords, err := s.acmSvc.GetCertificateValidationRecords(certificateARN)
			if err != nil {
				log.WithError(err).WithFields(map[string]interface{}{
					"domain_id":       id,
					"certificate_arn": certificateARN,
				}).Error("获取证书验证记录失败")
				return fmt.Errorf("获取证书验证记录失败: %w", err)
			}

			log.WithFields(map[string]interface{}{
				"domain_id":       id,
				"certificate_arn": certificateARN,
				"record_count":    len(validationRecords),
			}).Info("开始添加证书验证记录到Route53")

			// 将验证记录添加到 DNS 提供商
			for _, record := range validationRecords {
				if record.Type == "CNAME" {
					var err error
					if domain.DNSProvider == models.DNSProviderCloudflare {
						err = s.cloudflareSvc.CreateCNAMERecord(domain.HostedZoneID, record.Name, record.Value)
					} else {
						err = s.route53Svc.CreateCNAMERecord(domain.HostedZoneID, record.Name, record.Value)
					}
					if err != nil {
						log.WithError(err).WithFields(map[string]interface{}{
							"domain_id":    id,
							"record_name":  record.Name,
							"record_value": record.Value,
							"dns_provider": domain.DNSProvider,
						}).Error("添加CNAME验证记录失败")
						return fmt.Errorf("添加 CNAME 验证记录失败: %w", err)
					}
				}
			}

			log.WithFields(map[string]interface{}{
				"domain_id":       id,
				"certificate_arn": certificateARN,
			}).Info("证书验证记录添加成功，开始异步等待验证")

			// 更新证书状态
			s.db.Model(domain).Update("certificate_status", "pending_validation")

			// 异步等待证书验证
			go s.requestCertificateAsync(domain)

			return nil
		}

		// 如果证书已经是 ISSUED 或其他状态，直接返回
		log.WithFields(map[string]interface{}{
			"domain_id":       id,
			"certificate_arn": certificateARN,
			"status":          status,
		}).Info("证书已存在且状态为: " + status)
		return fmt.Errorf("证书已存在: %s (状态: %s)", certificateARN, status)
	}

	// 证书不存在，创建新证书
	log.WithFields(map[string]interface{}{
		"domain_id":   id,
		"domain_name": domain.DomainName,
	}).Info("证书不存在，开始创建新证书")
	certificateARN, err = s.acmSvc.RequestCertificate(domain.DomainName)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"domain_id":   id,
			"domain_name": domain.DomainName,
		}).Error("请求证书失败")
		return fmt.Errorf("请求证书失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"domain_id":       id,
		"certificate_arn": certificateARN,
	}).Info("证书请求成功")

	s.db.Model(domain).Updates(map[string]interface{}{
		"certificate_arn":    certificateARN,
		"certificate_status": "pending",
	})

	// 获取证书验证记录并添加到 Route 53
	if domain.HostedZoneID != "" {
		log.WithFields(map[string]interface{}{
			"domain_id":       id,
			"certificate_arn": certificateARN,
		}).Info("等待AWS生成验证记录")
		// 等待一下让 AWS 生成验证记录
		time.Sleep(2 * time.Second)

		validationRecords, err := s.acmSvc.GetCertificateValidationRecords(certificateARN)
		if err == nil && len(validationRecords) > 0 {
			log.WithFields(map[string]interface{}{
				"domain_id":    id,
				"record_count": len(validationRecords),
			}).Info("开始添加验证记录到Route53")
			// 将验证记录添加到 DNS 提供商
			for _, record := range validationRecords {
				if record.Type == "CNAME" {
					var err error
					if domain.DNSProvider == models.DNSProviderCloudflare {
						err = s.cloudflareSvc.CreateCNAMERecord(domain.HostedZoneID, record.Name, record.Value)
					} else {
						err = s.route53Svc.CreateCNAMERecord(domain.HostedZoneID, record.Name, record.Value)
					}
					if err != nil {
						log.WithError(err).WithFields(map[string]interface{}{
							"domain_id":    id,
							"record_name":  record.Name,
							"record_value": record.Value,
							"dns_provider": domain.DNSProvider,
						}).Error("添加验证记录失败")
					}
				}
			}
		}
	}

	// 异步等待证书验证
	go s.requestCertificateAsync(domain)

	return nil
}

// GetCertificateStatus 获取证书状态
func (s *DomainService) GetCertificateStatus(id uint) (string, error) {
	domain, err := s.GetDomain(id)
	if err != nil {
		return "", err
	}

	if domain.CertificateARN == "" {
		return "not_requested", nil
	}

	status, err := s.acmSvc.GetCertificateStatus(domain.CertificateARN)
	if err != nil {
		return domain.CertificateStatus, nil
	}

	// 更新数据库中的状态
	s.db.Model(domain).Update("certificate_status", status)

	return status, nil
}

// GetDomainStatus 获取域名转入状态
// 如果 Hosted Zone 存在，则返回 completed（域名已成功转入）
func (s *DomainService) GetDomainStatus(id uint) (models.DomainStatus, error) {
	domain, err := s.GetDomain(id)
	if err != nil {
		return "", err
	}

	// Cloudflare 托管的域名，如果 HostedZoneID 存在，说明已成功转入
	if domain.DNSProvider == models.DNSProviderCloudflare {
		if domain.HostedZoneID != "" {
			if domain.Status != models.DomainStatusCompleted {
				// 更新数据库中的状态
				s.db.Model(domain).Update("status", models.DomainStatusCompleted)
			}
			return models.DomainStatusCompleted, nil
		}
		return domain.Status, nil
	}

	// AWS 托管的域名：如果 Hosted Zone 存在，说明域名已成功转入，应该返回 completed
	// 这样可以修复之前因为证书验证失败而状态未更新的问题
	if domain.HostedZoneID != "" {
		// 验证 Hosted Zone 是否真的存在
		_, err := s.route53Svc.GetHostedZone(domain.HostedZoneID)
		if err == nil {
			// Hosted Zone 存在，域名已成功转入
			if domain.Status != models.DomainStatusCompleted {
				// 更新数据库中的状态
				s.db.Model(domain).Update("status", models.DomainStatusCompleted)
			}
			return models.DomainStatusCompleted, nil
		}
		// 如果 Hosted Zone 不存在，可能是被删除了，保持原状态
	}

	return domain.Status, nil
}

// UpdateDomainStatus 更新域名状态
func (s *DomainService) UpdateDomainStatus(id uint, status models.DomainStatus) error {
	return s.db.Model(&models.Domain{}).Where("id = ?", id).Update("status", status).Error
}

// DeleteDomain 删除域名
// 删除域名时会同时删除相关的 AWS 资源（Route53 Hosted Zone 和 ACM 证书）
// 对于 Cloudflare 托管的域名，只删除数据库记录
func (s *DomainService) DeleteDomain(id uint) error {
	log := logger.GetLogger()
	log.WithField("domain_id", id).Info("开始删除域名")

	// 获取域名信息
	domain, err := s.GetDomain(id)
	if err != nil {
		log.WithError(err).WithField("domain_id", id).Error("获取域名信息失败")
		return err
	}

	// 检查是否有下载包关联到此域名
	var downloadPackageCount int64
	if err := s.db.Model(&models.DownloadPackage{}).
		Where("domain_id = ? AND deleted_at IS NULL", id).
		Count(&downloadPackageCount).Error; err != nil {
		return fmt.Errorf("检查下载包关联失败: %w", err)
	}
	if downloadPackageCount > 0 {
		return fmt.Errorf("该域名下还有 %d 个下载包，无法删除。请先删除这些下载包后再删除域名", downloadPackageCount)
	}

	// 检查是否有重定向规则使用此域名
	var redirectCount int64
	if err := s.db.Model(&models.RedirectRule{}).
		Where("source_domain = ? AND deleted_at IS NULL", domain.DomainName).
		Count(&redirectCount).Error; err != nil {
		return fmt.Errorf("检查重定向规则关联失败: %w", err)
	}
	if redirectCount > 0 {
		return fmt.Errorf("该域名下还有 %d 个重定向规则，无法删除。请先删除这些重定向规则后再删除域名", redirectCount)
	}

	log.WithFields(map[string]interface{}{
		"domain_id":       id,
		"domain_name":     domain.DomainName,
		"dns_provider":    domain.DNSProvider,
		"certificate_arn": domain.CertificateARN,
		"hosted_zone_id":  domain.HostedZoneID,
	}).Info("获取域名信息成功，开始删除相关资源")

	// Cloudflare 托管的域名不需要删除 AWS 资源
	if domain.DNSProvider == models.DNSProviderCloudflare {
		log.WithFields(map[string]interface{}{
			"domain_id":    id,
			"domain_name":  domain.DomainName,
			"dns_provider": domain.DNSProvider,
		}).Info("Cloudflare 托管域名，只删除数据库记录")
	} else {
		// 删除 ACM 证书（如果存在）
		if domain.CertificateARN != "" {
			log.WithFields(map[string]interface{}{
				"domain_id":       id,
				"certificate_arn": domain.CertificateARN,
			}).Info("开始删除ACM证书")
			if err := s.acmSvc.DeleteCertificate(domain.CertificateARN); err != nil {
				log.WithError(err).WithFields(map[string]interface{}{
					"domain_id":       id,
					"certificate_arn": domain.CertificateARN,
				}).Warn("删除ACM证书失败（可能已不存在）")
			} else {
				log.WithFields(map[string]interface{}{
					"domain_id":       id,
					"certificate_arn": domain.CertificateARN,
				}).Info("ACM证书删除成功")
			}
		}

		// 删除 Route53 Hosted Zone（如果存在）
		if domain.HostedZoneID != "" {
			log.WithFields(map[string]interface{}{
				"domain_id":      id,
				"hosted_zone_id": domain.HostedZoneID,
			}).Info("开始删除Route53托管区域")
			if err := s.route53Svc.DeleteHostedZone(domain.HostedZoneID); err != nil {
				log.WithError(err).WithFields(map[string]interface{}{
					"domain_id":      id,
					"hosted_zone_id": domain.HostedZoneID,
				}).Warn("删除Route53托管区域失败（可能已不存在）")
			} else {
				log.WithFields(map[string]interface{}{
					"domain_id":      id,
					"hosted_zone_id": domain.HostedZoneID,
				}).Info("Route53托管区域删除成功")
			}
		}
	}

	// 从数据库删除域名记录（软删除）
	log.WithField("domain_id", id).Info("开始从数据库删除域名记录")
	if err := s.db.Delete(domain).Error; err != nil {
		log.WithError(err).WithField("domain_id", id).Error("删除域名记录失败")
		return fmt.Errorf("删除域名记录失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"domain_id":   id,
		"domain_name": domain.DomainName,
	}).Info("域名删除成功")

	return nil
}

// CreateCloudFrontCNAMERecord 创建指向 CloudFront 的 CNAME 记录
// 根据DNS提供商选择不同的实现
func (s *DomainService) CreateCloudFrontCNAMERecord(domain *models.Domain, cloudFrontDomainName string) error {
	// 确保cloudFrontDomainName以点结尾
	cloudFrontValue := cloudFrontDomainName
	if cloudFrontValue != "" && !strings.HasSuffix(cloudFrontValue, ".") {
		cloudFrontValue = cloudFrontValue + "."
	}

	if domain.DNSProvider == models.DNSProviderCloudflare {
		return s.cloudflareSvc.CreateCNAMERecord(domain.HostedZoneID, domain.DomainName, cloudFrontValue)
	} else {
		// AWS Route53: 使用Alias记录（A记录）
		return s.route53Svc.CreateAliasRecord(domain.HostedZoneID, domain.DomainName, cloudFrontDomainName)
	}
}

// CreateCloudFrontAliasRecord 创建指向 CloudFront 的 A 记录（Alias）- 仅用于AWS
func (s *DomainService) CreateCloudFrontAliasRecord(hostedZoneID, domainName, cloudFrontDomainName string) error {
	return s.route53Svc.CreateAliasRecord(hostedZoneID, domainName, cloudFrontDomainName)
}

// CheckCloudFrontAliasRecord 检查是否存在指向指定 CloudFront 分发的 A 记录（Alias）- 仅用于AWS
func (s *DomainService) CheckCloudFrontAliasRecord(hostedZoneID, domainName, cloudFrontDomainName string) (bool, error) {
	return s.route53Svc.CheckCloudFrontAliasRecord(hostedZoneID, domainName, cloudFrontDomainName)
}

// CheckCloudFrontCNAMERecord 检查是否存在指向 CloudFront 的 CNAME 记录
func (s *DomainService) CheckCloudFrontCNAMERecord(domain *models.Domain, cloudFrontDomainName string) (bool, error) {
	// 确保cloudFrontDomainName以点结尾
	cloudFrontValue := cloudFrontDomainName
	if cloudFrontValue != "" && !strings.HasSuffix(cloudFrontValue, ".") {
		cloudFrontValue = cloudFrontValue + "."
	}

	if domain.DNSProvider == models.DNSProviderCloudflare {
		return s.cloudflareSvc.CheckCNAMERecord(domain.HostedZoneID, domain.DomainName, cloudFrontValue)
	} else {
		// AWS Route53: 检查Alias记录
		return s.route53Svc.CheckCloudFrontAliasRecord(domain.HostedZoneID, domain.DomainName, cloudFrontDomainName)
	}
}

// CreateWWWCNAMERecord 为根域名创建 www 子域名的 CNAME 记录指向根域名
func (s *DomainService) CreateWWWCNAMERecord(domain *models.Domain) error {
	rootDomain := domain.DomainName
	// 确保 rootDomain 不是 www 子域名
	if strings.HasPrefix(rootDomain, "www.") {
		return nil // 如果已经是 www 子域名，不需要创建
	}

	// 构建 www 子域名
	wwwDomain := "www." + rootDomain

	// 确保 rootDomain 以点结尾（CNAME 值需要）
	rootDomainValue := rootDomain
	if rootDomainValue != "" && !strings.HasSuffix(rootDomainValue, ".") {
		rootDomainValue = rootDomainValue + "."
	}

	// 创建 CNAME 记录：www.example.com -> example.com
	if domain.DNSProvider == models.DNSProviderCloudflare {
		return s.cloudflareSvc.CreateCNAMERecord(domain.HostedZoneID, wwwDomain, rootDomainValue)
	} else {
		return s.route53Svc.CreateCNAMERecord(domain.HostedZoneID, wwwDomain, rootDomainValue)
	}
}

// CheckWWWCNAMERecord 检查是否存在 www 子域名的 CNAME 记录指向根域名
func (s *DomainService) CheckWWWCNAMERecord(domain *models.Domain) (bool, error) {
	rootDomain := domain.DomainName
	if strings.HasPrefix(rootDomain, "www.") {
		return true, nil // 如果已经是 www 子域名，不需要检查
	}

	wwwDomain := "www." + rootDomain
	rootDomainValue := rootDomain
	if rootDomainValue != "" && !strings.HasSuffix(rootDomainValue, ".") {
		rootDomainValue = rootDomainValue + "."
	}

	if domain.DNSProvider == models.DNSProviderCloudflare {
		return s.cloudflareSvc.CheckCNAMERecord(domain.HostedZoneID, wwwDomain, rootDomainValue)
	} else {
		return s.route53Svc.CheckWWWCNAMERecord(domain.HostedZoneID, rootDomain)
	}
}

// CertificateCheckResult 证书检查结果
type CertificateCheckResult struct {
	CertificateExists     bool     `json:"certificate_exists"`      // 证书是否存在
	CertificateStatus     string   `json:"certificate_status"`      // 证书状态
	ValidationRecords     []string `json:"validation_records"`      // 验证记录列表（格式：name:value）
	MissingCNAMERecords   []string `json:"missing_cname_records"`   // 缺失的CNAME记录
	IncorrectCNAMERecords []string `json:"incorrect_cname_records"` // 值不正确的CNAME记录
	HasIssues             bool     `json:"has_issues"`              // 是否有问题
	Issues                []string `json:"issues"`                  // 问题列表
}

// CheckCertificate 检查证书配置和CNAME记录
func (s *DomainService) CheckCertificate(id uint) (*CertificateCheckResult, error) {
	domain, err := s.GetDomain(id)
	if err != nil {
		return nil, err
	}

	// Cloudflare 托管的域名不需要检查证书
	if domain.DNSProvider == models.DNSProviderCloudflare {
		return &CertificateCheckResult{
			CertificateExists:     false,
			CertificateStatus:     "not_applicable",
			ValidationRecords:     []string{},
			MissingCNAMERecords:   []string{},
			IncorrectCNAMERecords: []string{},
			HasIssues:             false,
			Issues:                []string{"Cloudflare 托管的域名使用 CloudFront 默认证书，无需检查"},
		}, nil
	}

	result := &CertificateCheckResult{
		ValidationRecords:     []string{},
		MissingCNAMERecords:   []string{},
		IncorrectCNAMERecords: []string{},
		Issues:                []string{},
	}

	// 检查证书是否存在
	if domain.CertificateARN == "" {
		result.HasIssues = true
		result.Issues = append(result.Issues, "证书未申请")
		return result, nil
	}

	result.CertificateExists = true

	// 获取证书状态
	status, err := s.acmSvc.GetCertificateStatus(domain.CertificateARN)
	if err != nil {
		result.HasIssues = true
		result.Issues = append(result.Issues, fmt.Sprintf("获取证书状态失败: %v", err))
		return result, nil
	}

	result.CertificateStatus = status

	// 如果证书已签发，不需要检查验证记录
	if status == "issued" {
		return result, nil
	}

	// 如果证书状态是pending_validation或pending，需要检查验证记录
	if status == "pending_validation" || status == "pending" {
		// 获取证书验证记录
		validationRecords, err := s.acmSvc.GetCertificateValidationRecords(domain.CertificateARN)
		if err != nil {
			result.HasIssues = true
			result.Issues = append(result.Issues, fmt.Sprintf("获取证书验证记录失败: %v", err))
			return result, nil
		}

		if len(validationRecords) == 0 {
			result.HasIssues = true
			result.Issues = append(result.Issues, "证书验证记录为空")
			return result, nil
		}

		// 检查托管区域是否存在
		if domain.HostedZoneID == "" {
			result.HasIssues = true
			result.Issues = append(result.Issues, "缺少托管区域ID，无法检查CNAME记录")
			return result, nil
		}

		// 检查每个验证记录的CNAME是否存在于DNS提供商
		for _, record := range validationRecords {
			if record.Type == "CNAME" {
				recordDesc := fmt.Sprintf("%s -> %s", record.Name, record.Value)
				result.ValidationRecords = append(result.ValidationRecords, recordDesc)

				// 检查CNAME记录是否存在
				var exists bool
				var err error
				if domain.DNSProvider == models.DNSProviderCloudflare {
					exists, err = s.cloudflareSvc.CheckCNAMERecord(domain.HostedZoneID, record.Name, record.Value)
				} else {
					exists, err = s.route53Svc.CheckCertificateValidationCNAME(domain.HostedZoneID, record.Name, record.Value)
				}
				if err != nil {
					result.HasIssues = true
					result.Issues = append(result.Issues, fmt.Sprintf("检查CNAME记录失败 (%s): %v", record.Name, err))
					result.MissingCNAMERecords = append(result.MissingCNAMERecords, recordDesc)
					continue
				}

				if !exists {
					result.HasIssues = true
					result.MissingCNAMERecords = append(result.MissingCNAMERecords, recordDesc)
					result.Issues = append(result.Issues, fmt.Sprintf("CNAME记录缺失: %s", recordDesc))
				}
			}
		}
	}

	return result, nil
}

// FixCertificate 修复证书配置和CNAME记录
func (s *DomainService) FixCertificate(id uint) error {
	domain, err := s.GetDomain(id)
	if err != nil {
		return err
	}

	// Cloudflare 托管的域名：如果证书不存在或状态不正常，重新生成证书
	if domain.DNSProvider == models.DNSProviderCloudflare {
		// 如果证书不存在，直接生成
		if domain.CertificateARN == "" {
			return s.GenerateCertificate(id)
		}

		// 检查证书状态
		status, err := s.acmSvc.GetCertificateStatus(domain.CertificateARN)
		if err != nil {
			// 如果获取状态失败，可能是证书已被删除，重新生成
			return s.GenerateCertificate(id)
		}

		// 如果证书状态不是 issued，重新生成
		if status != "issued" {
			log := logger.GetLogger()
			log.WithFields(map[string]interface{}{
				"domain_id":       id,
				"certificate_arn": domain.CertificateARN,
				"status":          status,
			}).Info("Cloudflare 证书状态不正常，重新生成")
			return s.GenerateCertificate(id)
		}

		// 证书状态正常，无需修复
		return nil
	}

	// 如果证书不存在，尝试生成证书
	if domain.CertificateARN == "" {
		return s.GenerateCertificate(id)
	}

	// 获取证书状态
	status, err := s.acmSvc.GetCertificateStatus(domain.CertificateARN)
	if err != nil {
		return fmt.Errorf("获取证书状态失败: %w", err)
	}

	// 如果证书已签发，无需修复
	if status == "issued" {
		return nil
	}

	// 如果证书状态是pending_validation或pending，需要添加验证记录
	if status == "pending_validation" || status == "pending" {
		// 检查托管区域是否存在
		if domain.HostedZoneID == "" {
			return fmt.Errorf("缺少托管区域ID，无法添加验证记录")
		}

		// 获取证书验证记录
		validationRecords, err := s.acmSvc.GetCertificateValidationRecords(domain.CertificateARN)
		if err != nil {
			return fmt.Errorf("获取证书验证记录失败: %w", err)
		}

		// 添加缺失的CNAME记录
		for _, record := range validationRecords {
			if record.Type == "CNAME" {
				var exists bool
				var err error
				if domain.DNSProvider == models.DNSProviderCloudflare {
					exists, err = s.cloudflareSvc.CheckCNAMERecord(domain.HostedZoneID, record.Name, record.Value)
					if err != nil {
						// 如果检查失败，尝试直接创建（可能是记录不存在）
						if err := s.cloudflareSvc.CreateCNAMERecord(domain.HostedZoneID, record.Name, record.Value); err != nil {
							return fmt.Errorf("创建CNAME记录失败 (%s): %w", record.Name, err)
						}
						continue
					}
					if !exists {
						if err := s.cloudflareSvc.CreateCNAMERecord(domain.HostedZoneID, record.Name, record.Value); err != nil {
							return fmt.Errorf("创建CNAME记录失败 (%s): %w", record.Name, err)
						}
					}
				} else {
					exists, err = s.route53Svc.CheckCertificateValidationCNAME(domain.HostedZoneID, record.Name, record.Value)
					if err != nil {
						// 如果检查失败，尝试直接创建（可能是记录不存在）
						if err := s.route53Svc.CreateCNAMERecord(domain.HostedZoneID, record.Name, record.Value); err != nil {
							return fmt.Errorf("创建CNAME记录失败 (%s): %w", record.Name, err)
						}
						continue
					}
					if !exists {
						// 记录不存在，创建它
						if err := s.route53Svc.CreateCNAMERecord(domain.HostedZoneID, record.Name, record.Value); err != nil {
							return fmt.Errorf("创建CNAME记录失败 (%s): %w", record.Name, err)
						}
					}
				}
			}
		}

		// 更新证书状态
		s.db.Model(domain).Update("certificate_status", "pending_validation")

		// 异步等待证书验证
		go s.waitForCertificateValidationAsync(domain)
	}

	return nil
}

// waitForCertificateValidationAsync 异步等待现有证书的验证完成
func (s *DomainService) waitForCertificateValidationAsync(domain *models.Domain) {
	if domain.CertificateARN == "" {
		return
	}

	// 等待证书验证（最多等待 1 小时）
	if err := s.acmSvc.WaitForCertificateValidation(domain.CertificateARN, 1*time.Hour); err != nil {
		s.db.Model(domain).Update("certificate_status", "failed")
		return
	}

	// 证书验证成功，只更新证书状态
	// 域名转入状态已经在创建 Hosted Zone 时设置为 completed，这里不需要再更新
	s.db.Model(domain).Update("certificate_status", "issued")
}

// generateAndImportCloudflareCertificateAsync 异步生成Cloudflare Origin证书并导入ACM
// 注意：虽然 Cloudflare 托管的域名会自动获得免费 TLS 证书（用于 Cloudflare 边缘），
// 但要在 AWS CloudFront 中使用该域名，需要创建 Origin 证书并导入到 ACM。
func (s *DomainService) generateAndImportCloudflareCertificateAsync(domain *models.Domain) {
	log := logger.GetLogger()
	log.WithFields(map[string]interface{}{
		"domain_id":   domain.ID,
		"domain_name": domain.DomainName,
	}).Info("开始生成Cloudflare Origin泛域名证书（用于导入到AWS ACM）")

	// 如果旧证书存在且状态不正常，先删除它
	if domain.CertificateARN != "" {
		log.WithFields(map[string]interface{}{
			"domain_id":       domain.ID,
			"certificate_arn": domain.CertificateARN,
		}).Info("检测到旧证书，先删除旧证书")
		if err := s.acmSvc.DeleteCertificate(domain.CertificateARN); err != nil {
			log.WithError(err).WithFields(map[string]interface{}{
				"domain_id":       domain.ID,
				"certificate_arn": domain.CertificateARN,
			}).Warn("删除旧证书失败（可能已不存在），继续生成新证书")
		} else {
			log.WithFields(map[string]interface{}{
				"domain_id":       domain.ID,
				"certificate_arn": domain.CertificateARN,
			}).Info("旧证书删除成功")
		}
	}

	// 生成Cloudflare Origin证书
	originCert, err := s.cloudflareSvc.CreateOriginCertificate(domain.DomainName)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"domain_id":   domain.ID,
			"domain_name": domain.DomainName,
		}).Error("生成Cloudflare Origin证书失败")
		s.db.Model(domain).Update("certificate_status", "failed")
		return
	}

	log.WithFields(map[string]interface{}{
		"domain_id":   domain.ID,
		"domain_name": domain.DomainName,
	}).Info("Cloudflare Origin证书生成成功，开始导入ACM")

	// 导入证书到ACM（不需要证书链，Cloudflare Origin证书是自签名的）
	certificateARN, err := s.acmSvc.ImportCertificate(originCert.Certificate, originCert.PrivateKey, "")
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"domain_id":   domain.ID,
			"domain_name": domain.DomainName,
		}).Error("导入证书到ACM失败")
		s.db.Model(domain).Update("certificate_status", "failed")
		return
	}

	log.WithFields(map[string]interface{}{
		"domain_id":       domain.ID,
		"domain_name":     domain.DomainName,
		"certificate_arn": certificateARN,
	}).Info("证书导入ACM成功")

	// 更新数据库中的证书信息
	s.db.Model(domain).Updates(map[string]interface{}{
		"certificate_arn":    certificateARN,
		"certificate_status": "issued", // 导入的证书直接是issued状态，无需验证
	})
}
