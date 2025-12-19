package services

import (
	"aws_cdn/internal/models"
	"aws_cdn/internal/services/aws"
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
}

func NewDomainService(db *gorm.DB, route53Svc *aws.Route53Service, acmSvc *aws.ACMService, cloudFrontSvc *aws.CloudFrontService, s3Svc *aws.S3Service) *DomainService {
	return &DomainService{
		db:            db,
		route53Svc:    route53Svc,
		acmSvc:        acmSvc,
		cloudFrontSvc: cloudFrontSvc,
		s3Svc:         s3Svc,
	}
}

// TransferDomain 转入域名到 AWS
func (s *DomainService) TransferDomain(domainName, registrar string) (*models.Domain, error) {
	// 检查域名是否已存在
	var existingDomain models.Domain
	if err := s.db.Where("domain_name = ?", domainName).First(&existingDomain).Error; err == nil {
		return nil, fmt.Errorf("域名 %s 已存在", domainName)
	}

	// 创建托管区域
	hostedZoneID, nsServers, err := s.route53Svc.CreateHostedZone(domainName)
	if err != nil {
		return nil, fmt.Errorf("创建托管区域失败: %w", err)
	}

	// 格式化 NS 服务器为 JSON
	nsServersJSON, err := aws.FormatNServersJSON(nsServers)
	if err != nil {
		return nil, fmt.Errorf("格式化 NS 服务器失败: %w", err)
	}

	// 创建域名记录
	// 注意：域名转入成功与否应该基于 Route53 Hosted Zone 是否创建成功
	// 证书验证是独立的过程，不应该影响域名转入状态
	domain := &models.Domain{
		DomainName:   domainName,
		Registrar:    registrar,
		Status:       models.DomainStatusCompleted, // Hosted Zone 创建成功，域名已转入
		NServers:     nsServersJSON,
		HostedZoneID: hostedZoneID,
	}

	if err := s.db.Create(domain).Error; err != nil {
		return nil, fmt.Errorf("创建域名记录失败: %w", err)
	}

	// 异步请求证书（证书验证不影响域名转入状态）
	go s.requestCertificateAsync(domain)

	return domain, nil
}

// requestCertificateAsync 异步请求证书
// 注意：证书验证失败不应该影响域名转入状态，域名转入成功与否基于 Route53 Hosted Zone 是否创建成功
func (s *DomainService) requestCertificateAsync(domain *models.Domain) {
	certificateARN, err := s.acmSvc.RequestCertificate(domain.DomainName)
	if err != nil {
		// 证书请求失败，只更新证书状态，不影响域名转入状态
		s.db.Model(domain).Update("certificate_status", "failed")
		return
	}

	s.db.Model(domain).Updates(map[string]interface{}{
		"certificate_arn":    certificateARN,
		"certificate_status": "pending",
	})

	// 等待证书验证（最多等待 1 小时）
	if err := s.acmSvc.WaitForCertificateValidation(certificateARN, 1*time.Hour); err != nil {
		// 证书验证失败，只更新证书状态，不影响域名转入状态
		s.db.Model(domain).Update("certificate_status", "failed")
		return
	}

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

// ListDomains 列出所有域名
func (s *DomainService) ListDomains(page, pageSize int) ([]models.Domain, int64, error) {
	var domains []models.Domain
	var total int64

	offset := (page - 1) * pageSize

	if err := s.db.Model(&models.Domain{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := s.db.Order("id DESC").Offset(offset).Limit(pageSize).Find(&domains).Error; err != nil {
		return nil, 0, err
	}

	// 为每个有证书的域名查询最新的证书状态
	for i := range domains {
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
	}

	return domains, total, nil
}

// GetNServers 获取域名的 NS 服务器配置
func (s *DomainService) GetNServers(id uint) ([]string, error) {
	domain, err := s.GetDomain(id)
	if err != nil {
		return nil, err
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
	domain, err := s.GetDomain(id)
	if err != nil {
		return err
	}

	var certificateARN string
	if domain.CertificateARN != "" {
		// 证书已存在，检查状态
		certificateARN = domain.CertificateARN
		status, err := s.acmSvc.GetCertificateStatus(certificateARN)
		if err != nil {
			return fmt.Errorf("获取证书状态失败: %w", err)
		}

		// 如果证书状态是 PENDING_VALIDATION，需要添加验证记录
		if status == "PENDING_VALIDATION" {
			if domain.HostedZoneID == "" {
				return fmt.Errorf("证书已存在但缺少托管区域ID，无法添加验证记录")
			}

			// 获取证书验证记录
			validationRecords, err := s.acmSvc.GetCertificateValidationRecords(certificateARN)
			if err != nil {
				return fmt.Errorf("获取证书验证记录失败: %w", err)
			}

			// 将验证记录添加到 Route 53
			for _, record := range validationRecords {
				if record.Type == "CNAME" {
					if err := s.route53Svc.CreateCNAMERecord(domain.HostedZoneID, record.Name, record.Value); err != nil {
						return fmt.Errorf("添加 CNAME 验证记录失败: %w", err)
					}
				}
			}

			// 更新证书状态
			s.db.Model(domain).Update("certificate_status", "pending_validation")

			// 异步等待证书验证
			go s.requestCertificateAsync(domain)

			return nil
		}

		// 如果证书已经是 ISSUED 或其他状态，直接返回
		return fmt.Errorf("证书已存在: %s (状态: %s)", certificateARN, status)
	}

	// 证书不存在，创建新证书
	certificateARN, err = s.acmSvc.RequestCertificate(domain.DomainName)
	if err != nil {
		return fmt.Errorf("请求证书失败: %w", err)
	}

	s.db.Model(domain).Updates(map[string]interface{}{
		"certificate_arn":    certificateARN,
		"certificate_status": "pending",
	})

	// 获取证书验证记录并添加到 Route 53
	if domain.HostedZoneID != "" {
		// 等待一下让 AWS 生成验证记录
		time.Sleep(2 * time.Second)

		validationRecords, err := s.acmSvc.GetCertificateValidationRecords(certificateARN)
		if err == nil && len(validationRecords) > 0 {
			// 将验证记录添加到 Route 53
			for _, record := range validationRecords {
				if record.Type == "CNAME" {
					if err := s.route53Svc.CreateCNAMERecord(domain.HostedZoneID, record.Name, record.Value); err != nil {
						// 记录错误但不阻止流程继续
						// 可以记录日志，这里简化处理
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

	// 如果 Hosted Zone 存在，说明域名已成功转入，应该返回 completed
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
func (s *DomainService) DeleteDomain(id uint) error {
	// 获取域名信息
	domain, err := s.GetDomain(id)
	if err != nil {
		return err
	}

	// 删除 ACM 证书（如果存在）
	if domain.CertificateARN != "" {
		if err := s.acmSvc.DeleteCertificate(domain.CertificateARN); err != nil {
			// 记录错误但不阻止删除，因为证书可能已经被删除或不存在
			// 可以记录日志，这里简化处理
		}
	}

	// 删除 Route53 Hosted Zone（如果存在）
	if domain.HostedZoneID != "" {
		if err := s.route53Svc.DeleteHostedZone(domain.HostedZoneID); err != nil {
			// 记录错误但不阻止删除，因为托管区域可能已经被删除或不存在
			// 可以记录日志，这里简化处理
		}
	}

	// 从数据库删除域名记录（软删除）
	if err := s.db.Delete(domain).Error; err != nil {
		return fmt.Errorf("删除域名记录失败: %w", err)
	}

	return nil
}

// CreateCloudFrontAliasRecord 创建指向 CloudFront 的 A 记录（Alias）
func (s *DomainService) CreateCloudFrontAliasRecord(hostedZoneID, domainName, cloudFrontDomainName string) error {
	return s.route53Svc.CreateAliasRecord(hostedZoneID, domainName, cloudFrontDomainName)
}

// CheckCloudFrontAliasRecord 检查是否存在指向指定 CloudFront 分发的 A 记录（Alias）
func (s *DomainService) CheckCloudFrontAliasRecord(hostedZoneID, domainName, cloudFrontDomainName string) (bool, error) {
	return s.route53Svc.CheckCloudFrontAliasRecord(hostedZoneID, domainName, cloudFrontDomainName)
}

// CreateWWWCNAMERecord 为根域名创建 www 子域名的 CNAME 记录指向根域名
// hostedZoneID: Route 53 托管区域 ID
// rootDomain: 根域名（例如：example.com）
func (s *DomainService) CreateWWWCNAMERecord(hostedZoneID, rootDomain string) error {
	// 确保 rootDomain 不是 www 子域名
	if strings.HasPrefix(rootDomain, "www.") {
		return nil // 如果已经是 www 子域名，不需要创建
	}

	// 构建 www 子域名
	wwwDomain := "www." + rootDomain

	// 确保 rootDomain 以点结尾（CNAME 值需要）
	rootDomainValue := rootDomain
	if rootDomainValue != "" && rootDomainValue[len(rootDomainValue)-1] != '.' {
		rootDomainValue = rootDomainValue + "."
	}

	// 创建 CNAME 记录：www.example.com -> example.com
	return s.route53Svc.CreateCNAMERecord(hostedZoneID, wwwDomain, rootDomainValue)
}

// CheckWWWCNAMERecord 检查是否存在 www 子域名的 CNAME 记录指向根域名
// hostedZoneID: Route 53 托管区域 ID
// rootDomain: 根域名（例如：example.com）
func (s *DomainService) CheckWWWCNAMERecord(hostedZoneID, rootDomain string) (bool, error) {
	return s.route53Svc.CheckWWWCNAMERecord(hostedZoneID, rootDomain)
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

		// 检查每个验证记录的CNAME是否存在于Route53
		for _, record := range validationRecords {
			if record.Type == "CNAME" {
				recordDesc := fmt.Sprintf("%s -> %s", record.Name, record.Value)
				result.ValidationRecords = append(result.ValidationRecords, recordDesc)

				// 检查CNAME记录是否存在
				exists, err := s.route53Svc.CheckCertificateValidationCNAME(domain.HostedZoneID, record.Name, record.Value)
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

		// 添加缺失的CNAME记录
		for _, record := range validationRecords {
			if record.Type == "CNAME" {
				// 检查记录是否已存在
				exists, err := s.route53Svc.CheckCertificateValidationCNAME(domain.HostedZoneID, record.Name, record.Value)
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
