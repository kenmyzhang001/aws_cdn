package config

import (
	"os"
)

type Config struct {
	Database   DatabaseConfig
	Server     ServerConfig
	JWT        JWTConfig
	AWS        AWSConfig
	Cloudflare CloudflareConfig
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
	Port string
	Mode string
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
			Port: getEnv("SERVER_PORT", "8080"),
			Mode: getEnv("SERVER_MODE", "debug"),
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
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
