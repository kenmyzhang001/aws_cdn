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

