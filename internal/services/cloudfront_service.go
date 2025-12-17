package services

import (
	"fmt"

	awsSvc "aws_cdn/internal/services/aws"

	awsSDK "github.com/aws/aws-sdk-go/aws"
)

// CloudFrontService 封装 CloudFront 相关业务逻辑
type CloudFrontService struct {
	cloudFrontSvc *awsSvc.CloudFrontService
	s3Origin      string
}

// CloudFrontDistributionSummary CloudFront 分发列表信息
type CloudFrontDistributionSummary struct {
	ID         string   `json:"id"`
	DomainName string   `json:"domain_name"`
	Aliases    []string `json:"aliases"`
	Status     string   `json:"status"`
	Enabled    bool     `json:"enabled"`
	Comment    string   `json:"comment"`
}

// CloudFrontDistributionDetail CloudFront 分发详情
type CloudFrontDistributionDetail struct {
	ID             string   `json:"id"`
	DomainName     string   `json:"domain_name"`
	Aliases        []string `json:"aliases"`
	Status         string   `json:"status"`
	Enabled        bool     `json:"enabled"`
	Comment        string   `json:"comment"`
	CertificateARN string   `json:"certificate_arn"`
}

// NewCloudFrontService 创建 CloudFront 业务服务
// s3Origin 为默认 S3 源站域名，例如 "your-bucket.s3.amazonaws.com"
func NewCloudFrontService(cloudFrontSvc *awsSvc.CloudFrontService, s3Origin string) *CloudFrontService {
	return &CloudFrontService{
		cloudFrontSvc: cloudFrontSvc,
		s3Origin:      s3Origin,
	}
}

// ListDistributions 列出所有 CloudFront 分发
func (s *CloudFrontService) ListDistributions() ([]CloudFrontDistributionSummary, error) {
	list, err := s.cloudFrontSvc.ListDistributions()
	if err != nil {
		return nil, err
	}

	if list == nil || awsSDK.Int64Value(list.Quantity) == 0 || len(list.Items) == 0 {
		return []CloudFrontDistributionSummary{}, nil
	}

	var result []CloudFrontDistributionSummary
	for _, item := range list.Items {
		summary := CloudFrontDistributionSummary{
			ID:         awsSDK.StringValue(item.Id),
			DomainName: awsSDK.StringValue(item.DomainName),
			Status:     awsSDK.StringValue(item.Status),
			Enabled:    awsSDK.BoolValue(item.Enabled),
			Comment:    awsSDK.StringValue(item.Comment),
		}

		if item.Aliases != nil && awsSDK.Int64Value(item.Aliases.Quantity) > 0 {
			for _, alias := range item.Aliases.Items {
				summary.Aliases = append(summary.Aliases, awsSDK.StringValue(alias))
			}
		}

		result = append(result, summary)
	}

	return result, nil
}

// GetDistribution 获取 CloudFront 分发详情
func (s *CloudFrontService) GetDistribution(id string) (*CloudFrontDistributionDetail, error) {
	dist, err := s.cloudFrontSvc.GetDistribution(id)
	if err != nil {
		return nil, err
	}

	if dist == nil || dist.DistributionConfig == nil {
		return nil, fmt.Errorf("未找到分发配置")
	}

	cfg := dist.DistributionConfig

	detail := &CloudFrontDistributionDetail{
		ID:         awsSDK.StringValue(dist.Id),
		DomainName: awsSDK.StringValue(dist.DomainName),
		Status:     awsSDK.StringValue(dist.Status),
		Enabled:    awsSDK.BoolValue(cfg.Enabled),
		Comment:    awsSDK.StringValue(cfg.Comment),
	}

	if cfg.Aliases != nil && awsSDK.Int64Value(cfg.Aliases.Quantity) > 0 {
		for _, alias := range cfg.Aliases.Items {
			detail.Aliases = append(detail.Aliases, awsSDK.StringValue(alias))
		}
	}

	if cfg.ViewerCertificate != nil && cfg.ViewerCertificate.ACMCertificateArn != nil {
		detail.CertificateARN = awsSDK.StringValue(cfg.ViewerCertificate.ACMCertificateArn)
	}

	return detail, nil
}

// CreateDistribution 创建 CloudFront 分发并绑定域名和证书
func (s *CloudFrontService) CreateDistribution(domainName, certificateARN string) (string, error) {
	if s.s3Origin == "" {
		return "", fmt.Errorf("未配置默认 S3 源站，请在配置中设置 S3_BUCKET_NAME")
	}

	return s.cloudFrontSvc.CreateDistribution(domainName, certificateARN, s.s3Origin)
}

// UpdateDistribution 更新 CloudFront 分发（别名、证书、启用状态）
func (s *CloudFrontService) UpdateDistribution(id string, aliases []string, certificateARN string, enabled *bool) error {
	return s.cloudFrontSvc.UpdateDistribution(id, aliases, certificateARN, enabled)
}

// DeleteDistribution 删除 CloudFront 分发（需要先禁用）
func (s *CloudFrontService) DeleteDistribution(id string) error {
	return s.cloudFrontSvc.DeleteDistribution(id)
}


