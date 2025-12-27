package cloudflare

import (
	"aws_cdn/internal/config"
	"aws_cdn/internal/logger"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type CloudflareService struct {
	apiEmail string
	apiKey   string
	apiToken string
	client   *http.Client
}

func NewCloudflareService(cfg *config.CloudflareConfig) (*CloudflareService, error) {
	if cfg.APIToken == "" && (cfg.APIEmail == "" || cfg.APIKey == "") {
		return nil, fmt.Errorf("Cloudflare配置不完整：需要API Token或API Email+Key")
	}

	return &CloudflareService{
		apiEmail: cfg.APIEmail,
		apiKey:   cfg.APIKey,
		apiToken: cfg.APIToken,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// getAuthHeaders 获取认证头
func (s *CloudflareService) getAuthHeaders() map[string]string {
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	if s.apiToken != "" {
		headers["Authorization"] = "Bearer " + s.apiToken
	} else {
		headers["X-Auth-Email"] = s.apiEmail
		headers["X-Auth-Key"] = s.apiKey
	}
	return headers
}

// GetZoneID 根据域名获取Zone ID
func (s *CloudflareService) GetZoneID(domainName string) (string, error) {
	// 提取根域名（例如：www.example.com -> example.com）
	rootDomain := extractRootDomain(domainName)

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones?name=%s", rootDomain)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	for k, v := range s.getAuthHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API请求失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Success bool `json:"success"`
		Result  []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if !result.Success {
		if len(result.Errors) > 0 {
			return "", fmt.Errorf("Cloudflare API错误: %s", result.Errors[0].Message)
		}
		return "", fmt.Errorf("获取Zone ID失败")
	}

	if len(result.Result) == 0 {
		return "", fmt.Errorf("未找到域名 %s 的Zone", rootDomain)
	}

	return result.Result[0].ID, nil
}

// CreateCNAMERecord 创建CNAME记录
// proxied: 是否启用Cloudflare代理（橙色云朵），默认为 false（灰色云朵，仅DNS）
func (s *CloudflareService) CreateCNAMERecord(zoneID, name, value string, proxied ...bool) error {
	log := logger.GetLogger()

	// 确定是否启用代理（默认不启用）
	shouldProxy := false
	if len(proxied) > 0 {
		shouldProxy = proxied[0]
	}

	// 记录输入参数
	originalName := name
	originalValue := value

	log.WithFields(map[string]interface{}{
		"zone_id":        zoneID,
		"original_name":  originalName,
		"original_value": originalValue,
		"proxied":        shouldProxy,
	}).Info("开始创建Cloudflare CNAME记录")

	// 确保value以点结尾（Cloudflare要求）
	if value != "" && !strings.HasSuffix(value, ".") {
		value = value + "."
	}

	// 确保name不以点结尾（Cloudflare不需要）
	name = strings.TrimSuffix(name, ".")

	log.WithFields(map[string]interface{}{
		"zone_id":         zoneID,
		"original_name":   originalName,
		"processed_name":  name,
		"original_value":  originalValue,
		"processed_value": value,
	}).Info("处理后的CNAME记录参数")

	// 构建请求体
	payload := map[string]interface{}{
		"type":    "CNAME",
		"name":    name,
		"content": value,
		"ttl":     300,         // 5分钟
		"proxied": shouldProxy, // Cloudflare代理设置
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"zone_id": zoneID,
			"name":    name,
			"value":   value,
		}).Error("序列化请求体失败")
		return fmt.Errorf("序列化请求失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"zone_id": zoneID,
		"name":    name,
		"value":   value,
		"payload": string(jsonData),
	}).Info("准备发送创建CNAME记录请求")

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", zoneID)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"zone_id": zoneID,
			"url":     url,
		}).Error("创建HTTP请求失败")
		return fmt.Errorf("创建请求失败: %w", err)
	}

	for k, v := range s.getAuthHeaders() {
		req.Header.Set(k, v)
	}

	log.WithFields(map[string]interface{}{
		"zone_id": zoneID,
		"url":     url,
		"method":  "POST",
		"proxied": shouldProxy,
	}).Info("发送创建CNAME记录请求到Cloudflare API")

	resp, err := s.client.Do(req)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"zone_id": zoneID,
			"url":     url,
		}).Error("HTTP请求执行失败")
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"zone_id":     zoneID,
			"status_code": resp.StatusCode,
		}).Error("读取响应体失败")
		return fmt.Errorf("读取响应失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"zone_id":     zoneID,
		"name":        name,
		"status_code": resp.StatusCode,
		"response":    string(body),
	}).Info("收到Cloudflare API响应")

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
			errorCode := errorResp.Errors[0].Code

			log.WithFields(map[string]interface{}{
				"zone_id":    zoneID,
				"name":       name,
				"value":      value,
				"error_msg":  errorMsg,
				"error_code": errorCode,
			}).Warn("Cloudflare API返回错误，检查是否需要更新记录")

			// 如果记录已存在，尝试更新
			if strings.Contains(errorMsg, "already exists") {
				log.WithFields(map[string]interface{}{
					"zone_id": zoneID,
					"name":    name,
				}).Info("记录已存在，尝试更新")
				return s.UpdateCNAMERecord(zoneID, name, value)
			}

			log.WithFields(map[string]interface{}{
				"zone_id":    zoneID,
				"name":       name,
				"value":      value,
				"error_msg":  errorMsg,
				"error_code": errorCode,
			}).Error("创建CNAME记录失败")
			return fmt.Errorf("创建CNAME记录失败: %s", errorMsg)
		}

		log.WithFields(map[string]interface{}{
			"zone_id":     zoneID,
			"name":        name,
			"status_code": resp.StatusCode,
			"response":    string(body),
		}).Error("创建CNAME记录失败，无法解析错误响应")
		return fmt.Errorf("创建CNAME记录失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Success bool `json:"success"`
		Result  struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Content string `json:"content"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"zone_id":  zoneID,
			"name":     name,
			"response": string(body),
		}).Error("解析响应JSON失败")
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if !result.Success {
		var errorMsg string
		if len(result.Errors) > 0 {
			errorMsg = result.Errors[0].Message
			log.WithFields(map[string]interface{}{
				"zone_id":    zoneID,
				"name":       name,
				"value":      value,
				"error_msg":  errorMsg,
				"error_code": result.Errors[0].Code,
			}).Error("Cloudflare API返回失败状态")
		} else {
			errorMsg = "未知错误"
			log.WithFields(map[string]interface{}{
				"zone_id": zoneID,
				"name":    name,
				"value":   value,
			}).Error("Cloudflare API返回失败状态，但无错误信息")
		}
		return fmt.Errorf("Cloudflare API错误: %s", errorMsg)
	}

	log.WithFields(map[string]interface{}{
		"zone_id":        zoneID,
		"name":           name,
		"value":          value,
		"proxied":        shouldProxy,
		"record_id":      result.Result.ID,
		"record_name":    result.Result.Name,
		"record_content": result.Result.Content,
	}).Info("CNAME记录创建成功")

	return nil
}

// UpdateCNAMERecord 更新CNAME记录
func (s *CloudflareService) UpdateCNAMERecord(zoneID, name, value string) error {
	log := logger.GetLogger()

	log.WithFields(map[string]interface{}{
		"zone_id": zoneID,
		"name":    name,
		"value":   value,
	}).Info("开始更新Cloudflare CNAME记录")

	// 确保value以点结尾
	if value != "" && !strings.HasSuffix(value, ".") {
		value = value + "."
	}

	// 确保name不以点结尾
	name = strings.TrimSuffix(name, ".")

	// 先查找记录ID
	recordID, err := s.findCNAMERecordID(zoneID, name)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"zone_id": zoneID,
			"name":    name,
		}).Error("查找CNAME记录ID失败")
		return fmt.Errorf("查找记录失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"zone_id":   zoneID,
		"name":      name,
		"record_id": recordID,
	}).Info("找到CNAME记录ID，准备更新")

	// 构建请求体
	payload := map[string]interface{}{
		"type":    "CNAME",
		"name":    name,
		"content": value,
		"ttl":     300,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"zone_id":   zoneID,
			"record_id": recordID,
			"name":      name,
			"value":     value,
		}).Error("序列化更新请求体失败")
		return fmt.Errorf("序列化请求失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"zone_id":   zoneID,
		"record_id": recordID,
		"name":      name,
		"value":     value,
		"payload":   string(jsonData),
	}).Info("准备发送更新CNAME记录请求")

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", zoneID, recordID)
	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonData))
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"zone_id":   zoneID,
			"record_id": recordID,
			"url":       url,
		}).Error("创建更新HTTP请求失败")
		return fmt.Errorf("创建请求失败: %w", err)
	}

	for k, v := range s.getAuthHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"zone_id":   zoneID,
			"record_id": recordID,
			"url":       url,
		}).Error("更新HTTP请求执行失败")
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"zone_id":     zoneID,
			"record_id":   recordID,
			"status_code": resp.StatusCode,
		}).Error("读取更新响应体失败")
		return fmt.Errorf("读取响应失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"zone_id":     zoneID,
		"record_id":   recordID,
		"name":        name,
		"status_code": resp.StatusCode,
		"response":    string(body),
	}).Info("收到Cloudflare更新API响应")

	if resp.StatusCode != http.StatusOK {
		log.WithFields(map[string]interface{}{
			"zone_id":     zoneID,
			"record_id":   recordID,
			"name":        name,
			"status_code": resp.StatusCode,
			"response":    string(body),
		}).Error("更新CNAME记录失败，状态码非200")
		return fmt.Errorf("更新CNAME记录失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Success bool `json:"success"`
		Result  struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Content string `json:"content"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"zone_id":   zoneID,
			"record_id": recordID,
			"name":      name,
			"response":  string(body),
		}).Error("解析更新响应JSON失败")
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if !result.Success {
		var errorMsg string
		if len(result.Errors) > 0 {
			errorMsg = result.Errors[0].Message
			log.WithFields(map[string]interface{}{
				"zone_id":    zoneID,
				"record_id":  recordID,
				"name":       name,
				"value":      value,
				"error_msg":  errorMsg,
				"error_code": result.Errors[0].Code,
			}).Error("Cloudflare更新API返回失败状态")
		} else {
			errorMsg = "未知错误"
			log.WithFields(map[string]interface{}{
				"zone_id":   zoneID,
				"record_id": recordID,
				"name":      name,
				"value":     value,
			}).Error("Cloudflare更新API返回失败状态，但无错误信息")
		}
		return fmt.Errorf("Cloudflare API错误: %s", errorMsg)
	}

	log.WithFields(map[string]interface{}{
		"zone_id":         zoneID,
		"record_id":       recordID,
		"name":            name,
		"value":           value,
		"updated_name":    result.Result.Name,
		"updated_content": result.Result.Content,
	}).Info("CNAME记录更新成功")

	return nil
}

// findCNAMERecordID 查找CNAME记录的ID
func (s *CloudflareService) findCNAMERecordID(zoneID, name string) (string, error) {
	log := logger.GetLogger()

	originalName := name
	// 确保name不以点结尾
	name = strings.TrimSuffix(name, ".")

	log.WithFields(map[string]interface{}{
		"zone_id":        zoneID,
		"original_name":  originalName,
		"processed_name": name,
	}).Info("开始查找CNAME记录ID")

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records?type=CNAME&name=%s", zoneID, name)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"zone_id": zoneID,
			"name":    name,
			"url":     url,
		}).Error("创建查找请求失败")
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	for k, v := range s.getAuthHeaders() {
		req.Header.Set(k, v)
	}

	log.WithFields(map[string]interface{}{
		"zone_id": zoneID,
		"name":    name,
		"url":     url,
	}).Info("发送查找CNAME记录请求")

	resp, err := s.client.Do(req)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"zone_id": zoneID,
			"name":    name,
			"url":     url,
		}).Error("查找HTTP请求执行失败")
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"zone_id":     zoneID,
			"name":        name,
			"status_code": resp.StatusCode,
		}).Error("读取查找响应体失败")
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"zone_id":     zoneID,
		"name":        name,
		"status_code": resp.StatusCode,
		"response":    string(body),
	}).Info("收到查找CNAME记录API响应")

	if resp.StatusCode != http.StatusOK {
		log.WithFields(map[string]interface{}{
			"zone_id":     zoneID,
			"name":        name,
			"status_code": resp.StatusCode,
			"response":    string(body),
		}).Error("查找CNAME记录失败，状态码非200")
		return "", fmt.Errorf("查找记录失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Success bool `json:"success"`
		Result  []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"zone_id":  zoneID,
			"name":     name,
			"response": string(body),
		}).Error("解析查找响应JSON失败")
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if !result.Success {
		var errorMsg string
		if len(result.Errors) > 0 {
			errorMsg = result.Errors[0].Message
			log.WithFields(map[string]interface{}{
				"zone_id":    zoneID,
				"name":       name,
				"error_msg":  errorMsg,
				"error_code": result.Errors[0].Code,
			}).Error("Cloudflare查找API返回失败状态")
		} else {
			errorMsg = "未知错误"
			log.WithFields(map[string]interface{}{
				"zone_id": zoneID,
				"name":    name,
			}).Error("Cloudflare查找API返回失败状态，但无错误信息")
		}
		return "", fmt.Errorf("Cloudflare API错误: %s", errorMsg)
	}

	if len(result.Result) == 0 {
		log.WithFields(map[string]interface{}{
			"zone_id": zoneID,
			"name":    name,
		}).Warn("未找到CNAME记录")
		return "", fmt.Errorf("未找到CNAME记录: %s", name)
	}

	recordID := result.Result[0].ID
	log.WithFields(map[string]interface{}{
		"zone_id":     zoneID,
		"name":        name,
		"record_id":   recordID,
		"found_count": len(result.Result),
	}).Info("成功找到CNAME记录ID")

	return recordID, nil
}

// CheckCNAMERecord 检查CNAME记录是否存在且值正确
func (s *CloudflareService) CheckCNAMERecord(zoneID, name, expectedValue string) (bool, error) {
	log := logger.GetLogger()

	originalName := name
	originalExpectedValue := expectedValue

	// 确保name不以点结尾
	name = strings.TrimSuffix(name, ".")
	// 确保expectedValue以点结尾
	if expectedValue != "" && !strings.HasSuffix(expectedValue, ".") {
		expectedValue = expectedValue + "."
	}

	log.WithFields(map[string]interface{}{
		"zone_id":            zoneID,
		"original_name":      originalName,
		"processed_name":     name,
		"original_expected":  originalExpectedValue,
		"processed_expected": expectedValue,
	}).Info("开始检查Cloudflare CNAME记录")

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records?type=CNAME&name=%s", zoneID, name)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"zone_id": zoneID,
			"name":    name,
			"url":     url,
		}).Error("创建检查请求失败")
		return false, fmt.Errorf("创建请求失败: %w", err)
	}

	for k, v := range s.getAuthHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"zone_id": zoneID,
			"name":    name,
			"url":     url,
		}).Error("检查HTTP请求执行失败")
		return false, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"zone_id":     zoneID,
			"name":        name,
			"status_code": resp.StatusCode,
		}).Error("读取检查响应体失败")
		return false, fmt.Errorf("读取响应失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"zone_id":     zoneID,
		"name":        name,
		"status_code": resp.StatusCode,
		"response":    string(body),
	}).Info("收到检查CNAME记录API响应")

	if resp.StatusCode != http.StatusOK {
		log.WithFields(map[string]interface{}{
			"zone_id":     zoneID,
			"name":        name,
			"status_code": resp.StatusCode,
			"response":    string(body),
		}).Error("检查CNAME记录失败，状态码非200")
		return false, fmt.Errorf("检查记录失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Success bool `json:"success"`
		Result  []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Content string `json:"content"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"zone_id":  zoneID,
			"name":     name,
			"response": string(body),
		}).Error("解析检查响应JSON失败")
		return false, fmt.Errorf("解析响应失败: %w", err)
	}

	if !result.Success {
		var errorMsg string
		if len(result.Errors) > 0 {
			errorMsg = result.Errors[0].Message
			log.WithFields(map[string]interface{}{
				"zone_id":    zoneID,
				"name":       name,
				"error_msg":  errorMsg,
				"error_code": result.Errors[0].Code,
			}).Error("Cloudflare检查API返回失败状态")
		} else {
			log.WithFields(map[string]interface{}{
				"zone_id": zoneID,
				"name":    name,
			}).Warn("Cloudflare检查API返回失败状态，但无错误信息，记录可能不存在")
		}
		return false, nil
	}

	if len(result.Result) == 0 {
		log.WithFields(map[string]interface{}{
			"zone_id": zoneID,
			"name":    name,
		}).Info("未找到CNAME记录")
		return false, nil
	}

	// 检查值是否匹配
	actualValue := result.Result[0].Content
	// 确保两个值都统一格式（都以点结尾）
	if actualValue != "" && !strings.HasSuffix(actualValue, ".") {
		actualValue = actualValue + "."
	}

	matches := actualValue == expectedValue
	log.WithFields(map[string]interface{}{
		"zone_id":        zoneID,
		"name":           name,
		"expected_value": expectedValue,
		"actual_value":   actualValue,
		"matches":        matches,
		"record_id":      result.Result[0].ID,
		"record_name":    result.Result[0].Name,
	}).Info("CNAME记录检查完成")

	return matches, nil
}

// OriginCertificate Cloudflare Origin证书信息
type OriginCertificate struct {
	Certificate string `json:"certificate"` // 证书内容
	PrivateKey  string `json:"private_key"` // 私钥
}

// generateCSR 生成证书签名请求（CSR）
func generateCSR(hostnames []string) (string, *rsa.PrivateKey, error) {
	// 生成 RSA 私钥（2048 位）
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", nil, fmt.Errorf("生成私钥失败: %w", err)
	}

	// 提取根域名作为 CN（Common Name）
	var cn string
	if len(hostnames) > 0 {
		// 使用第一个主机名，去掉通配符
		cn = strings.TrimPrefix(hostnames[0], "*.")
	} else {
		cn = "example.com"
	}

	// 创建 CSR 模板
	template := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: cn,
		},
		DNSNames: hostnames,
	}

	// 生成 CSR
	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &template, privateKey)
	if err != nil {
		return "", nil, fmt.Errorf("创建CSR失败: %w", err)
	}

	// 将 CSR 编码为 PEM 格式
	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrBytes,
	})

	return string(csrPEM), privateKey, nil
}

// encodePrivateKeyToPEM 将 RSA 私钥编码为 PEM 格式
func encodePrivateKeyToPEM(privateKey *rsa.PrivateKey) (string, error) {
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})
	return string(privateKeyPEM), nil
}

// CreateOriginCertificate 创建Cloudflare Origin泛域名证书
// 注意：Cloudflare 托管的域名会自动获得免费 TLS 证书（用于 Cloudflare 边缘服务器），
// 但要在 AWS CloudFront 中使用该域名，需要创建 Origin 证书并导入到 ACM。
// 返回证书和私钥
func (s *CloudflareService) CreateOriginCertificate(domainName string) (*OriginCertificate, error) {
	// 提取根域名
	rootDomain := extractRootDomain(domainName)

	// 构建主机名列表：包含根域名和泛域名
	hostnames := []string{
		rootDomain,
		"*." + rootDomain,
	}

	// 生成 CSR 和私钥
	csr, privateKey, err := generateCSR(hostnames)
	if err != nil {
		return nil, fmt.Errorf("生成CSR失败: %w", err)
	}

	// 构建请求体，包含 CSR
	// requested_validity 最大值为 5475 天（15年），但某些账户可能有更低的限制
	payload := map[string]interface{}{
		"request_type":       "origin-rsa", // 使用RSA类型
		"hostnames":          hostnames,
		"requested_validity": 5475, // 15年（最大有效期，单位：天）
		"csr":                csr,  // 包含生成的 CSR
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	// Cloudflare Origin 证书使用 /certificates 端点
	url := "https://api.cloudflare.com/client/v4/certificates"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	for k, v := range s.getAuthHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	// 打印响应 body 用于调试
	log := logger.GetLogger()
	log.WithFields(map[string]interface{}{
		"status_code": resp.StatusCode,
		"body":        string(body),
	}).Info("Cloudflare API 响应")

	if resp.StatusCode != http.StatusOK {
		var errorResp struct {
			Success bool `json:"success"`
			Errors  []struct {
				Message string `json:"message"`
				Code    int    `json:"code"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(body, &errorResp); err == nil && len(errorResp.Errors) > 0 {
			// 记录完整的错误信息以便调试
			errMsg := errorResp.Errors[0].Message
			if errorResp.Errors[0].Code != 0 {
				errMsg = fmt.Sprintf("%s (code: %d)", errMsg, errorResp.Errors[0].Code)
			}
			return nil, fmt.Errorf("创建Origin证书失败: %s", errMsg)
		}
		return nil, fmt.Errorf("创建Origin证书失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Success bool `json:"success"`
		Result  struct {
			ID          string   `json:"id"`
			Certificate string   `json:"certificate"`
			PrivateKey  string   `json:"private_key"`
			Hostnames   []string `json:"hostnames"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if !result.Success {
		if len(result.Errors) > 0 {
			return nil, fmt.Errorf("Cloudflare API错误: %s", result.Errors[0].Message)
		}
		return nil, fmt.Errorf("创建Origin证书失败")
	}

	// 如果提供了 CSR，Cloudflare 不会返回私钥，需要使用我们生成的私钥
	privateKeyPEM := result.Result.PrivateKey
	if privateKeyPEM == "" {
		// Cloudflare 没有返回私钥，使用我们生成的私钥
		privateKeyPEM, err = encodePrivateKeyToPEM(privateKey)
		if err != nil {
			return nil, fmt.Errorf("编码私钥失败: %w", err)
		}
	}

	return &OriginCertificate{
		Certificate: result.Result.Certificate,
		PrivateKey:  privateKeyPEM,
	}, nil
}

// extractRootDomain 提取根域名
// 例如: www.example.com -> example.com, sub.example.com -> example.com
func extractRootDomain(domain string) string {
	parts := strings.Split(domain, ".")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], ".")
	}
	return domain
}
