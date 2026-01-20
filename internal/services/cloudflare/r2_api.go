package cloudflare

import (
	"aws_cdn/internal/logger"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// R2APIService Cloudflare R2 API 服务（用于管理 R2 存储桶、自定义域名、缓存规则等）
type R2APIService struct {
	R2APIToken string
	client     *http.Client
}

// NewR2APIService 创建 R2 API 服务
func NewR2APIService(R2APIToken string) *R2APIService {
	return &R2APIService{
		R2APIToken: R2APIToken,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// getAuthHeaders 获取认证头
func (s *R2APIService) getAuthHeaders() map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + s.R2APIToken,
		"Content-Type":  "application/json",
	}
}

// EnableR2 启用 R2（检查账户是否已启用 R2）
func (s *R2APIService) EnableR2(accountID string) error {
	log := logger.GetLogger()

	// 通过列出存储桶来检查 R2 是否已启用
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/r2/buckets", accountID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	for k, v := range s.getAuthHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		log.WithError(err).Error("检查 R2 启用状态失败")
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errorResp struct {
			Success bool `json:"success"`
			Errors  []struct {
				Message string `json:"message"`
				Code    int    `json:"code"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(body, &errorResp); err == nil && len(errorResp.Errors) > 0 {
			errorMsg := errorResp.Errors[0].Message
			log.WithFields(map[string]interface{}{
				"account_id":    accountID,
				"status_code":   resp.StatusCode,
				"error_code":    errorResp.Errors[0].Code,
				"error_message": errorMsg,
			}).Error("R2 启用检查失败")
			return fmt.Errorf("R2 未启用或权限不足: %s (错误代码: %d)", errorMsg, errorResp.Errors[0].Code)
		}
		log.WithFields(map[string]interface{}{
			"account_id":    accountID,
			"status_code":   resp.StatusCode,
			"response_body": string(body),
		}).Error("R2 启用检查失败")
		return fmt.Errorf("检查 R2 状态失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	log.WithField("account_id", accountID).Info("R2 已启用")
	return nil
}

// CreateBucket 创建 R2 存储桶
func (s *R2APIService) CreateBucket(accountID, bucketName, location string) error {
	log := logger.GetLogger()

	payload := map[string]interface{}{
		"name": bucketName,
	}
	if location != "" {
		payload["location"] = location
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %w", err)
	}

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/r2/buckets", accountID)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	for k, v := range s.getAuthHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		log.WithError(err).WithField("bucket_name", bucketName).Error("创建 R2 存储桶失败")
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var errorResp struct {
			Success bool `json:"success"`
			Errors  []struct {
				Message string `json:"message"`
				Code    int    `json:"code"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(body, &errorResp); err == nil && len(errorResp.Errors) > 0 {
			errorMsg := errorResp.Errors[0].Message
			log.WithFields(map[string]interface{}{
				"account_id":    accountID,
				"bucket_name":   bucketName,
				"status_code":   resp.StatusCode,
				"error_code":    errorResp.Errors[0].Code,
				"error_message": errorMsg,
			}).Error("创建存储桶失败")
			if resp.StatusCode == http.StatusConflict {
				return fmt.Errorf("存储桶已存在: %s", errorMsg)
			}
			if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
				return fmt.Errorf("认证失败或权限不足: %s (错误代码: %d)。请检查 API Token 是否有效且具有 R2 相关权限", errorMsg, errorResp.Errors[0].Code)
			}
			return fmt.Errorf("创建存储桶失败: %s (错误代码: %d)", errorMsg, errorResp.Errors[0].Code)
		}
		log.WithFields(map[string]interface{}{
			"account_id":    accountID,
			"bucket_name":   bucketName,
			"status_code":   resp.StatusCode,
			"response_body": string(body),
		}).Error("创建存储桶失败")
		return fmt.Errorf("创建存储桶失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	log.WithFields(map[string]interface{}{
		"account_id":  accountID,
		"bucket_name": bucketName,
	}).Info("R2 存储桶创建成功")
	return nil
}

// ConfigureCORS 配置 CORS
func (s *R2APIService) ConfigureCORS(accountID, bucketName string, corsConfig []map[string]interface{}) error {
	log := logger.GetLogger()

	payload := map[string]interface{}{
		"cors": corsConfig,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %w", err)
	}

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/r2/buckets/%s/cors", accountID, bucketName)
	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	for k, v := range s.getAuthHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		log.WithError(err).WithField("bucket_name", bucketName).Error("配置 CORS 失败")
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errorResp struct {
			Success bool `json:"success"`
			Errors  []struct {
				Message string `json:"message"`
				Code    int    `json:"code"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(body, &errorResp); err == nil && len(errorResp.Errors) > 0 {
			return fmt.Errorf("配置 CORS 失败: %s", errorResp.Errors[0].Message)
		}
		return fmt.Errorf("配置 CORS 失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	log.WithFields(map[string]interface{}{
		"account_id":  accountID,
		"bucket_name": bucketName,
	}).Info("CORS 配置成功")
	return nil
}

// AddCustomDomain 添加自定义域名
func (s *R2APIService) AddCustomDomain(accountID, bucketName, domain string) (string, error) {
	log := logger.GetLogger()

	payload := map[string]interface{}{
		"domain": domain,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/r2/buckets/%s/custom_domains", accountID, bucketName)

	log.WithFields(map[string]interface{}{
		"account_id":  accountID,
		"bucket_name": bucketName,
		"domain":      domain,
		"url":         url,
	}).Info("准备添加 R2 自定义域名")

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	for k, v := range s.getAuthHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"account_id":  accountID,
			"bucket_name": bucketName,
			"domain":      domain,
			"url":         url,
		}).Error("添加自定义域名失败：请求失败")
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var errorResp struct {
			Success bool `json:"success"`
			Errors  []struct {
				Message string `json:"message"`
				Code    int    `json:"code"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(body, &errorResp); err == nil && len(errorResp.Errors) > 0 {
			log.WithFields(map[string]interface{}{
				"account_id":    accountID,
				"bucket_name":   bucketName,
				"domain":        domain,
				"url":           url,
				"status_code":   resp.StatusCode,
				"error_code":    errorResp.Errors[0].Code,
				"error_message": errorResp.Errors[0].Message,
				"response_body": string(body),
			}).Error("添加自定义域名失败：API 返回错误")
			return "", fmt.Errorf("添加自定义域名失败: %s (Code: %d)", errorResp.Errors[0].Message, errorResp.Errors[0].Code)
		}
		log.WithFields(map[string]interface{}{
			"account_id":    accountID,
			"bucket_name":   bucketName,
			"domain":        domain,
			"url":           url,
			"status_code":   resp.StatusCode,
			"response_body": string(body),
		}).Error("添加自定义域名失败：非预期状态码")
		return "", fmt.Errorf("添加自定义域名失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Success bool `json:"success"`
		Result  struct {
			ID     string `json:"id"`
			Domain string `json:"domain"`
			Status string `json:"status"`
		} `json:"result"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if !result.Success {
		return "", fmt.Errorf("添加自定义域名失败")
	}

	log.WithFields(map[string]interface{}{
		"account_id":  accountID,
		"bucket_name": bucketName,
		"domain":      domain,
		"domain_id":   result.Result.ID,
	}).Info("自定义域名添加成功")
	return result.Result.ID, nil
}

// CreateCacheRule 创建缓存规则
func (s *R2APIService) CreateCacheRule(zoneID, ruleName, expression, cacheStatus, edgeTTL, browserTTL string) (string, error) {
	log := logger.GetLogger()

	payload := map[string]interface{}{
		"name":       ruleName,
		"expression": expression,
		"action": map[string]interface{}{
			"cache": map[string]interface{}{
				"status":      cacheStatus,
				"edge_ttl":    map[string]interface{}{"mode": "override", "value": edgeTTL},
				"browser_ttl": map[string]interface{}{"mode": "override", "value": browserTTL},
			},
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/rulesets/cache", zoneID)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	for k, v := range s.getAuthHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"zone_id":   zoneID,
			"rule_name": ruleName,
		}).Error("创建缓存规则失败")
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var errorResp struct {
			Success bool `json:"success"`
			Errors  []struct {
				Message string `json:"message"`
				Code    int    `json:"code"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(body, &errorResp); err == nil && len(errorResp.Errors) > 0 {
			return "", fmt.Errorf("创建缓存规则失败: %s", errorResp.Errors[0].Message)
		}
		return "", fmt.Errorf("创建缓存规则失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Success bool `json:"success"`
		Result  struct {
			ID string `json:"id"`
		} `json:"result"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if !result.Success {
		return "", fmt.Errorf("创建缓存规则失败")
	}

	log.WithFields(map[string]interface{}{
		"zone_id":   zoneID,
		"rule_name": ruleName,
		"rule_id":   result.Result.ID,
	}).Info("缓存规则创建成功")
	return result.Result.ID, nil
}
