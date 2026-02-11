package redis

import (
	"aws_cdn/internal/config"
	"aws_cdn/internal/models"
	"context"
	"encoding/json"
	"fmt"
	"time"

	redisv9 "github.com/redis/go-redis/v9"
)

const fullChannelNamesKey = "game_stats:full_channel_names"

const allSitesDataKeyPrefix = "allSitesData"

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

// GetAllSitesData 从 Redis 按日期+小时读取站点日数据缓存，key 格式：allSitesData_YYYY-MM-DD_HH
func GetAllSitesData(ctx context.Context, client *redisv9.Client, startDate string, hour int) ([]models.SiteDailyData, error) {
	if client == nil {
		return nil, redisv9.Nil
	}
	key := fmt.Sprintf("%s_%s_%02d", allSitesDataKeyPrefix, startDate, hour)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	data, err := client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}
	var list []models.SiteDailyData
	if err := json.Unmarshal(data, &list); err != nil {
		// Redis 存的是 JSON 字符串（string），需先反序列化为 string 再反序列化为 []SiteDailyData
		var raw string
		if err2 := json.Unmarshal(data, &raw); err2 != nil {
			return nil, err
		}
		if err2 := json.Unmarshal([]byte(raw), &list); err2 != nil {
			return nil, err2
		}
	}
	return list, nil
}
