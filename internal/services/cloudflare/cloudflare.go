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

// extractRootDomain 提取根域名
// 例如: www.example.com -> example.com, sub.example.com -> example.com
func extractRootDomain(domain string) string {
	parts := strings.Split(domain, ".")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], ".")
	}
	return domain
}

// AddCustomDomain 添加自定义域名
// domain: 要添加的自定义域名（例如：prefix.example-domain.com）
// zoneID: 域名所在的 Zone ID（可选，如果为空则 Cloudflare 会自动查找）
// enabled: 是否启用该自定义域名（默认为 true）
func (s *CloudflareService) AddCustomDomain(accountID, bucketName, domain, zoneID string, enabled bool) (string, error) {
	log := logger.GetLogger()

	payload := map[string]interface{}{
		"domain":  domain,
		"enabled": enabled,
	}

	// 如果提供了 zoneID，则添加到 payload 中
	if zoneID != "" {
		payload["zoneId"] = zoneID
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/r2/buckets/%s/domains/custom", accountID, bucketName)

	log.WithFields(map[string]interface{}{
		"account_id":  accountID,
		"bucket_name": bucketName,
		"domain":      domain,
		"url":         url,
		"jsonData":    string(jsonData),
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

// ConfigureCORS 配置 CORS
func (s *CloudflareService) ConfigureCORS(accountID, bucketName string, corsConfig []map[string]interface{}) error {
	log := logger.GetLogger()

	payload := map[string]interface{}{
		"rules": corsConfig,
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

// CreateCORSTransformRule 创建 CORS Transform Rule（用于在域名级别添加 CORS 响应头）
// zoneID: 域名所在的 Zone ID
// domain: 要匹配的域名（例如：test111.wkljm.com）
// allowOrigin: 允许的来源（例如："*" 或 "https://yourdomain.com"）
func (s *CloudflareService) CreateCORSTransformRule(zoneID, domain, allowOrigin string) (string, error) {
	log := logger.GetLogger()

	// 构建匹配表达式
	expression := fmt.Sprintf(`(http.host eq "%s")`, domain)
	description := fmt.Sprintf("Add CORS headers for R2 domain %s", domain)

	// 构建响应头设置
	headers := []map[string]interface{}{
		{
			"name":  "Access-Control-Allow-Origin",
			"value": allowOrigin,
			"op":    "set",
		},
		{
			"name":  "Access-Control-Allow-Methods",
			"value": "GET, HEAD, OPTIONS",
			"op":    "set",
		},
		{
			"name":  "Access-Control-Allow-Headers",
			"value": "*",
			"op":    "set",
		},
		{
			"name":  "Access-Control-Expose-Headers",
			"value": "ETag, Content-Length, Content-Type, Content-Range, Content-Disposition",
			"op":    "set",
		},
		{
			"name":  "Access-Control-Max-Age",
			"value": "3600",
			"op":    "set",
		},
	}

	// 构建规则
	rule := map[string]interface{}{
		"expression": expression,
		"action": map[string]interface{}{
			"response_headers": map[string]interface{}{
				"headers": headers,
			},
		},
		"description": description,
		"enabled":     true,
	}

	// 步骤1: 获取或创建 http_response_header_transformation ruleset
	rulesetID, err := s.getOrCreateTransformRuleset(zoneID)
	if err != nil {
		return "", fmt.Errorf("获取或创建 ruleset 失败: %w", err)
	}

	// 步骤2: 获取该 ruleset 的所有 rules，检查是否已存在相同 domain 的 rule
	existingRuleID, err := s.findRuleByExpression(zoneID, rulesetID, expression)
	if err != nil {
		log.WithError(err).WithFields(map[string]interface{}{
			"zone_id":    zoneID,
			"ruleset_id": rulesetID,
			"domain":     domain,
		}).Warn("查找现有 rule 失败，尝试创建新 rule")
		existingRuleID = ""
	}

	// 步骤3: 如果存在，使用 PATCH 更新；否则使用 POST 添加
	var ruleID string
	if existingRuleID != "" {
		// 更新已存在的 rule - 必须包含 id 字段
		rule["id"] = existingRuleID
		// 添加 ref 字段（如果不存在）
		if _, exists := rule["ref"]; !exists {
			rule["ref"] = fmt.Sprintf("cors_%s", domain)
		}
		ruleID, err = s.updateRule(zoneID, rulesetID, existingRuleID, rule)
		if err != nil {
			return "", fmt.Errorf("更新 rule 失败: %w", err)
		}
		log.WithFields(map[string]interface{}{
			"zone_id":    zoneID,
			"ruleset_id": rulesetID,
			"rule_id":    ruleID,
			"domain":     domain,
		}).Info("CORS Transform Rule 更新成功")
	} else {
		// 添加新 rule - 创建不包含 id 的副本，并添加 ref 字段
		newRule := make(map[string]interface{})
		for k, v := range rule {
			newRule[k] = v
		}
		// 添加 ref 字段（用户定义的引用，用于保持一致性）
		newRule["ref"] = fmt.Sprintf("cors_%s", domain)
		// 确保不包含 id 字段（由服务器生成）
		delete(newRule, "id")

		ruleID, err = s.addRule(zoneID, rulesetID, newRule)
		if err != nil {
			return "", fmt.Errorf("添加 rule 失败: %w", err)
		}
		log.WithFields(map[string]interface{}{
			"zone_id":    zoneID,
			"ruleset_id": rulesetID,
			"rule_id":    ruleID,
			"domain":     domain,
		}).Info("CORS Transform Rule 创建成功")
	}

	return ruleID, nil
}

// getOrCreateTransformRuleset 获取或创建 http_response_header_transformation ruleset
func (s *CloudflareService) getOrCreateTransformRuleset(zoneID string) (string, error) {
	log := logger.GetLogger()

	// 先尝试获取现有的 ruleset
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/rulesets", zoneID)
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

	if resp.StatusCode == http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err == nil {
			var rulesetsResp struct {
				Success bool `json:"success"`
				Result  []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
					Kind string `json:"kind"`
				} `json:"result"`
			}
			if err := json.Unmarshal(body, &rulesetsResp); err == nil && rulesetsResp.Success {
				// 查找 http_response_header_transformation ruleset
				for _, rs := range rulesetsResp.Result {
					if rs.Kind == "zone" && rs.Name == "http_response_header_transformation" {
						return rs.ID, nil
					}
				}
			}
		}
	}

	// 如果不存在，创建新的 ruleset
	log.WithFields(map[string]interface{}{
		"zone_id": zoneID,
	}).Info("未找到 http_response_header_transformation ruleset，尝试创建")

	payload := map[string]interface{}{
		"name":  "http_response_header_transformation",
		"kind":  "zone",
		"phase": "http_response_headers_transform",
		"rules": []map[string]interface{}{},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	url = fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/rulesets", zoneID)
	req, err = http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	for k, v := range s.getAuthHeaders() {
		req.Header.Set(k, v)
	}

	resp, err = s.client.Do(req)
	if err != nil {
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
			return "", fmt.Errorf("创建 ruleset 失败: %s (Code: %d)", errorResp.Errors[0].Message, errorResp.Errors[0].Code)
		}
		return "", fmt.Errorf("创建 ruleset 失败 (状态码: %d): %s", resp.StatusCode, string(body))
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
		return "", fmt.Errorf("创建 ruleset 失败")
	}

	return result.Result.ID, nil
}

// findRuleByExpression 根据 expression 查找 rule ID
func (s *CloudflareService) findRuleByExpression(zoneID, rulesetID, expression string) (string, error) {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/rulesets/%s", zoneID, rulesetID)
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

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("获取 ruleset 失败 (状态码: %d)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	var rulesetResp struct {
		Success bool `json:"success"`
		Result  struct {
			Rules []struct {
				ID         string `json:"id"`
				Expression string `json:"expression"`
			} `json:"rules"`
		} `json:"result"`
	}

	if err := json.Unmarshal(body, &rulesetResp); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if !rulesetResp.Success {
		return "", fmt.Errorf("获取 ruleset 失败")
	}

	// 查找匹配的 rule
	for _, rule := range rulesetResp.Result.Rules {
		if rule.Expression == expression {
			return rule.ID, nil
		}
	}

	return "", nil // 未找到
}

// addRule 添加 rule 到 ruleset
func (s *CloudflareService) addRule(zoneID, rulesetID string, rule map[string]interface{}) (string, error) {
	jsonData, err := json.Marshal(rule)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/rulesets/%s/rules", zoneID, rulesetID)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
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

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var errorResp struct {
			Success bool `json:"success"`
			Errors  []struct {
				Message string `json:"message"`
				Code    int    `json:"code"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(body, &errorResp); err == nil && len(errorResp.Errors) > 0 {
			return "", fmt.Errorf("添加 rule 失败: %s (Code: %d)", errorResp.Errors[0].Message, errorResp.Errors[0].Code)
		}
		return "", fmt.Errorf("添加 rule 失败 (状态码: %d): %s", resp.StatusCode, string(body))
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
		return "", fmt.Errorf("添加 rule 失败")
	}

	return result.Result.ID, nil
}

// updateRule 使用 PATCH 更新 rule
func (s *CloudflareService) updateRule(zoneID, rulesetID, ruleID string, rule map[string]interface{}) (string, error) {
	jsonData, err := json.Marshal(rule)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/rulesets/%s/rules/%s", zoneID, rulesetID, ruleID)
	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
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
		var errorResp struct {
			Success bool `json:"success"`
			Errors  []struct {
				Message string `json:"message"`
				Code    int    `json:"code"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(body, &errorResp); err == nil && len(errorResp.Errors) > 0 {
			return "", fmt.Errorf("更新 rule 失败: %s (Code: %d)", errorResp.Errors[0].Message, errorResp.Errors[0].Code)
		}
		return "", fmt.Errorf("更新 rule 失败 (状态码: %d): %s", resp.StatusCode, string(body))
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
		return "", fmt.Errorf("更新 rule 失败")
	}

	return result.Result.ID, nil
}
