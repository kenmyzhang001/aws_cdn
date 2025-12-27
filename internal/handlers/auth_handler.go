package handlers

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authService *services.AuthService
}

func NewAuthHandler(authService *services.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

// Login 登录接口：账号 + 密码 + 谷歌验证码（如果启用）
func (h *AuthHandler) Login(c *gin.Context) {
	log := logger.GetLogger()
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
		OTPCode  string `json:"otp_code"` // 谷歌验证码（如果启用二步验证则为必填）
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).WithField("username", req.Username).Error("登录失败：请求参数验证失败")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	token, err := h.authService.Authenticate(req.Username, req.Password, req.OTPCode)
	if err != nil {
		log.WithError(err).WithField("username", req.Username).Error("登录失败：认证失败")
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	log.WithField("username", req.Username).Info("用户登录成功")
	c.JSON(http.StatusOK, gin.H{
		"token":    token,
		"username": req.Username,
	})
}


