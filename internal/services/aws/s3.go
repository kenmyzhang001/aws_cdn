package aws

import (
	"aws_cdn/internal/config"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

type S3Service struct {
	client *s3.S3
	config *config.AWSConfig
}

func NewS3Service(cfg *config.AWSConfig) (*S3Service, error) {
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

	return &S3Service{
		client: s3.New(sess),
		config: cfg,
	}, nil
}

// BucketExists 检查存储桶是否存在
func (s *S3Service) BucketExists(bucketName string) (bool, error) {
	input := &s3.HeadBucketInput{
		Bucket: aws.String(bucketName),
	}

	_, err := s.client.HeadBucket(input)
	if err != nil {
		// 检查是否是404错误（存储桶不存在）
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "404") {
			return false, nil
		}
		return false, fmt.Errorf("检查存储桶是否存在失败: %w", err)
	}

	return true, nil
}

// CreateBucket 创建 S3 存储桶
func (s *S3Service) CreateBucket(bucketName string) error {
	input := &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	}

	// 对于非 us-east-1 区域，需要指定 LocationConstraint
	if s.config.Region != "us-east-1" {
		input.CreateBucketConfiguration = &s3.CreateBucketConfiguration{
			LocationConstraint: aws.String(s.config.Region),
		}
	}

	_, err := s.client.CreateBucket(input)
	if err != nil {
		return fmt.Errorf("创建存储桶失败: %w", err)
	}

	return nil
}

// EnsureBucketExists 确保存储桶存在，如果不存在则创建
func (s *S3Service) EnsureBucketExists(bucketName string) error {
	exists, err := s.BucketExists(bucketName)
	if err != nil {
		return err
	}

	if !exists {
		return s.CreateBucket(bucketName)
	}

	return nil
}

// UploadFile 上传文件到 S3
func (s *S3Service) UploadFile(bucketName, key string, body io.ReadSeeker, contentType string) error {
	return s.UploadFileWithACL(bucketName, key, body, contentType, "public-read")
}

// UploadFileWithACL 上传文件到 S3（支持自定义ACL）
// 如果存储桶不支持ACL，会自动重试不使用ACL
func (s *S3Service) UploadFileWithACL(bucketName, key string, body io.ReadSeeker, contentType string, acl string) error {
	// 确保存储桶存在
	if err := s.EnsureBucketExists(bucketName); err != nil {
		return fmt.Errorf("确保存储桶存在失败: %w", err)
	}

	// 重置body的位置，以便重试时可以使用
	if seeker, ok := body.(io.Seeker); ok {
		seeker.Seek(0, io.SeekStart)
	}

	input := &s3.PutObjectInput{
		Bucket:      aws.String(bucketName),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
		ACL:         aws.String(acl), // 设置ACL为public-read，允许公开访问
	}

	_, err := s.client.PutObject(input)
	if err != nil {
		// 检查是否是ACL不支持的错误
		if strings.Contains(err.Error(), "AccessControlListNotSupported") ||
			strings.Contains(err.Error(), "does not allow ACLs") {
			// 存储桶不支持ACL，重试不使用ACL
			// 重置body位置
			if seeker, ok := body.(io.Seeker); ok {
				seeker.Seek(0, io.SeekStart)
			}

			// 不使用ACL重试上传
			inputWithoutACL := &s3.PutObjectInput{
				Bucket:      aws.String(bucketName),
				Key:         aws.String(key),
				Body:        body,
				ContentType: aws.String(contentType),
				// 不设置ACL，依赖存储桶策略来管理访问权限
			}

			_, retryErr := s.client.PutObject(inputWithoutACL)
			if retryErr != nil {
				// 检查是否是权限错误
				if strings.Contains(retryErr.Error(), "AccessDenied") || strings.Contains(retryErr.Error(), "Access Denied") || strings.Contains(retryErr.Error(), "403") {
					return fmt.Errorf("S3访问被拒绝，请检查AWS凭证权限。需要s3:PutObject权限。错误详情: %w", retryErr)
				}
				return fmt.Errorf("上传文件失败（存储桶不支持ACL，且不使用ACL重试也失败）: %w", retryErr)
			}
			// 不使用ACL上传成功
			return nil
		}

		// 检查是否是权限错误
		if strings.Contains(err.Error(), "AccessDenied") || strings.Contains(err.Error(), "Access Denied") || strings.Contains(err.Error(), "403") {
			return fmt.Errorf("S3访问被拒绝，请检查AWS凭证权限。需要s3:PutObject和s3:PutObjectAcl权限。错误详情: %w", err)
		}
		return fmt.Errorf("上传文件失败: %w", err)
	}

	return nil
}

// GetBucketDomain 获取存储桶的域名
// 对于 CloudFront Origin，使用标准的 S3 REST API 端点格式
func (s *S3Service) GetBucketDomain(bucketName string) string {
	// us-east-1 区域使用特殊格式（没有区域标识）
	if s.config.Region == "us-east-1" {
		return fmt.Sprintf("%s.s3.amazonaws.com", bucketName)
	}
	// 其他区域使用标准格式
	return fmt.Sprintf("%s.s3.%s.amazonaws.com", bucketName, s.config.Region)
}

// UploadString 上传字符串内容到 S3
func (s *S3Service) UploadString(bucketName, key string, content string, contentType string) error {
	body := strings.NewReader(content)
	return s.UploadFile(bucketName, key, body, contentType)
}

// UploadHTML 上传 HTML 内容到 S3（便捷方法）
func (s *S3Service) UploadHTML(bucketName, key string, htmlContent string) error {
	return s.UploadString(bucketName, key, htmlContent, "text/html; charset=utf-8")
}

// DeleteObjectsWithPrefix 删除指定前缀的所有对象
func (s *S3Service) DeleteObjectsWithPrefix(bucketName, prefix string) error {
	// 列出所有匹配前缀的对象
	listInput := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucketName),
		Prefix: aws.String(prefix),
	}

	var objectsToDelete []*s3.ObjectIdentifier

	// 分页列出所有对象
	err := s.client.ListObjectsV2Pages(listInput, func(page *s3.ListObjectsV2Output, lastPage bool) bool {
		for _, obj := range page.Contents {
			objectsToDelete = append(objectsToDelete, &s3.ObjectIdentifier{
				Key: obj.Key,
			})
		}
		return true
	})
	if err != nil {
		return fmt.Errorf("列出对象失败: %w", err)
	}

	// 如果没有对象需要删除，直接返回
	if len(objectsToDelete) == 0 {
		return nil
	}

	// 批量删除对象（每次最多删除1000个）
	const maxDeleteBatch = 1000
	for i := 0; i < len(objectsToDelete); i += maxDeleteBatch {
		end := i + maxDeleteBatch
		if end > len(objectsToDelete) {
			end = len(objectsToDelete)
		}

		deleteInput := &s3.DeleteObjectsInput{
			Bucket: aws.String(bucketName),
			Delete: &s3.Delete{
				Objects: objectsToDelete[i:end],
				Quiet:   aws.Bool(true),
			},
		}

		_, err := s.client.DeleteObjects(deleteInput)
		if err != nil {
			return fmt.Errorf("删除对象失败: %w", err)
		}
	}

	return nil
}

// DeleteObject 删除单个S3对象
func (s *S3Service) DeleteObject(bucketName, key string) error {
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(key),
	}

	_, err := s.client.DeleteObject(input)
	if err != nil {
		return fmt.Errorf("删除S3对象失败: %w", err)
	}

	return nil
}

// ObjectExists 检查S3对象是否存在
func (s *S3Service) ObjectExists(bucketName, key string) (bool, error) {
	input := &s3.HeadObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(key),
	}

	_, err := s.client.HeadObject(input)
	if err != nil {
		// 检查是否是404错误（对象不存在）
		if strings.Contains(err.Error(), "NoSuchKey") || strings.Contains(err.Error(), "404") {
			return false, nil
		}
		// 检查是否是权限错误
		if strings.Contains(err.Error(), "AccessDenied") || strings.Contains(err.Error(), "Access Denied") || strings.Contains(err.Error(), "403") {
			return false, fmt.Errorf("S3访问被拒绝，请检查AWS凭证权限。需要s3:GetObject权限。错误详情: %w", err)
		}
		return false, fmt.Errorf("检查对象是否存在失败: %w", err)
	}

	return true, nil
}

// EnsurePublicAccessBlockDisabled 确保 Block Public Access 设置允许公共策略
func (s *S3Service) EnsurePublicAccessBlockDisabled(bucketName string) error {
	// 检查当前的 Block Public Access 设置
	getInput := &s3.GetPublicAccessBlockInput{
		Bucket: aws.String(bucketName),
	}

	publicAccessBlock, err := s.client.GetPublicAccessBlock(getInput)
	if err != nil {
		// 如果没有设置，说明 Block Public Access 是关闭的，可以直接返回
		if strings.Contains(err.Error(), "NoSuchPublicAccessBlockConfiguration") {
			return nil
		}
		// 其他错误返回
		return fmt.Errorf("获取 Block Public Access 设置失败: %w", err)
	}

	// 如果 BlockPublicPolicy 是启用的，需要禁用它
	if publicAccessBlock.PublicAccessBlockConfiguration != nil &&
		aws.BoolValue(publicAccessBlock.PublicAccessBlockConfiguration.BlockPublicPolicy) {
		// 禁用 Block Public Policy，但保留其他设置
		putInput := &s3.PutPublicAccessBlockInput{
			Bucket: aws.String(bucketName),
			PublicAccessBlockConfiguration: &s3.PublicAccessBlockConfiguration{
				BlockPublicAcls:       publicAccessBlock.PublicAccessBlockConfiguration.BlockPublicAcls,
				IgnorePublicAcls:      publicAccessBlock.PublicAccessBlockConfiguration.IgnorePublicAcls,
				BlockPublicPolicy:     aws.Bool(false), // 禁用 Block Public Policy
				RestrictPublicBuckets: publicAccessBlock.PublicAccessBlockConfiguration.RestrictPublicBuckets,
			},
		}

		_, err = s.client.PutPublicAccessBlock(putInput)
		if err != nil {
			return fmt.Errorf("禁用 Block Public Policy 失败: %w", err)
		}
	}

	return nil
}

// EnsureBucketPolicyForPublicAccess 确保存储桶策略允许公开访问 redirects/* 路径
func (s *S3Service) EnsureBucketPolicyForPublicAccess(bucketName string) error {
	// 首先确保 Block Public Access 设置允许公共策略
	if err := s.EnsurePublicAccessBlockDisabled(bucketName); err != nil {
		return fmt.Errorf("配置 Block Public Access 设置失败: %w", err)
	}

	// 构建 bucket policy，允许公开访问 redirects/* 路径（包括所有子路径）
	// 使用两个 Statement 来确保覆盖所有可能的路径格式
	policy := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Sid":    "PublicReadAccessRedirects",
				"Effect": "Allow",
				"Principal": map[string]interface{}{
					"AWS": "*",
				},
				"Action":   "s3:GetObject",
				"Resource": fmt.Sprintf("arn:aws:s3:::%s/redirects/*", bucketName),
			},
			{
				"Sid":    "PublicReadAccessRedirectsSubdirs",
				"Effect": "Allow",
				"Principal": map[string]interface{}{
					"AWS": "*",
				},
				"Action":   "s3:GetObject",
				"Resource": fmt.Sprintf("arn:aws:s3:::%s/redirects/*/*", bucketName),
			},
		},
	}

	policyJSON, err := json.Marshal(policy)
	if err != nil {
		return fmt.Errorf("序列化 bucket policy 失败: %w", err)
	}

	policyString := string(policyJSON)

	// 检查是否已有 bucket policy
	getPolicyInput := &s3.GetBucketPolicyInput{
		Bucket: aws.String(bucketName),
	}
	existingPolicy, err := s.client.GetBucketPolicy(getPolicyInput)
	if err == nil && existingPolicy.Policy != nil {
		// 如果已有 policy，检查是否包含我们需要的规则
		existingPolicyStr := *existingPolicy.Policy
		if strings.Contains(existingPolicyStr, "redirects/*") {
			// 已包含 redirects/* 的访问规则，不需要更新
			return nil
		}
		// 如果已有 policy 但不包含我们的规则，合并策略（这里简化处理，直接使用新策略）
		// 注意：实际生产环境应该合并策略而不是替换
	}

	// 设置 bucket policy
	putPolicyInput := &s3.PutBucketPolicyInput{
		Bucket: aws.String(bucketName),
		Policy: aws.String(policyString),
	}

	_, err = s.client.PutBucketPolicy(putPolicyInput)
	if err != nil {
		return fmt.Errorf("设置 bucket policy 失败: %w", err)
	}

	return nil
}

// CheckBucketPolicyForPublicAccess 检查存储桶策略是否允许公开访问 redirects/* 路径
func (s *S3Service) CheckBucketPolicyForPublicAccess(bucketName string) (bool, error) {
	// 检查是否已有 bucket policy
	getPolicyInput := &s3.GetBucketPolicyInput{
		Bucket: aws.String(bucketName),
	}
	existingPolicy, err := s.client.GetBucketPolicy(getPolicyInput)
	if err != nil {
		// 如果没有 policy 或获取失败，返回 false
		return false, nil
	}

	if existingPolicy.Policy == nil {
		return false, nil
	}

	// 检查 policy 是否包含 redirects/* 的访问规则
	existingPolicyStr := *existingPolicy.Policy
	if strings.Contains(existingPolicyStr, "redirects/*") {
		return true, nil
	}

	return false, nil
}
