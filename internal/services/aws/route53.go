package aws

import (
	"aws_cdn/internal/config"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
)

type Route53Service struct {
	client *route53.Route53
	config *config.AWSConfig
}

func NewRoute53Service(cfg *config.AWSConfig) (*Route53Service, error) {
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

	return &Route53Service{
		client: route53.New(sess),
		config: cfg,
	}, nil
}

// CreateHostedZone 创建托管区域
func (s *Route53Service) CreateHostedZone(domainName string) (string, []string, error) {
	callerRef := fmt.Sprintf("%s-%d", domainName, time.Now().Unix())
	input := &route53.CreateHostedZoneInput{
		Name:            aws.String(domainName),
		CallerReference: aws.String(callerRef),
	}

	result, err := s.client.CreateHostedZone(input)
	if err != nil {
		return "", nil, fmt.Errorf("创建托管区域失败: %w", err)
	}

	// 提取 NS 服务器
	var nsServers []string
	for _, ns := range result.DelegationSet.NameServers {
		nsServers = append(nsServers, *ns)
	}

	return *result.HostedZone.Id, nsServers, nil
}

// GetHostedZone 获取托管区域信息
func (s *Route53Service) GetHostedZone(hostedZoneID string) (*route53.HostedZone, error) {
	input := &route53.GetHostedZoneInput{
		Id: aws.String(hostedZoneID),
	}

	result, err := s.client.GetHostedZone(input)
	if err != nil {
		return nil, fmt.Errorf("获取托管区域失败: %w", err)
	}

	return result.HostedZone, nil
}

// GetNameServers 获取 NS 服务器列表
func (s *Route53Service) GetNameServers(hostedZoneID string) ([]string, error) {
	input := &route53.GetHostedZoneInput{
		Id: aws.String(hostedZoneID),
	}

	result, err := s.client.GetHostedZone(input)
	if err != nil {
		return nil, fmt.Errorf("获取托管区域失败: %w", err)
	}

	var nsServers []string
	if result.DelegationSet != nil && result.DelegationSet.NameServers != nil {
		for _, ns := range result.DelegationSet.NameServers {
			nsServers = append(nsServers, *ns)
		}
	}

	return nsServers, nil
}

// FormatNServersJSON 格式化 NS 服务器为 JSON
func FormatNServersJSON(nsServers []string) (string, error) {
	data, err := json.Marshal(nsServers)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ParseNServersJSON 解析 JSON 格式的 NS 服务器
func ParseNServersJSON(jsonStr string) ([]string, error) {
	var nsServers []string
	if err := json.Unmarshal([]byte(jsonStr), &nsServers); err != nil {
		return nil, err
	}
	return nsServers, nil
}

// CreateCNAMERecord 创建 CNAME 记录
func (s *Route53Service) CreateCNAMERecord(hostedZoneID, name, value string) error {
	// 确保 name 以点结尾（Route 53 要求）
	if name != "" && name[len(name)-1] != '.' {
		name = name + "."
	}

	change := &route53.Change{
		Action: aws.String("UPSERT"),
		ResourceRecordSet: &route53.ResourceRecordSet{
			Name:            aws.String(name),
			Type:            aws.String("CNAME"),
			TTL:             aws.Int64(300), // 5 分钟
			ResourceRecords: []*route53.ResourceRecord{{Value: aws.String(value)}},
		},
	}

	changeBatch := &route53.ChangeBatch{
		Changes: []*route53.Change{change},
	}

	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(hostedZoneID),
		ChangeBatch:  changeBatch,
	}

	_, err := s.client.ChangeResourceRecordSets(input)
	if err != nil {
		return fmt.Errorf("创建 CNAME 记录失败: %w", err)
	}

	return nil
}

// CreateAliasRecord 创建 A 记录（Alias）指向 CloudFront 分发
// hostedZoneID: Route 53 托管区域 ID
// name: 记录名称（域名）
// cloudFrontDomainName: CloudFront 分发域名（例如：d1234567890.cloudfront.net）
func (s *Route53Service) CreateAliasRecord(hostedZoneID, name, cloudFrontDomainName string) error {
	// 确保 name 以点结尾（Route 53 要求）
	if name != "" && name[len(name)-1] != '.' {
		name = name + "."
	}

	// 确保 CloudFront 域名不以点结尾（AliasTarget 的 DNSName 不应该以点结尾）
	cfDomain := cloudFrontDomainName
	if cfDomain != "" && cfDomain[len(cfDomain)-1] == '.' {
		cfDomain = cfDomain[:len(cfDomain)-1]
	}

	// CloudFront 的 Hosted Zone ID 是固定的（所有 CloudFront 分发使用同一个）
	cloudFrontHostedZoneID := "Z2FDTNDATAQYW2"

	change := &route53.Change{
		Action: aws.String("UPSERT"),
		ResourceRecordSet: &route53.ResourceRecordSet{
			Name: aws.String(name),
			Type: aws.String("A"),
			AliasTarget: &route53.AliasTarget{
				DNSName:              aws.String(cfDomain),
				HostedZoneId:         aws.String(cloudFrontHostedZoneID),
				EvaluateTargetHealth: aws.Bool(false),
			},
		},
	}

	changeBatch := &route53.ChangeBatch{
		Changes: []*route53.Change{change},
	}

	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(hostedZoneID),
		ChangeBatch:  changeBatch,
	}

	_, err := s.client.ChangeResourceRecordSets(input)
	if err != nil {
		return fmt.Errorf("创建 A 记录（Alias）失败: %w", err)
	}

	return nil
}

// CheckCloudFrontAliasRecord 检查是否存在指向指定 CloudFront 分发的 A 记录（Alias）
// hostedZoneID: Route 53 托管区域 ID
// domainName: 域名
// cloudFrontDomainName: CloudFront 分发域名（例如：d1234567890.cloudfront.net），如果为空则只检查是否指向 CloudFront
// 返回 true 表示记录存在且指向正确的 CloudFront，false 表示不存在或指向错误，error 表示查询失败
func (s *Route53Service) CheckCloudFrontAliasRecord(hostedZoneID, domainName, cloudFrontDomainName string) (bool, error) {
	// 确保 domainName 以点结尾（Route 53 要求）
	recordName := domainName
	if recordName != "" && recordName[len(recordName)-1] != '.' {
		recordName = recordName + "."
	}

	// 标准化 CloudFront 域名（去掉末尾的点）
	expectedCFDomain := cloudFrontDomainName
	if expectedCFDomain != "" && expectedCFDomain[len(expectedCFDomain)-1] == '.' {
		expectedCFDomain = expectedCFDomain[:len(expectedCFDomain)-1]
	}

	// 列出所有记录
	listInput := &route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(hostedZoneID),
	}

	result, err := s.client.ListResourceRecordSets(listInput)
	if err != nil {
		return false, fmt.Errorf("列出 Route 53 记录失败: %w", err)
	}

	// 查找匹配的 A 记录（Alias）
	for _, record := range result.ResourceRecordSets {
		if record.Name != nil && *record.Name == recordName && record.Type != nil && *record.Type == "A" {
			// 检查是否是 Alias 记录且指向 CloudFront
			if record.AliasTarget != nil {
				// CloudFront 的 Hosted Zone ID 是固定的
				if record.AliasTarget.HostedZoneId != nil && *record.AliasTarget.HostedZoneId == "Z2FDTNDATAQYW2" {
					// 如果指定了 CloudFront 域名，验证是否匹配
					if expectedCFDomain != "" {
						if record.AliasTarget.DNSName != nil {
							actualCFDomain := *record.AliasTarget.DNSName
							// 去掉末尾的点（如果有）
							if actualCFDomain != "" && actualCFDomain[len(actualCFDomain)-1] == '.' {
								actualCFDomain = actualCFDomain[:len(actualCFDomain)-1]
							}
							// 检查是否匹配
							if actualCFDomain == expectedCFDomain {
								return true, nil
							}
							// 指向了 CloudFront 但不是正确的分发
							return false, nil
						}
					} else {
						// 没有指定 CloudFront 域名，只要指向 CloudFront 就返回 true
						return true, nil
					}
				}
			}
		}
	}

	return false, nil
}

// DeleteHostedZone 删除托管区域
// 注意：删除托管区域前需要先删除所有记录（除了默认的 NS 和 SOA 记录）
func (s *Route53Service) DeleteHostedZone(hostedZoneID string) error {
	// 首先列出所有记录
	listInput := &route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(hostedZoneID),
	}

	result, err := s.client.ListResourceRecordSets(listInput)
	if err != nil {
		return fmt.Errorf("列出记录失败: %w", err)
	}

	// 删除所有非默认记录（NS 和 SOA 记录是默认的，不能删除）
	var changes []*route53.Change
	for _, record := range result.ResourceRecordSets {
		// 跳过默认的 NS 和 SOA 记录
		if *record.Type == "NS" || *record.Type == "SOA" {
			continue
		}

		change := &route53.Change{
			Action:            aws.String("DELETE"),
			ResourceRecordSet: record,
		}
		changes = append(changes, change)
	}

	// 如果有需要删除的记录，先删除它们
	if len(changes) > 0 {
		changeBatch := &route53.ChangeBatch{
			Changes: changes,
		}
		changeInput := &route53.ChangeResourceRecordSetsInput{
			HostedZoneId: aws.String(hostedZoneID),
			ChangeBatch:  changeBatch,
		}

		_, err = s.client.ChangeResourceRecordSets(changeInput)
		if err != nil {
			return fmt.Errorf("删除记录失败: %w", err)
		}
	}

	// 删除托管区域
	deleteInput := &route53.DeleteHostedZoneInput{
		Id: aws.String(hostedZoneID),
	}

	_, err = s.client.DeleteHostedZone(deleteInput)
	if err != nil {
		return fmt.Errorf("删除托管区域失败: %w", err)
	}

	return nil
}
