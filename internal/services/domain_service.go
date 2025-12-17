package services

import (
	"aws_cdn/internal/models"
	"aws_cdn/internal/services/aws"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type DomainService struct {
	db            *gorm.DB
	route53Svc    *aws.Route53Service
	acmSvc        *aws.ACMService
	cloudFrontSvc  *aws.CloudFrontService
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
	domain := &models.Domain{
		DomainName:   domainName,
		Registrar:    registrar,
		Status:       models.DomainStatusInProgress,
		NServers:     nsServersJSON,
		HostedZoneID: hostedZoneID,
	}

	if err := s.db.Create(domain).Error; err != nil {
		return nil, fmt.Errorf("创建域名记录失败: %w", err)
	}

	// 异步请求证书
	go s.requestCertificateAsync(domain)

	return domain, nil
}

// requestCertificateAsync 异步请求证书
func (s *DomainService) requestCertificateAsync(domain *models.Domain) {
	certificateARN, err := s.acmSvc.RequestCertificate(domain.DomainName)
	if err != nil {
		s.db.Model(domain).Updates(map[string]interface{}{
			"certificate_status": "failed",
			"status":             models.DomainStatusFailed,
		})
		return
	}

	s.db.Model(domain).Updates(map[string]interface{}{
		"certificate_arn":     certificateARN,
		"certificate_status":  "pending",
	})

	// 等待证书验证（最多等待 1 小时）
	if err := s.acmSvc.WaitForCertificateValidation(certificateARN, 1*time.Hour); err != nil {
		s.db.Model(domain).Update("certificate_status", "failed")
		return
	}

	// 证书验证成功，更新状态
	s.db.Model(domain).Updates(map[string]interface{}{
		"certificate_status": "issued",
		"status":             models.DomainStatusCompleted,
	})
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

	if err := s.db.Offset(offset).Limit(pageSize).Find(&domains).Error; err != nil {
		return nil, 0, err
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

	if domain.CertificateARN != "" {
		return fmt.Errorf("证书已存在: %s", domain.CertificateARN)
	}

	certificateARN, err := s.acmSvc.RequestCertificate(domain.DomainName)
	if err != nil {
		return fmt.Errorf("请求证书失败: %w", err)
	}

	s.db.Model(domain).Updates(map[string]interface{}{
		"certificate_arn":    certificateARN,
		"certificate_status": "pending",
	})

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
func (s *DomainService) GetDomainStatus(id uint) (models.DomainStatus, error) {
	domain, err := s.GetDomain(id)
	if err != nil {
		return "", err
	}
	return domain.Status, nil
}

// UpdateDomainStatus 更新域名状态
func (s *DomainService) UpdateDomainStatus(id uint, status models.DomainStatus) error {
	return s.db.Model(&models.Domain{}).Where("id = ?", id).Update("status", status).Error
}

