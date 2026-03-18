package cloudflare

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
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

// pagesAssetManifestKey 与 Cloudflare REST API 文档一致：manifest 键为无前导斜杠的站点路径。
// 文档示例：manifest='{"index.html":"...","style.css":"..."}'。
// 若写成 "/index.html"，部署接口常仍返回 success，但资源未正确映射，*.pages.dev 根路径会 404。
func pagesAssetManifestKey(rel string) string {
	rel = strings.TrimSpace(rel)
	rel = strings.TrimPrefix(rel, "./")
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return "index.html"
	}
	return path.Clean(rel)
}

// CreatePagesDeployment 通过 Direct Upload 方式创建部署。
// files: key 为站点内路径（如 index.html、css/a.css），manifest 中为同名规范键。
// pagesBuildOutputDir 非空时才会提交该字段；传空则与 Wrangler 纯静态部署行为一致（不强行写 "."）。
func (s *CloudflareService) CreatePagesDeployment(accountID, projectName, branch, commitMessage, pagesBuildOutputDir string, files map[string][]byte) (*PagesDeployment, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("部署文件列表不能为空")
	}
	normalized := make(map[string][]byte, len(files))
	for name, content := range files {
		key := pagesAssetManifestKey(name)
		normalized[key] = content
	}
	manifest := make(map[string]string, len(normalized))
	for name, content := range normalized {
		sum := sha1.Sum(content)
		manifest[name] = hex.EncodeToString(sum[:])
	}
	manifestJSON, _ := json.Marshal(manifest)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("branch", branch)
	_ = w.WriteField("commit_dirty", "false")
	_ = w.WriteField("commit_hash", randomHex(12))
	_ = w.WriteField("commit_message", commitMessage)
	_ = w.WriteField("manifest", string(manifestJSON))
	if strings.TrimSpace(pagesBuildOutputDir) != "" {
		_ = w.WriteField("pages_build_output_dir", pagesBuildOutputDir)
	}

	// multipart 中每个文件段的 name 为 manifest 中的 hash；filename 与 manifest 键一致。
	for name, content := range normalized {
		fieldName := manifest[name]
		filename := name
		if filename == "" || filename == "." {
			filename = "index.html"
		}
		fw, err := w.CreateFormFile(fieldName, filename)
		if err != nil {
			_ = w.Close()
			return nil, fmt.Errorf("创建文件字段失败: %w", err)
		}
		if _, err := fw.Write(content); err != nil {
			_ = w.Close()
			return nil, fmt.Errorf("写入文件内容失败: %w", err)
		}
	}
	_ = w.Close()

	url := s.pagesAPIURL(accountID, "pages", "projects", projectName, "deployments")
	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return nil, err
	}
	for k, v := range s.getAuthHeaders() {
		// 覆盖 JSON 的 Content-Type
		if strings.ToLower(k) == "content-type" {
			continue
		}
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

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


