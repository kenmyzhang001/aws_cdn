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
	return s.UploadFileWithACL(bucketName, key, body, contentType, "public-read")
}

// UploadFileWithACL 上传文件到 S3（支持自定义ACL）
func (s *S3Service) UploadFileWithACL(bucketName, key string, body io.ReadSeeker, contentType string, acl string) error {
	input := &s3.PutObjectInput{
		Bucket:      aws.String(bucketName),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
		ACL:         aws.String(acl), // 设置ACL为public-read，允许公开访问
	}

	_, err := s.client.PutObject(input)
	if err != nil {
		// 检查是否是权限错误
		if strings.Contains(err.Error(), "AccessDenied") || strings.Contains(err.Error(), "Access Denied") || strings.Contains(err.Error(), "403") {
			return fmt.Errorf("S3访问被拒绝，请检查AWS凭证权限。需要s3:PutObject和s3:PutObjectAcl权限。错误详情: %w", err)
		}
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

