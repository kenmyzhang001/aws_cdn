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

// WorkerAPIService Cloudflare Workers API 服务
type WorkerAPIService struct {
	APIToken  string
	AccountID string
	client    *http.Client
}

// NewWorkerAPIService 创建 Worker API 服务
func NewWorkerAPIService(APIToken, AccountID string) *WorkerAPIService {
	return &WorkerAPIService{
		APIToken:  APIToken,
		AccountID: AccountID,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// getAuthHeaders 获取认证头
func (s *WorkerAPIService) getAuthHeaders() map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + s.APIToken,
		"Content-Type":  "application/javascript",
	}
}

// CreateWorker 创建或更新 Worker 脚本
func (s *WorkerAPIService) CreateWorker(workerName, script string) error {
	log := logger.GetLogger()

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/workers/scripts/%s", s.AccountID, workerName)

	log.WithFields(map[string]interface{}{
		"account_id":  s.AccountID,
		"worker_name": workerName,
		"url":         url,
	}).Info("开始创建 Worker 脚本")

	req, err := http.NewRequest("PUT", url, bytes.NewBufferString(script))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	for k, v := range s.getAuthHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"worker_name": workerName,
		"status_code": resp.StatusCode,
		"response":    string(body),
	}).Info("Worker 脚本创建响应")

	if resp.StatusCode != http.StatusOK {
		var errorResponse struct {
			Success bool `json:"success"`
			Errors  []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(body, &errorResponse); err == nil {
			if len(errorResponse.Errors) > 0 {
				return fmt.Errorf("创建 Worker 失败: %s", errorResponse.Errors[0].Message)
			}
		}
		return fmt.Errorf("创建 Worker 失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	log.WithFields(map[string]interface{}{
		"worker_name": workerName,
	}).Info("Worker 脚本创建成功")

	return nil
}

// DeleteWorker 删除 Worker 脚本
func (s *WorkerAPIService) DeleteWorker(workerName string) error {
	log := logger.GetLogger()

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/workers/scripts/%s", s.AccountID, workerName)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	headers := s.getAuthHeaders()
	headers["Content-Type"] = "application/json"
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("删除 Worker 失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	log.WithFields(map[string]interface{}{
		"worker_name": workerName,
	}).Info("Worker 脚本删除成功")

	return nil
}

// CreateWorkerRoute 创建 Worker 路由（绑定域名）
func (s *WorkerAPIService) CreateWorkerRoute(zoneID, pattern, workerName string) (string, error) {
	log := logger.GetLogger()

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/workers/routes", zoneID)

	payload := map[string]interface{}{
		"pattern": pattern,
		"script":  workerName,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	headers := s.getAuthHeaders()
	headers["Content-Type"] = "application/json"
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	log.WithFields(map[string]interface{}{
		"zone_id":     zoneID,
		"pattern":     pattern,
		"worker_name": workerName,
	}).Info("开始创建 Worker 路由")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"zone_id":     zoneID,
		"status_code": resp.StatusCode,
		"response":    string(body),
	}).Info("Worker 路由创建响应")

	if resp.StatusCode != http.StatusOK {
		var errorResponse struct {
			Success bool `json:"success"`
			Errors  []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(body, &errorResponse); err == nil {
			if len(errorResponse.Errors) > 0 {
				return "", fmt.Errorf("创建 Worker 路由失败: %s", errorResponse.Errors[0].Message)
			}
		}
		return "", fmt.Errorf("创建 Worker 路由失败 (状态码: %d): %s", resp.StatusCode, string(body))
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

	log.WithFields(map[string]interface{}{
		"zone_id":  zoneID,
		"route_id": result.Result.ID,
	}).Info("Worker 路由创建成功")

	return result.Result.ID, nil
}

// DeleteWorkerRoute 删除 Worker 路由
func (s *WorkerAPIService) DeleteWorkerRoute(zoneID, routeID string) error {
	log := logger.GetLogger()

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/workers/routes/%s", zoneID, routeID)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	headers := s.getAuthHeaders()
	headers["Content-Type"] = "application/json"
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("删除 Worker 路由失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	log.WithFields(map[string]interface{}{
		"zone_id":  zoneID,
		"route_id": routeID,
	}).Info("Worker 路由删除成功")

	return nil
}

// AddWorkerCustomDomain 为 Worker 添加自定义域名
func (s *WorkerAPIService) AddWorkerCustomDomain(workerName, hostname, zoneID string) (string, error) {
	log := logger.GetLogger()

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/workers/domains", s.AccountID)

	payload := map[string]interface{}{
		"hostname":    hostname,
		"service":     workerName,
		"environment": "production",
	}

	// 如果提供了 zoneID，则添加到 payload 中
	if zoneID != "" {
		payload["zone_id"] = zoneID
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	headers := s.getAuthHeaders()
	headers["Content-Type"] = "application/json"
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	log.WithFields(map[string]interface{}{
		"hostname":    hostname,
		"worker_name": workerName,
		"zone_id":     zoneID,
	}).Info("开始添加 Worker 自定义域名")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"hostname":    hostname,
		"status_code": resp.StatusCode,
		"response":    string(body),
	}).Info("Worker 自定义域名添加响应")

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var errorResponse struct {
			Success bool `json:"success"`
			Errors  []struct {
				Message string `json:"message"`
				Code    int    `json:"code"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(body, &errorResponse); err == nil {
			if len(errorResponse.Errors) > 0 {
				return "", fmt.Errorf("添加 Worker 自定义域名失败: %s (Code: %d)", errorResponse.Errors[0].Message, errorResponse.Errors[0].Code)
			}
		}
		return "", fmt.Errorf("添加 Worker 自定义域名失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Success bool `json:"success"`
		Result  struct {
			ID       string `json:"id"`
			Hostname string `json:"hostname"`
		} `json:"result"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"hostname":  hostname,
		"domain_id": result.Result.ID,
	}).Info("Worker 自定义域名添加成功")

	return result.Result.ID, nil
}

// DeleteWorkerCustomDomain 删除 Worker 自定义域名
func (s *WorkerAPIService) DeleteWorkerCustomDomain(domainID string) error {
	log := logger.GetLogger()

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/workers/domains/%s", s.AccountID, domainID)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	headers := s.getAuthHeaders()
	headers["Content-Type"] = "application/json"
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("删除 Worker 自定义域名失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	log.WithFields(map[string]interface{}{
		"domain_id": domainID,
	}).Info("Worker 自定义域名删除成功")

	return nil
}

// GenerateWorkerScript 生成 Worker 脚本
func GenerateWorkerScript(targetDomain string) string {
	return fmt.Sprintf(`async function handle(request) {
  const target = "%s";
  const ua = request.headers.get("user-agent") || "";
  
  /**
   * 社交 / 短信 WebView（缅甸真实主流）
   * Telegram / Viber / Line / WhatsApp / 微信 / 系统 WebView
   */
  if (/Telegram|Viber|Line|WhatsApp|MicroMessenger|WebView|wv/i.test(ua)) {
    return new Response(
      '<!doctype html><meta charset="utf-8"><script>location.replace("' + target + '")</script>',
      {
        status: 200,
        headers: {
          "Content-Type": "text/html; charset=UTF-8",
          "Cache-Control": "public, max-age=1800, s-maxage=1800",
          "CDN-Cache-Control": "public, max-age=1800",
          "Surrogate-Control": "max-age=1800",
          "X-Entry-Worker": "webview-redirect"
        }
      }
    );
  }
  
  /**
   * 普通浏览器（Chrome / 系统浏览器）
   * → 0ms 302
   */
  return new Response(null, {
    status: 302,
    headers: {
      "Location": target,
      "Cache-Control": "public, max-age=1800, s-maxage=1800",
      "CDN-Cache-Control": "public, max-age=1800",
      "Surrogate-Control": "max-age=1800",
      "Vary": "*",
      "X-Entry-Worker": "standard-302"
    }
  });
}

addEventListener("fetch", (event) => {
  event.respondWith(handle(event.request));
});
`, targetDomain)
}
