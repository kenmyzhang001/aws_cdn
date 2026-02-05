package aws

import (
	"aws_cdn/internal/config"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/acm"
)

type ACMService struct {
	client *acm.ACM
	config *config.AWSConfig
}

func NewACMService(cfg *config.AWSConfig) (*ACMService, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-east-1"),
		Credentials: credentials.NewStaticCredentials(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			"",
		),
	})
	if err != nil {
		return nil, fmt.Errorf("创建 AWS session 失败: %w", err)
	}

	return &ACMService{
		client: acm.New(sess),
		config: cfg,
	}, nil
}

// RequestCertificate 请求证书
func (s *ACMService) RequestCertificate(domainName string) (string, error) {
	input := &acm.RequestCertificateInput{
		DomainName:       aws.String(domainName),
		ValidationMethod: aws.String("DNS"),
	}

	result, err := s.client.RequestCertificate(input)
	if err != nil {
		return "", fmt.Errorf("请求证书失败: %w", err)
	}

	return *result.CertificateArn, nil
}

// GetCertificateStatus 获取证书状态
func (s *ACMService) GetCertificateStatus(certificateARN string) (string, error) {
	input := &acm.DescribeCertificateInput{
		CertificateArn: aws.String(certificateARN),
	}

	result, err := s.client.DescribeCertificate(input)
	if err != nil {
		return "", fmt.Errorf("获取证书状态失败: %w", err)
	}

	status := *result.Certificate.Status
	// 将 AWS 返回的大写状态转换为小写，以保持与数据库的一致性
	return strings.ToLower(status), nil
}

// WaitForCertificateValidation 等待证书验证完成
func (s *ACMService) WaitForCertificateValidation(certificateARN string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		status, err := s.GetCertificateStatus(certificateARN)
		if err != nil {
			return err
		}

		if status == "ISSUED" {
			return nil
		}

		if status == "FAILED" || status == "VALIDATION_TIMED_OUT" {
			return fmt.Errorf("证书验证失败，状态: %s", status)
		}

		<-ticker.C
	}

	return fmt.Errorf("等待证书验证超时")
}

// DeleteCertificate 删除证书
func (s *ACMService) DeleteCertificate(certificateARN string) error {
	input := &acm.DeleteCertificateInput{
		CertificateArn: aws.String(certificateARN),
	}

	_, err := s.client.DeleteCertificate(input)
	if err != nil {
		return fmt.Errorf("删除证书失败: %w", err)
	}

	return nil
}

// CertificateValidationRecord 证书验证记录
type CertificateValidationRecord struct {
	Name  string // CNAME 记录名称
	Type  string // 记录类型，通常是 "CNAME"
	Value string // CNAME 记录值
}

// GetCertificateValidationRecords 获取证书的验证记录
func (s *ACMService) GetCertificateValidationRecords(certificateARN string) ([]CertificateValidationRecord, error) {
	input := &acm.DescribeCertificateInput{
		CertificateArn: aws.String(certificateARN),
	}

	result, err := s.client.DescribeCertificate(input)
	if err != nil {
		return nil, fmt.Errorf("获取证书详情失败: %w", err)
	}

	var records []CertificateValidationRecord
	if result.Certificate != nil && result.Certificate.DomainValidationOptions != nil {
		for _, option := range result.Certificate.DomainValidationOptions {
			if option.ResourceRecord != nil {
				record := CertificateValidationRecord{
					Name:  aws.StringValue(option.ResourceRecord.Name),
					Type:  aws.StringValue(option.ResourceRecord.Type),
					Value: aws.StringValue(option.ResourceRecord.Value),
				}
				records = append(records, record)
			}
		}
	}

	return records, nil
}

// ImportCertificate 导入证书到ACM
// certificate: 证书内容（PEM格式）
// privateKey: 私钥内容（PEM格式）
// certificateChain: 证书链（可选，PEM格式）
func (s *ACMService) ImportCertificate(certificate, privateKey, certificateChain string) (string, error) {
	input := &acm.ImportCertificateInput{
		Certificate: []byte(certificate),
		PrivateKey:  []byte(privateKey),
	}

	// 如果提供了证书链，添加到输入中
	if certificateChain != "" {
		input.CertificateChain = []byte(certificateChain)
	}

	result, err := s.client.ImportCertificate(input)
	if err != nil {
		return "", fmt.Errorf("导入证书失败: %w", err)
	}

	return *result.CertificateArn, nil
}

// FindCertificateByDomain 根据域名查找已存在的证书
// 返回证书ARN和是否找到
// 优先匹配精确域名，其次匹配泛域名证书（如 *.example.com 可以匹配 www.example.com）
// 如果 domainName 本身是泛域名（如 *.example.com），则只匹配泛域名证书
func (s *ACMService) FindCertificateByDomain(domainName string) (string, bool, error) {
	// 检查是否是泛域名请求
	isWildcardRequest := strings.HasPrefix(domainName, "*.")

	// 提取根域名（用于泛域名匹配）
	var rootDomain string
	var isSubdomain bool
	if !isWildcardRequest {
		rootDomain = extractRootDomainFromFullDomain(domainName)
		isSubdomain = domainName != rootDomain
	}

	// 列出所有证书
	input := &acm.ListCertificatesInput{
		MaxItems: aws.Int64(1000), // 最多列出1000个证书
	}

	var exactMatchARN string
	var wildcardMatchARN string

	// 分页获取所有证书
	for {
		result, err := s.client.ListCertificates(input)
		if err != nil {
			return "", false, fmt.Errorf("列出证书失败: %w", err)
		}

		// 检查每个证书
		for _, certSummary := range result.CertificateSummaryList {
			certARN := aws.StringValue(certSummary.CertificateArn)

			// 获取证书详情以检查域名
			descInput := &acm.DescribeCertificateInput{
				CertificateArn: aws.String(certARN),
			}
			descResult, err := s.client.DescribeCertificate(descInput)
			if err != nil {
				// 如果获取详情失败，跳过这个证书
				continue
			}

			cert := descResult.Certificate
			if cert == nil {
				continue
			}

			// 检查证书状态，只考虑已签发或待验证的证书
			status := aws.StringValue(cert.Status)
			if status != "ISSUED" && status != "PENDING_VALIDATION" {
				continue
			}

			// 检查主域名
			certDomain := aws.StringValue(cert.DomainName)
			if certDomain == domainName {
				exactMatchARN = certARN
			}

			// 如果请求的是泛域名，检查证书是否也是泛域名且根域名匹配
			if isWildcardRequest {
				if strings.HasPrefix(certDomain, "*.") {
					if certDomain == domainName {
						exactMatchARN = certARN
					}
				}
			} else {
				// 如果请求的是普通域名，检查是否是泛域名证书且能匹配
				if strings.HasPrefix(certDomain, "*.") {
					wildcardRoot := strings.TrimPrefix(certDomain, "*.")
					if isSubdomain && wildcardRoot == rootDomain {
						if wildcardMatchARN == "" {
							wildcardMatchARN = certARN
						}
					}
				}
			}

			// 检查 SubjectAlternativeNames（SAN）
			if cert.SubjectAlternativeNames != nil {
				for _, san := range cert.SubjectAlternativeNames {
					sanDomain := aws.StringValue(san)
					if sanDomain == domainName {
						exactMatchARN = certARN
					}

					// 如果请求的是泛域名，检查SAN是否也是泛域名且匹配
					if isWildcardRequest {
						if strings.HasPrefix(sanDomain, "*.") && sanDomain == domainName {
							exactMatchARN = certARN
						}
					} else {
						// 如果请求的是普通域名，检查SAN是否是泛域名且能匹配
						if strings.HasPrefix(sanDomain, "*.") {
							wildcardRoot := strings.TrimPrefix(sanDomain, "*.")
							if isSubdomain && wildcardRoot == rootDomain {
								if wildcardMatchARN == "" {
									wildcardMatchARN = certARN
								}
							}
						}
					}
				}
			}
		}

		// 如果还有更多证书，继续获取
		if result.NextToken == nil {
			break
		}
		input.NextToken = result.NextToken
	}

	// 优先返回精确匹配，其次返回泛域名匹配
	if exactMatchARN != "" {
		return exactMatchARN, true, nil
	}
	if wildcardMatchARN != "" {
		return wildcardMatchARN, true, nil
	}

	return "", false, nil
}

// extractRootDomainFromFullDomain 从完整域名中提取根域名
// 例如: www.example.com -> example.com, sub.example.com -> example.com
func extractRootDomainFromFullDomain(domain string) string {
	domain = strings.TrimSuffix(domain, ".")
	parts := strings.Split(domain, ".")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], ".")
	}
	return domain
}
