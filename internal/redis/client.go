package redis

import (
	"aws_cdn/internal/config"
	"context"
	"time"

	redisv9 "github.com/redis/go-redis/v9"
)

const fullChannelNamesKey = "game_stats:full_channel_names"

// NewClient 根据配置创建 Redis 客户端
func NewClient(cfg *config.RedisConfig) *redisv9.Client {
	if cfg == nil || cfg.Addr == "" {
		return nil
	}
	return redisv9.NewClient(&redisv9.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
}

// GetFullChannelNames 从 Redis 集合获取全部渠道名称
func GetFullChannelNames(ctx context.Context, client *redisv9.Client) ([]string, error) {
	if client == nil {
		return nil, redisv9.Nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return client.SMembers(ctx, fullChannelNamesKey).Result()
}
