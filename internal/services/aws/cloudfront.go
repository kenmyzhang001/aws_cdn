package aws

import (
	"aws_cdn/internal/config"
	"fmt"
	"strings"
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
	return s.CreateDistributionWithPath(domainName, certificateARN, s3Origin, "")
}

// CreateDistributionWithPath 创建 CloudFront 分发（支持指定 S3 路径）
// 如果已存在相同域名的分发，则返回现有分发ID，不重复创建
func (s *CloudFrontService) CreateDistributionWithPath(domainName string, certificateARN string, s3Origin string, originPath string) (string, error) {
	// 先检查是否已存在相同域名的分发
	existingID, err := s.findDistributionByDomain(domainName)
	if err != nil {
		return "", fmt.Errorf("检查现有分发失败: %w", err)
	}
	if existingID != "" {
		// 已存在相同域名的分发，返回现有分发ID
		return existingID, nil
	}

	callerRef := fmt.Sprintf("%s-%d", domainName, time.Now().Unix())
	originId := fmt.Sprintf("S3-%s-%s", s.config.S3BucketName, domainName)

	origin := &cloudfront.Origin{
		Id:         aws.String(originId),
		DomainName: aws.String(s3Origin),
		S3OriginConfig: &cloudfront.S3OriginConfig{
			OriginAccessIdentity: aws.String(""),
		},
	}

	// 如果指定了路径，设置 OriginPath
	// OriginPath 必须以 / 开头，不能以 / 结尾（除非是根路径 /）
	if originPath != "" {
		// 确保以 / 开头
		path := originPath
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		// 去掉末尾的 /（除非是根路径）
		if path != "/" && strings.HasSuffix(path, "/") {
			path = strings.TrimSuffix(path, "/")
		}
		// 确保没有连续的 /
		path = strings.ReplaceAll(path, "//", "/")
		origin.OriginPath = aws.String(path)
	}

	input := &cloudfront.CreateDistributionInput{
		DistributionConfig: &cloudfront.DistributionConfig{
			CallerReference: aws.String(callerRef),
			Comment:         aws.String(fmt.Sprintf("CloudFront distribution for %s", domainName)),
			Aliases: &cloudfront.Aliases{
				Quantity: aws.Int64(1),
				Items:    []*string{aws.String(domainName)},
			},
			DefaultRootObject: aws.String("index.html"),
			Origins: &cloudfront.Origins{
				Quantity: aws.Int64(1),
				Items:    []*cloudfront.Origin{origin},
			},
			DefaultCacheBehavior: &cloudfront.DefaultCacheBehavior{
				TargetOriginId:       aws.String(originId),
				ViewerProtocolPolicy: aws.String("redirect-to-https"),
				AllowedMethods: &cloudfront.AllowedMethods{
					Quantity: aws.Int64(2),
					Items: []*string{
						aws.String("GET"),
						aws.String("HEAD"),
					},
				},
				Compress: aws.Bool(true),
				ForwardedValues: &cloudfront.ForwardedValues{
					QueryString: aws.Bool(false), // 不转发查询字符串
					Cookies: &cloudfront.CookiePreference{
						Forward: aws.String("none"), // 不转发 cookies
					},
					Headers: &cloudfront.Headers{
						Quantity: aws.Int64(0), // 不转发请求头
					},
				},
				MinTTL:     aws.Int64(0),        // 最小缓存时间（秒）
				DefaultTTL: aws.Int64(86400),    // 默认缓存时间（24小时）
				MaxTTL:     aws.Int64(31536000), // 最大缓存时间（1年）
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

// findDistributionByDomain 根据域名查找现有的 CloudFront 分发
func (s *CloudFrontService) findDistributionByDomain(domainName string) (string, error) {
	distList, err := s.ListDistributions()
	if err != nil {
		return "", err
	}

	if distList == nil || distList.Items == nil {
		return "", nil
	}

	// 遍历所有分发，查找匹配的域名别名
	for _, distSummary := range distList.Items {
		if distSummary == nil {
			continue
		}

		// DistributionSummary 直接包含 Aliases 字段
		aliases := distSummary.Aliases
		if aliases != nil && aliases.Items != nil {
			for _, alias := range aliases.Items {
				if alias != nil && *alias == domainName {
					// 找到匹配的域名，返回分发ID
					return *distSummary.Id, nil
				}
			}
		}
	}

	return "", nil
}

// ListDistributions 列出所有 CloudFront 分发
func (s *CloudFrontService) ListDistributions() (*cloudfront.DistributionList, error) {
	input := &cloudfront.ListDistributionsInput{}

	result, err := s.client.ListDistributions(input)
	if err != nil {
		return nil, fmt.Errorf("列出 CloudFront 分发失败: %w", err)
	}

	if result.DistributionList == nil {
		return &cloudfront.DistributionList{}, nil
	}

	return result.DistributionList, nil
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

// UpdateDistribution 更新分发配置（域名别名、证书、启用状态）
func (s *CloudFrontService) UpdateDistribution(distributionID string, aliases []string, certificateARN string, enabled *bool) error {
	// 获取当前配置
	getInput := &cloudfront.GetDistributionInput{
		Id: aws.String(distributionID),
	}
	getResult, err := s.client.GetDistribution(getInput)
	if err != nil {
		return fmt.Errorf("获取分发配置失败: %w", err)
	}

	config := getResult.Distribution.DistributionConfig

	// 更新别名
	if aliases != nil {
		if len(aliases) == 0 {
			config.Aliases = &cloudfront.Aliases{
				Quantity: aws.Int64(0),
			}
		} else {
			config.Aliases = &cloudfront.Aliases{
				Quantity: aws.Int64(int64(len(aliases))),
				Items:    aws.StringSlice(aliases),
			}
		}
	}

	// 更新证书
	if certificateARN != "" {
		if config.ViewerCertificate == nil {
			config.ViewerCertificate = &cloudfront.ViewerCertificate{}
		}
		config.ViewerCertificate.ACMCertificateArn = aws.String(certificateARN)
		config.ViewerCertificate.SSLSupportMethod = aws.String("sni-only")
		config.ViewerCertificate.MinimumProtocolVersion = aws.String("TLSv1.2_2021")
	}

	// 更新启用状态
	if enabled != nil {
		config.Enabled = aws.Bool(*enabled)
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

// DeleteDistribution 删除 CloudFront 分发（需先禁用）
func (s *CloudFrontService) DeleteDistribution(distributionID string) error {
	getInput := &cloudfront.GetDistributionInput{
		Id: aws.String(distributionID),
	}
	getResult, err := s.client.GetDistribution(getInput)
	if err != nil {
		return fmt.Errorf("获取分发配置失败: %w", err)
	}

	if getResult.Distribution != nil && getResult.Distribution.DistributionConfig != nil &&
		aws.BoolValue(getResult.Distribution.DistributionConfig.Enabled) {
		return fmt.Errorf("删除前请先禁用该 CloudFront 分发")
	}

	deleteInput := &cloudfront.DeleteDistributionInput{
		Id:      aws.String(distributionID),
		IfMatch: getResult.ETag,
	}

	_, err = s.client.DeleteDistribution(deleteInput)
	if err != nil {
		return fmt.Errorf("删除 CloudFront 分发失败: %w", err)
	}

	return nil
}
