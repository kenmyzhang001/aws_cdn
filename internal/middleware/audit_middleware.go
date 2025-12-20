package middleware

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/services"
	"bytes"
	"io"
	"time"

	"github.com/gin-gonic/gin"
)

// AuditMiddleware 审计日志中间件
func AuditMiddleware(auditService *services.AuditService) gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()
		log := logger.GetLogger()

		// 获取请求信息
		method := c.Request.Method
		path := c.Request.URL.Path
		ip := c.ClientIP()
		userAgent := c.Request.UserAgent()

		// 读取请求体（用于记录请求数据）
		var requestBody []byte
		if c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// 创建响应写入器包装器，用于捕获响应
		responseWriter := &responseBodyWriter{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
		}
		c.Writer = responseWriter

		// 处理请求
		c.Next()

		// 计算耗时
		duration := time.Since(startTime)

		// 获取用户信息
		userID, _ := c.Get("user_id")
		username, _ := c.Get("username")
		var userIDUint uint
		var usernameStr string
		if userID != nil {
			if id, ok := userID.(uint); ok {
				userIDUint = id
			}
		}
		if username != nil {
			if name, ok := username.(string); ok {
				usernameStr = name
			}
		}

		// 获取状态码
		status := c.Writer.Status()

		// 记录请求日志
		log.WithFields(map[string]interface{}{
			"method":      method,
			"path":        path,
			"ip":          ip,
			"user_agent":  userAgent,
			"user_id":     userIDUint,
			"username":    usernameStr,
			"status":      status,
			"duration_ms": duration.Milliseconds(),
		}).Info("HTTP请求")

		// 只对需要审计的操作记录审计日志（POST, PUT, DELETE）
		if method == "POST" || method == "PUT" || method == "DELETE" {
			// 确定资源类型和ID
			resource, resourceID := extractResourceInfo(path)

			// 确定操作类型
			action := extractAction(method, path)

			// 解析请求数据（限制大小）
			var requestData interface{}
			if len(requestBody) > 0 && len(requestBody) < 10000 {
				requestData = string(requestBody)
			}

			// 解析响应数据（限制大小）
			var responseData interface{}
			responseBody := responseWriter.body.Bytes()
			if len(responseBody) > 0 && len(responseBody) < 10000 {
				responseData = string(responseBody)
			}

			// 创建审计日志
			auditLog := auditService.CreateAuditLog(
				userIDUint,
				usernameStr,
				action,
				resource,
				resourceID,
				method,
				path,
				ip,
				userAgent,
				requestData,
				responseData,
				status,
				getActionMessage(action, resource, resourceID),
				nil,
				duration,
			)

			// 如果请求失败，记录错误信息
			if status >= 400 {
				auditLog.Error = "请求失败"
			}

			// 保存审计日志（异步，不阻塞请求）
			go func() {
				if err := auditService.LogAudit(auditLog); err != nil {
					log.WithError(err).Error("记录审计日志失败")
				}
			}()
		}
	}
}

// responseBodyWriter 响应体写入器包装器
type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *responseBodyWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// extractResourceInfo 从路径中提取资源类型和ID
func extractResourceInfo(path string) (resource, resourceID string) {
	// 示例路径: /api/v1/domains/123, /api/v1/redirects/456
	for _, part := range []string{"domains", "redirects", "cloudfront", "download-packages"} {
		if contains(path, "/"+part+"/") {
			resource = part
			// 尝试提取ID
			idx := indexOf(path, "/"+part+"/")
			if idx >= 0 {
				remaining := path[idx+len("/"+part+"/"):]
				// 提取ID（直到下一个斜杠或结束）
				for i, r := range remaining {
					if r == '/' || r == '?' {
						resourceID = remaining[:i]
						break
					}
					if i == len(remaining)-1 {
						resourceID = remaining
					}
				}
			}
			break
		}
	}
	return
}

// extractAction 从方法和路径中提取操作类型
func extractAction(method, path string) string {
	// 根据路径和方法确定操作类型
	if contains(path, "/certificate") {
		if method == "POST" {
			return "generate_certificate"
		}
		if contains(path, "/fix") {
			return "fix_certificate"
		}
		if contains(path, "/check") {
			return "check_certificate"
		}
	}
	if contains(path, "/bind-cloudfront") {
		return "bind_domain"
	}
	if contains(path, "/fix") {
		return "fix_resource"
	}
	if contains(path, "/check") {
		return "check_resource"
	}
	if contains(path, "/targets") {
		if method == "POST" {
			return "add_target"
		}
		if method == "DELETE" {
			return "remove_target"
		}
	}
	if contains(path, "/distributions") {
		if method == "POST" {
			return "create_distribution"
		}
		if method == "PUT" {
			return "update_distribution"
		}
		if method == "DELETE" {
			return "delete_distribution"
		}
	}
	switch method {
	case "POST":
		return "create"
	case "PUT":
		return "update"
	case "DELETE":
		return "delete"
	default:
		return "unknown"
	}
}

// getActionMessage 生成操作描述消息
func getActionMessage(action, resource, resourceID string) string {
	resourceMap := map[string]string{
		"domains":           "域名",
		"redirects":         "重定向规则",
		"cloudfront":        "CloudFront分发",
		"download-packages": "下载包",
	}
	resourceName := resourceMap[resource]
	if resourceName == "" {
		resourceName = resource
	}

	actionMap := map[string]string{
		"create":               "创建",
		"update":               "更新",
		"delete":               "删除",
		"generate_certificate": "生成证书",
		"fix_certificate":      "修复证书",
		"check_certificate":    "检查证书",
		"bind_domain":          "绑定域名",
		"add_target":           "添加目标",
		"remove_target":        "删除目标",
		"create_distribution":  "创建分发",
		"update_distribution":  "更新分发",
		"delete_distribution":  "删除分发",
		"fix_resource":         "修复资源",
		"check_resource":       "检查资源",
	}
	actionName := actionMap[action]
	if actionName == "" {
		actionName = action
	}

	if resourceID != "" {
		return actionName + " " + resourceName + " (ID: " + resourceID + ")"
	}
	return actionName + " " + resourceName
}

// 辅助函数
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
