package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"gopkg.in/natefinch/lumberjack.v2"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// SpeedProbeStatus 探测状态
type SpeedProbeStatus string

const (
	SpeedProbeStatusSuccess SpeedProbeStatus = "success" // 成功
	SpeedProbeStatusFailed  SpeedProbeStatus = "failed"  // 失败
	SpeedProbeStatusTimeout SpeedProbeStatus = "timeout" // 超时
)

type SpeedProbeResult struct {
	ID             uint             `json:"id" gorm:"primaryKey"`
	URL            string           `json:"url" gorm:"type:varchar(1000);not null;index:idx_url"`                       // 探测的URL
	ClientIP       string           `json:"client_ip" gorm:"type:varchar(50);not null;index:idx_client_ip"`             // 客户端IP地址
	SpeedKbps      float64          `json:"speed_kbps" gorm:"type:decimal(10,2);not null"`                              // 下载速度 KB/s
	FileSize       *int64           `json:"file_size,omitempty" gorm:"default:null"`                                    // 文件大小（字节）
	DownloadTimeMs *int64           `json:"download_time_ms,omitempty" gorm:"default:null"`                             // 下载耗时（毫秒）
	Status         SpeedProbeStatus `json:"status" gorm:"type:varchar(20);not null;default:'success';index:idx_status"` // 探测状态
	ErrorMessage   string           `json:"error_message,omitempty" gorm:"type:text"`                                   // 错误信息
	UserAgent      string           `json:"user_agent,omitempty" gorm:"type:varchar(500)"`                              // 客户端User-Agent
	CreatedAt      time.Time        `json:"created_at" gorm:"index:idx_created_at;index:idx_url_ip_created"`            // 创建时间
}

// TableName 指定表名
func (SpeedProbeResult) TableName() string {
	return "speed_probe_results"
}

// ProbeRequest 请求结构体
type ProbeRequest struct {
	URLs []string `json:"urls" binding:"required"`
	Type string   `json:"type"`
}

// ProbeResponse 响应结构体
type ProbeResponse struct {
	AvailableURLs []string `json:"available_urls"`
}

// probeURLs 从数据库查询链接的探测结果，按照 speed_kbps 倒序返回
func probeURLs(db *gorm.DB, urls []string, traceID string) []string {
	log.Printf("[TraceID: %s] probeURLs 开始 - URLs数量: %d, URLs: %v", traceID, len(urls), urls)
	startTime := time.Now()

	if len(urls) == 0 {
		log.Printf("[TraceID: %s] probeURLs 结束 - URLs为空，耗时: %v", traceID, time.Since(startTime))
		return []string{}
	}

	var results []SpeedProbeResult
	queryStartTime := time.Now()

	// 一次性查询所有URL的探测结果
	err := db.Where("url IN ? AND created_at > ?",
		urls,
		time.Now().Add(-time.Minute*35)).
		Order("speed_kbps DESC").
		Find(&results).Error

	queryDuration := time.Since(queryStartTime)
	log.Printf("[TraceID: %s] 数据库查询完成 - 耗时: %v, 结果数量: %d", traceID, queryDuration, len(results))

	if err != nil {
		log.Printf("[TraceID: %s] 查询探测结果失败 - 错误: %v, 耗时: %v", traceID, err, time.Since(startTime))
		return []string{}
	}

	if len(results) == 0 {
		log.Printf("[TraceID: %s] 数据库无结果，切换到实时探测模式", traceID)
		probeResults := realTimeProbe(urls, traceID)
		log.Printf("[TraceID: %s] probeURLs 结束（实时探测） - 结果数量: %d, 总耗时: %v", traceID, len(probeResults), time.Since(startTime))
		return probeResults
	}

	// 使用map去重，每个URL只保留speed最高的那条记录
	urlMap := make(map[string]SpeedProbeResult)
	for _, result := range results {
		// 如果URL不存在，或者当前记录速度更快，则更新
		if existing, exists := urlMap[result.URL]; !exists || result.SpeedKbps > existing.SpeedKbps {
			urlMap[result.URL] = result
		}
	}
	log.Printf("[TraceID: %s] 去重后唯一URL数量: %d", traceID, len(urlMap))

	// 按原始urls顺序返回可用的URL
	var availableURLs []string
	for _, url := range urls {
		if result, exists := urlMap[url]; exists {
			log.Printf("[TraceID: %s] 匹配到URL: %s, 速度: %.2f KB/s, 记录时间: %v",
				traceID, result.URL, result.SpeedKbps, result.CreatedAt.Format("2006-01-02 15:04:05"))
			availableURLs = append(availableURLs, result.URL)
		} else {
			log.Printf("[TraceID: %s] 未匹配到URL: %s", traceID, url)
		}
	}

	log.Printf("[TraceID: %s] probeURLs 结束 - 返回URLs数量: %d, 总耗时: %v", traceID, len(availableURLs), time.Since(startTime))
	return availableURLs
}

func probeURLsByType(db *gorm.DB, urls []string, traceID string) []string {
	log.Printf("[TraceID: %s] probeURLsByType 开始 - URLs数量: %d, URLs: %v", traceID, len(urls), urls)
	startTime := time.Now()

	if len(urls) == 0 {
		log.Printf("[TraceID: %s] probeURLsByType 结束 - URLs为空，耗时: %v", traceID, time.Since(startTime))
		return []string{}
	}

	var results []SpeedProbeResult
	queryStartTime := time.Now()

	// 一次性查询所有URL的探测结果
	err := db.Where("url IN ? AND created_at > ?",
		urls,
		time.Now().Add(-time.Minute*35)).
		Order("speed_kbps DESC").
		Find(&results).Error

	queryDuration := time.Since(queryStartTime)
	log.Printf("[TraceID: %s] 数据库查询完成（按类型） - 耗时: %v, 结果数量: %d", traceID, queryDuration, len(results))

	if err != nil {
		log.Printf("[TraceID: %s] 查询探测结果失败（按类型） - 错误: %v, 耗时: %v", traceID, err, time.Since(startTime))
		return []string{}
	}

	if len(results) == 0 {
		log.Printf("[TraceID: %s] 数据库无结果（按类型），切换到实时探测模式", traceID)
		probeResults := realTimeProbe(urls, traceID)
		log.Printf("[TraceID: %s] probeURLsByType 结束（实时探测） - 结果数量: %d, 总耗时: %v", traceID, len(probeResults), time.Since(startTime))
		return probeResults
	}

	// 使用map去重，每个URL只保留speed最高的那条记录
	urlMap := make(map[string]SpeedProbeResult)
	for _, result := range results {
		// 如果URL不存在，或者当前记录速度更快，则更新
		if existing, exists := urlMap[result.URL]; !exists || result.SpeedKbps > existing.SpeedKbps {
			urlMap[result.URL] = result
		}
	}
	log.Printf("[TraceID: %s] 去重后唯一URL数量: %d", traceID, len(urlMap))

	// 将结果转换为数组，按speed_kbps倒序排序
	var sortedResults []SpeedProbeResult
	for _, result := range urlMap {
		sortedResults = append(sortedResults, result)
	}

	sort.Slice(sortedResults, func(i, j int) bool {
		return sortedResults[i].SpeedKbps > sortedResults[j].SpeedKbps
	})

	log.Printf("[TraceID: %s] 按速度排序完成", traceID)

	// 返回按速度排序的URL列表
	var availableURLs []string
	for idx, result := range sortedResults {
		log.Printf("[TraceID: %s] 排序结果 #%d: URL: %s, 速度: %.2f KB/s, 记录时间: %v",
			traceID, idx+1, result.URL, result.SpeedKbps, result.CreatedAt.Format("2006-01-02 15:04:05"))
		availableURLs = append(availableURLs, result.URL)
	}

	log.Printf("[TraceID: %s] probeURLsByType 结束 - 返回URLs数量: %d, 总耗时: %v", traceID, len(availableURLs), time.Since(startTime))
	return availableURLs
}

// probeURL 实时探测单个URL，下载前50KB，返回速度（KB/s）
func probeURL(url string, traceID string) (float64, error) {
	log.Printf("[TraceID: %s] probeURL 开始探测 - URL: %s", traceID, url)
	overallStart := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		log.Printf("[TraceID: %s] probeURL 失败 - URL: %s, 错误: 创建请求失败 - %v, 耗时: %v",
			traceID, url, err, time.Since(overallStart))
		return 0, fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置Range header，只下载前50KB
	req.Header.Set("Range", "bytes=0-51199")
	log.Printf("[TraceID: %s] 设置Range请求头 - URL: %s, Range: bytes=0-51199", traceID, url)

	client := &http.Client{
		Timeout: 1 * time.Second,
	}

	requestStartTime := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[TraceID: %s] probeURL 失败 - URL: %s, 错误: HTTP请求失败 - %v, 请求耗时: %v, 总耗时: %v",
			traceID, url, err, time.Since(requestStartTime), time.Since(overallStart))
		return 0, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("[TraceID: %s] HTTP响应收到 - URL: %s, 状态码: %d, Content-Length: %d, 请求耗时: %v",
		traceID, url, resp.StatusCode, resp.ContentLength, time.Since(requestStartTime))

	// 读取响应体
	downloadStartTime := time.Now()
	bytesRead, err := io.Copy(io.Discard, resp.Body)
	downloadDuration := time.Since(downloadStartTime)

	if err != nil && err != io.EOF && err != context.DeadlineExceeded {
		log.Printf("[TraceID: %s] probeURL 失败 - URL: %s, 错误: 读取响应失败 - %v, 已读取: %d 字节, 下载耗时: %v, 总耗时: %v",
			traceID, url, err, bytesRead, downloadDuration, time.Since(overallStart))
		return 0, fmt.Errorf("读取响应失败: %w", err)
	}

	totalDuration := time.Since(overallStart)
	if totalDuration == 0 {
		log.Printf("[TraceID: %s] probeURL 失败 - URL: %s, 错误: 下载时间为0", traceID, url)
		return 0, fmt.Errorf("下载时间为0")
	}

	// 计算速度 KB/s
	speedKbps := float64(bytesRead) / 1024 / totalDuration.Seconds()

	log.Printf("[TraceID: %s] probeURL 成功 - URL: %s, 下载字节: %d, 下载速度: %.2f KB/s, 下载耗时: %v, 总耗时: %v",
		traceID, url, bytesRead, speedKbps, downloadDuration, totalDuration)

	return speedKbps, nil
}

// realTimeProbe 并发实时探测多个URL，返回按速度排序的URL列表
func realTimeProbe(urls []string, traceID string) []string {
	log.Printf("[TraceID: %s] realTimeProbe 开始 - 待探测URLs数量: %d", traceID, len(urls))
	startTime := time.Now()

	type urlSpeed struct {
		url   string
		speed float64
	}

	var (
		wg         sync.WaitGroup
		mu         sync.Mutex
		results    []urlSpeed
		successCnt int
		failedCnt  int
	)

	// 并发探测所有 URL
	log.Printf("[TraceID: %s] 启动并发探测 - Goroutines数量: %d", traceID, len(urls))
	for idx, url := range urls {
		wg.Add(1)
		go func(index int, u string) {
			defer wg.Done()
			goroutineStart := time.Now()
			log.Printf("[TraceID: %s] Goroutine #%d 开始探测 - URL: %s", traceID, index+1, u)

			speed, err := probeURL(u, traceID)
			goroutineDuration := time.Since(goroutineStart)

			if err != nil {
				log.Printf("[TraceID: %s] Goroutine #%d 探测失败 - URL: %s, 错误: %v, 耗时: %v",
					traceID, index+1, u, err, goroutineDuration)
				mu.Lock()
				failedCnt++
				mu.Unlock()
				return
			}

			log.Printf("[TraceID: %s] Goroutine #%d 探测成功 - URL: %s, 速度: %.2f KB/s, 耗时: %v",
				traceID, index+1, u, speed, goroutineDuration)

			// 使用互斥锁保护共享数据
			mu.Lock()
			results = append(results, urlSpeed{url: u, speed: speed})
			successCnt++
			mu.Unlock()
		}(idx, url)
	}

	// 等待所有 goroutine 完成
	log.Printf("[TraceID: %s] 等待所有Goroutines完成...", traceID)
	wg.Wait()
	waitDuration := time.Since(startTime)
	log.Printf("[TraceID: %s] 所有Goroutines已完成 - 成功: %d, 失败: %d, 总耗时: %v",
		traceID, successCnt, failedCnt, waitDuration)

	// 按速度排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].speed > results[j].speed
	})
	log.Printf("[TraceID: %s] 结果排序完成", traceID)

	// 提取URL列表
	var sortedURLs []string
	for idx, result := range results {
		log.Printf("[TraceID: %s] 实时探测排序结果 #%d: URL: %s, 速度: %.2f KB/s",
			traceID, idx+1, result.url, result.speed)
		sortedURLs = append(sortedURLs, result.url)
	}

	log.Printf("[TraceID: %s] realTimeProbe 结束 - 返回URLs数量: %d, 总耗时: %v",
		traceID, len(sortedURLs), time.Since(startTime))
	return sortedURLs
}

// probeHandler 探活接口处理函数
func probeHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 生成 trace_id
		traceID := uuid.New().String()
		requestStartTime := time.Now()

		log.Printf("[TraceID: %s] ========== 新请求开始 ==========", traceID)
		log.Printf("[TraceID: %s] 请求信息 - 方法: %s, 路径: %s, 客户端IP: %s, User-Agent: %s",
			traceID, c.Request.Method, c.Request.URL.Path, c.ClientIP(), c.Request.UserAgent())

		var req ProbeRequest
		// 绑定JSON请求体
		bindStartTime := time.Now()
		if err := c.ShouldBindJSON(&req); err != nil {
			log.Printf("[TraceID: %s] 请求参数解析失败 - 错误: %v, 耗时: %v",
				traceID, err, time.Since(bindStartTime))
			c.JSON(http.StatusBadRequest, gin.H{
				"error":    "无效的请求格式，需要urls字段（字符串数组）",
				"trace_id": traceID,
			})
			return
		}
		log.Printf("[TraceID: %s] 请求参数解析成功 - Type: %s, URLs数量: %d, 耗时: %v",
			traceID, req.Type, len(req.URLs), time.Since(bindStartTime))
		log.Printf("[TraceID: %s] 请求URLs列表: %v", traceID, req.URLs)

		var availableURLs []string
		queryStartTime := time.Now()

		// 从数据库查询链接
		if req.Type == "" {
			log.Printf("[TraceID: %s] 使用 probeURLs 模式（保持原顺序）", traceID)
			availableURLs = probeURLs(db, req.URLs, traceID)
		} else {
			log.Printf("[TraceID: %s] 使用 probeURLsByType 模式（按速度排序）- Type: %s", traceID, req.Type)
			availableURLs = probeURLsByType(db, req.URLs, traceID)
		}

		queryDuration := time.Since(queryStartTime)
		log.Printf("[TraceID: %s] 查询处理完成 - 耗时: %v", traceID, queryDuration)

		totalDuration := time.Since(requestStartTime)
		log.Printf("[TraceID: %s] 请求参数解析成功 - Type: %s, URL: %v,响应结果 - 可用URLs数量: %d, 可用URLs: %v",
			traceID, req.Type, req.URLs, len(availableURLs), availableURLs)
		log.Printf("[TraceID: %s] ========== 请求完成 - 总耗时: %v ==========", traceID, totalDuration)
		// 返回可用的链接数组
		c.JSON(http.StatusOK, ProbeResponse{
			AvailableURLs: availableURLs,
		})
	}
}

func main() {
	// 确保 logs 目录存在
	if err := os.MkdirAll("logs", 0755); err != nil {
		fmt.Printf("创建 logs 目录失败: %v\n", err)
		os.Exit(1)
	}

	// 配置日志输出到文件，支持自动切割
	logFile := &lumberjack.Logger{
		Filename:   "logs/probe.log", // 日志文件路径
		MaxSize:    50,               // 每个日志文件最大50MB
		MaxBackups: 10,               // 最多保留10个旧日志文件
		MaxAge:     30,               // 日志文件最多保留30天
		Compress:   true,             // 是否压缩旧日志文件
		LocalTime:  true,             // 使用本地时间
	}

	// 同时输出到文件和控制台（方便调试）
	multiWriter := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(multiWriter)
	// 设置日志格式：日期 时间 文件名:行号
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// 同时配置 Gin 的日志输出
	gin.DefaultWriter = multiWriter
	gin.DefaultErrorWriter = multiWriter

	log.Println("========== Probe 服务启动 ==========")
	log.Printf("日志文件配置 - 路径: %s, 最大大小: %dMB, 最多保留: %d个文件",
		logFile.Filename, logFile.MaxSize, logFile.MaxBackups)

	// 加载配置
	// 加载环境变量
	if err := godotenv.Load(); err != nil {
		log.Println("未找到 .env 文件，使用环境变量")
	}
	cfg := Load()
	log.Printf("配置加载完成 - DB Host: %s, DB Port: %s, DB Name: %s",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.DBName)

	// 初始化数据库连接
	log.Println("开始连接数据库...")
	db, err := Initialize(DatabaseConfig{
		Host:     cfg.Database.Host,
		Port:     cfg.Database.Port,
		User:     cfg.Database.User,
		Password: cfg.Database.Password,
		DBName:   cfg.Database.DBName,
		SSLMode:  cfg.Database.SSLMode,
	})
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	log.Println("数据库连接成功")

	// 创建Gin路由
	log.Println("初始化Gin路由...")
	r := gin.Default()

	// 定义探活接口
	r.POST("/probe", probeHandler(db))
	log.Println("注册路由 - POST /probe")

	// 可选：添加健康检查接口
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})
	log.Println("注册路由 - GET /health")

	// 启动服务器，监听8080端口
	log.Println("========== 服务器启动在 :8080 ==========")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

func Initialize(cfg DatabaseConfig) (*gorm.DB, error) {
	// MySQL DSN 格式: user:password@tcp(host:port)/dbname?charset=utf8mb4&parseTime=True&loc=Local
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.DBName,
	)
	log.Printf("数据库 DSN: %s:***@tcp(%s:%s)/%s", cfg.User, cfg.Host, cfg.Port, cfg.DBName)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}

	return db, nil
}

type Config struct {
	Database DatabaseConfig
}

type ServerConfig struct {
	Port     string
	Mode     string
	Sitename string
}

type JWTConfig struct {
	Secret      string
	ExpireHours int
}

type AWSConfig struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	CloudFrontID    string
	S3BucketName    string
}

type CloudflareConfig struct {
	APIEmail string
	APIKey   string
	APIToken string // 如果使用Token认证，优先使用Token
}

type ScheduledTaskConfig struct {
	EnableSpeedProbeAlert           bool // 是否启用速度探测告警检查任务
	EnableCleanOldResults           bool // 是否启用清理旧探测结果任务
	EnableUpdateCustomDownloadLinks bool // 是否启用更新自定义下载链接实际URL任务
}

func Load() *Config {
	return &Config{
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "38.60.244.146"),
			Port:     getEnv("DB_PORT", "3306"),
			User:     getEnv("DB_USER", "debian-sys-maint"),
			Password: getEnv("DB_PASSWORD", "DWRjndrcvb9A9zuF"),
			DBName:   getEnv("DB_NAME", "probe"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getBoolEnv(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		boolValue, err := strconv.ParseBool(value)
		if err != nil {
			return defaultValue
		}
		return boolValue
	}
	return defaultValue
}
