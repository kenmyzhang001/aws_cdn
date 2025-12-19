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
		// 已存在相同域名的分发，检查并更新 OriginPath
		// 格式化期望的 originPath
		expectedPath := originPath
		if expectedPath != "" {
			if !strings.HasPrefix(expectedPath, "/") {
				expectedPath = "/" + expectedPath
			}
			if expectedPath != "/" && strings.HasSuffix(expectedPath, "/") {
				expectedPath = strings.TrimSuffix(expectedPath, "/")
			}
			expectedPath = strings.ReplaceAll(expectedPath, "//", "/")
		}

		// 获取当前的 OriginPath
		currentPath, err := s.GetDistributionOriginPath(existingID)
		if err != nil {
			return "", fmt.Errorf("获取现有分发 OriginPath 失败: %w", err)
		}

		// 如果路径不匹配，更新它
		if currentPath != expectedPath {
			if err := s.UpdateDistributionOriginPath(existingID, expectedPath); err != nil {
				return "", fmt.Errorf("更新现有分发 OriginPath 失败: %w", err)
			}
		}

		return existingID, nil
	}

	callerRef := fmt.Sprintf("%s-%d", domainName, time.Now().Unix())
	originId := fmt.Sprintf("S3-%s-%s", s.config.S3BucketName, domainName)

	// 验证 S3 origin 域名格式
	if s3Origin == "" {
		return "", fmt.Errorf("S3 origin 域名不能为空")
	}

	// 验证域名格式（应该是 bucket.s3.region.amazonaws.com 或 bucket.s3.amazonaws.com）
	if !strings.Contains(s3Origin, ".s3") || !strings.HasSuffix(s3Origin, ".amazonaws.com") {
		return "", fmt.Errorf("S3 origin 域名格式不正确: %s，应该是 bucket.s3.region.amazonaws.com 或 bucket.s3.amazonaws.com 格式", s3Origin)
	}

	origin := &cloudfront.Origin{
		Id:         aws.String(originId),
		DomainName: aws.String(s3Origin),
		// 使用 CustomOriginConfig 来配置 S3 REST API 端点作为 origin
		// 当使用 S3 REST API 端点（bucket.s3.amazonaws.com）时，必须使用 CustomOriginConfig
		// 而不是 S3OriginConfig（S3OriginConfig 用于 S3 website endpoints）
		CustomOriginConfig: &cloudfront.CustomOriginConfig{
			HTTPPort:             aws.Int64(80),
			HTTPSPort:            aws.Int64(443),
			OriginProtocolPolicy: aws.String("http-only"), // S3 REST API 端点使用 HTTP
			OriginSslProtocols: &cloudfront.OriginSslProtocols{
				Quantity: aws.Int64(1),
				Items:    []*string{aws.String("TLSv1.2")},
			},
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
		// 提供更详细的错误信息，包括使用的 origin 域名
		return "", fmt.Errorf("创建 CloudFront 分发失败 (Origin: %s, OriginPath: %s): %w", s3Origin, originPath, err)
	}

	return *result.Distribution.Id, nil
}

// CreateDistributionForLargeFileDownload 创建用于大文件下载的 CloudFront 分发
// 针对大文件下载优化：支持Range请求（断点续传）、合适的缓存策略
func (s *CloudFrontService) CreateDistributionForLargeFileDownload(domainName string, certificateARN string, s3Origin string, originPath string) (string, error) {
	// 先检查是否已存在相同域名的分发
	existingID, err := s.findDistributionByDomain(domainName)
	if err != nil {
		return "", fmt.Errorf("检查现有分发失败: %w", err)
	}
	if existingID != "" {
		// 已存在相同域名的分发，检查并更新 OriginPath
		// 格式化期望的 originPath
		expectedPath := originPath
		if expectedPath != "" {
			if !strings.HasPrefix(expectedPath, "/") {
				expectedPath = "/" + expectedPath
			}
			if expectedPath != "/" && strings.HasSuffix(expectedPath, "/") {
				expectedPath = strings.TrimSuffix(expectedPath, "/")
			}
			expectedPath = strings.ReplaceAll(expectedPath, "//", "/")
		}

		// 获取当前的 OriginPath
		currentPath, err := s.GetDistributionOriginPath(existingID)
		if err != nil {
			return "", fmt.Errorf("获取现有分发 OriginPath 失败: %w", err)
		}

		// 如果路径不匹配，更新它
		if currentPath != expectedPath {
			if err := s.UpdateDistributionOriginPath(existingID, expectedPath); err != nil {
				return "", fmt.Errorf("更新现有分发 OriginPath 失败: %w", err)
			}
		}

		return existingID, nil
	}

	callerRef := fmt.Sprintf("%s-%d", domainName, time.Now().Unix())
	originId := fmt.Sprintf("S3-%s-%s", s.config.S3BucketName, domainName)

	// 验证 S3 origin 域名格式
	if s3Origin == "" {
		return "", fmt.Errorf("S3 origin 域名不能为空")
	}

	if !strings.Contains(s3Origin, ".s3") || !strings.HasSuffix(s3Origin, ".amazonaws.com") {
		return "", fmt.Errorf("S3 origin 域名格式不正确: %s", s3Origin)
	}

	origin := &cloudfront.Origin{
		Id:         aws.String(originId),
		DomainName: aws.String(s3Origin),
		CustomOriginConfig: &cloudfront.CustomOriginConfig{
			HTTPPort:             aws.Int64(80),
			HTTPSPort:            aws.Int64(443),
			OriginProtocolPolicy: aws.String("http-only"),
			OriginSslProtocols: &cloudfront.OriginSslProtocols{
				Quantity: aws.Int64(1),
				Items:    []*string{aws.String("TLSv1.2")},
			},
		},
	}

	// 如果指定了路径，设置 OriginPath
	if originPath != "" {
		path := originPath
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		if path != "/" && strings.HasSuffix(path, "/") {
			path = strings.TrimSuffix(path, "/")
		}
		path = strings.ReplaceAll(path, "//", "/")
		origin.OriginPath = aws.String(path)
	}

	// 构建转发头列表，包含Range头以支持断点续传
	forwardedHeaders := []*string{
		aws.String("Range"),         // 支持Range请求（断点续传）
		aws.String("If-Range"),      // 支持条件Range请求
		aws.String("If-Match"),      // 支持ETag验证
		aws.String("If-None-Match"), // 支持ETag验证
	}

	input := &cloudfront.CreateDistributionInput{
		DistributionConfig: &cloudfront.DistributionConfig{
			CallerReference: aws.String(callerRef),
			Comment:         aws.String(fmt.Sprintf("CloudFront distribution for large file download: %s", domainName)),
			Aliases: &cloudfront.Aliases{
				Quantity: aws.Int64(1),
				Items:    []*string{aws.String(domainName)},
			},
			DefaultRootObject: aws.String(""), // 大文件下载不需要默认根对象
			Origins: &cloudfront.Origins{
				Quantity: aws.Int64(1),
				Items:    []*cloudfront.Origin{origin},
			},
			DefaultCacheBehavior: &cloudfront.DefaultCacheBehavior{
				TargetOriginId:       aws.String(originId),
				ViewerProtocolPolicy: aws.String("redirect-to-https"),
				// 支持GET和HEAD方法（HEAD用于获取文件信息，GET用于下载）
				AllowedMethods: &cloudfront.AllowedMethods{
					Quantity: aws.Int64(2),
					Items: []*string{
						aws.String("GET"),
						aws.String("HEAD"),
					},
				},
				// 大文件通常已经压缩，不需要CloudFront压缩
				Compress: aws.Bool(false),
				// 转发必要的请求头以支持Range请求
				ForwardedValues: &cloudfront.ForwardedValues{
					QueryString: aws.Bool(false),
					Cookies: &cloudfront.CookiePreference{
						Forward: aws.String("none"),
					},
					Headers: &cloudfront.Headers{
						Quantity: aws.Int64(int64(len(forwardedHeaders))),
						Items:    forwardedHeaders,
					},
				},
				// 大文件下载的缓存策略：较短的TTL，确保文件更新能及时反映
				MinTTL:     aws.Int64(0),     // 最小缓存时间（秒）
				DefaultTTL: aws.Int64(3600),  // 默认缓存时间（1小时）
				MaxTTL:     aws.Int64(86400), // 最大缓存时间（24小时）
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
		return "", fmt.Errorf("创建 CloudFront 分发失败 (Origin: %s, OriginPath: %s): %w", s3Origin, originPath, err)
	}

	return *result.Distribution.Id, nil
}

// GetDistributionDomain 获取CloudFront分发的域名
func (s *CloudFrontService) GetDistributionDomain(distributionID string) (string, error) {
	dist, err := s.GetDistribution(distributionID)
	if err != nil {
		return "", err
	}

	if dist.DomainName != nil {
		return *dist.DomainName, nil
	}

	return "", fmt.Errorf("无法获取分发域名")
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

	// 确保 DefaultRootObject 被设置（如果未设置，设置为 "index.html"）
	// 这对于重定向规则很重要，确保访问根路径时能自动返回 index.html
	if config.DefaultRootObject == nil || *config.DefaultRootObject == "" {
		config.DefaultRootObject = aws.String("index.html")
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

// GetDistributionOriginPath 获取 CloudFront 分发的 OriginPath
func (s *CloudFrontService) GetDistributionOriginPath(distributionID string) (string, error) {
	dist, err := s.GetDistribution(distributionID)
	if err != nil {
		return "", err
	}

	if dist.DistributionConfig == nil || dist.DistributionConfig.Origins == nil {
		return "", nil
	}

	// 获取第一个 Origin 的 OriginPath
	for _, origin := range dist.DistributionConfig.Origins.Items {
		if origin != nil && origin.OriginPath != nil {
			return *origin.OriginPath, nil
		}
	}

	return "", nil
}

// UpdateDistributionOriginPath 更新 CloudFront 分发的 OriginPath
func (s *CloudFrontService) UpdateDistributionOriginPath(distributionID string, originPath string) error {
	// 获取当前配置
	getInput := &cloudfront.GetDistributionInput{
		Id: aws.String(distributionID),
	}
	getResult, err := s.client.GetDistribution(getInput)
	if err != nil {
		return fmt.Errorf("获取分发配置失败: %w", err)
	}

	config := getResult.Distribution.DistributionConfig

	// 格式化 originPath
	path := originPath
	if path != "" {
		// 确保以 / 开头
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		// 去掉末尾的 /（除非是根路径）
		if path != "/" && strings.HasSuffix(path, "/") {
			path = strings.TrimSuffix(path, "/")
		}
		// 确保没有连续的 /
		path = strings.ReplaceAll(path, "//", "/")
	}

	// 更新第一个 Origin 的 OriginPath
	if config.Origins != nil && len(config.Origins.Items) > 0 && config.Origins.Items[0] != nil {
		if path == "" {
			config.Origins.Items[0].OriginPath = nil
		} else {
			config.Origins.Items[0].OriginPath = aws.String(path)
		}
	}

	// 确保 DefaultRootObject 设置为 "index.html"（用于重定向规则）
	// 这样访问根路径时会自动返回 index.html
	if config.DefaultRootObject == nil || *config.DefaultRootObject != "index.html" {
		config.DefaultRootObject = aws.String("index.html")
	}

	updateInput := &cloudfront.UpdateDistributionInput{
		Id:                 aws.String(distributionID),
		DistributionConfig: config,
		IfMatch:            getResult.ETag,
	}

	_, err = s.client.UpdateDistribution(updateInput)
	if err != nil {
		return fmt.Errorf("更新分发 OriginPath 失败: %w", err)
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

// CreateInvalidation 创建 CloudFront 缓存失效
// distributionID: CloudFront 分发 ID
// paths: 要失效的路径列表，例如 ["/index.html"] 或 ["/*"] 来失效所有缓存
// 返回 invalidation ID
func (s *CloudFrontService) CreateInvalidation(distributionID string, paths []string) (string, error) {
	if len(paths) == 0 {
		// 如果没有指定路径，默认失效 index.html
		paths = []string{"/index.html"}
	}

	callerRef := fmt.Sprintf("invalidation-%d", time.Now().UnixNano())

	input := &cloudfront.CreateInvalidationInput{
		DistributionId: aws.String(distributionID),
		InvalidationBatch: &cloudfront.InvalidationBatch{
			CallerReference: aws.String(callerRef),
			Paths: &cloudfront.Paths{
				Quantity: aws.Int64(int64(len(paths))),
				Items:    aws.StringSlice(paths),
			},
		},
	}

	result, err := s.client.CreateInvalidation(input)
	if err != nil {
		return "", fmt.Errorf("创建 CloudFront 缓存失效失败: %w", err)
	}

	if result.Invalidation != nil && result.Invalidation.Id != nil {
		return *result.Invalidation.Id, nil
	}

	return "", fmt.Errorf("创建缓存失效成功但未返回 ID")
}
