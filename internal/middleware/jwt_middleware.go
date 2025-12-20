package middleware

import (
	"aws_cdn/internal/auth"
	"aws_cdn/internal/logger"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// JWTAuth JWT 鉴权中间件
func JWTAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		log := logger.GetLogger()
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			log.WithFields(map[string]interface{}{
				"path": c.Request.URL.Path,
				"ip":   c.ClientIP(),
			}).Warn("未授权访问尝试")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			log.WithFields(map[string]interface{}{
				"path": c.Request.URL.Path,
				"ip":   c.ClientIP(),
			}).Warn("授权头格式错误")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "授权头格式错误"})
			return
		}

		claims, err := auth.ParseToken(parts[1], secret)
		if err != nil {
			log.WithError(err).WithFields(map[string]interface{}{
				"path": c.Request.URL.Path,
				"ip":   c.ClientIP(),
			}).Warn("令牌验证失败")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "令牌无效或已过期"})
			return
		}

		// 将用户信息放入上下文
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)

		log.WithFields(map[string]interface{}{
			"user_id":  claims.UserID,
			"username": claims.Username,
			"path":     c.Request.URL.Path,
		}).Debug("用户认证成功")

		c.Next()
	}
}


