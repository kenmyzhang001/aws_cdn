package handlers

import (
	"aws_cdn/internal/redis"
	"net/http"

	"github.com/gin-gonic/gin"
	redisv9 "github.com/redis/go-redis/v9"
)

// GameStatsHandler 游戏统计相关接口（如从 Redis 读取渠道名）
type GameStatsHandler struct {
	redisClient *redisv9.Client
}

// NewGameStatsHandler 创建 GameStatsHandler
func NewGameStatsHandler(redisClient *redisv9.Client) *GameStatsHandler {
	return &GameStatsHandler{redisClient: redisClient}
}

// ListFullChannelNames 返回 Redis 集合 game_stats:full_channel_names 中的全部渠道名称
// GET /api/v1/game-stats/full-channel-names
func (h *GameStatsHandler) ListFullChannelNames(c *gin.Context) {
	if h.redisClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Redis 未配置"})
		return
	}
	names, err := redis.GetFullChannelNames(c.Request.Context(), h.redisClient)
	if err != nil {
		if err == redisv9.Nil {
			c.JSON(http.StatusOK, gin.H{"data": []string{}, "total": 0})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取渠道列表失败: " + err.Error()})
		return
	}
	if names == nil {
		names = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"data": names, "total": len(names)})
}
