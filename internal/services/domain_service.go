package services

import (
	"aws_cdn/internal/config"
	"aws_cdn/internal/logger"
	"aws_cdn/internal/models"
	"aws_cdn/internal/services/aws"
	"aws_cdn/internal/services/cloudflare"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

type DomainService struct {
	db               *gorm.DB
	route53Svc       *aws.Route53Service
	acmSvc           *aws.ACMService
	cloudFrontSvc    *aws.CloudFrontService
	s3Svc            *aws.S3Service
	cloudflareSvc    *cloudflare.CloudflareService
	cfAccountService *CFAccountService
}

func NewDomainService(db *gorm.DB, route53Svc *aws.Route53Service, acmSvc *aws.ACMService, cloudFrontSvc *aws.CloudFrontService, s3Svc *aws.S3Service, cloudflareSvc *cloudflare.CloudflareService, cfAccountService *CFAccountService) *DomainService {
	return &DomainService{
		db:               db,
		route53Svc:       route53Svc,
		acmSvc:           acmSvc,
		cloudFrontSvc:    cloudFrontSvc,
		s3Svc:            s3Svc,
		cloudflareSvc:    cloudflareSvc,
		cfAccountService: cfAccountService,
	}
}

// createCloudflareService 根据 CF 账号 ID 创建 CloudflareService
func (s *DomainService) createCloudflareService(cfAccountID *uint) (*cloudflare.CloudflareService, error) {
	// 如果没有提供 CF 账号 ID，使用全局的 cloudflareSvc（向后兼容）
	if cfAccountID == nil || *cfAccountID == 0 {
		if s.cloudflareSvc != nil {
			return s.cloudflareSvc, nil
		}
		return nil, fmt.Errorf("未提供 CF 账号 ID 且全局 CloudflareService 未配置")
	}

	// 获取 CF 账号信息
	cfAccount, err := s.cfAccountService.GetCFAccount(*cfAccountID)
	if err != nil {
		return nil, fmt.Errorf("获取 Cloudflare 账号失败: %w", err)
	}

	// 获取 API Token（优先使用 APIToken，如果没有则使用 R2APIToken）
	apiToken := s.cfAccountService.GetAPIToken(cfAccount)
	if apiToken == "" {
		apiToken = s.cfAccountService.GetR2APIToken(cfAccount)
	}

	if apiToken == "" {
		return nil, fmt.Errorf("Cloudflare账号未配置 API Token")
	}

	// 创建临时配置
	cfg := &config.CloudflareConfig{
		APIToken: apiToken,
	}

	// 创建 CloudflareService
	cloudflareSvc, err := cloudflare.NewCloudflareService(cfg)
	if err != nil {
		return nil, fmt.Errorf("创建 CloudflareService 失败: %w", err)
	}

	return cloudflareSvc, nil
}

// TransferDomain 转入域名到 AWS 或 Cloudflare
func (s *DomainService) TransferDomain(domainName, registrar string, dnsProvider models.DNSProvider, cfAccountID *uint, groupID *uint) (*models.Domain, error) {
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

		// 根据 cfAccountID 创建 CloudflareService
		cloudflareSvc, err := s.createCloudflareService(cfAccountID)
		if err != nil {
			log.WithError(err).WithFields(map[string]interface{}{
				"domain_name":   domainName,
				"cf_account_id": cfAccountID,
			}).Error("创建 CloudflareService 失败")
			return nil, fmt.Errorf("创建 CloudflareService 失败: %w", err)
		}

		zoneID, err := cloudflareSvc.GetZoneID(domainName)
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
		Registrar:    registrar, // 保留字段以保持向后兼容，但不再强制要求
		GroupID:      finalGroupID,
		CFAccountID:  cfAccountID, // 关联的 CF 账号 ID
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

	// 所有DNS提供商都使用AWS Certificate Manager请求证书
	log.WithFields(map[string]interface{}{
		"domain_id":    domain.ID,
		"domain_name":  domainName,
		"dns_provider": dnsProvider,
	}).Info("开始异步请求证书")
	// 异步请求证书（证书验证不影响域名转入状态）
	go s.requestCertificateAsync(domain)

	return domain, nil
}

// requestCertificateAsync 异步请求证书
// 注意：证书验证失败不应该影响域名转入状态，域名转入成功与否基于 Hosted Zone/Zone ID 是否创建成功
func (s *DomainService) requestCertificateAsync(domain *models.Domain) {
	log := logger.GetLogger()
	log.WithFields(map[string]interface{}{
		"domain_id":    domain.ID,
		"domain_name":  domain.DomainName,
		"dns_provider": domain.DNSProvider,
	}).Info("开始异步请求证书")
	rootDomain := extractRootDomain(domain.DomainName)

	requestCetificateDomain := ""
	if rootDomain == domain.DomainName {
		requestCetificateDomain = domain.DomainName
	} else {
		requestCetificateDomain = "*." + rootDomain
	}

	// 先检查AWS是否已有对应域名的证书
	log.WithFields(map[string]interface{}{
		"domain_id":      domain.ID,
		"domain_name":    domain.DomainName,
		"request_domain": requestCetificateDomain,
	}).Info("检查AWS是否已有对应域名的证书")

	var certificateARN string
	var isNewCertificate bool

	existingCertARN, found, err := s.acmSvc.FindCertificateByDomain(requestCetificateDomain)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"domain_id":   domain.ID,
			"domain_name": domain.DomainName,
		}).Warn("查找现有证书时出错，将继续创建新证书")
		found = false
	}

	if found {
		log.WithFields(map[string]interface{}{
			"domain_id":       domain.ID,
			"domain_name":     domain.DomainName,
			"certificate_arn": existingCertARN,
		}).Info("找到已存在的证书，直接复用")

		certificateARN = existingCertARN
		isNewCertificate = false

		// 更新数据库中的证书ARN
		s.db.Model(domain).Updates(map[string]interface{}{
			"certificate_arn":    certificateARN,
			"certificate_status": "pending",
		})

		// 检查证书状态
		status, err := s.acmSvc.GetCertificateStatus(existingCertARN)
		if err == nil {
			s.db.Model(domain).Update("certificate_status", status)

			// 如果证书已签发，直接返回
			if status == "issued" {
				log.WithFields(map[string]interface{}{
					"domain_id":       domain.ID,
					"domain_name":     domain.DomainName,
					"certificate_arn": existingCertARN,
				}).Info("证书已签发，无需等待验证")
				return
			}
		}
	} else {
		// 没有找到现有证书，创建新证书
		var err error
		certificateARN, err = s.acmSvc.RequestCertificate(requestCetificateDomain)
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
		}).Info("证书请求成功，开始添加验证记录")

		s.db.Model(domain).Updates(map[string]interface{}{
			"certificate_arn":    certificateARN,
			"certificate_status": "pending",
		})

		isNewCertificate = true
	}

	// 获取证书验证记录并添加到 DNS 提供商
	if domain.HostedZoneID != "" {
		// 重试获取验证记录（AWS可能需要一些时间生成验证记录）
		var validationRecords []aws.CertificateValidationRecord
		maxRetries := 10
		for i := 0; i < maxRetries; i++ {
			records, err := s.acmSvc.GetCertificateValidationRecords(certificateARN)
			if err == nil && len(records) > 0 {
				validationRecords = records
				break
			}
			if i < maxRetries-1 {
				log.WithFields(map[string]interface{}{
					"domain_id":    domain.ID,
					"retry":        i + 1,
					"max_retries":  maxRetries,
					"wait_seconds": 3,
				}).Info("验证记录尚未生成，等待后重试")
				time.Sleep(3 * time.Second)
			}
		}

		if len(validationRecords) > 0 {
			log.WithFields(map[string]interface{}{
				"domain_id":    domain.ID,
				"record_count": len(validationRecords),
				"dns_provider": domain.DNSProvider,
				"is_new_cert":  isNewCertificate,
			}).Info("开始添加验证记录到DNS提供商")

			// 记录添加失败的记录数量
			failedCount := 0
			successCount := 0
			skippedCount := 0

			// 将验证记录添加到 DNS 提供商
			// 如果是 Cloudflare，先创建 CloudflareService
			var cloudflareSvc *cloudflare.CloudflareService
			if domain.DNSProvider == models.DNSProviderCloudflare {
				var err error
				cloudflareSvc, err = s.createCloudflareService(domain.CFAccountID)
				if err != nil {
					log.WithError(err).WithFields(map[string]interface{}{
						"domain_id":     domain.ID,
						"cf_account_id": domain.CFAccountID,
					}).Error("创建 CloudflareService 失败")
					// 继续执行，但会在后续操作中失败
				}
			}

			for _, record := range validationRecords {
				if record.Type == "CNAME" {
					var err error
					recordName := record.Name
					var exists bool
					var checkErr error

					if domain.DNSProvider == models.DNSProviderCloudflare {
						// 对于 Cloudflare，如果记录名称是完整域名，提取相对域名部分
						// AWS ACM 返回的格式可能是：_abc123.example.com. 或 _abc123.example.com
						// Cloudflare 需要：_abc123（相对于根域名）
						recordName = extractRelativeDomainName(record.Name, domain.DomainName)

						// 先检查记录是否已存在
						if cloudflareSvc != nil {
							exists, checkErr = cloudflareSvc.CheckCNAMERecord(domain.HostedZoneID, recordName, record.Value)
						} else {
							checkErr = fmt.Errorf("CloudflareService 未创建")
						}
						if checkErr != nil {
							log.WithError(checkErr).WithFields(map[string]interface{}{
								"domain_id":    domain.ID,
								"record_name":  record.Name,
								"dns_provider": domain.DNSProvider,
							}).Warn("检查验证记录是否存在时出错，将尝试创建")
						}

						if exists {
							skippedCount++
							log.WithFields(map[string]interface{}{
								"domain_id":     domain.ID,
								"record_name":   record.Name,
								"relative_name": recordName,
								"record_value":  record.Value,
								"dns_provider":  domain.DNSProvider,
							}).Info("CNAME验证记录已存在，跳过创建")
							continue
						}

						log.WithFields(map[string]interface{}{
							"domain_id":     domain.ID,
							"original_name": record.Name,
							"relative_name": recordName,
							"record_value":  record.Value,
							"zone_id":       domain.HostedZoneID,
							"root_domain":   domain.DomainName,
						}).Info("添加验证记录到Cloudflare")

						if cloudflareSvc != nil {
							err = cloudflareSvc.CreateCNAMERecord(domain.HostedZoneID, recordName, record.Value)
						} else {
							err = fmt.Errorf("CloudflareService 未创建")
						}
					} else {
						// 先检查记录是否已存在
						exists, checkErr = s.route53Svc.CheckCertificateValidationCNAME(domain.HostedZoneID, record.Name, record.Value)
						if checkErr != nil {
							log.WithError(checkErr).WithFields(map[string]interface{}{
								"domain_id":    domain.ID,
								"record_name":  record.Name,
								"dns_provider": domain.DNSProvider,
							}).Warn("检查验证记录是否存在时出错，将尝试创建")
						}

						if exists {
							skippedCount++
							log.WithFields(map[string]interface{}{
								"domain_id":    domain.ID,
								"record_name":  record.Name,
								"record_value": record.Value,
								"dns_provider": domain.DNSProvider,
							}).Info("CNAME验证记录已存在，跳过创建")
							continue
						}

						err = s.route53Svc.CreateCNAMERecord(domain.HostedZoneID, record.Name, record.Value)
					}
					if err != nil {
						failedCount++
						log.WithError(err).WithFields(map[string]interface{}{
							"domain_id":     domain.ID,
							"record_name":   recordName,
							"original_name": record.Name,
							"record_value":  record.Value,
							"dns_provider":  domain.DNSProvider,
						}).Error("添加验证记录失败")
					} else {
						successCount++
						log.WithFields(map[string]interface{}{
							"domain_id":    domain.ID,
							"record_name":  recordName,
							"record_value": record.Value,
							"dns_provider": domain.DNSProvider,
						}).Info("验证记录添加成功")
					}
				}
			}

			if failedCount > 0 {
				log.WithFields(map[string]interface{}{
					"domain_id":    domain.ID,
					"success":      successCount,
					"failed":       failedCount,
					"skipped":      skippedCount,
					"dns_provider": domain.DNSProvider,
				}).Warn("部分验证记录添加失败，证书验证可能会失败")
			} else {
				log.WithFields(map[string]interface{}{
					"domain_id":    domain.ID,
					"success":      successCount,
					"skipped":      skippedCount,
					"dns_provider": domain.DNSProvider,
				}).Info("所有验证记录处理完成")
			}
		} else {
			log.WithFields(map[string]interface{}{
				"domain_id":       domain.ID,
				"certificate_arn": certificateARN,
			}).Warn("未能获取到验证记录，证书验证可能会失败")
		}
	} else {
		log.WithFields(map[string]interface{}{
			"domain_id":       domain.ID,
			"certificate_arn": certificateARN,
		}).Warn("缺少HostedZoneID，无法添加验证记录")
	}

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
	// 域名转入状态已经在创建 Hosted Zone/Zone ID 时设置为 completed，这里不需要再更新
	s.db.Model(domain).Update("certificate_status", "issued")
}

// GetDomain 获取域名信息
func (s *DomainService) GetDomain(id uint) (*models.Domain, error) {
	var domain models.Domain
	if err := s.db.Preload("Group").Preload("CFAccount").First(&domain, id).Error; err != nil {
		return nil, fmt.Errorf("域名不存在: %w", err)
	}
	return &domain, nil
}

func (s *DomainService) GetDomainByName(domainName string) (*models.Domain, error) {
	var domain models.Domain
	if err := s.db.Preload("Group").Preload("CFAccount").Where("domain_name = ? AND deleted_at IS NULL", domainName).First(&domain).Error; err != nil {
		return nil, fmt.Errorf("域名不存在: %w", err)
	}
	return &domain, nil
}

// DomainWithUsage 带使用状态的域名信息
type DomainWithUsage struct {
	models.Domain
	UsedByRedirect        bool   `json:"used_by_redirect"`         // 是否被重定向使用
	UsedByDownloadPackage bool   `json:"used_by_download_package"` // 是否被下载包使用
	CloudflareAccount     string `json:"cloudflare_account"`       // CF账号邮箱（用于显示）
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

// ListDomains 列出所有域名，支持按分组筛选和搜索
// usedStatus: nil 表示全部, "used" 表示已使用, "unused" 表示未使用
func (s *DomainService) ListDomains(page, pageSize int, groupID *uint, search *string, cfAccountID *uint, usedStatus *string) ([]DomainWithUsage, int64, error) {
	var domains []models.Domain
	var total int64

	offset := (page - 1) * pageSize

	// 构建基础查询，预加载关联数据
	query := s.db.Model(&models.Domain{}).Preload("Group").Preload("CFAccount").Where("domains.deleted_at IS NULL")

	// 分组筛选
	if groupID != nil {
		query = query.Where("domains.group_id = ?", *groupID)
	}

	// 搜索筛选
	if search != nil && *search != "" {
		searchPattern := "%" + *search + "%"
		query = query.Where("domains.domain_name LIKE ?", searchPattern)
	}

	// CF账号筛选
	if cfAccountID != nil {
		query = query.Where("domains.cf_account_id = ?", *cfAccountID)
	}

	// 使用状态筛选：使用子查询检查域名是否被使用
	if usedStatus != nil && (*usedStatus == "used" || *usedStatus == "unused") {
		// 检查是否被重定向使用
		redirectSubQuery := s.db.Table("redirect_rules").
			Select("1").
			Where("redirect_rules.source_domain = domains.domain_name").
			Where("redirect_rules.deleted_at IS NULL").
			Limit(1)

		// 检查是否被下载包使用
		downloadPackageSubQuery := s.db.Table("download_packages").
			Select("1").
			Where("download_packages.domain_name = domains.domain_name").
			Where("download_packages.deleted_at IS NULL").
			Limit(1)

		if *usedStatus == "used" {
			// 已使用：被重定向或下载包使用
			query = query.Where("EXISTS (?) OR EXISTS (?)", redirectSubQuery, downloadPackageSubQuery)
		} else {
			// 未使用：既不被重定向使用，也不被下载包使用
			query = query.Where("NOT EXISTS (?) AND NOT EXISTS (?)", redirectSubQuery, downloadPackageSubQuery)
		}
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("domains.id DESC").Offset(offset).Limit(pageSize).Find(&domains).Error; err != nil {
		return nil, 0, err
	}

	// 转换为带使用状态的域名列表
	result := make([]DomainWithUsage, len(domains))

	// 如果页面大小小于20，才进行状态检查（避免大量并发请求）
	if pageSize < 20 {
		var wg sync.WaitGroup
		var mu sync.Mutex

		for i := range domains {
			cfAccount := ""
			if domains[i].CFAccount != nil {
				cfAccount = domains[i].CFAccount.Email
			}
			result[i] = DomainWithUsage{
				Domain:            domains[i],
				CloudflareAccount: cfAccount,
			}

			wg.Add(1)
			go func(idx int, domain models.Domain) {
				defer wg.Done()

				// 并发查询证书状态
				var certStatus string
				if domain.CertificateARN != "" {
					status, err := s.acmSvc.GetCertificateStatus(domain.CertificateARN)
					if err == nil {
						certStatus = status
						// 更新证书状态（如果状态有变化，也更新数据库）
						if domain.CertificateStatus != status {
							// 异步更新数据库，不阻塞列表返回
							go func(domainID uint, certStatus string) {
								s.db.Model(&models.Domain{}).Where("id = ?", domainID).Update("certificate_status", certStatus)
							}(domain.ID, status)
						}
					} else {
						certStatus = domain.CertificateStatus
					}
				} else {
					certStatus = domain.CertificateStatus
				}

				// 并发检查域名使用情况
				usedByRedirect, usedByDownloadPackage, err := s.CheckDomainUsage(domain.DomainName)
				if err != nil {
					// 如果检查失败，记录错误但不阻止返回
					fmt.Printf("检查域名 %s 使用状态失败: %v\n", domain.DomainName, err)
				}

				// 线程安全地更新结果
				mu.Lock()
				result[idx].Domain.CertificateStatus = certStatus
				result[idx].UsedByRedirect = usedByRedirect
				result[idx].UsedByDownloadPackage = usedByDownloadPackage
				mu.Unlock()
			}(i, domains[i])
		}

		// 等待所有goroutine完成
		wg.Wait()
	} else {
		// 页面大小较大时，只初始化结果，不查询状态
		for i := range domains {
			cfAccount := ""
			if domains[i].CFAccount != nil {
				cfAccount = domains[i].CFAccount.Email
			}
			result[i] = DomainWithUsage{
				Domain:            domains[i],
				CloudflareAccount: cfAccount,
			}
		}
	}

	return result, total, nil
}

// ListDomainsForSelect 列出域名用于下拉选择框（轻量级，不查询证书状态）
func (s *DomainService) ListDomainsForSelect(dnsProvider string) ([]DomainWithUsage, error) {
	query := s.db.Model(&models.Domain{}).Preload("CFAccount").Where("deleted_at IS NULL")

	// 如果指定了DNS提供商，则过滤
	if dnsProvider != "" {
		query = query.Where("dns_provider = ?", dnsProvider)
	}

	var domains []models.Domain
	if err := query.Order("id DESC").Find(&domains).Error; err != nil {
		return nil, err
	}

	// 批量检查域名使用情况（优化性能）
	domainNames := make([]string, len(domains))
	for i := range domains {
		domainNames[i] = domains[i].DomainName
	}

	// 批量查询重定向使用情况
	redirectMap := make(map[string]bool)
	if len(domainNames) > 0 {
		var redirectDomains []string
		s.db.Model(&models.RedirectRule{}).
			Where("source_domain IN ? AND deleted_at IS NULL", domainNames).
			Pluck("source_domain", &redirectDomains)
		for _, domainName := range redirectDomains {
			redirectMap[domainName] = true
		}
	}

	// 批量查询下载包使用情况
	downloadPackageMap := make(map[string]bool)
	if len(domainNames) > 0 {
		var downloadPackageDomains []string
		s.db.Model(&models.DownloadPackage{}).
			Where("domain_name IN ? AND deleted_at IS NULL", domainNames).
			Pluck("domain_name", &downloadPackageDomains)
		for _, domainName := range downloadPackageDomains {
			downloadPackageMap[domainName] = true
		}
	}

	// 转换为带使用状态的域名列表（不查询证书状态）
	result := make([]DomainWithUsage, len(domains))
	for i := range domains {
		cfAccount := ""
		if domains[i].CFAccount != nil {
			cfAccount = domains[i].CFAccount.Email
		}
		result[i] = DomainWithUsage{
			Domain:                domains[i],
			UsedByRedirect:        redirectMap[domains[i].DomainName],
			UsedByDownloadPackage: downloadPackageMap[domains[i].DomainName],
			CloudflareAccount:     cfAccount,
		}
	}

	return result, nil
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
		if status == "pending_validation" {
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
				"dns_provider":    domain.DNSProvider,
				"hosted_zone_id":  domain.HostedZoneID,
			}).Info("开始添加证书验证记录到DNS提供商")

			// 统计信息
			cnameCount := 0
			skippedCount := 0
			successCount := 0

			// 如果是 Cloudflare，先创建 CloudflareService
			var cloudflareSvc *cloudflare.CloudflareService
			if domain.DNSProvider == models.DNSProviderCloudflare {
				var err error
				cloudflareSvc, err = s.createCloudflareService(domain.CFAccountID)
				if err != nil {
					log.WithError(err).WithFields(map[string]interface{}{
						"domain_id":     id,
						"cf_account_id": domain.CFAccountID,
					}).Error("创建 CloudflareService 失败")
					return fmt.Errorf("创建 CloudflareService 失败: %w", err)
				}
			}

			// 将验证记录添加到 DNS 提供商
			for i, record := range validationRecords {
				log.WithFields(map[string]interface{}{
					"domain_id":     id,
					"record_index":  i + 1,
					"total_records": len(validationRecords),
					"record_type":   record.Type,
					"record_name":   record.Name,
					"record_value":  record.Value,
					"dns_provider":  domain.DNSProvider,
				}).Info("处理验证记录")

				if record.Type == "CNAME" {
					cnameCount++

					// 先检查记录是否已存在
					var exists bool
					var checkErr error
					recordName := record.Name

					if domain.DNSProvider == models.DNSProviderCloudflare {
						// 对于 Cloudflare，提取相对域名部分
						recordName = extractRelativeDomainName(record.Name, domain.DomainName)
						if cloudflareSvc != nil {
							exists, checkErr = cloudflareSvc.CheckCNAMERecord(domain.HostedZoneID, recordName, record.Value)
						} else {
							checkErr = fmt.Errorf("CloudflareService 未创建")
						}
					} else {
						exists, checkErr = s.route53Svc.CheckCertificateValidationCNAME(domain.HostedZoneID, record.Name, record.Value)
					}

					if checkErr != nil {
						log.WithError(checkErr).WithFields(map[string]interface{}{
							"domain_id":    id,
							"record_index": i + 1,
							"record_name":  record.Name,
							"dns_provider": domain.DNSProvider,
						}).Warn("检查验证记录是否存在时出错，将尝试创建")
					}

					if exists {
						skippedCount++
						log.WithFields(map[string]interface{}{
							"domain_id":     id,
							"record_index":  i + 1,
							"record_name":   record.Name,
							"record_value":  record.Value,
							"dns_provider":  domain.DNSProvider,
							"skipped_count": skippedCount,
						}).Info("CNAME验证记录已存在，跳过创建")
						continue
					}

					log.WithFields(map[string]interface{}{
						"domain_id":      id,
						"record_index":   i + 1,
						"record_name":    record.Name,
						"record_value":   record.Value,
						"dns_provider":   domain.DNSProvider,
						"hosted_zone_id": domain.HostedZoneID,
					}).Info("开始添加CNAME验证记录")

					var err error
					if domain.DNSProvider == models.DNSProviderCloudflare {
						log.WithFields(map[string]interface{}{
							"domain_id":     id,
							"record_name":   record.Name,
							"relative_name": recordName,
							"record_value":  record.Value,
							"zone_id":       domain.HostedZoneID,
						}).Info("调用Cloudflare API创建CNAME记录")
						if cloudflareSvc != nil {
							err = cloudflareSvc.CreateCNAMERecord(domain.HostedZoneID, recordName, record.Value)
						} else {
							err = fmt.Errorf("CloudflareService 未创建")
						}
					} else {
						log.WithFields(map[string]interface{}{
							"domain_id":      id,
							"record_name":    record.Name,
							"record_value":   record.Value,
							"hosted_zone_id": domain.HostedZoneID,
						}).Info("调用Route53 API创建CNAME记录")
						err = s.route53Svc.CreateCNAMERecord(domain.HostedZoneID, record.Name, record.Value)
					}
					if err != nil {
						log.WithError(err).WithFields(map[string]interface{}{
							"domain_id":      id,
							"record_index":   i + 1,
							"record_name":    record.Name,
							"record_value":   record.Value,
							"dns_provider":   domain.DNSProvider,
							"hosted_zone_id": domain.HostedZoneID,
							"success_count":  successCount,
							"failed_count":   1,
						}).Error("添加CNAME验证记录失败")
						return fmt.Errorf("添加 CNAME 验证记录失败: %w", err)
					}

					successCount++
					log.WithFields(map[string]interface{}{
						"domain_id":     id,
						"record_index":  i + 1,
						"record_name":   record.Name,
						"record_value":  record.Value,
						"dns_provider":  domain.DNSProvider,
						"success_count": successCount,
						"total_cname":   cnameCount,
					}).Info("CNAME验证记录添加成功")
				} else {
					skippedCount++
					log.WithFields(map[string]interface{}{
						"domain_id":     id,
						"record_index":  i + 1,
						"record_type":   record.Type,
						"record_name":   record.Name,
						"skipped_count": skippedCount,
					}).Info("跳过非CNAME类型的验证记录")
				}
			}

			log.WithFields(map[string]interface{}{
				"domain_id":       id,
				"certificate_arn": certificateARN,
				"total_records":   len(validationRecords),
				"cname_count":     cnameCount,
				"success_count":   successCount,
				"skipped_count":   skippedCount,
				"dns_provider":    domain.DNSProvider,
			}).Info("所有验证记录处理完成")

			log.WithFields(map[string]interface{}{
				"domain_id":       id,
				"certificate_arn": certificateARN,
			}).Info("证书验证记录处理完成，开始异步等待验证")

			// 更新证书状态
			s.db.Model(domain).Update("certificate_status", "pending_validation")

			// 证书已存在且有验证记录，只需要等待验证完成，不需要重复请求证书
			go s.waitForCertificateValidationAsync(domain)

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
			}).Info("开始添加验证记录到DNS提供商")
			// 如果是 Cloudflare，先创建 CloudflareService
			var cloudflareSvc *cloudflare.CloudflareService
			if domain.DNSProvider == models.DNSProviderCloudflare {
				var err error
				cloudflareSvc, err = s.createCloudflareService(domain.CFAccountID)
				if err != nil {
					log.WithError(err).WithFields(map[string]interface{}{
						"domain_id":     id,
						"cf_account_id": domain.CFAccountID,
					}).Error("创建 CloudflareService 失败")
					return fmt.Errorf("创建 CloudflareService 失败: %w", err)
				}
			}

			// 将验证记录添加到 DNS 提供商
			for _, record := range validationRecords {
				if record.Type == "CNAME" {
					var err error
					if domain.DNSProvider == models.DNSProviderCloudflare {
						if cloudflareSvc != nil {
							err = cloudflareSvc.CreateCNAMERecord(domain.HostedZoneID, record.Name, record.Value)
						} else {
							err = fmt.Errorf("CloudflareService 未创建")
						}
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

// UpdateDomainNote 更新域名备注
func (s *DomainService) UpdateDomainNote(id uint, note string) error {
	return s.db.Model(&models.Domain{}).Where("id = ?", id).Update("note", note).Error
}

// MoveDomainToGroup 将域名移动到指定分组
// 如果 groupID 为 nil，则自动添加到默认分组
func (s *DomainService) MoveDomainToGroup(domainID uint, groupID *uint) error {
	log := logger.GetLogger()

	// 检查域名是否存在
	domain, err := s.GetDomain(domainID)
	if err != nil {
		log.WithError(err).WithField("domain_id", domainID).Error("移动域名到分组失败：域名不存在")
		return fmt.Errorf("域名不存在: %w", err)
	}

	var finalGroupID *uint

	// 如果没有指定分组ID，自动获取或创建默认分组
	if groupID == nil {
		groupService := NewGroupService(s.db)
		defaultGroup, err := groupService.GetOrCreateDefaultGroup()
		if err != nil {
			log.WithError(err).WithField("domain_id", domainID).Error("移动域名到分组失败：获取默认分组失败")
			return fmt.Errorf("获取默认分组失败: %w", err)
		}
		finalGroupID = &defaultGroup.ID
		log.WithFields(map[string]interface{}{
			"domain_id":        domainID,
			"domain_name":      domain.DomainName,
			"default_group_id": defaultGroup.ID,
		}).Info("未指定分组，自动使用默认分组")
	} else {
		// 如果指定了分组ID，验证分组是否存在
		groupService := NewGroupService(s.db)
		_, err := groupService.GetGroup(*groupID)
		if err != nil {
			log.WithError(err).WithFields(map[string]interface{}{
				"domain_id": domainID,
				"group_id":  *groupID,
			}).Error("移动域名到分组失败：分组不存在")
			return fmt.Errorf("分组不存在: %w", err)
		}
		finalGroupID = groupID
	}

	// 更新域名的分组ID
	if err := s.db.Model(domain).Update("group_id", finalGroupID).Error; err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"domain_id": domainID,
			"group_id":  finalGroupID,
		}).Error("移动域名到分组失败：更新数据库失败")
		return fmt.Errorf("更新域名分组失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"domain_id":   domainID,
		"domain_name": domain.DomainName,
		"group_id":    finalGroupID,
	}).Info("域名移动分组成功")
	return nil
}

// DeleteDomain 删除域名
// 删除域名时会同时删除相关的 AWS 资源（Route53 Hosted Zone 和 ACM 证书）
// 对于 Cloudflare 托管的域名，只删除 ACM 证书和数据库记录（不删除 Cloudflare Zone）
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

	// 不要删除 ACM 证书
	//if domain.CertificateARN != "" {
	//	log.WithFields(map[string]interface{}{
	//		"domain_id":       id,
	//		"certificate_arn": domain.CertificateARN,
	//	}).Info("开始删除ACM证书")
	//	if err := s.acmSvc.DeleteCertificate(domain.CertificateARN); err != nil {
	//		log.WithError(err).WithFields(map[string]interface{}{
	//			"domain_id":       id,
	//			"certificate_arn": domain.CertificateARN,
	//		}).Warn("删除ACM证书失败（可能已不存在）")
	//	} else {
	//		log.WithFields(map[string]interface{}{
	//			"domain_id":       id,
	//			"certificate_arn": domain.CertificateARN,
	//		}).Info("ACM证书删除成功")
	//	}
	//}

	// 删除 Route53 Hosted Zone（如果存在且是AWS托管的域名）
	if domain.DNSProvider == models.DNSProviderAWS && domain.HostedZoneID != "" {
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
		cloudflareSvc, err := s.createCloudflareService(domain.CFAccountID)
		if err != nil {
			return fmt.Errorf("创建 CloudflareService 失败: %w", err)
		}
		return cloudflareSvc.CreateCNAMERecord(domain.HostedZoneID, domain.DomainName, cloudFrontValue)
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
		cloudflareSvc, err := s.createCloudflareService(domain.CFAccountID)
		if err != nil {
			return false, fmt.Errorf("创建 CloudflareService 失败: %w", err)
		}
		return cloudflareSvc.CheckCNAMERecord(domain.HostedZoneID, domain.DomainName, cloudFrontValue)
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
		cloudflareSvc, err := s.createCloudflareService(domain.CFAccountID)
		if err != nil {
			return fmt.Errorf("创建 CloudflareService 失败: %w", err)
		}
		return cloudflareSvc.CreateCNAMERecord(domain.HostedZoneID, wwwDomain, rootDomainValue)
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
		cloudflareSvc, err := s.createCloudflareService(domain.CFAccountID)
		if err != nil {
			return false, fmt.Errorf("创建 CloudflareService 失败: %w", err)
		}
		return cloudflareSvc.CheckCNAMERecord(domain.HostedZoneID, wwwDomain, rootDomainValue)
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

		// 如果是 Cloudflare，先创建 CloudflareService
		var cloudflareSvc *cloudflare.CloudflareService
		if domain.DNSProvider == models.DNSProviderCloudflare {
			var err error
			cloudflareSvc, err = s.createCloudflareService(domain.CFAccountID)
			if err != nil {
				result.HasIssues = true
				result.Issues = append(result.Issues, fmt.Sprintf("创建 CloudflareService 失败: %v", err))
				return result, nil
			}
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
					if cloudflareSvc != nil {
						exists, err = cloudflareSvc.CheckCNAMERecord(domain.HostedZoneID, record.Name, record.Value)
					} else {
						err = fmt.Errorf("CloudflareService 未创建")
					}
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

		// 如果是 Cloudflare，先创建 CloudflareService
		var cloudflareSvc *cloudflare.CloudflareService
		if domain.DNSProvider == models.DNSProviderCloudflare {
			var err error
			cloudflareSvc, err = s.createCloudflareService(domain.CFAccountID)
			if err != nil {
				return fmt.Errorf("创建 CloudflareService 失败: %w", err)
			}
		}

		// 添加缺失的CNAME记录
		for _, record := range validationRecords {
			if record.Type == "CNAME" {
				var exists bool
				var err error
				if domain.DNSProvider == models.DNSProviderCloudflare {
					if cloudflareSvc != nil {
						exists, err = cloudflareSvc.CheckCNAMERecord(domain.HostedZoneID, record.Name, record.Value)
					} else {
						err = fmt.Errorf("CloudflareService 未创建")
					}
					if err != nil {
						// 如果检查失败，尝试直接创建（可能是记录不存在）
						if cloudflareSvc != nil {
							if err := cloudflareSvc.CreateCNAMERecord(domain.HostedZoneID, record.Name, record.Value); err != nil {
								return fmt.Errorf("创建CNAME记录失败 (%s): %w", record.Name, err)
							}
						} else {
							return fmt.Errorf("CloudflareService 未创建，无法创建CNAME记录 (%s)", record.Name)
						}
						continue
					}
					if !exists {
						if cloudflareSvc != nil {
							if err := cloudflareSvc.CreateCNAMERecord(domain.HostedZoneID, record.Name, record.Value); err != nil {
								return fmt.Errorf("创建CNAME记录失败 (%s): %w", record.Name, err)
							}
						} else {
							return fmt.Errorf("CloudflareService 未创建，无法创建CNAME记录 (%s)", record.Name)
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

// extractRootDomain 提取根域名
// 例如: www.example.com -> example.com, sub.example.com -> example.com, dl.95058.cc -> 95058.cc
func extractRootDomain(domain string) string {
	parts := strings.Split(domain, ".")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], ".")
	}
	return domain
}

// extractRelativeDomainName 从完整域名中提取相对域名部分
// 例如: _abc123.example.com. -> _abc123, _abc123.example.com -> _abc123
// 如果 recordName 已经是相对域名，直接返回
func extractRelativeDomainName(recordName, rootDomain string) string {
	// 去掉末尾的点
	recordName = strings.TrimSuffix(recordName, ".")
	rootDomain = strings.TrimSuffix(rootDomain, ".")

	// 如果 recordName 以 rootDomain 结尾，提取前缀部分
	if strings.HasSuffix(recordName, "."+rootDomain) {
		relativeName := strings.TrimSuffix(recordName, "."+rootDomain)
		return relativeName
	}

	// 如果 recordName 等于 rootDomain，返回 @（根域名记录）
	if recordName == rootDomain {
		return "@"
	}

	// 如果 recordName 不包含 rootDomain，可能是相对域名，直接返回
	return recordName
}

// FindCertificateARNForDomain 查找适合域名的证书ARN
// 对于子域名（如 dl.95058.cc），优先查找根域名的泛域名证书（*.95058.cc），如果找不到则使用根域名的证书
// 对于根域名（如 95058.cc），直接使用该域名的证书
func (s *DomainService) FindCertificateARNForDomain(domainName string) (string, error) {
	// 提取根域名
	rootDomain := extractRootDomain(domainName)

	// 判断是否是子域名
	isSubdomain := domainName != rootDomain

	if isSubdomain {
		// 对于子域名，优先查找泛域名证书（*.rootDomain）
		wildcardDomain := "*." + rootDomain
		var wildcardDomainRecord models.Domain
		if err := s.db.Where("domain_name = ? AND certificate_status = ? AND certificate_arn != ''", wildcardDomain, "issued").First(&wildcardDomainRecord).Error; err == nil {
			log := logger.GetLogger()
			log.WithFields(map[string]interface{}{
				"domain_name":     domainName,
				"root_domain":     rootDomain,
				"wildcard_domain": wildcardDomain,
				"certificate_arn": wildcardDomainRecord.CertificateARN,
			}).Info("找到泛域名证书，使用泛域名证书")
			return wildcardDomainRecord.CertificateARN, nil
		}

		// 如果没找到泛域名证书，查找根域名的证书（假设根域名的证书是泛域名证书）
		var rootDomainRecord models.Domain
		if err := s.db.Where("domain_name = ? AND certificate_status = ? AND certificate_arn != ''", rootDomain, "issued").First(&rootDomainRecord).Error; err == nil {
			log := logger.GetLogger()
			log.WithFields(map[string]interface{}{
				"domain_name":     domainName,
				"root_domain":     rootDomain,
				"certificate_arn": rootDomainRecord.CertificateARN,
			}).Info("使用根域名的证书（假设为泛域名证书）")
			return rootDomainRecord.CertificateARN, nil
		}

		// 如果都没找到，返回空字符串（表示未找到证书）
		return "", nil
	} else {
		// 对于根域名，直接查找该域名的证书
		var domainRecord models.Domain
		if err := s.db.Where("domain_name = ? AND certificate_status = ? AND certificate_arn != ''", domainName, "issued").First(&domainRecord).Error; err == nil {
			return domainRecord.CertificateARN, nil
		}

		// 如果没找到，返回空字符串（表示未找到证书）
		return "", nil
	}
}
