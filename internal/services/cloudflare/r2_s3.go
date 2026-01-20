package cloudflare

import (
	"aws_cdn/internal/logger"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

// R2S3Service R2 S3 兼容服务（用于文件操作）
type R2S3Service struct {
	client     *s3.S3
	accountID  string
	bucketName string
}

// R2S3Config R2 S3 配置
type R2S3Config struct {
	AccountID       string
	AccessKeyID     string
	SecretAccessKey string
	BucketName      string
}

// NewR2S3Service 创建 R2 S3 服务
func NewR2S3Service(cfg *R2S3Config) (*R2S3Service, error) {
	log := logger.GetLogger()

	if cfg.AccountID == "" || cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, fmt.Errorf("R2配置不完整：需要AccountID、AccessKeyID和SecretAccessKey")
	}

	// 验证 Account ID 格式（应该是32位字符）
	if len(cfg.AccountID) != 32 {
		return nil, fmt.Errorf("Account ID 格式不正确：应该是32位字符，当前长度: %d", len(cfg.AccountID))
	}

	// 验证 Access Key ID 格式（通常以特定前缀开头）
	if len(cfg.AccessKeyID) < 20 {
		return nil, fmt.Errorf("Access Key ID 格式可能不正确：长度过短 (%d 字符)", len(cfg.AccessKeyID))
	}

	// 构建 R2 端点 URL
	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.AccountID)

	log.WithFields(map[string]interface{}{
		"account_id":     cfg.AccountID,
		"bucket_name":    cfg.BucketName,
		"endpoint":       endpoint,
		"access_key_id":  cfg.AccessKeyID[:min(8, len(cfg.AccessKeyID))] + "...", // 只记录前8位，保护隐私
		"has_secret_key": cfg.SecretAccessKey != "",
		"secret_key_len": len(cfg.SecretAccessKey),
	}).Info("创建 R2 S3 服务")

	// 创建 AWS session（使用 R2 的 S3 兼容端点）
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("auto"), // R2 使用 "auto" 作为区域
		Credentials: credentials.NewStaticCredentials(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			"",
		),
		Endpoint:         aws.String(endpoint),
		S3ForcePathStyle: aws.Bool(true), // R2 需要路径样式
		DisableSSL:       aws.Bool(false),
	})
	if err != nil {
		log.WithError(err).Error("创建 R2 session 失败")
		return nil, fmt.Errorf("创建 R2 session 失败: %w", err)
	}

	return &R2S3Service{
		client:     s3.New(sess),
		accountID:  cfg.AccountID,
		bucketName: cfg.BucketName,
	}, nil
}

// min 辅助函数
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// UploadFile 上传文件到 R2
func (s *R2S3Service) UploadFile(key string, body io.ReadSeeker, contentType string) error {
	log := logger.GetLogger()
	log.WithFields(map[string]interface{}{
		"bucket_name":  s.bucketName,
		"key":          key,
		"content_type": contentType,
	}).Info("开始上传文件到 R2")

	input := &s3.PutObjectInput{
		Bucket:      aws.String(s.bucketName),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	}

	_, err := s.client.PutObject(input)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"bucket_name": s.bucketName,
			"key":         key,
		}).Error("上传文件到 R2 失败")
		return fmt.Errorf("上传文件失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"bucket_name": s.bucketName,
		"key":         key,
	}).Info("文件上传到 R2 成功")

	return nil
}

// CreateDirectory 创建目录（实际上是在 R2 中创建一个空对象，以 "/" 结尾）
func (s *R2S3Service) CreateDirectory(prefix string) error {
	log := logger.GetLogger()

	// 确保前缀以 "/" 结尾
	if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}

	log.WithFields(map[string]interface{}{
		"bucket_name": s.bucketName,
		"prefix":      prefix,
	}).Info("开始创建 R2 目录")

	// 创建一个空对象作为目录标记
	input := &s3.PutObjectInput{
		Bucket:      aws.String(s.bucketName),
		Key:         aws.String(prefix),
		ContentType: aws.String("application/x-directory"),
	}

	_, err := s.client.PutObject(input)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"bucket_name": s.bucketName,
			"prefix":      prefix,
		}).Error("创建 R2 目录失败")
		return fmt.Errorf("创建目录失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"bucket_name": s.bucketName,
		"prefix":      prefix,
	}).Info("R2 目录创建成功")

	return nil
}

// ListFiles 列出存储桶中的文件
func (s *R2S3Service) ListFiles(prefix string) ([]string, error) {
	log := logger.GetLogger()
	log.WithFields(map[string]interface{}{
		"bucket_name": s.bucketName,
		"prefix":      prefix,
		"account_id":  s.accountID,
		"endpoint":    fmt.Sprintf("https://%s.r2.cloudflarestorage.com", s.accountID),
	}).Info("开始列出 R2 文件")

	// 先尝试 HeadBucket 来验证认证和权限
	headInput := &s3.HeadBucketInput{
		Bucket: aws.String(s.bucketName),
	}
	_, headErr := s.client.HeadBucket(headInput)
	if headErr != nil {
		log.WithError(headErr).WithFields(map[string]interface{}{
			"bucket_name": s.bucketName,
			"account_id":  s.accountID,
		}).Error("HeadBucket 验证失败")
		// 不直接返回，继续尝试 ListObjectsV2
	}

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucketName),
	}
	if prefix != "" {
		input.Prefix = aws.String(prefix)
	}

	log.WithFields(map[string]interface{}{
		"bucket_name": s.bucketName,
		"prefix":      prefix,
		"max_keys":    aws.Int64Value(input.MaxKeys),
	}).Debug("准备调用 ListObjectsV2")

	result, err := s.client.ListObjectsV2(input)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"bucket_name": s.bucketName,
			"prefix":      prefix,
			"account_id":  s.accountID,
			"endpoint":    fmt.Sprintf("https://%s.r2.cloudflarestorage.com", s.accountID),
		}).Error("列出 R2 文件失败")

		// 提供更详细的错误信息
		errMsg := err.Error()
		if strings.Contains(errMsg, "Unauthorized") || strings.Contains(errMsg, "401") {
			return nil, fmt.Errorf("认证失败 (401 Unauthorized): R2 Access Key ID 或 Secret Access Key 无效或权限不足。请检查：1) 在 Cloudflare Dashboard → R2 → Manage R2 API Tokens 中确认 Token 存在且未过期 2) 确认 Token 权限为 'Read and Write' 3) 确认 Token 允许访问存储桶 '%s'（或选择 'All buckets'）4) 确认 CF 账号设置中的 Access Key ID 和 Secret Access Key 与 Token 完全一致（注意不要有多余空格）5) 确认 Account ID '%s' 与 Token 属于同一个账户。原始错误: %w", s.bucketName, s.accountID, err)
		}
		return nil, fmt.Errorf("列出文件失败: %w", err)
	}

	var keys []string
	for _, obj := range result.Contents {
		keys = append(keys, *obj.Key)
	}

	log.WithFields(map[string]interface{}{
		"bucket_name": s.bucketName,
		"prefix":      prefix,
		"count":       len(keys),
	}).Info("列出 R2 文件成功")

	return keys, nil
}

// DeleteFile 删除文件
func (s *R2S3Service) DeleteFile(key string) error {
	log := logger.GetLogger()
	log.WithFields(map[string]interface{}{
		"bucket_name": s.bucketName,
		"key":         key,
	}).Info("开始从 R2 删除文件")

	input := &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	}

	_, err := s.client.DeleteObject(input)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"bucket_name": s.bucketName,
			"key":         key,
		}).Error("从 R2 删除文件失败")
		return fmt.Errorf("删除文件失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"bucket_name": s.bucketName,
		"key":         key,
	}).Info("从 R2 删除文件成功")

	return nil
}

// FileExists 检查文件是否存在
func (s *R2S3Service) FileExists(key string) (bool, error) {
	input := &s3.HeadObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	}

	_, err := s.client.HeadObject(input)
	if err != nil {
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "404") {
			return false, nil
		}
		return false, fmt.Errorf("检查文件是否存在失败: %w", err)
	}

	return true, nil
}
