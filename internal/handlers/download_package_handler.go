package handlers

import (
	"aws_cdn/internal/services"
	"net/http"
	"strconv"

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

	// 创建下载包（使用domainID，服务层会从域名获取domain_name）
	pkg, err := h.service.CreateDownloadPackage(uint(domainID), fileName, fileReader, fileSize)
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

// ListDownloadPackages 列出所有下载包
func (h *DownloadPackageHandler) ListDownloadPackages(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	packages, total, err := h.service.ListDownloadPackages(page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  packages,
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
