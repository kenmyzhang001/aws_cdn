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
		Region: aws.String(cfg.Region),
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
