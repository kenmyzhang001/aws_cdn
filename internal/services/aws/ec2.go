package aws

import (
	"aws_cdn/internal/config"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

// NewEC2Client 创建指定区域的 EC2 客户端
func NewEC2Client(cfg *config.AWSConfig, region string) (*ec2.EC2, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region),
		Credentials: credentials.NewStaticCredentials(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			"",
		),
	})
	if err != nil {
		return nil, fmt.Errorf("创建 AWS session 失败: %w", err)
	}
	return ec2.New(sess), nil
}

// RunInstance 在指定区域启动一台 EC2 实例，返回实例 ID
func RunInstance(client *ec2.EC2, amiID, instanceType, securityGroupID, name string) (string, error) {
	input := &ec2.RunInstancesInput{
		ImageId:      aws.String(amiID),
		InstanceType: aws.String(instanceType),
		MinCount:     aws.Int64(1),
		MaxCount:     aws.Int64(1),
		SecurityGroupIds: aws.StringSlice([]string{securityGroupID}),
	}
	if name != "" {
		input.SetTagSpecifications([]*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeInstance),
				Tags: []*ec2.Tag{
					{Key: aws.String("Name"), Value: aws.String(name)},
				},
			},
		})
	}
	out, err := client.RunInstances(input)
	if err != nil {
		return "", fmt.Errorf("启动实例失败: %w", err)
	}
	if len(out.Instances) == 0 {
		return "", fmt.Errorf("未返回实例 ID")
	}
	return aws.StringValue(out.Instances[0].InstanceId), nil
}

// TerminateInstance 终止 EC2 实例
func TerminateInstance(client *ec2.EC2, instanceID string) error {
	_, err := client.TerminateInstances(&ec2.TerminateInstancesInput{
		InstanceIds: aws.StringSlice([]string{instanceID}),
	})
	return err
}

// GetInstancesPublicIPs 批量查询实例公网 IP，返回 instanceID -> publicIP 映射
func GetInstancesPublicIPs(client *ec2.EC2, instanceIDs []string) (map[string]string, error) {
	if len(instanceIDs) == 0 {
		return nil, nil
	}
	out, err := client.DescribeInstances(&ec2.DescribeInstancesInput{
		InstanceIds: aws.StringSlice(instanceIDs),
	})
	if err != nil {
		return nil, err
	}
	m := make(map[string]string)
	for _, r := range out.Reservations {
		for _, inst := range r.Instances {
			if inst.InstanceId == nil {
				continue
			}
			id := aws.StringValue(inst.InstanceId)
			if inst.PublicIpAddress != nil {
				m[id] = aws.StringValue(inst.PublicIpAddress)
			}
		}
	}
	return m, nil
}
