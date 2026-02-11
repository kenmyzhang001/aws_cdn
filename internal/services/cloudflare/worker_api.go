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

// GenerateWorkerScript 生成 Worker 脚本（单链接）
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

// WorkerScriptConfig 多目标/轮播 Worker 配置
type WorkerScriptConfig struct {
	Targets     []string // 目标链接列表
	FallbackURL string   // 兜底链接（可选）
	Mode        string   // time / random / probe
	RotateDays  int      // 时间轮播每 N 天
	BaseDate    string   // 时间轮播基准日期 ISO
}

// GenerateWorkerScriptAdvanced 生成多目标+探活+轮播 Worker 脚本
func GenerateWorkerScriptAdvanced(cfg WorkerScriptConfig) (string, error) {
	if len(cfg.Targets) == 0 {
		return "", fmt.Errorf("targets 不能为空")
	}
	targetsJSON, err := json.Marshal(cfg.Targets)
	if err != nil {
		return "", err
	}
	fallbackEscaped := escapeJSString(cfg.FallbackURL)
	probePrimary := "https://probe1.aglobalpay.com/probe"
	probeBackup := "https://probe2.aglobalpay.com/probe"

	var pickFunction string
	switch cfg.Mode {
	case "time":
		days := cfg.RotateDays
		if days <= 0 {
			days = 7
		}
		baseDate := cfg.BaseDate
		if baseDate == "" {
			baseDate = time.Now().Format("2006-01-02")
		}
		pickFunction = fmt.Sprintf(`
async function pick() {
  const availableTargets = await getAvailableTargets();
  if (availableTargets.length === 0) return FALLBACK_URL || '';
  const baseDate = new Date('%sT00:00:00Z');
  const today = new Date();
  const baseDateOnly = new Date(baseDate.getFullYear(), baseDate.getMonth(), baseDate.getDate());
  const todayOnly = new Date(today.getFullYear(), today.getMonth(), today.getDate());
  const diffTime = todayOnly - baseDateOnly;
  const diffDays = Math.floor(diffTime / (1000 * 60 * 60 * 24));
  const index = Math.floor(diffDays / %d) %% availableTargets.length;
  return availableTargets[index];
}`, baseDate, days)
	case "probe":
		pickFunction = `
async function pick() {
  const availableTargets = await probeUrlsWithType(TARGETS, 'probe');
  if (availableTargets.length === 0) return FALLBACK_URL || '';
  return availableTargets[0];
}`
	default: // random
		pickFunction = `
async function pick() {
  const availableTargets = await getAvailableTargets();
  if (availableTargets.length === 0) return FALLBACK_URL || '';
  return availableTargets[Math.floor(Math.random() * availableTargets.length)];
}`
	}

	script := fmt.Sprintf(`addEventListener("fetch", event => {
  event.respondWith(handle(event.request));
});

const TARGETS = %s;
const FALLBACK_URL = %s;
const PROBE_PRIMARY = "%s";
const PROBE_BACKUP = "%s";

let probeCache = { targets: [], timestamp: 0 };
const PROBE_CACHE_TTL = 60000;

async function probeUrls(urls) {
  const probeNodes = [PROBE_PRIMARY, PROBE_BACKUP];
  const timeout = 5000;
  for (const probeNode of probeNodes) {
    try {
      const timeoutPromise = new Promise((_, reject) => setTimeout(() => reject(new Error('Timeout')), timeout));
      const fetchPromise = fetch(probeNode, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ urls: urls })
      });
      const response = await Promise.race([fetchPromise, timeoutPromise]);
      if (response.ok) {
        const data = await response.json();
        if (data.available_urls && Array.isArray(data.available_urls)) return data.available_urls;
      }
    } catch (e) { continue; }
  }
  return [];
}

async function probeUrlsWithType(urls, type) {
  const probeNodes = [PROBE_PRIMARY, PROBE_BACKUP];
  const timeout = 5000;
  for (const probeNode of probeNodes) {
    try {
      const timeoutPromise = new Promise((_, reject) => setTimeout(() => reject(new Error('Timeout')), timeout));
      const fetchPromise = fetch(probeNode, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ urls: urls, type: type })
      });
      const response = await Promise.race([fetchPromise, timeoutPromise]);
      if (response.ok) {
        const data = await response.json();
        if (data.available_urls && Array.isArray(data.available_urls)) return data.available_urls;
      }
    } catch (e) { continue; }
  }
  return [];
}

async function getAvailableTargets() {
  const now = Date.now();
  if (probeCache.targets.length > 0 && (now - probeCache.timestamp) < PROBE_CACHE_TTL)
    return probeCache.targets;
  const availableTargets = await probeUrls(TARGETS);
  probeCache = { targets: availableTargets, timestamp: now };
  return availableTargets;
}

%s

async function handle(request) {
  const target = await pick();
  const ua = request.headers.get("user-agent") || "";
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
          "X-Entry-Worker": "xbbsh-webview"
        }
      }
    );
  }
  return new Response(null, {
    status: 302,
    headers: {
      "Location": target,
      "Cache-Control": "public, max-age=1800, s-maxage=1800",
      "CDN-Cache-Control": "public, max-age=1800",
      "Surrogate-Control": "max-age=1800",
      "Vary": "*",
      "X-Entry-Worker": "xbbsh-302"
    }
  });
}
`, string(targetsJSON), fallbackEscaped, probePrimary, probeBackup, pickFunction)
	return script, nil
}

func escapeJSString(s string) string {
	// 输出为 JS 字符串字面量，用双引号包裹
	b, _ := json.Marshal(s)
	return string(b)
}
