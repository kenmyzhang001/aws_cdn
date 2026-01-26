package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
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
func probeURLs(db *gorm.DB, urls []string) []string {
	if len(urls) == 0 {
		return []string{}
	}

	var results []SpeedProbeResult

	// 一次性查询所有URL的探测结果
	err := db.Where("url IN ? AND created_at > ?",
		urls,
		time.Now().Add(-time.Minute*35)).
		Order("speed_kbps DESC").
		Find(&results).Error

	if err != nil {
		log.Printf("查询探测结果失败: %v", err)
		return []string{}
	}

	// 使用map去重，每个URL只保留speed最高的那条记录
	urlMap := make(map[string]SpeedProbeResult)
	for _, result := range results {
		// 如果URL不存在，或者当前记录速度更快，则更新
		if existing, exists := urlMap[result.URL]; !exists || result.SpeedKbps > existing.SpeedKbps {
			urlMap[result.URL] = result
		}
	}

	// 按原始urls顺序返回可用的URL
	var availableURLs []string
	for _, url := range urls {
		if result, exists := urlMap[url]; exists {
			fmt.Printf("url: %s, speed: %.2f, created_at: %v\n", result.URL, result.SpeedKbps, result.CreatedAt)
			availableURLs = append(availableURLs, result.URL)
		}
	}

	return availableURLs
}

func probeURLsByType(db *gorm.DB, urls []string) []string {
	if len(urls) == 0 {
		return []string{}
	}

	var results []SpeedProbeResult

	// 一次性查询所有URL的探测结果
	err := db.Where("url IN ? AND created_at > ?",
		urls,
		time.Now().Add(-time.Minute*35)).
		Order("speed_kbps DESC").
		Find(&results).Error

	if err != nil {
		log.Printf("查询探测结果失败: %v", err)
		return []string{}
	}

	// 使用map去重，每个URL只保留speed最高的那条记录
	urlMap := make(map[string]SpeedProbeResult)
	for _, result := range results {
		// 如果URL不存在，或者当前记录速度更快，则更新
		if existing, exists := urlMap[result.URL]; !exists || result.SpeedKbps > existing.SpeedKbps {
			urlMap[result.URL] = result
		}
	}

	// 将结果转换为数组，按speed_kbps倒序排序
	var sortedResults []SpeedProbeResult
	for _, result := range urlMap {
		sortedResults = append(sortedResults, result)
	}

	sort.Slice(sortedResults, func(i, j int) bool {
		return sortedResults[i].SpeedKbps > sortedResults[j].SpeedKbps
	})

	// 返回按速度排序的URL列表
	var availableURLs []string
	for _, result := range sortedResults {
		fmt.Printf("url: %s, speed: %.2f, created_at: %v\n", result.URL, result.SpeedKbps, result.CreatedAt)
		availableURLs = append(availableURLs, result.URL)
	}

	return availableURLs
}

// probeHandler 探活接口处理函数
func probeHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ProbeRequest
		var availableURLs []string
		// 绑定JSON请求体
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "无效的请求格式，需要urls字段（字符串数组）",
			})
			return
		}

		// 从数据库查询链接
		if req.Type == "" {
			availableURLs = probeURLs(db, req.URLs)
		} else {
			availableURLs = probeURLsByType(db, req.URLs)
		}

		// 打印出参
		log.Printf("[ProbeHandler] 请求参数 - Type: %s, URLs数量: %d, URLs: %v, 响应结果 - 可用URLs数量: %d, 可用URLs: %v", req.Type, len(req.URLs), req.URLs, len(availableURLs), availableURLs)

		// 返回可用的链接数组
		c.JSON(http.StatusOK, ProbeResponse{
			AvailableURLs: availableURLs,
		})
	}
}

func main() {
	// 加载配置
	// 加载环境变量
	if err := godotenv.Load(); err != nil {
		log.Println("未找到 .env 文件，使用环境变量")
	}
	cfg := Load()

	// 初始化数据库连接
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

	// 创建Gin路由
	r := gin.Default()

	// 定义探活接口
	r.POST("/probe", probeHandler(db))

	// 可选：添加健康检查接口
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	// 启动服务器，监听8080端口
	r.Run(":8080")
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
	fmt.Println("dsn:", dsn)

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
