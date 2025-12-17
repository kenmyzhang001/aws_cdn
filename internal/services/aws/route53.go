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

