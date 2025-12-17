package aws

import (
	"aws_cdn/internal/config"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"
)

type CloudFrontService struct {
	client *cloudfront.CloudFront
	config *config.AWSConfig
}

func NewCloudFrontService(cfg *config.AWSConfig) (*CloudFrontService, error) {
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

	return &CloudFrontService{
		client: cloudfront.New(sess),
		config: cfg,
	}, nil
}

// CreateDistribution 创建 CloudFront 分发
func (s *CloudFrontService) CreateDistribution(domainName string, certificateARN string, s3Origin string) (string, error) {
	callerRef := fmt.Sprintf("%s-%d", domainName, time.Now().Unix())
	input := &cloudfront.CreateDistributionInput{
		DistributionConfig: &cloudfront.DistributionConfig{
			CallerReference: aws.String(callerRef),
			Aliases: &cloudfront.Aliases{
				Quantity: aws.Int64(1),
				Items:    []*string{aws.String(domainName)},
			},
			DefaultRootObject: aws.String("index.html"),
			Origins: &cloudfront.Origins{
				Quantity: aws.Int64(1),
				Items: []*cloudfront.Origin{
					{
						Id:         aws.String("S3-" + s.config.S3BucketName),
						DomainName: aws.String(s3Origin),
						S3OriginConfig: &cloudfront.S3OriginConfig{
							OriginAccessIdentity: aws.String(""),
						},
					},
				},
			},
			DefaultCacheBehavior: &cloudfront.DefaultCacheBehavior{
				TargetOriginId:       aws.String("S3-" + s.config.S3BucketName),
				ViewerProtocolPolicy: aws.String("redirect-to-https"),
				AllowedMethods: &cloudfront.AllowedMethods{
					Quantity: aws.Int64(7),
					Items: []*string{
						aws.String("GET"),
						aws.String("HEAD"),
						aws.String("OPTIONS"),
						aws.String("PUT"),
						aws.String("POST"),
						aws.String("PATCH"),
						aws.String("DELETE"),
					},
				},
				Compress: aws.Bool(true),
			},
			ViewerCertificate: &cloudfront.ViewerCertificate{
				ACMCertificateArn:      aws.String(certificateARN),
				SSLSupportMethod:       aws.String("sni-only"),
				MinimumProtocolVersion: aws.String("TLSv1.2_2021"),
			},
			Enabled: aws.Bool(true),
		},
	}

	result, err := s.client.CreateDistribution(input)
	if err != nil {
		return "", fmt.Errorf("创建 CloudFront 分发失败: %w", err)
	}

	return *result.Distribution.Id, nil
}

// GetDistribution 获取分发信息
func (s *CloudFrontService) GetDistribution(distributionID string) (*cloudfront.Distribution, error) {
	input := &cloudfront.GetDistributionInput{
		Id: aws.String(distributionID),
	}

	result, err := s.client.GetDistribution(input)
	if err != nil {
		return nil, fmt.Errorf("获取分发信息失败: %w", err)
	}

	return result.Distribution, nil
}

// UpdateDistributionAliases 更新分发的域名别名
func (s *CloudFrontService) UpdateDistributionAliases(distributionID string, aliases []string) error {
	// 获取当前配置
	getInput := &cloudfront.GetDistributionInput{
		Id: aws.String(distributionID),
	}
	getResult, err := s.client.GetDistribution(getInput)
	if err != nil {
		return fmt.Errorf("获取分发配置失败: %w", err)
	}

	config := getResult.Distribution.DistributionConfig
	config.Aliases = &cloudfront.Aliases{
		Quantity: aws.Int64(int64(len(aliases))),
		Items:    aws.StringSlice(aliases),
	}

	updateInput := &cloudfront.UpdateDistributionInput{
		Id:                 aws.String(distributionID),
		DistributionConfig: config,
		IfMatch:            getResult.ETag,
	}

	_, err = s.client.UpdateDistribution(updateInput)
	if err != nil {
		return fmt.Errorf("更新分发配置失败: %w", err)
	}

	return nil
}

