package services

import (
	"aws_cdn/internal/logger"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TelegramService Telegram 服务
type TelegramService struct {
	botToken string
	chatID   int64
	client   *http.Client
}

// NewTelegramService 创建 Telegram 服务
func NewTelegramService(botToken string, chatID int64) *TelegramService {
	return &TelegramService{
		botToken: botToken,
		chatID:   chatID,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SendMessage 发送消息到 Telegram
func (s *TelegramService) SendMessage(text string) error {
	log := logger.GetLogger()

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.botToken)

	payload := map[string]interface{}{
		"chat_id": s.chatID,
		"text":    text,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.WithError(err).Error("序列化 Telegram 消息失败")
		return fmt.Errorf("序列化消息失败: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		log.WithError(err).Error("创建 Telegram 请求失败")
		return fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		log.WithError(err).Error("发送 Telegram 消息失败")
		return fmt.Errorf("发送消息失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.WithError(err).Error("读取 Telegram 响应失败")
		return fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.WithFields(map[string]interface{}{
			"status_code": resp.StatusCode,
			"response":    string(body),
		}).Error("Telegram API 返回错误")
		return fmt.Errorf("Telegram API 返回错误: HTTP %d, %s", resp.StatusCode, string(body))
	}

	log.WithField("chat_id", s.chatID).Info("Telegram 消息发送成功")
	return nil
}

// SendMessagesBatch 批量发送消息，每5条合并发送，每条消息间隔1秒
func (s *TelegramService) SendMessagesBatch(urls []string) error {
	if len(urls) == 0 {
		return nil
	}

	const batchSize = 5
	totalBatches := (len(urls) + batchSize - 1) / batchSize

	for i := 0; i < totalBatches; i++ {
		start := i * batchSize
		end := start + batchSize
		if end > len(urls) {
			end = len(urls)
		}

		batch := urls[start:end]
		message := "检测到不可用的下载链接：\n\n"
		for j, url := range batch {
			message += fmt.Sprintf("%d. %s", start+j+1, url)
			if j < len(batch)-1 {
				message += "\n"
			}
		}

		if err := s.SendMessage(message); err != nil {
			return fmt.Errorf("发送第 %d 批消息失败: %w", i+1, err)
		}

		// 如果不是最后一批，等待1秒
		if i < totalBatches-1 {
			time.Sleep(1 * time.Second)
		}
	}

	return nil
}
