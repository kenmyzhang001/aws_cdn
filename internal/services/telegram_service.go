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
	botToken           string
	chatID             int64
	sitename           string
	client             *http.Client
	speedCheckCallback func() error // 下载速度探测回调函数
}

// NewTelegramService 创建 Telegram 服务
func NewTelegramService(botToken string, chatID int64, sitename string) *TelegramService {
	return &TelegramService{
		botToken: botToken,
		chatID:   chatID,
		sitename: sitename,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetSpeedCheckCallback 设置下载速度探测回调函数
func (s *TelegramService) SetSpeedCheckCallback(callback func() error) {
	s.speedCheckCallback = callback
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
		if s.sitename != "" {
			message = fmt.Sprintf("[%s] 检测到不可用的下载链接：\n\n", s.sitename)
		}
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

// TelegramUpdate Telegram webhook 更新结构
type TelegramUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		MessageID int64 `json:"message_id"`
		From      *struct {
			ID        int64  `json:"id"`
			IsBot     bool   `json:"is_bot"`
			FirstName string `json:"first_name"`
			Username  string `json:"username"`
		} `json:"from"`
		Chat struct {
			ID    int64  `json:"id"`
			Type  string `json:"type"`
			Title string `json:"title"`
		} `json:"chat"`
		Date     int64  `json:"date"`
		Text     string `json:"text"`
		Entities []struct {
			Type   string `json:"type"`
			Offset int    `json:"offset"`
			Length int    `json:"length"`
		} `json:"entities"`
	} `json:"message"`
}

// HandleWebhook 处理 Telegram webhook 更新
func (s *TelegramService) HandleWebhook(update TelegramUpdate) error {
	log := logger.GetLogger()

	// 只处理来自指定chatID的消息
	if update.Message == nil || update.Message.Chat.ID != s.chatID {
		return nil
	}

	// 检查是否是命令
	text := update.Message.Text
	if len(text) == 0 || text[0] != '/' {
		return nil
	}

	// 解析命令
	command := text
	if len(text) > 1 {
		// 移除 / 符号
		command = text[1:]
		// 移除可能的 @botname
		if idx := len(command); idx > 0 {
			for i := 0; i < idx; i++ {
				if command[i] == ' ' || command[i] == '@' {
					command = command[:i]
					break
				}
			}
		}
	}

	log.WithFields(map[string]interface{}{
		"chat_id": update.Message.Chat.ID,
		"command": command,
		"text":    text,
	}).Info("收到 Telegram 命令")

	// 处理命令
	switch command {
	case "speed", "speedcheck", "check_speed":
		if s.speedCheckCallback != nil {
			// 发送确认消息
			s.SendMessage("正在执行下载速度探测，请稍候...")
			// 异步执行探测
			go func() {
				if err := s.speedCheckCallback(); err != nil {
					log.WithError(err).Error("下载速度探测失败")
					s.SendMessage(fmt.Sprintf("下载速度探测失败: %v", err))
				}
			}()
		} else {
			s.SendMessage("下载速度探测功能未配置")
		}
	default:
		// 未知命令，发送帮助信息
		helpText := "可用命令：\n"
		helpText += "/speed - 执行下载速度探测"
		s.SendMessage(helpText)
	}

	return nil
}
