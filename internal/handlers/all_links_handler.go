package handlers

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

type AllLinksHandler struct {
	downloadPackageService    *services.DownloadPackageService
	customDownloadLinkService *services.CustomDownloadLinkService
	r2CustomDomainService     *services.R2CustomDomainService
}

func NewAllLinksHandler(
	downloadPackageService *services.DownloadPackageService,
	customDownloadLinkService *services.CustomDownloadLinkService,
	r2CustomDomainService *services.R2CustomDomainService,
) *AllLinksHandler {
	return &AllLinksHandler{
		downloadPackageService:    downloadPackageService,
		customDownloadLinkService: customDownloadLinkService,
		r2CustomDomainService:     r2CustomDomainService,
	}
}

// LinkItem 统一的链接项结构
type LinkItem struct {
	ID          uint   `json:"id"`
	URL         string `json:"url"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"` // download_package, custom_download_link, r2_custom_domain
	Status      string `json:"status,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// AllLinksResponse 所有链接的响应结构
type AllLinksResponse struct {
	DownloadPackages    []LinkItem `json:"download_packages"`
	CustomDownloadLinks []LinkItem `json:"custom_download_links"`
	R2CustomDomains     []LinkItem `json:"r2_custom_domains"`
	Total               int        `json:"total"`
}

// GetAllLinks 获取所有类型的链接
func (h *AllLinksHandler) GetAllLinks(c *gin.Context) {
	log := logger.GetLogger()

	var response AllLinksResponse
	response.DownloadPackages = []LinkItem{}
	response.CustomDownloadLinks = []LinkItem{}
	response.R2CustomDomains = []LinkItem{}

	// 1. 获取所有下载包
	downloadPackages, err := h.downloadPackageService.ListAllDownloadPackages()
	if err != nil {
		log.WithError(err).Error("获取下载包列表失败")
	} else {
		for _, pkg := range downloadPackages {
			item := LinkItem{
				ID:          pkg.ID,
				URL:         pkg.DownloadURL,
				Name:        pkg.FileName,
				Description: pkg.Note,
				Type:        "download_package",
				Status:      string(pkg.Status),
				CreatedAt:   pkg.CreatedAt.Format("2006-01-02 15:04:05"),
			}
			response.DownloadPackages = append(response.DownloadPackages, item)
		}
	}

	// 2. 获取所有自定义下载链接
	customLinks, err := h.customDownloadLinkService.ListAllCustomDownloadLinks()
	if err != nil {
		log.WithError(err).Error("获取自定义下载链接列表失败")
	} else {
		for _, link := range customLinks {
			item := LinkItem{
				ID:          link.ID,
				URL:         link.URL,
				Name:        link.Name,
				Description: link.Description,
				Type:        "custom_download_link",
				Status:      string(link.Status),
				CreatedAt:   link.CreatedAt.Format("2006-01-02 15:04:05"),
			}
			response.CustomDownloadLinks = append(response.CustomDownloadLinks, item)
		}
	}

	// 3. 获取所有 R2 自定义域名
	r2Domains, err := h.r2CustomDomainService.ListAllR2CustomDomains()
	if err != nil {
		log.WithError(err).Error("获取R2自定义域名列表失败")
	} else {
		for _, domain := range r2Domains {
			// 构建完整的域名 URL
			url := "https://" + domain.Domain

			item := LinkItem{
				ID:          domain.ID,
				URL:         url,
				Name:        domain.Domain,
				Description: domain.Note,
				Type:        "r2_custom_domain",
				Status:      domain.Status,
				CreatedAt:   domain.CreatedAt.Format("2006-01-02 15:04:05"),
			}
			response.R2CustomDomains = append(response.R2CustomDomains, item)
		}
	}

	// 计算总数
	response.Total = len(response.DownloadPackages) + len(response.CustomDownloadLinks) + len(response.R2CustomDomains)

	c.JSON(http.StatusOK, response)
}
