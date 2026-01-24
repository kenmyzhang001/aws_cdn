package config

import (
	"os"
	"strconv"
)

type Config struct {
	Database      DatabaseConfig
	Server        ServerConfig
	JWT           JWTConfig
	AWS           AWSConfig
	Cloudflare    CloudflareConfig
	ScheduledTask ScheduledTaskConfig
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
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
	EnableSpeedProbeAlert       bool // 是否启用速度探测告警检查任务
	EnableCleanOldResults       bool // 是否启用清理旧探测结果任务
	EnableUpdateCustomDownloadLinks bool // 是否启用更新自定义下载链接实际URL任务
}

func Load() *Config {
	return &Config{
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "3306"),
			User:     getEnv("DB_USER", "root"),
			Password: getEnv("DB_PASSWORD", ""),
			DBName:   getEnv("DB_NAME", "aws_cdn"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Server: ServerConfig{
			Port:     getEnv("SERVER_PORT", "8080"),
			Mode:     getEnv("SERVER_MODE", "debug"),
			Sitename: getEnv("SITENAME", ""),
		},
		JWT: JWTConfig{
			Secret:      getEnv("JWT_SECRET", "your-secret-key"),
			ExpireHours: 24,
		},
		AWS: AWSConfig{
			Region:          getEnv("AWS_REGION", "us-east-1"),
			AccessKeyID:     getEnv("AWS_ACCESS_KEY_ID", ""),
			SecretAccessKey: getEnv("AWS_SECRET_ACCESS_KEY", ""),
			CloudFrontID:    getEnv("CLOUDFRONT_DISTRIBUTION_ID", ""),
			S3BucketName:    getEnv("S3_BUCKET_NAME", ""),
		},
		Cloudflare: CloudflareConfig{
			APIEmail: getEnv("CLOUDFLARE_API_EMAIL", ""),
			APIKey:   getEnv("CLOUDFLARE_API_KEY", ""),
			APIToken: getEnv("CLOUDFLARE_API_TOKEN", ""),
		},
		ScheduledTask: ScheduledTaskConfig{
			EnableSpeedProbeAlert:           getBoolEnv("ENABLE_SPEED_PROBE_ALERT", true),
			EnableCleanOldResults:           getBoolEnv("ENABLE_CLEAN_OLD_RESULTS", true),
			EnableUpdateCustomDownloadLinks: getBoolEnv("ENABLE_UPDATE_CUSTOM_DOWNLOAD_LINKS", true),
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
