package handlers

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/services"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

type AllLinksHandler struct {
	downloadPackageService    *services.DownloadPackageService
	customDownloadLinkService *services.CustomDownloadLinkService
	r2CustomDomainService     *services.R2CustomDomainService
	r2FileService             *services.R2FileService
}

func NewAllLinksHandler(
	downloadPackageService *services.DownloadPackageService,
	customDownloadLinkService *services.CustomDownloadLinkService,
	r2CustomDomainService *services.R2CustomDomainService,
	r2FileService *services.R2FileService,
) *AllLinksHandler {
	return &AllLinksHandler{
		downloadPackageService:    downloadPackageService,
		customDownloadLinkService: customDownloadLinkService,
		r2CustomDomainService:     r2CustomDomainService,
		r2FileService:             r2FileService,
	}
}

// LinkItem 统一的链接项结构
type LinkItem struct {
	ID                   uint   `json:"id"`
	URL                  string `json:"url"`
	Name                 string `json:"name"`
	Description          string `json:"description"`
	Type                 string `json:"type"` // download_package, custom_download_link, r2_apk_file
	Status               string `json:"status,omitempty"`
	FilePath             string `json:"file_path,omitempty"` // R2文件路径
	Domain               string `json:"domain,omitempty"`    // R2域名
	GroupID              *uint  `json:"group_id,omitempty"`
	GroupName            string `json:"group_name,omitempty"`
	ProbeEnabled         bool   `json:"probe_enabled"`
	ProbeIntervalMinutes int    `json:"probe_interval_minutes"`
	CreatedAt            string `json:"created_at"`
}

// AllLinksResponse 所有链接的响应结构
type AllLinksResponse struct {
	Links []LinkItem `json:"links"`
	Total int        `json:"total"`
}

// GetAllLinks 获取所有类型的链接
func (h *AllLinksHandler) GetAllLinks(c *gin.Context) {
	log := logger.GetLogger()

	var response AllLinksResponse
	response.Links = []LinkItem{}

	// 1. 获取所有下载包
	downloadPackages, err := h.downloadPackageService.ListAllDownloadPackages()
	if err != nil {
		log.WithError(err).Error("获取下载包列表失败")
	} else {
		for _, pkg := range downloadPackages {
			// 过滤掉没有 .apk 结尾的链接
			if !strings.HasSuffix(strings.ToLower(pkg.DownloadURL), ".apk") {
				continue
			}

			// 获取分组信息
			probeEnabled := true // 默认启用探测
			probeInterval := 60  // 默认60分钟
			var groupName string
			if pkg.Group != nil {
				groupName = pkg.Group.Name
				probeEnabled = pkg.Group.ProbeEnabled
				probeInterval = pkg.Group.ProbeIntervalMinutes
			}

			item := LinkItem{
				ID:                   pkg.ID,
				URL:                  pkg.DownloadURL,
				Name:                 pkg.FileName,
				Description:          pkg.Note,
				Type:                 "download_package",
				Status:               string(pkg.Status),
				GroupID:              pkg.GroupID,
				GroupName:            groupName,
				ProbeEnabled:         probeEnabled,
				ProbeIntervalMinutes: probeInterval,
				CreatedAt:            pkg.CreatedAt.Format("2006-01-02 15:04:05"),
			}
			response.Links = append(response.Links, item)
		}
	}

	// 2. 获取所有自定义下载链接
	customLinks, err := h.customDownloadLinkService.ListAllCustomDownloadLinks()
	if err != nil {
		log.WithError(err).Error("获取自定义下载链接列表失败")
	} else {
		for _, link := range customLinks {
			// 获取分组信息
			probeEnabled := true // 默认启用探测
			probeInterval := 60  // 默认60分钟
			var groupName string
			if link.Group != nil {
				groupName = link.Group.Name
				probeEnabled = link.Group.ProbeEnabled
				probeInterval = link.Group.ProbeIntervalMinutes
			}

			// 过滤掉没有 .apk 结尾的链接
			if !strings.HasSuffix(strings.ToLower(link.URL), ".apk") {
				if !strings.HasSuffix(strings.ToLower(link.ActualURL), ".apk") {
					continue
				} else {
					item := LinkItem{
						ID:                   link.ID,
						URL:                  link.ActualURL,
						Name:                 link.Name,
						Description:          link.Description,
						Type:                 "custom_download_link",
						Status:               string(link.Status),
						GroupID:              link.GroupID,
						GroupName:            groupName,
						ProbeEnabled:         probeEnabled,
						ProbeIntervalMinutes: probeInterval,
						CreatedAt:            link.CreatedAt.Format("2006-01-02 15:04:05"),
					}
					response.Links = append(response.Links, item)
				}
			}
			item := LinkItem{
				ID:                   link.ID,
				URL:                  link.URL,
				Name:                 link.Name,
				Description:          link.Description,
				Type:                 "custom_download_link",
				Status:               string(link.Status),
				GroupID:              link.GroupID,
				GroupName:            groupName,
				ProbeEnabled:         probeEnabled,
				ProbeIntervalMinutes: probeInterval,
				CreatedAt:            link.CreatedAt.Format("2006-01-02 15:04:05"),
			}
			response.Links = append(response.Links, item)
		}
	}

	// 3. 获取所有 R2 APK 文件
	r2APKFiles, err := h.r2FileService.ListAllAPKFileRecords()
	if err != nil {
		log.WithError(err).Error("获取R2 APK文件列表失败")
	} else {
		// 为每个 APK 文件获取对应的自定义域名并生成完整URL
		for _, file := range r2APKFiles {
			// 获取该存储桶的自定义域名列表
			domains, err := h.r2CustomDomainService.ListR2CustomDomains(file.R2BucketID)
			if err != nil {
				log.WithError(err).Errorf("获取存储桶 %d 的自定义域名失败", file.R2BucketID)
				continue
			}

			// 如果没有自定义域名，跳过该文件
			if len(domains) == 0 {
				continue
			}

			// 为每个域名生成一个链接
			for _, domain := range domains {
				// 只处理 active 状态的域名
				if domain.Status != "active" {
					continue
				}

				// 构建完整 URL
				// 文件路径需要正确编码：路径分隔符 / 保持不变，其他特殊字符需要编码
				pathParts := strings.Split(file.FilePath, "/")
				encodedParts := make([]string, len(pathParts))
				for i, part := range pathParts {
					encodedParts[i] = url.PathEscape(part)
				}
				encodedPath := strings.Join(encodedParts, "/")
				fullURL := "https://" + domain.Domain + "/" + encodedPath

				// R2文件没有分组信息，使用默认值
				item := LinkItem{
					ID:                   file.ID,
					URL:                  fullURL,
					Name:                 file.FileName,
					Description:          file.Note,
					Type:                 "r2_apk_file",
					Status:               file.Status,
					FilePath:             file.FilePath,
					Domain:               domain.Domain,
					ProbeEnabled:         true, // R2文件默认启用探测
					ProbeIntervalMinutes: 60,   // R2文件默认10分钟间隔
					CreatedAt:            file.CreatedAt.Format("2006-01-02 15:04:05"),
				}
				response.Links = append(response.Links, item)
			}
		}
	}

	// 计算总数
	response.Total = len(response.Links)

	c.JSON(http.StatusOK, response)
}
