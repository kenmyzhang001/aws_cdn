package handlers

import (
	"aws_cdn/internal/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

// TelegramHandler Telegram webhook 处理器
type TelegramHandler struct {
	telegramService *services.TelegramService
}

// NewTelegramHandler 创建 Telegram handler
func NewTelegramHandler(telegramService *services.TelegramService) *TelegramHandler {
	return &TelegramHandler{
		telegramService: telegramService,
	}
}

// HandleWebhook 处理 Telegram webhook
func (h *TelegramHandler) HandleWebhook(c *gin.Context) {
	var update services.TelegramUpdate
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求格式"})
		return
	}

	// 处理 webhook
	if err := h.telegramService.HandleWebhook(update); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "处理 webhook 失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
