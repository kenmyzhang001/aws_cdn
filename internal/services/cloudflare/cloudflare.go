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
func (s *CloudflareService) CreateCNAMERecord(zoneID, name, value string) error {
	// 确保value以点结尾（Cloudflare要求）
	if value != "" && !strings.HasSuffix(value, ".") {
		value = value + "."
	}

	// 确保name不以点结尾（Cloudflare不需要）
	name = strings.TrimSuffix(name, ".")

	// 构建请求体
	payload := map[string]interface{}{
		"type":    "CNAME",
		"name":    name,
		"content": value,
		"ttl":     300, // 5分钟
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %w", err)
	}

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", zoneID)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
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

	if resp.StatusCode != http.StatusOK {
		var errorResp struct {
			Success bool `json:"success"`
			Errors  []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(body, &errorResp); err == nil && len(errorResp.Errors) > 0 {
			// 如果记录已存在，尝试更新
			if strings.Contains(errorResp.Errors[0].Message, "already exists") {
				return s.UpdateCNAMERecord(zoneID, name, value)
			}
			return fmt.Errorf("创建CNAME记录失败: %s", errorResp.Errors[0].Message)
		}
		return fmt.Errorf("创建CNAME记录失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if !result.Success {
		if len(result.Errors) > 0 {
			return fmt.Errorf("Cloudflare API错误: %s", result.Errors[0].Message)
		}
		return fmt.Errorf("创建CNAME记录失败")
	}

	return nil
}

// UpdateCNAMERecord 更新CNAME记录
func (s *CloudflareService) UpdateCNAMERecord(zoneID, name, value string) error {
	// 确保value以点结尾
	if value != "" && !strings.HasSuffix(value, ".") {
		value = value + "."
	}

	// 确保name不以点结尾
	name = strings.TrimSuffix(name, ".")

	// 先查找记录ID
	recordID, err := s.findCNAMERecordID(zoneID, name)
	if err != nil {
		return fmt.Errorf("查找记录失败: %w", err)
	}

	// 构建请求体
	payload := map[string]interface{}{
		"type":    "CNAME",
		"name":    name,
		"content": value,
		"ttl":     300,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %w", err)
	}

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", zoneID, recordID)
	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonData))
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

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("更新CNAME记录失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if !result.Success {
		if len(result.Errors) > 0 {
			return fmt.Errorf("Cloudflare API错误: %s", result.Errors[0].Message)
		}
		return fmt.Errorf("更新CNAME记录失败")
	}

	return nil
}

// findCNAMERecordID 查找CNAME记录的ID
func (s *CloudflareService) findCNAMERecordID(zoneID, name string) (string, error) {
	// 确保name不以点结尾
	name = strings.TrimSuffix(name, ".")

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records?type=CNAME&name=%s", zoneID, name)
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
		} `json:"errors"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if !result.Success {
		if len(result.Errors) > 0 {
			return "", fmt.Errorf("Cloudflare API错误: %s", result.Errors[0].Message)
		}
		return "", fmt.Errorf("查找记录失败")
	}

	if len(result.Result) == 0 {
		return "", fmt.Errorf("未找到CNAME记录: %s", name)
	}

	return result.Result[0].ID, nil
}

// CheckCNAMERecord 检查CNAME记录是否存在且值正确
func (s *CloudflareService) CheckCNAMERecord(zoneID, name, expectedValue string) (bool, error) {
	// 确保name不以点结尾
	name = strings.TrimSuffix(name, ".")
	// 确保expectedValue以点结尾
	if expectedValue != "" && !strings.HasSuffix(expectedValue, ".") {
		expectedValue = expectedValue + "."
	}

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records?type=CNAME&name=%s", zoneID, name)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, fmt.Errorf("创建请求失败: %w", err)
	}

	for k, v := range s.getAuthHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
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
		} `json:"errors"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return false, fmt.Errorf("解析响应失败: %w", err)
	}

	if !result.Success {
		if len(result.Errors) > 0 {
			return false, fmt.Errorf("Cloudflare API错误: %s", result.Errors[0].Message)
		}
		return false, nil
	}

	if len(result.Result) == 0 {
		return false, nil
	}

	// 检查值是否匹配
	actualValue := result.Result[0].Content
	// 确保两个值都统一格式（都以点结尾）
	if actualValue != "" && !strings.HasSuffix(actualValue, ".") {
		actualValue = actualValue + "."
	}

	return actualValue == expectedValue, nil
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
