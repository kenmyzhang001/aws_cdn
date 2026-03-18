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

// CreatePagesDeployment 通过 Direct Upload 方式创建部署。
// files: key 为相对路径（如 index.html），value 为文件内容字节。
func (s *CloudflareService) CreatePagesDeployment(accountID, projectName, branch, commitMessage, pagesBuildOutputDir string, files map[string][]byte) (*PagesDeployment, error) {
	if pagesBuildOutputDir == "" {
		pagesBuildOutputDir = "."
	}
	manifest := map[string]string{}
	for name, content := range files {
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
	_ = w.WriteField("pages_build_output_dir", pagesBuildOutputDir)

	// Cloudflare Pages 部署 API 使用 manifest 的 hash 作为 file field name（与文档示例一致的 manifest 结构）。
	for name, content := range files {
		fieldName := manifest[name]
		fw, err := w.CreateFormFile(fieldName, name)
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


