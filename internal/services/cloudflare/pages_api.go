package cloudflare

import (
	"aws_cdn/internal/logger"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strings"
)

type pagesEnvelope[T any] struct {
	Success bool `json:"success"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
	Result T `json:"result"`
}

type PagesProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type PagesDeployment struct {
	ID      string   `json:"id"`
	Aliases []string `json:"aliases"`
	URL     string   `json:"url"`
}

type PagesDomain struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

func randomHex(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, (n+1)/2)
	_, _ = rand.Read(b)
	s := hex.EncodeToString(b)
	if len(s) > n {
		return s[:n]
	}
	return s
}

func (s *CloudflareService) pagesAPIURL(accountID string, parts ...string) string {
	p := path.Join(append([]string{"/client/v4/accounts", accountID}, parts...)...)
	return "https://api.cloudflare.com" + p
}

func (s *CloudflareService) GetPagesProject(accountID, projectName string) (*PagesProject, error) {
	url := s.pagesAPIURL(accountID, "pages", "projects", projectName)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range s.getAuthHeaders() {
		req.Header.Set(k, v)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("project_not_found")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("获取 Pages 项目失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}
	var env pagesEnvelope[PagesProject]
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if !env.Success {
		msg := "获取 Pages 项目失败"
		if len(env.Errors) > 0 && env.Errors[0].Message != "" {
			msg = env.Errors[0].Message
		}
		return nil, fmt.Errorf(msg)
	}
	return &env.Result, nil
}

func (s *CloudflareService) CreatePagesProject(accountID, projectName, productionBranch string) (*PagesProject, error) {
	url := s.pagesAPIURL(accountID, "pages", "projects")
	payload := map[string]any{
		"name":              projectName,
		"production_branch": productionBranch,
	}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	for k, v := range s.getAuthHeaders() {
		req.Header.Set(k, v)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("创建 Pages 项目失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}
	var env pagesEnvelope[PagesProject]
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if !env.Success {
		msg := "创建 Pages 项目失败"
		if len(env.Errors) > 0 && env.Errors[0].Message != "" {
			msg = env.Errors[0].Message
		}
		return nil, fmt.Errorf(msg)
	}
	return &env.Result, nil
}

// pagesNormalizeFilename 规范化文件名（去除前导斜杠/./，路径清理），用于内部 map key。
func pagesNormalizeFilename(rel string) string {
	rel = strings.TrimSpace(rel)
	rel = strings.TrimPrefix(rel, "./")
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return "index.html"
	}
	return path.Clean(rel)
}

// pagesFileHash 计算文件的 CF Pages 资产哈希。
// 与 Wrangler 保持相同输入结构：SHA-256(base64(content) + extension)[:32]
// （Wrangler 使用 BLAKE3，但 CF 仅将 hash 当作内容寻址 key，不做算法校验）
func pagesFileHash(content []byte, filename string) string {
	ext := strings.TrimPrefix(strings.ToLower(path.Ext(filename)), ".")
	b64 := base64.StdEncoding.EncodeToString(content)
	sum := sha256.Sum256([]byte(b64 + ext))
	return hex.EncodeToString(sum[:])[:32]
}

// pagesAssetContentHash 为历史测试兼容保留：输入可能带前导 / 的路径。
func pagesAssetContentHash(content []byte, filename string) string {
	return pagesFileHash(content, pagesNormalizeFilename(filename))
}

// mimeTypeByFilename 根据文件名后缀返回 MIME 类型。
func mimeTypeByFilename(filename string) string {
	switch strings.ToLower(path.Ext(filename)) {
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css"
	case ".js", ".mjs":
		return "application/javascript"
	case ".json":
		return "application/json"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".txt":
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// getPagesUploadJWT 从 CF 获取短期 JWT，用于资产上传接口鉴权。
func (s *CloudflareService) getPagesUploadJWT(accountID, projectName string) (string, error) {
	url := s.pagesAPIURL(accountID, "pages", "projects", projectName, "upload-token")
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	for k, v := range s.getAuthHeaders() {
		if strings.ToLower(k) == "content-type" {
			continue
		}
		req.Header.Set(k, v)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("获取 Pages 上传 JWT 失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}
	var env pagesEnvelope[struct {
		JWT string `json:"jwt"`
	}]
	if err := json.Unmarshal(body, &env); err != nil {
		return "", fmt.Errorf("解析 upload-token 响应失败: %w", err)
	}
	if !env.Success || env.Result.JWT == "" {
		return "", fmt.Errorf("获取 Pages 上传 JWT 失败: 响应中无有效 jwt 字段，body=%s", string(body))
	}
	return env.Result.JWT, nil
}

// pagesCheckMissing 查询哪些文件哈希在 CF 资产存储中不存在（需要上传）。
func (s *CloudflareService) pagesCheckMissing(jwt string, hashes []string) ([]string, error) {
	b, _ := json.Marshal(map[string]any{"hashes": hashes})
	req, err := http.NewRequest("POST", "https://api.cloudflare.com/client/v4/pages/assets/check-missing", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("check-missing 失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}
	var env pagesEnvelope[[]string]
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("解析 check-missing 响应失败: %w", err)
	}
	if !env.Success {
		return nil, fmt.Errorf("check-missing API 返回失败: %s", string(body))
	}
	return env.Result, nil
}

type pagesUploadPayload struct {
	Key      string            `json:"key"`
	Value    string            `json:"value"`
	Metadata map[string]string `json:"metadata"`
	Base64   bool              `json:"base64"`
}

// pagesUploadAssets 将文件内容上传到 CF Pages 资产存储（base64 编码，JWT 鉴权）。
func (s *CloudflareService) pagesUploadAssets(jwt string, payloads []pagesUploadPayload) error {
	b, _ := json.Marshal(payloads)
	req, err := http.NewRequest("POST", "https://api.cloudflare.com/client/v4/pages/assets/upload", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("上传 Pages 资产失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// pagesUpsertHashes 通知 CF 当前部署包含的所有文件哈希（包括未变更的）。
func (s *CloudflareService) pagesUpsertHashes(jwt string, hashes []string) error {
	b, _ := json.Marshal(map[string]any{"hashes": hashes})
	req, err := http.NewRequest("POST", "https://api.cloudflare.com/client/v4/pages/assets/upsert-hashes", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("upsert-hashes 失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// CreatePagesDeployment 通过 CF Pages Direct Upload 方式创建部署，遵循 Wrangler 的五步流程：
//  1. 获取短期上传 JWT
//  2. 检查哪些文件哈希缺失（避免重复上传）
//  3. 上传缺失文件（base64 编码，JWT 鉴权）
//  4. 注册全部哈希（upsert-hashes）
//  5. 创建部署（仅含 manifest，不含文件内容；manifest key 带前导 /）
func (s *CloudflareService) CreatePagesDeployment(accountID, projectName, branch, commitMessage, pagesBuildOutputDir string, files map[string][]byte) (*PagesDeployment, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("部署文件列表不能为空")
	}

	// 规范化文件名 → 计算哈希
	normalized := make(map[string][]byte, len(files))
	for name, content := range files {
		normalized[pagesNormalizeFilename(name)] = content
	}
	fileHashes := make(map[string]string, len(normalized)) // filename → hash
	for name, content := range normalized {
		fileHashes[name] = pagesFileHash(content, name)
	}

	// Step 1: 获取上传 JWT
	jwt, err := s.getPagesUploadJWT(accountID, projectName)
	if err != nil {
		return nil, fmt.Errorf("Pages 上传 JWT: %w", err)
	}

	// Step 2: 查询缺失哈希
	allHashes := make([]string, 0, len(fileHashes))
	for _, h := range fileHashes {
		allHashes = append(allHashes, h)
	}
	missing, err := s.pagesCheckMissing(jwt, allHashes)
	if err != nil {
		return nil, fmt.Errorf("Pages check-missing: %w", err)
	}
	missingSet := make(map[string]bool, len(missing))
	for _, h := range missing {
		missingSet[h] = true
	}

	// Step 3: 上传缺失文件
	var payloads []pagesUploadPayload
	for name, content := range normalized {
		hash := fileHashes[name]
		if !missingSet[hash] {
			continue
		}
		payloads = append(payloads, pagesUploadPayload{
			Key:   hash,
			Value: base64.StdEncoding.EncodeToString(content),
			Metadata: map[string]string{
				"contentType": mimeTypeByFilename(name),
			},
			Base64: true,
		})
	}
	if len(payloads) > 0 {
		if err := s.pagesUploadAssets(jwt, payloads); err != nil {
			return nil, fmt.Errorf("Pages 资产上传: %w", err)
		}
	}

	// Step 4: 注册全部哈希
	if err := s.pagesUpsertHashes(jwt, allHashes); err != nil {
		return nil, fmt.Errorf("Pages upsert-hashes: %w", err)
	}

	// Step 5: 创建部署（manifest key 带前导 /，与 Wrangler 保持一致）
	manifest := make(map[string]string, len(fileHashes))
	for name, hash := range fileHashes {
		manifest["/"+name] = hash
	}
	manifestJSON, _ := json.Marshal(manifest)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("branch", branch)
	_ = mw.WriteField("commit_dirty", "false")
	_ = mw.WriteField("commit_hash", randomHex(12))
	_ = mw.WriteField("commit_message", commitMessage)
	_ = mw.WriteField("manifest", string(manifestJSON))
	if strings.TrimSpace(pagesBuildOutputDir) != "" {
		_ = mw.WriteField("pages_build_output_dir", pagesBuildOutputDir)
	}
	_ = mw.Close()

	deployURL := s.pagesAPIURL(accountID, "pages", "projects", projectName, "deployments")
	req, err := http.NewRequest("POST", deployURL, &buf)
	if err != nil {
		return nil, err
	}
	for k, v := range s.getAuthHeaders() {
		if strings.ToLower(k) == "content-type" {
			continue
		}
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("创建 Pages 部署失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}
	var env pagesEnvelope[PagesDeployment]
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if !env.Success {
		msg := "创建 Pages 部署失败"
		if len(env.Errors) > 0 && env.Errors[0].Message != "" {
			msg = env.Errors[0].Message
		}
		return nil, fmt.Errorf(msg)
	}
	return &env.Result, nil
}

func (s *CloudflareService) AddPagesDomain(accountID, projectName, domainName string) (*PagesDomain, error) {
	url := s.pagesAPIURL(accountID, "pages", "projects", projectName, "domains")
	payload := map[string]any{"name": domainName}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	for k, v := range s.getAuthHeaders() {
		req.Header.Set(k, v)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("绑定 Pages 域名失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}
	var env pagesEnvelope[PagesDomain]
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if !env.Success {
		msg := "绑定 Pages 域名失败"
		if len(env.Errors) > 0 && env.Errors[0].Message != "" {
			msg = env.Errors[0].Message
		}
		return nil, fmt.Errorf(msg)
	}
	return &env.Result, nil
}

func (s *CloudflareService) ListPagesDomains(accountID, projectName string) ([]PagesDomain, error) {
	url := s.pagesAPIURL(accountID, "pages", "projects", projectName, "domains")
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range s.getAuthHeaders() {
		req.Header.Set(k, v)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("获取 Pages 域名列表失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}
	var env pagesEnvelope[[]PagesDomain]
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if !env.Success {
		msg := "获取 Pages 域名列表失败"
		if len(env.Errors) > 0 && env.Errors[0].Message != "" {
			msg = env.Errors[0].Message
		}
		return nil, fmt.Errorf(msg)
	}
	return env.Result, nil
}

func (s *CloudflareService) DeletePagesDomainByID(accountID, projectName, domainID string) error {
	log := logger.GetLogger()
	log.WithFields(map[string]any{
		"account_id":   accountID,
		"project_name": projectName,
		"domain_id":    domainID,
	}).Info("删除 Pages 域名")
	domainID = strings.TrimSpace(domainID)
	if domainID == "" {
		log.Error("删除 Pages 域名失败：domainID 不能为空")
		return nil
	}
	url := s.pagesAPIURL(accountID, "pages", "projects", projectName, "domains", domainID)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		log.WithError(err).Error("删除 Pages 域名失败：创建请求失败")
		return err
	}
	for k, v := range s.getAuthHeaders() {
		req.Header.Set(k, v)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		log.WithError(err).Error("删除 Pages 域名失败：请求失败")
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		log.Error("删除 Pages 域名失败：域名不存在")
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.WithError(fmt.Errorf("删除 Pages 域名失败 (状态码: %d): %s", resp.StatusCode, string(body))).Error("删除 Pages 域名失败")
		return fmt.Errorf("删除 Pages 域名失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}
	log.WithFields(map[string]any{
		"account_id":   accountID,
		"project_name": projectName,
		"domain_id":    domainID,
	}).Info("删除 Pages 域名成功")
	return nil
}

// DeletePagesDomainByName 按域名解绑 Pages 自定义域名（不存在时返回 nil）。
func (s *CloudflareService) DeletePagesDomainByName(accountID, projectName, domainName string) error {
	log := logger.GetLogger()
	domainName = strings.TrimSpace(strings.ToLower(domainName))
	if domainName == "" {
		return nil
	}
	list, err := s.ListPagesDomains(accountID, projectName)
	if err != nil {
		return err
	}
	for _, d := range list {
		log.WithFields(map[string]any{
			"account_id":      accountID,
			"project_name":    projectName,
			"domain_name":     domainName,
			"domain_id":       d.ID,
			"old_domain_name": d.Name,
			"status":          d.Status,
		}).Info("查询 Pages 域名列表")
		if strings.TrimSpace(strings.ToLower(d.Name)) == domainName {
			log.WithFields(map[string]any{
				"account_id":      accountID,
				"project_name":    projectName,
				"domain_name":     domainName,
				"domain_id":       d.ID,
				"old_domain_name": d.Name,
				"status":          d.Status,
			}).Info("删除 Pages 域名")
			if err := s.DeletePagesDomainByID(accountID, projectName, d.ID); err != nil {
				log.WithError(err).Error("删除 Pages 域名失败")
				return err
			}
			log.WithFields(map[string]any{
				"account_id":      accountID,
				"project_name":    projectName,
				"domain_name":     domainName,
				"domain_id":       d.ID,
				"new_domain_name": d.Name,
				"status":          d.Status,
			}).Info("删除 Pages 域名成功")
			return nil
		}
	}
	return nil
}

func (s *CloudflareService) DeletePagesProject(accountID, projectName string) error {
	url := s.pagesAPIURL(accountID, "pages", "projects", projectName)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	for k, v := range s.getAuthHeaders() {
		req.Header.Set(k, v)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("删除 Pages 项目失败 (状态码: %d): %s", resp.StatusCode, string(body))
	}
	return nil
}
