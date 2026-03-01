package cloudflare

import (
	"aws_cdn/internal/logger"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// KVAPIService Cloudflare KV API 服务
type KVAPIService struct {
	APIToken  string
	AccountID string
	client    *http.Client
}

// NewKVAPIService 创建 KV API 服务
func NewKVAPIService(APIToken, AccountID string) *KVAPIService {
	return &KVAPIService{
		APIToken:  APIToken,
		AccountID: AccountID,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CreateKVNamespaceRequest 创建 KV 命名空间请求
type CreateKVNamespaceRequest struct {
	Title string `json:"title"`
}

// CreateKVNamespaceResponse 创建 KV 命名空间响应
type CreateKVNamespaceResponse struct {
	Success bool `json:"success"`
	Result  struct {
		ID string `json:"id"`
	} `json:"result"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// CreateKVNamespace 创建 KV 命名空间，返回 namespace_id
func (s *KVAPIService) CreateKVNamespace(title string) (string, error) {
	log := logger.GetLogger()
	apiURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/storage/kv/namespaces", s.AccountID)
	body, _ := json.Marshal(CreateKVNamespaceRequest{Title: title})
	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.APIToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result CreateKVNamespaceResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}
	if !result.Success {
		if len(result.Errors) > 0 {
			return "", fmt.Errorf("创建 KV 命名空间失败: %s", result.Errors[0].Message)
		}
		return "", fmt.Errorf("创建 KV 命名空间失败 (状态码: %d): %s", resp.StatusCode, string(respBody))
	}
	log.WithFields(map[string]interface{}{"title": title, "namespace_id": result.Result.ID}).Info("KV 命名空间创建成功")
	return result.Result.ID, nil
}

// WriteKVEntry 写入 KV 键值（key 与 value 均为字符串）
func (s *KVAPIService) WriteKVEntry(namespaceID, key, value string) error {
	log := logger.GetLogger()
	// key 需要 URL 编码
	encodedKey := url.PathEscape(key)
	apiURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/storage/kv/namespaces/%s/values/%s",
		s.AccountID, namespaceID, encodedKey)
	req, err := http.NewRequest("PUT", apiURL, bytes.NewReader([]byte(value)))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.APIToken)
	req.Header.Set("Content-Type", "text/plain")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		log.WithFields(map[string]interface{}{"key": key, "status": resp.StatusCode, "body": string(respBody)}).Warn("写入 KV 失败")
		return fmt.Errorf("写入 KV 失败 (状态码: %d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// DeleteKVKey 删除 KV 中指定 key（解绑域名或更新 domain_paths 时清理）
func (s *KVAPIService) DeleteKVKey(namespaceID, key string) error {
	if namespaceID == "" || key == "" {
		return nil
	}
	encodedKey := url.PathEscape(key)
	apiURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/storage/kv/namespaces/%s/values/%s",
		s.AccountID, namespaceID, encodedKey)
	req, err := http.NewRequest("DELETE", apiURL, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.APIToken)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("删除 KV 键失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// DeleteKVNamespace 删除 KV 命名空间（可选，用于 Worker 删除时清理）
func (s *KVAPIService) DeleteKVNamespace(namespaceID string) error {
	log := logger.GetLogger()
	apiURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/storage/kv/namespaces/%s", s.AccountID, namespaceID)
	req, err := http.NewRequest("DELETE", apiURL, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.APIToken)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("删除 KV 命名空间失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}
	log.WithField("namespace_id", namespaceID).Info("KV 命名空间已删除")
	return nil
}
