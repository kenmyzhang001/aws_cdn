package aws

import (
	"aws_cdn/internal/config"
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

// CreateBucket 创建 S3 存储桶
func (s *S3Service) CreateBucket(bucketName string) error {
	input := &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	}

	_, err := s.client.CreateBucket(input)
	if err != nil {
		return fmt.Errorf("创建存储桶失败: %w", err)
	}

	return nil
}

// UploadFile 上传文件到 S3
func (s *S3Service) UploadFile(bucketName, key string, body io.ReadSeeker, contentType string) error {
	input := &s3.PutObjectInput{
		Bucket:      aws.String(bucketName),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	}

	_, err := s.client.PutObject(input)
	if err != nil {
		return fmt.Errorf("上传文件失败: %w", err)
	}

	return nil
}

// GetBucketDomain 获取存储桶的域名
func (s *S3Service) GetBucketDomain(bucketName string) string {
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

