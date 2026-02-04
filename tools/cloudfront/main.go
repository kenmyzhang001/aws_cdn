package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/aws/aws-sdk-go/service/cloudfront"
)

// AWSConfig AWS配置
type AWSConfig struct {
	AccessKeyID     string
	SecretAccessKey string
	Region          string
}

// CloudflareConfig Cloudflare配置
type CloudflareConfig struct {
	APIToken string
	ZoneID   string
}

// CloudFrontManager CloudFront管理器
type CloudFrontManager struct {
	cfClient         *cloudfront.CloudFront
	acmClient        *acm.ACM
	cloudflareConfig *CloudflareConfig
	config           *AWSConfig
}

func main() {
	// AWS凭证配置
	accessKey := flag.String("access-key", "", "AWS Access Key ID (必需)")
	secretKey := flag.String("secret-key", "", "AWS Secret Access Key (必需)")
	region := flag.String("region", "us-east-1", "AWS Region")

	// Cloudflare配置
	cfAPIToken := flag.String("cf-api-token", "", "Cloudflare API Token (必需)")

	// 必需参数
	domainName := flag.String("domain", "", "自定义域名 (必需, 例如: cdn.example.com)")
	s3Origin := flag.String("s3-origin", "", "S3源站域名 (必需, 例如: mybucket.s3.amazonaws.com)")

	// 可选参数
	originPath := flag.String("origin-path", "", "S3源站路径前缀 (可选, 例如: /myapp; 如不提供则自动使用域名作为路径)")
	enableCompression := flag.Bool("enable-compression", true, "启用Gzip压缩")
	defaultRootObject := flag.String("default-root-object", "index.html", "默认根对象")
	cacheTTL := flag.Int64("cache-ttl", 86400, "默认缓存时间(秒), 默认24小时")

	// 证书选项
	skipCert := flag.Bool("skip-cert", false, "跳过证书配置，使用CloudFront默认证书（仅HTTP访问）")
	certTimeout := flag.Int("cert-timeout", 30, "等待证书验证的超时时间（分钟）")

	flag.Parse()

	// 验证必需参数
	if *accessKey == "" || *secretKey == "" {
		fmt.Println("错误: 必须提供AWS凭证")
		fmt.Println("\n使用方法:")
		fmt.Println("  ./cloudfront -access-key KEY -secret-key SECRET \\")
		fmt.Println("    -cf-api-token CF_TOKEN \\")
		fmt.Println("    -domain cdn.example.com -s3-origin mybucket.s3.amazonaws.com")
		flag.Usage()
		os.Exit(1)
	}

	if *cfAPIToken == "" {
		fmt.Println("错误: 必须提供Cloudflare API Token")
		fmt.Println("\n使用方法:")
		fmt.Println("  ./cloudfront -access-key KEY -secret-key SECRET \\")
		fmt.Println("    -cf-api-token CF_TOKEN \\")
		fmt.Println("    -domain cdn.example.com -s3-origin mybucket.s3.amazonaws.com")
		flag.Usage()
		os.Exit(1)
	}

	if *domainName == "" || *s3Origin == "" {
		fmt.Println("错误: 必须提供 -domain 和 -s3-origin 参数")
		fmt.Println("\n使用方法:")
		fmt.Println("  ./cloudfront -access-key KEY -secret-key SECRET \\")
		fmt.Println("    -cf-api-token CF_TOKEN \\")
		fmt.Println("    -domain cdn.example.com -s3-origin mybucket.s3.amazonaws.com")
		flag.Usage()
		os.Exit(1)
	}

	// 创建AWS配置
	awsConfig := &AWSConfig{
		AccessKeyID:     *accessKey,
		SecretAccessKey: *secretKey,
		Region:          *region,
	}

	log.Printf("========================================")
	log.Printf("CloudFront CDN 一键部署工具 (Cloudflare DNS)")
	log.Printf("========================================")
	log.Printf("")

	// 查询Cloudflare Zone ID
	log.Printf("正在查询Cloudflare Zone ID...")
	zoneID, zoneName, err := QueryCloudflareZoneID(*cfAPIToken, *domainName)
	if err != nil {
		log.Fatalf("❌ 查询Cloudflare Zone ID失败: %v", err)
	}
	log.Printf("✓ 找到Zone: %s (ID: %s)", zoneName, zoneID)
	log.Printf("")

	// 创建Cloudflare配置
	cfConfig := &CloudflareConfig{
		APIToken: *cfAPIToken,
		ZoneID:   zoneID,
	}

	// 创建管理器
	manager, err := NewCloudFrontManager(awsConfig, cfConfig)
	if err != nil {
		log.Fatalf("❌ 创建CloudFront管理器失败: %v", err)
	}

	// 如果没有指定origin-path，自动使用域名作为路径
	finalOriginPath := *originPath
	if finalOriginPath == "" {
		finalOriginPath = "/" + *domainName
		log.Printf("  自动设置源站路径为: %s", finalOriginPath)
	}

	log.Printf("配置信息:")
	log.Printf("  域名: %s", *domainName)
	log.Printf("  S3源站: %s", *s3Origin)
	log.Printf("  源站路径: %s", finalOriginPath)
	log.Printf("  Cloudflare Zone: %s", zoneName)
	log.Printf("  Cloudflare Zone ID: %s", zoneID)
	log.Printf("  启用HTTPS: %v", !*skipCert)
	log.Printf("")

	// 执行完整流程
	if err := manager.DeployCloudFrontCDN(DeployConfig{
		DomainName:        *domainName,
		S3Origin:          *s3Origin,
		OriginPath:        finalOriginPath,
		EnableCompression: *enableCompression,
		DefaultRootObject: *defaultRootObject,
		CacheTTL:          *cacheTTL,
		SkipCert:          *skipCert,
		CertTimeout:       time.Duration(*certTimeout) * time.Minute,
	}); err != nil {
		log.Fatalf("\n❌ 部署失败: %v", err)
	}

	log.Printf("\n========================================")
	log.Printf("✓ CDN部署完成!")
	log.Printf("========================================")
	log.Printf("\n您的CDN已经配置完成，请访问:")
	if *skipCert {
		log.Printf("  http://%s", *domainName)
	} else {
		log.Printf("  https://%s", *domainName)
	}
	log.Printf("\n注意: CloudFront需要15-20分钟完成全球部署")
}

// DeployConfig 部署配置
type DeployConfig struct {
	DomainName        string
	S3Origin          string
	OriginPath        string
	EnableCompression bool
	DefaultRootObject string
	CacheTTL          int64
	SkipCert          bool
	CertTimeout       time.Duration
}

// NewCloudFrontManager 创建CloudFront管理器
func NewCloudFrontManager(awsConfig *AWSConfig, cfConfig *CloudflareConfig) (*CloudFrontManager, error) {
	// 创建AWS Session
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsConfig.Region),
		Credentials: credentials.NewStaticCredentials(
			awsConfig.AccessKeyID,
			awsConfig.SecretAccessKey,
			"",
		),
	})
	if err != nil {
		return nil, fmt.Errorf("创建AWS session失败: %w", err)
	}

	return &CloudFrontManager{
		cfClient:         cloudfront.New(sess),
		acmClient:        acm.New(sess),
		cloudflareConfig: cfConfig,
		config:           awsConfig,
	}, nil
}

// DeployCloudFrontCDN 一键部署CloudFront CDN（完整流程）
func (m *CloudFrontManager) DeployCloudFrontCDN(cfg DeployConfig) error {
	var certificateARN string

	// 步骤1: 处理SSL证书
	if !cfg.SkipCert {
		log.Printf("【步骤 1/4】处理SSL证书...")

		// 查找现有证书
		log.Printf("  查找现有证书...")
		arn, found, err := m.FindCertificateByDomain(cfg.DomainName)
		if err != nil {
			return fmt.Errorf("查找证书失败: %w", err)
		}

		if found {
			// 检查证书状态
			status, err := m.GetCertificateStatus(arn)
			if err != nil {
				return fmt.Errorf("获取证书状态失败: %w", err)
			}

			if status == "issued" {
				log.Printf("  ✓ 找到已签发的证书")
				log.Printf("    ARN: %s", arn)
				certificateARN = arn
			} else if status == "pending_validation" {
				log.Printf("  ✓ 找到待验证的证书，等待验证完成...")
				log.Printf("    ARN: %s", arn)
				certificateARN = arn

				// 获取验证记录并创建DNS记录
				if err := m.CreateCertificateValidationRecords(arn); err != nil {
					return fmt.Errorf("创建证书验证记录失败: %w", err)
				}

				// 等待验证完成
				if err := m.WaitForCertificateValidation(arn, cfg.CertTimeout); err != nil {
					return fmt.Errorf("等待证书验证超时: %w", err)
				}
			} else {
				log.Printf("  ⚠️  找到的证书状态不可用 (%s)，将申请新证书", status)
				found = false
			}
		}

		if !found {
			// 申请新证书
			log.Printf("  申请新的ACM证书...")
			arn, err := m.RequestCertificate(cfg.DomainName)
			if err != nil {
				return fmt.Errorf("申请证书失败: %w", err)
			}
			log.Printf("  ✓ 证书申请成功")
			log.Printf("    ARN: %s", arn)
			certificateARN = arn

			// 等待证书验证记录可用（ACM可能需要几秒钟生成验证记录）
			log.Printf("  等待证书验证记录生成...")
			time.Sleep(5 * time.Second)

			// 创建DNS验证记录
			if err := m.CreateCertificateValidationRecords(arn); err != nil {
				return fmt.Errorf("创建证书验证记录失败: %w", err)
			}

			// 等待证书验证完成
			log.Printf("  等待证书验证完成（最多%v）...", cfg.CertTimeout)
			if err := m.WaitForCertificateValidation(arn, cfg.CertTimeout); err != nil {
				return fmt.Errorf("等待证书验证超时: %w", err)
			}
		}

		log.Printf("  ✓ SSL证书就绪")
		log.Printf("")
	} else {
		log.Printf("【步骤 1/4】跳过SSL证书配置")
		log.Printf("")
	}

	// 步骤2: 创建CloudFront Distribution
	log.Printf("【步骤 2/4】创建CloudFront Distribution...")
	distID, cfDomain, err := m.CreateDistribution(
		cfg.DomainName,
		cfg.S3Origin,
		cfg.OriginPath,
		certificateARN,
		cfg.EnableCompression,
		cfg.DefaultRootObject,
		cfg.CacheTTL,
	)
	if err != nil {
		return fmt.Errorf("创建Distribution失败: %w", err)
	}
	log.Printf("  ✓ Distribution创建成功")
	log.Printf("    ID: %s", distID)
	log.Printf("    域名: %s", cfDomain)
	log.Printf("")

	// 步骤3: 创建Cloudflare DNS记录
	log.Printf("【步骤 3/4】创建Cloudflare DNS记录...")
	if err := m.CreateCloudflareCNAME(cfg.DomainName, cfDomain); err != nil {
		return fmt.Errorf("创建DNS记录失败: %w", err)
	}
	log.Printf("  ✓ DNS CNAME记录创建成功")
	log.Printf("    %s -> %s", cfg.DomainName, cfDomain)
	log.Printf("")

	// 步骤4: 完成
	log.Printf("【步骤 4/4】配置完成")
	log.Printf("  Distribution ID: %s", distID)
	log.Printf("  CloudFront域名: %s", cfDomain)
	if !cfg.SkipCert {
		log.Printf("  SSL证书: %s", certificateARN)
	}
	log.Printf("  DNS记录: %s (CNAME -> CloudFront)", cfg.DomainName)

	return nil
}

// DistributionInfo Distribution信息
type DistributionInfo struct {
	ID             string
	DomainName     string
	Aliases        []string
	Status         string
	Enabled        bool
	Comment        string
	CertificateARN string
	Origins        []OriginInfo
}

// OriginInfo 源站信息
type OriginInfo struct {
	ID         string
	DomainName string
	OriginPath string
}

// CreateDistribution 创建CloudFront Distribution
func (m *CloudFrontManager) CreateDistribution(
	domainName string,
	s3Origin string,
	originPath string,
	certificateARN string,
	enableCompression bool,
	defaultRootObject string,
	cacheTTL int64,
) (string, string, error) {
	// 验证S3 origin域名格式
	if !strings.Contains(s3Origin, ".s3") || !strings.HasSuffix(s3Origin, ".amazonaws.com") {
		return "", "", fmt.Errorf("S3 origin域名格式不正确: %s，应该是 bucket.s3.region.amazonaws.com 格式", s3Origin)
	}

	callerRef := fmt.Sprintf("%s-%d", domainName, time.Now().Unix())
	originID := fmt.Sprintf("S3-%s", domainName)

	// 构建Origin配置
	origin := &cloudfront.Origin{
		Id:         aws.String(originID),
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

	// 设置OriginPath
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

	// 构建ViewerCertificate
	var viewerCert *cloudfront.ViewerCertificate
	if certificateARN == "" {
		viewerCert = &cloudfront.ViewerCertificate{
			CloudFrontDefaultCertificate: aws.Bool(true),
		}
	} else {
		viewerCert = &cloudfront.ViewerCertificate{
			ACMCertificateArn:      aws.String(certificateARN),
			SSLSupportMethod:       aws.String("sni-only"),
			MinimumProtocolVersion: aws.String("TLSv1.2_2021"),
		}
	}

	// 创建Distribution配置
	input := &cloudfront.CreateDistributionInput{
		DistributionConfig: &cloudfront.DistributionConfig{
			CallerReference: aws.String(callerRef),
			Comment:         aws.String(fmt.Sprintf("CloudFront distribution for %s", domainName)),
			Aliases: &cloudfront.Aliases{
				Quantity: aws.Int64(1),
				Items:    []*string{aws.String(domainName)},
			},
			DefaultRootObject: aws.String(defaultRootObject),
			Origins: &cloudfront.Origins{
				Quantity: aws.Int64(1),
				Items:    []*cloudfront.Origin{origin},
			},
			DefaultCacheBehavior: &cloudfront.DefaultCacheBehavior{
				TargetOriginId:       aws.String(originID),
				ViewerProtocolPolicy: aws.String("redirect-to-https"),
				AllowedMethods: &cloudfront.AllowedMethods{
					Quantity: aws.Int64(2),
					Items: []*string{
						aws.String("GET"),
						aws.String("HEAD"),
					},
				},
				Compress: aws.Bool(enableCompression),
				ForwardedValues: &cloudfront.ForwardedValues{
					QueryString: aws.Bool(false),
					Cookies: &cloudfront.CookiePreference{
						Forward: aws.String("none"),
					},
					Headers: &cloudfront.Headers{
						Quantity: aws.Int64(0),
					},
				},
				MinTTL:     aws.Int64(0),
				DefaultTTL: aws.Int64(cacheTTL),
				MaxTTL:     aws.Int64(31536000), // 1年
			},
			ViewerCertificate: viewerCert,
			Enabled:           aws.Bool(true),
		},
	}

	// 创建Distribution
	result, err := m.cfClient.CreateDistribution(input)
	if err != nil {
		return "", "", fmt.Errorf("创建CloudFront Distribution失败: %w", err)
	}

	return *result.Distribution.Id, *result.Distribution.DomainName, nil
}

// ListDistributions 列出所有Distributions
func (m *CloudFrontManager) ListDistributions() ([]DistributionInfo, error) {
	input := &cloudfront.ListDistributionsInput{
		MaxItems: aws.Int64(200),
	}

	result, err := m.cfClient.ListDistributions(input)
	if err != nil {
		return nil, fmt.Errorf("列出Distributions失败: %w", err)
	}

	if result.DistributionList == nil || result.DistributionList.Items == nil {
		return []DistributionInfo{}, nil
	}

	var distributions []DistributionInfo
	for _, item := range result.DistributionList.Items {
		dist := DistributionInfo{
			ID:         aws.StringValue(item.Id),
			DomainName: aws.StringValue(item.DomainName),
			Status:     aws.StringValue(item.Status),
			Enabled:    aws.BoolValue(item.Enabled),
			Comment:    aws.StringValue(item.Comment),
		}

		// 提取别名
		if item.Aliases != nil && item.Aliases.Items != nil {
			for _, alias := range item.Aliases.Items {
				dist.Aliases = append(dist.Aliases, aws.StringValue(alias))
			}
		}

		distributions = append(distributions, dist)
	}

	return distributions, nil
}

// GetDistribution 获取Distribution详情
func (m *CloudFrontManager) GetDistribution(distributionID string) (*DistributionInfo, error) {
	input := &cloudfront.GetDistributionInput{
		Id: aws.String(distributionID),
	}

	result, err := m.cfClient.GetDistribution(input)
	if err != nil {
		return nil, fmt.Errorf("获取Distribution失败: %w", err)
	}

	dist := result.Distribution
	cfg := dist.DistributionConfig

	info := &DistributionInfo{
		ID:         aws.StringValue(dist.Id),
		DomainName: aws.StringValue(dist.DomainName),
		Status:     aws.StringValue(dist.Status),
		Enabled:    aws.BoolValue(cfg.Enabled),
		Comment:    aws.StringValue(cfg.Comment),
	}

	// 提取别名
	if cfg.Aliases != nil && cfg.Aliases.Items != nil {
		for _, alias := range cfg.Aliases.Items {
			info.Aliases = append(info.Aliases, aws.StringValue(alias))
		}
	}

	// 提取证书ARN
	if cfg.ViewerCertificate != nil && cfg.ViewerCertificate.ACMCertificateArn != nil {
		info.CertificateARN = aws.StringValue(cfg.ViewerCertificate.ACMCertificateArn)
	}

	// 提取源站信息
	if cfg.Origins != nil && cfg.Origins.Items != nil {
		for _, origin := range cfg.Origins.Items {
			originInfo := OriginInfo{
				ID:         aws.StringValue(origin.Id),
				DomainName: aws.StringValue(origin.DomainName),
			}
			if origin.OriginPath != nil {
				originInfo.OriginPath = aws.StringValue(origin.OriginPath)
			}
			info.Origins = append(info.Origins, originInfo)
		}
	}

	return info, nil
}

// DeleteDistribution 删除Distribution
func (m *CloudFrontManager) DeleteDistribution(distributionID string) error {
	// 获取当前配置
	getInput := &cloudfront.GetDistributionInput{
		Id: aws.String(distributionID),
	}
	getResult, err := m.cfClient.GetDistribution(getInput)
	if err != nil {
		return fmt.Errorf("获取Distribution配置失败: %w", err)
	}

	// 检查是否已禁用
	if aws.BoolValue(getResult.Distribution.DistributionConfig.Enabled) {
		return fmt.Errorf("Distribution必须先被禁用才能删除。请先禁用该Distribution")
	}

	// 删除Distribution
	deleteInput := &cloudfront.DeleteDistributionInput{
		Id:      aws.String(distributionID),
		IfMatch: getResult.ETag,
	}

	_, err = m.cfClient.DeleteDistribution(deleteInput)
	if err != nil {
		return fmt.Errorf("删除Distribution失败: %w", err)
	}

	return nil
}

// ========== Cloudflare DNS管理函数 ==========

// CloudflareDNSRecord Cloudflare DNS记录请求
type CloudflareDNSRecord struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

// CloudflareAPIResponse Cloudflare API响应
type CloudflareAPIResponse struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result interface{} `json:"result"`
}

// QueryCloudflareZoneID 根据域名查询Cloudflare Zone ID
func QueryCloudflareZoneID(apiToken, domain string) (string, string, error) {
	// 提取根域名
	rootDomain := extractRootDomain(domain)

	// 调用Cloudflare API查询zones
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones?name=%s", rootDomain)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", "", fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("API请求失败: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("读取响应失败: %w", err)
	}

	var apiResp CloudflareAPIResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return "", "", fmt.Errorf("解析响应失败: %w", err)
	}

	if !apiResp.Success {
		if len(apiResp.Errors) > 0 {
			return "", "", fmt.Errorf("Cloudflare API错误: %s", apiResp.Errors[0].Message)
		}
		return "", "", fmt.Errorf("Cloudflare API请求失败")
	}

	// 解析结果
	if resultArray, ok := apiResp.Result.([]interface{}); ok && len(resultArray) > 0 {
		if zone, ok := resultArray[0].(map[string]interface{}); ok {
			zoneID, _ := zone["id"].(string)
			zoneName, _ := zone["name"].(string)
			if zoneID != "" && zoneName != "" {
				return zoneID, zoneName, nil
			}
		}
	}

	return "", "", fmt.Errorf("未找到域名 %s 对应的Cloudflare Zone，请确认域名已托管在Cloudflare", rootDomain)
}

// extractRootDomain 从完整域名中提取根域名
func extractRootDomain(domain string) string {
	// 移除末尾的点
	domain = strings.TrimSuffix(domain, ".")

	// 分割域名
	parts := strings.Split(domain, ".")
	if len(parts) <= 2 {
		// 已经是根域名，如 example.com
		return domain
	}

	// 提取最后两个部分作为根域名
	// 例如: cdn.example.com -> example.com
	// 注意：这个简单实现不处理.co.uk等特殊后缀
	return strings.Join(parts[len(parts)-2:], ".")
}

// cloudflareAPIRequest 发送Cloudflare API请求
func (m *CloudFrontManager) cloudflareAPIRequest(method, path string, body interface{}) (*CloudflareAPIResponse, error) {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4%s", path)

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("JSON序列化失败: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+m.cloudflareConfig.APIToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API请求失败: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var apiResp CloudflareAPIResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if !apiResp.Success {
		if len(apiResp.Errors) > 0 {
			return nil, fmt.Errorf("Cloudflare API错误: %s", apiResp.Errors[0].Message)
		}
		return nil, fmt.Errorf("Cloudflare API请求失败")
	}

	return &apiResp, nil
}

// CreateCloudflareCNAME 创建Cloudflare CNAME记录
func (m *CloudFrontManager) CreateCloudflareCNAME(domainName, cloudFrontDomain string) error {
	// 提取子域名
	subdomain := m.extractSubdomain(domainName)

	record := CloudflareDNSRecord{
		Type:    "CNAME",
		Name:    subdomain,
		Content: cloudFrontDomain,
		TTL:     1,     // Auto TTL
		Proxied: false, // CloudFront不能被Cloudflare代理
	}

	path := fmt.Sprintf("/zones/%s/dns_records", m.cloudflareConfig.ZoneID)
	_, err := m.cloudflareAPIRequest("POST", path, record)
	if err != nil {
		// 如果记录已存在，尝试更新
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "Record already exists") {
			return m.UpdateCloudflareCNAME(domainName, cloudFrontDomain)
		}
		return fmt.Errorf("创建Cloudflare CNAME记录失败: %w", err)
	}

	return nil
}

// UpdateCloudflareCNAME 更新Cloudflare CNAME记录
func (m *CloudFrontManager) UpdateCloudflareCNAME(domainName, cloudFrontDomain string) error {
	// 先查找记录ID
	subdomain := m.extractSubdomain(domainName)
	recordID, err := m.findCloudflareDNSRecord(subdomain, "CNAME")
	if err != nil {
		return err
	}

	if recordID == "" {
		// 记录不存在，创建新记录
		return m.CreateCloudflareCNAME(domainName, cloudFrontDomain)
	}

	// 更新记录
	record := CloudflareDNSRecord{
		Type:    "CNAME",
		Name:    subdomain,
		Content: cloudFrontDomain,
		TTL:     1,
		Proxied: false,
	}

	path := fmt.Sprintf("/zones/%s/dns_records/%s", m.cloudflareConfig.ZoneID, recordID)
	_, err = m.cloudflareAPIRequest("PUT", path, record)
	if err != nil {
		return fmt.Errorf("更新Cloudflare CNAME记录失败: %w", err)
	}

	return nil
}

// findCloudflareDNSRecord 查找DNS记录ID
func (m *CloudFrontManager) findCloudflareDNSRecord(name, recordType string) (string, error) {
	path := fmt.Sprintf("/zones/%s/dns_records?name=%s&type=%s",
		m.cloudflareConfig.ZoneID, name, recordType)

	resp, err := m.cloudflareAPIRequest("GET", path, nil)
	if err != nil {
		return "", err
	}

	// 解析结果
	if resultArray, ok := resp.Result.([]interface{}); ok && len(resultArray) > 0 {
		if record, ok := resultArray[0].(map[string]interface{}); ok {
			if id, ok := record["id"].(string); ok {
				return id, nil
			}
		}
	}

	return "", nil
}

// extractSubdomain 从完整域名中提取子域名
func (m *CloudFrontManager) extractSubdomain(fullDomain string) string {
	// 移除末尾的点
	fullDomain = strings.TrimSuffix(fullDomain, ".")
	return fullDomain
}

// ========== ACM证书管理函数 ==========

// FindCertificateByDomain 查找域名对应的证书
func (m *CloudFrontManager) FindCertificateByDomain(domainName string) (string, bool, error) {
	input := &acm.ListCertificatesInput{
		MaxItems: aws.Int64(1000),
	}

	for {
		result, err := m.acmClient.ListCertificates(input)
		if err != nil {
			return "", false, fmt.Errorf("列出证书失败: %w", err)
		}

		for _, certSummary := range result.CertificateSummaryList {
			certARN := aws.StringValue(certSummary.CertificateArn)

			// 获取证书详情
			descInput := &acm.DescribeCertificateInput{
				CertificateArn: aws.String(certARN),
			}
			descResult, err := m.acmClient.DescribeCertificate(descInput)
			if err != nil {
				continue
			}

			cert := descResult.Certificate
			if cert == nil {
				continue
			}

			// 检查主域名
			if aws.StringValue(cert.DomainName) == domainName {
				return certARN, true, nil
			}

			// 检查SAN
			if cert.SubjectAlternativeNames != nil {
				for _, san := range cert.SubjectAlternativeNames {
					if aws.StringValue(san) == domainName {
						return certARN, true, nil
					}
				}
			}
		}

		if result.NextToken == nil {
			break
		}
		input.NextToken = result.NextToken
	}

	return "", false, nil
}

// GetCertificateStatus 获取证书状态
func (m *CloudFrontManager) GetCertificateStatus(certificateARN string) (string, error) {
	input := &acm.DescribeCertificateInput{
		CertificateArn: aws.String(certificateARN),
	}

	result, err := m.acmClient.DescribeCertificate(input)
	if err != nil {
		return "", fmt.Errorf("获取证书状态失败: %w", err)
	}

	status := aws.StringValue(result.Certificate.Status)
	return strings.ToLower(status), nil
}

// RequestCertificate 申请新证书
func (m *CloudFrontManager) RequestCertificate(domainName string) (string, error) {
	input := &acm.RequestCertificateInput{
		DomainName:       aws.String(domainName),
		ValidationMethod: aws.String("DNS"),
	}

	result, err := m.acmClient.RequestCertificate(input)
	if err != nil {
		return "", fmt.Errorf("申请证书失败: %w", err)
	}

	return aws.StringValue(result.CertificateArn), nil
}

// CreateCertificateValidationRecords 创建证书验证DNS记录（使用Cloudflare）
func (m *CloudFrontManager) CreateCertificateValidationRecords(certificateARN string) error {
	// 获取证书验证记录
	input := &acm.DescribeCertificateInput{
		CertificateArn: aws.String(certificateARN),
	}

	result, err := m.acmClient.DescribeCertificate(input)
	if err != nil {
		return fmt.Errorf("获取证书详情失败: %w", err)
	}

	if result.Certificate == nil || result.Certificate.DomainValidationOptions == nil {
		return fmt.Errorf("证书验证记录不可用")
	}

	// 为每个域创建验证记录
	for _, option := range result.Certificate.DomainValidationOptions {
		if option.ResourceRecord == nil {
			continue
		}

		recordName := aws.StringValue(option.ResourceRecord.Name)
		recordValue := aws.StringValue(option.ResourceRecord.Value)

		// 移除末尾的点
		recordName = strings.TrimSuffix(recordName, ".")
		recordValue = strings.TrimSuffix(recordValue, ".")

		log.Printf("  创建DNS验证记录: %s", recordName)

		// 创建Cloudflare CNAME记录
		record := CloudflareDNSRecord{
			Type:    "CNAME",
			Name:    recordName,
			Content: recordValue,
			TTL:     1, // Auto TTL
			Proxied: false,
		}

		path := fmt.Sprintf("/zones/%s/dns_records", m.cloudflareConfig.ZoneID)
		_, err := m.cloudflareAPIRequest("POST", path, record)
		if err != nil {
			// 如果记录已存在，忽略错误
			if !strings.Contains(err.Error(), "already exists") && !strings.Contains(err.Error(), "Record already exists") {
				return fmt.Errorf("创建验证记录失败: %w", err)
			}
			log.Printf("    验证记录已存在，跳过")
		}
	}

	log.Printf("  ✓ DNS验证记录创建完成")
	return nil
}

// WaitForCertificateValidation 等待证书验证完成
func (m *CloudFrontManager) WaitForCertificateValidation(certificateARN string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	lastStatus := ""
	for time.Now().Before(deadline) {
		status, err := m.GetCertificateStatus(certificateARN)
		if err != nil {
			return err
		}

		if status != lastStatus {
			log.Printf("  证书状态: %s", status)
			lastStatus = status
		}

		if status == "issued" {
			log.Printf("  ✓ 证书验证完成并已签发")
			return nil
		}

		if status == "failed" || status == "validation_timed_out" {
			return fmt.Errorf("证书验证失败，状态: %s", status)
		}

		<-ticker.C
	}

	return fmt.Errorf("等待证书验证超时（超过%v）", timeout)
}
