package handlers

import (
	"aws_cdn/internal/models"
	"aws_cdn/internal/redis"
	"net/http"
	"strconv"
	"time"

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

// ListSiteDailyData 按时间查询站点日数据（从 Redis 缓存读取）
// GET /api/v1/game-stats/site-daily?date=2006-01-02&hour=0-23
// 默认：当前日期、当前小时
func (h *GameStatsHandler) ListSiteDailyData(c *gin.Context) {
	if h.redisClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Redis 未配置"})
		return
	}
	now := time.Now()
	dateStr := c.DefaultQuery("date", now.Format("2006-01-02"))
	hourStr := c.DefaultQuery("hour", strconv.Itoa(now.Hour()))
	hour, err := strconv.Atoi(hourStr)
	if err != nil || hour < 0 || hour > 23 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "hour 必须为 0-23"})
		return
	}
	list, err := redis.GetAllSitesData(c.Request.Context(), h.redisClient, dateStr, hour)
	if err != nil {
		if err == redisv9.Nil {
			c.JSON(http.StatusOK, gin.H{"data": []models.SiteDailyData{}, "total": 0})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取站点日数据失败: " + err.Error()})
		return
	}
	if list == nil {
		list = []models.SiteDailyData{}
	}
	c.JSON(http.StatusOK, gin.H{"data": list, "total": len(list)})
}
