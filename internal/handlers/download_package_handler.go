package handlers

import (
	"aws_cdn/internal/models"
	"aws_cdn/internal/services"
	"bytes"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type DownloadPackageHandler struct {
	service *services.DownloadPackageService
}

func NewDownloadPackageHandler(service *services.DownloadPackageService) *DownloadPackageHandler {
	return &DownloadPackageHandler{service: service}
}

// CreateDownloadPackage 创建下载包
func (h *DownloadPackageHandler) CreateDownloadPackage(c *gin.Context) {
	// 获取表单数据
	domainIDStr := c.PostForm("domain_id")
	fileName := c.PostForm("file_name")

	if domainIDStr == "" || fileName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "domain_id 和 file_name 是必需的"})
		return
	}

	domainID, err := strconv.ParseUint(domainIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 domain_id"})
		return
	}

	// 获取上传的文件
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件上传失败: " + err.Error()})
		return
	}

	// 打开文件
	fileReader, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "打开文件失败: " + err.Error()})
		return
	}
	defer fileReader.Close()

	// 获取文件大小
	fileSize := file.Size

	// 将文件内容读取到内存中，以便在异步goroutine中安全使用
	// 这样可以避免文件读取器在goroutine使用前被关闭
	fileData := make([]byte, fileSize)
	n, err := io.ReadFull(fileReader, fileData)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取文件内容失败: " + err.Error()})
		return
	}
	if int64(n) != fileSize {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "文件读取不完整"})
		return
	}

	// 使用bytes.Reader创建可重定位的读取器
	fileDataReader := bytes.NewReader(fileData)

	// 创建下载包（使用domainID，服务层会从域名获取domain_name）
	pkg, err := h.service.CreateDownloadPackage(uint(domainID), fileName, fileDataReader, fileSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, pkg)
}

// GetDownloadPackage 获取下载包信息
func (h *DownloadPackageHandler) GetDownloadPackage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的下载包 ID"})
		return
	}

	pkg, err := h.service.GetDownloadPackage(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, pkg)
}

// DownloadPackageResponse 下载包响应结构（包含CloudFront状态）
type DownloadPackageResponse struct {
	ID                           uint   `json:"id"`
	DomainID                     uint   `json:"domain_id"`
	DomainName                   string `json:"domain_name"`
	FileName                     string `json:"file_name"`
	FileSize                     int64  `json:"file_size"`
	FileType                     string `json:"file_type"`
	S3Key                        string `json:"s3_key"`
	CloudFrontID                 string `json:"cloudfront_id"`
	CloudFrontDomain             string `json:"cloudfront_domain"`
	CloudFrontStatus             string `json:"cloudfront_status"`               // CloudFront部署状态
	CloudFrontEnabled            bool   `json:"cloudfront_enabled"`              // CloudFront启用状态
	CloudFrontOriginPathCurrent  string `json:"cloudfront_origin_path_current"`  // CloudFront当前OriginPath
	CloudFrontOriginPathExpected string `json:"cloudfront_origin_path_expected"` // CloudFront期望OriginPath
	DownloadURL                  string `json:"download_url"`
	Status                       string `json:"status"`
	ErrorMessage                 string `json:"error_message"`
	CreatedAt                    string `json:"created_at"`
	UpdatedAt                    string `json:"updated_at"`
}

// ListDownloadPackagesByDomain 列出指定域名下的所有下载包
func (h *DownloadPackageHandler) ListDownloadPackagesByDomain(c *gin.Context) {
	domainIDStr := c.Query("domain_id")
	if domainIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "domain_id 参数是必需的"})
		return
	}

	domainID, err := strconv.ParseUint(domainIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 domain_id"})
		return
	}

	packages, err := h.service.ListDownloadPackagesByDomain(uint(domainID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 为每个下载包获取CloudFront状态
	responses := make([]DownloadPackageResponse, len(packages))
	for i, pkg := range packages {
		responses[i] = DownloadPackageResponse{
			ID:               pkg.ID,
			DomainID:         pkg.DomainID,
			DomainName:       pkg.DomainName,
			FileName:         pkg.FileName,
			FileSize:         pkg.FileSize,
			FileType:         pkg.FileType,
			S3Key:            pkg.S3Key,
			CloudFrontID:     pkg.CloudFrontID,
			CloudFrontDomain: pkg.CloudFrontDomain,
			DownloadURL:      pkg.DownloadURL,
			Status:           string(pkg.Status),
			ErrorMessage:     pkg.ErrorMessage,
			CreatedAt:        pkg.CreatedAt.Format(time.RFC3339),
			UpdatedAt:        pkg.UpdatedAt.Format(time.RFC3339),
		}

		// 获取CloudFront状态和启用状态
		if pkg.CloudFrontID != "" {
			status, err := h.service.GetCloudFrontStatus(pkg.CloudFrontID)
			if err != nil {
				responses[i].CloudFrontStatus = "unknown"
			} else {
				responses[i].CloudFrontStatus = status
			}

			enabled, err := h.service.GetCloudFrontEnabled(pkg.CloudFrontID)
			if err != nil {
				responses[i].CloudFrontEnabled = false
			} else {
				responses[i].CloudFrontEnabled = enabled
			}
		} else {
			responses[i].CloudFrontStatus = ""
			responses[i].CloudFrontEnabled = false
		}

		// 获取 CloudFront OriginPath 信息
		currentPath, expectedPath, err := h.service.GetCloudFrontOriginPathInfo(&pkg)
		if err == nil {
			responses[i].CloudFrontOriginPathCurrent = currentPath
			responses[i].CloudFrontOriginPathExpected = expectedPath
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data": responses,
	})
}

// ListDownloadPackages 列出所有下载包
func (h *DownloadPackageHandler) ListDownloadPackages(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	packages, total, err := h.service.ListDownloadPackages(page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 为每个下载包获取CloudFront状态（并发查询）
	responses := make([]DownloadPackageResponse, len(packages))
	var wg sync.WaitGroup

	for i, pkg := range packages {
		responses[i] = DownloadPackageResponse{
			ID:               pkg.ID,
			DomainID:         pkg.DomainID,
			DomainName:       pkg.DomainName,
			FileName:         pkg.FileName,
			FileSize:         pkg.FileSize,
			FileType:         pkg.FileType,
			S3Key:            pkg.S3Key,
			CloudFrontID:     pkg.CloudFrontID,
			CloudFrontDomain: pkg.CloudFrontDomain,
			DownloadURL:      pkg.DownloadURL,
			Status:           string(pkg.Status),
			ErrorMessage:     pkg.ErrorMessage,
			CreatedAt:        pkg.CreatedAt.Format(time.RFC3339),
			UpdatedAt:        pkg.UpdatedAt.Format(time.RFC3339),
		}

		// 初始化默认值
		if pkg.CloudFrontID == "" {
			responses[i].CloudFrontStatus = ""
			responses[i].CloudFrontEnabled = false
		}

		// 并发查询所有状态
		wg.Add(1)
		go func(idx int, pkgCopy models.DownloadPackage) {
			defer wg.Done()

			// 查询 CloudFront 状态和启用状态
			if pkgCopy.CloudFrontID != "" {
				var statusWg sync.WaitGroup

				// 并发查询状态和启用状态
				statusWg.Add(1)
				go func() {
					defer statusWg.Done()
					status, err := h.service.GetCloudFrontStatus(pkgCopy.CloudFrontID)
					if err != nil {
						responses[idx].CloudFrontStatus = "unknown"
					} else {
						responses[idx].CloudFrontStatus = status
					}
				}()

				statusWg.Add(1)
				go func() {
					defer statusWg.Done()
					enabled, err := h.service.GetCloudFrontEnabled(pkgCopy.CloudFrontID)
					if err != nil {
						responses[idx].CloudFrontEnabled = false
					} else {
						responses[idx].CloudFrontEnabled = enabled
					}
				}()

				statusWg.Wait()
			}

			// 获取 CloudFront OriginPath 信息
			currentPath, expectedPath, err := h.service.GetCloudFrontOriginPathInfo(&pkgCopy)
			if err == nil {
				responses[idx].CloudFrontOriginPathCurrent = currentPath
				responses[idx].CloudFrontOriginPathExpected = expectedPath
			}
		}(i, pkg)
	}

	wg.Wait()

	c.JSON(http.StatusOK, gin.H{
		"data":  responses,
		"total": total,
		"page":  page,
		"size":  pageSize,
	})
}

// DeleteDownloadPackage 删除下载包
func (h *DownloadPackageHandler) DeleteDownloadPackage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的下载包 ID"})
		return
	}

	if err := h.service.DeleteDownloadPackage(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "下载包删除成功"})
}

// CheckDownloadPackage 检查下载包状态
func (h *DownloadPackageHandler) CheckDownloadPackage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的下载包 ID"})
		return
	}

	status, err := h.service.CheckDownloadPackage(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, status)
}

// FixDownloadPackage 修复下载包
func (h *DownloadPackageHandler) FixDownloadPackage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的下载包 ID"})
		return
	}

	if err := h.service.FixDownloadPackage(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "修复成功"})
}
