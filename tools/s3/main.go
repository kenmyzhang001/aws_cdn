package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

// S3Config S3配置
type S3Config struct {
	AccessKeyID     string
	SecretAccessKey string
	Region          string
	BucketName      string
}

// S3Migrator S3迁移器
type S3Migrator struct {
	sourceClient *s3.S3
	targetClient *s3.S3
	sourceBucket string
	targetBucket string
	stats        *MigrationStats
}

// MigrationStats 迁移统计
type MigrationStats struct {
	sync.Mutex
	TotalFiles       int
	ProcessedFiles   int
	SuccessFiles     int
	FailedFiles      int
	TotalBytes       int64
	TransferredBytes int64
	StartTime        time.Time
}

func main() {
	// 源S3配置
	srcAccessKey := flag.String("src-access-key", "", "源AWS Access Key ID")
	srcSecretKey := flag.String("src-secret-key", "", "源AWS Secret Access Key")
	srcRegion := flag.String("src-region", "us-east-1", "源AWS Region")
	srcBucket := flag.String("src-bucket", "", "源S3桶名称")
	srcPrefix := flag.String("src-prefix", "", "源S3对象前缀（可选，用于过滤）")

	// 目标S3配置
	dstAccessKey := flag.String("dst-access-key", "", "目标AWS Access Key ID")
	dstSecretKey := flag.String("dst-secret-key", "", "目标AWS Secret Access Key")
	dstRegion := flag.String("dst-region", "us-east-1", "目标AWS Region")
	dstBucket := flag.String("dst-bucket", "", "目标S3桶名称")
	dstPrefix := flag.String("dst-prefix", "", "目标S3对象前缀（可选，用于添加前缀）")

	// 其他选项
	workers := flag.Int("workers", 5, "并发工作线程数")
	dryRun := flag.Bool("dry-run", false, "仅显示将要迁移的文件，不实际执行")

	flag.Parse()

	// 验证必需参数
	if *srcAccessKey == "" || *srcSecretKey == "" || *srcBucket == "" {
		fmt.Println("错误: 必须提供源S3配置参数 (src-access-key, src-secret-key, src-bucket)")
		flag.Usage()
		os.Exit(1)
	}

	if *dstAccessKey == "" || *dstSecretKey == "" || *dstBucket == "" {
		fmt.Println("错误: 必须提供目标S3配置参数 (dst-access-key, dst-secret-key, dst-bucket)")
		flag.Usage()
		os.Exit(1)
	}

	// 创建源和目标配置
	srcConfig := S3Config{
		AccessKeyID:     *srcAccessKey,
		SecretAccessKey: *srcSecretKey,
		Region:          *srcRegion,
		BucketName:      *srcBucket,
	}

	dstConfig := S3Config{
		AccessKeyID:     *dstAccessKey,
		SecretAccessKey: *dstSecretKey,
		Region:          *dstRegion,
		BucketName:      *dstBucket,
	}

	// 创建迁移器
	migrator, err := NewS3Migrator(srcConfig, dstConfig)
	if err != nil {
		log.Fatalf("创建S3迁移器失败: %v", err)
	}

	log.Printf("开始S3文件迁移")
	log.Printf("源桶: %s (Region: %s, Prefix: %s)", *srcBucket, *srcRegion, *srcPrefix)
	log.Printf("目标桶: %s (Region: %s, Prefix: %s)", *dstBucket, *dstRegion, *dstPrefix)
	log.Printf("并发数: %d", *workers)
	if *dryRun {
		log.Printf("模式: 试运行（不实际迁移文件）")
	}

	// 执行迁移
	if *dryRun {
		err = migrator.ListFiles(*srcPrefix)
	} else {
		err = migrator.Migrate(*srcPrefix, *dstPrefix, *workers)
	}

	if err != nil {
		log.Fatalf("迁移失败: %v", err)
	}

	log.Printf("迁移完成!")
}

// NewS3Migrator 创建S3迁移器
func NewS3Migrator(srcConfig, dstConfig S3Config) (*S3Migrator, error) {
	// 创建源S3客户端
	srcSess, err := session.NewSession(&aws.Config{
		Region: aws.String(srcConfig.Region),
		Credentials: credentials.NewStaticCredentials(
			srcConfig.AccessKeyID,
			srcConfig.SecretAccessKey,
			"",
		),
	})
	if err != nil {
		return nil, fmt.Errorf("创建源AWS session失败: %w", err)
	}

	// 创建目标S3客户端
	dstSess, err := session.NewSession(&aws.Config{
		Region: aws.String(dstConfig.Region),
		Credentials: credentials.NewStaticCredentials(
			dstConfig.AccessKeyID,
			dstConfig.SecretAccessKey,
			"",
		),
	})
	if err != nil {
		return nil, fmt.Errorf("创建目标AWS session失败: %w", err)
	}

	return &S3Migrator{
		sourceClient: s3.New(srcSess),
		targetClient: s3.New(dstSess),
		sourceBucket: srcConfig.BucketName,
		targetBucket: dstConfig.BucketName,
		stats: &MigrationStats{
			StartTime: time.Now(),
		},
	}, nil
}

// ListFiles 列出源桶中的文件
func (m *S3Migrator) ListFiles(prefix string) error {
	log.Printf("列出源桶中的文件...")

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(m.sourceBucket),
	}
	if prefix != "" {
		input.Prefix = aws.String(prefix)
	}

	var totalFiles int
	var totalSize int64

	err := m.sourceClient.ListObjectsV2Pages(input, func(page *s3.ListObjectsV2Output, lastPage bool) bool {
		for _, obj := range page.Contents {
			totalFiles++
			totalSize += *obj.Size
			log.Printf("[%d] %s (%.2f MB)", totalFiles, *obj.Key, float64(*obj.Size)/(1024*1024))
		}
		return true
	})

	if err != nil {
		return fmt.Errorf("列出对象失败: %w", err)
	}

	log.Printf("总共找到 %d 个文件, 总大小: %.2f GB", totalFiles, float64(totalSize)/(1024*1024*1024))
	return nil
}

// Migrate 执行迁移
func (m *S3Migrator) Migrate(srcPrefix, dstPrefix string, workers int) error {
	// 检查目标桶是否存在
	if err := m.checkTargetBucket(); err != nil {
		return fmt.Errorf("检查目标桶失败: %w", err)
	}

	// 列出源桶中的所有对象
	objects, err := m.listAllObjects(srcPrefix)
	if err != nil {
		return fmt.Errorf("列出源对象失败: %w", err)
	}

	if len(objects) == 0 {
		log.Printf("源桶中没有找到任何文件")
		return nil
	}

	m.stats.TotalFiles = len(objects)
	for _, obj := range objects {
		m.stats.TotalBytes += *obj.Size
	}

	log.Printf("找到 %d 个文件，总大小: %.2f GB", m.stats.TotalFiles, float64(m.stats.TotalBytes)/(1024*1024*1024))

	// 启动进度报告
	stopProgress := make(chan bool)
	go m.reportProgress(stopProgress)

	// 创建工作池
	objectChan := make(chan *s3.Object, workers)
	var wg sync.WaitGroup

	// 启动worker
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for obj := range objectChan {
				if err := m.copyObject(obj, srcPrefix, dstPrefix); err != nil {
					log.Printf("[Worker %d] 复制失败 %s: %v", workerID, *obj.Key, err)
					m.stats.Lock()
					m.stats.FailedFiles++
					m.stats.Unlock()
				} else {
					m.stats.Lock()
					m.stats.SuccessFiles++
					m.stats.Unlock()
				}
				m.stats.Lock()
				m.stats.ProcessedFiles++
				m.stats.Unlock()
			}
		}(i)
	}

	// 发送对象到worker
	for _, obj := range objects {
		objectChan <- obj
	}
	close(objectChan)

	// 等待所有worker完成
	wg.Wait()
	stopProgress <- true

	// 输出最终统计
	m.printFinalStats()

	return nil
}

// listAllObjects 列出所有对象
func (m *S3Migrator) listAllObjects(prefix string) ([]*s3.Object, error) {
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(m.sourceBucket),
	}
	if prefix != "" {
		input.Prefix = aws.String(prefix)
	}

	var objects []*s3.Object

	err := m.sourceClient.ListObjectsV2Pages(input, func(page *s3.ListObjectsV2Output, lastPage bool) bool {
		objects = append(objects, page.Contents...)
		return true
	})

	if err != nil {
		return nil, err
	}

	return objects, nil
}

// checkTargetBucket 检查目标桶是否存在
func (m *S3Migrator) checkTargetBucket() error {
	_, err := m.targetClient.HeadBucket(&s3.HeadBucketInput{
		Bucket: aws.String(m.targetBucket),
	})
	if err != nil {
		return fmt.Errorf("目标桶不存在或无访问权限: %w", err)
	}
	return nil
}

// copyObject 复制单个对象
func (m *S3Migrator) copyObject(obj *s3.Object, srcPrefix, dstPrefix string) error {
	sourceKey := *obj.Key

	// 计算目标key
	targetKey := sourceKey
	if srcPrefix != "" && dstPrefix != "" {
		// 移除源前缀，添加目标前缀
		if len(sourceKey) > len(srcPrefix) {
			relativePath := sourceKey[len(srcPrefix):]
			targetKey = dstPrefix + relativePath
		}
	} else if dstPrefix != "" {
		// 只添加目标前缀
		targetKey = dstPrefix + sourceKey
	}

	// 检查目标对象是否已存在
	_, err := m.targetClient.HeadObject(&s3.HeadObjectInput{
		Bucket: aws.String(m.targetBucket),
		Key:    aws.String(targetKey),
	})
	if err == nil {
		// 对象已存在，跳过
		log.Printf("跳过已存在的文件: %s", targetKey)
		m.stats.Lock()
		m.stats.TransferredBytes += *obj.Size
		m.stats.Unlock()
		return nil
	}

	// 下载源对象
	getResult, err := m.sourceClient.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(m.sourceBucket),
		Key:    aws.String(sourceKey),
	})
	if err != nil {
		return fmt.Errorf("下载对象失败: %w", err)
	}
	defer getResult.Body.Close()

	// 读取对象内容到buffer
	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, getResult.Body)
	if err != nil {
		return fmt.Errorf("读取对象内容失败: %w", err)
	}

	// 上传到目标桶
	_, err = m.targetClient.PutObject(&s3.PutObjectInput{
		Bucket:        aws.String(m.targetBucket),
		Key:           aws.String(targetKey),
		Body:          bytes.NewReader(buf.Bytes()),
		ContentType:   getResult.ContentType,
		ContentLength: getResult.ContentLength,
		Metadata:      getResult.Metadata,
	})
	if err != nil {
		return fmt.Errorf("上传对象失败: %w", err)
	}

	m.stats.Lock()
	m.stats.TransferredBytes += *obj.Size
	m.stats.Unlock()

	log.Printf("已复制: %s -> %s (%.2f MB)", sourceKey, targetKey, float64(*obj.Size)/(1024*1024))
	return nil
}

// reportProgress 报告进度
func (m *S3Migrator) reportProgress(stop chan bool) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.printProgress()
		case <-stop:
			return
		}
	}
}

// printProgress 打印进度
func (m *S3Migrator) printProgress() {
	m.stats.Lock()
	defer m.stats.Unlock()

	elapsed := time.Since(m.stats.StartTime)
	progress := float64(m.stats.ProcessedFiles) / float64(m.stats.TotalFiles) * 100
	transferredGB := float64(m.stats.TransferredBytes) / (1024 * 1024 * 1024)
	totalGB := float64(m.stats.TotalBytes) / (1024 * 1024 * 1024)
	speed := float64(m.stats.TransferredBytes) / elapsed.Seconds() / (1024 * 1024) // MB/s

	log.Printf("进度: %d/%d (%.2f%%), 成功: %d, 失败: %d, 已传输: %.2f/%.2f GB, 速度: %.2f MB/s, 用时: %s",
		m.stats.ProcessedFiles,
		m.stats.TotalFiles,
		progress,
		m.stats.SuccessFiles,
		m.stats.FailedFiles,
		transferredGB,
		totalGB,
		speed,
		elapsed.Round(time.Second),
	)
}

// printFinalStats 打印最终统计
func (m *S3Migrator) printFinalStats() {
	m.stats.Lock()
	defer m.stats.Unlock()

	elapsed := time.Since(m.stats.StartTime)
	transferredGB := float64(m.stats.TransferredBytes) / (1024 * 1024 * 1024)
	avgSpeed := float64(m.stats.TransferredBytes) / elapsed.Seconds() / (1024 * 1024)

	log.Printf("\n======================================")
	log.Printf("迁移完成统计:")
	log.Printf("总文件数: %d", m.stats.TotalFiles)
	log.Printf("成功: %d", m.stats.SuccessFiles)
	log.Printf("失败: %d", m.stats.FailedFiles)
	log.Printf("已传输: %.2f GB", transferredGB)
	log.Printf("总用时: %s", elapsed.Round(time.Second))
	log.Printf("平均速度: %.2f MB/s", avgSpeed)
	log.Printf("======================================\n")
}
