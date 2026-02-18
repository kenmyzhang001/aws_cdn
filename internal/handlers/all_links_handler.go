package handlers

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/services"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

type AllLinksHandler struct {
	downloadPackageService    *services.DownloadPackageService
	customDownloadLinkService *services.CustomDownloadLinkService
	r2CustomDomainService     *services.R2CustomDomainService
	r2FileService             *services.R2FileService
	focusProbeLinkService     *services.FocusProbeLinkService
	speedProbeService         *services.SpeedProbeService
	redirectService           *services.RedirectService
}

func NewAllLinksHandler(
	downloadPackageService *services.DownloadPackageService,
	customDownloadLinkService *services.CustomDownloadLinkService,
	r2CustomDomainService *services.R2CustomDomainService,
	r2FileService *services.R2FileService,
	focusProbeLinkService *services.FocusProbeLinkService,
	speedProbeService *services.SpeedProbeService,
	redirectService *services.RedirectService,
) *AllLinksHandler {
	return &AllLinksHandler{
		downloadPackageService:    downloadPackageService,
		customDownloadLinkService: customDownloadLinkService,
		r2CustomDomainService:     r2CustomDomainService,
		r2FileService:             r2FileService,
		focusProbeLinkService:     focusProbeLinkService,
		speedProbeService:         speedProbeService,
		redirectService:           redirectService,
	}
}

// LinkItem 统一的链接项结构
type LinkItem struct {
	ID          uint   `json:"id"`
	URL         string `json:"url"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"` // download_package, custom_download_link, r2_apk_file, redirect_rule
	Status      string `json:"status,omitempty"`
	FilePath    string `json:"file_path,omitempty"` // R2文件路径
	Domain      string `json:"domain,omitempty"`    // R2域名
	CreatedAt   string `json:"created_at"`
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

	if c.Query("debug") == "true" {
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

				item := LinkItem{
					ID:          pkg.ID,
					URL:         pkg.DownloadURL,
					Name:        pkg.FileName,
					Description: pkg.Note,
					Type:        "download_package",
					Status:      string(pkg.Status),
					CreatedAt:   pkg.CreatedAt.Format("2006-01-02 15:04:05"),
				}
				response.Links = append(response.Links, item)
			}
		}
	}

	// 2. 获取所有自定义下载链接
	customLinks, err := h.customDownloadLinkService.ListAllCustomDownloadLinks()
	if err != nil {
		log.WithError(err).Error("获取自定义下载链接列表失败")
	} else {
		for _, link := range customLinks {

			// 过滤掉没有 .apk 结尾的链接
			if !strings.HasSuffix(strings.ToLower(link.URL), ".apk") {
				if !strings.HasSuffix(strings.ToLower(link.ActualURL), ".apk") {
					continue
				} else {
					item := LinkItem{
						ID:          link.ID,
						URL:         link.ActualURL,
						Name:        link.Name,
						Description: link.Description,
						Type:        "custom_download_link",
						Status:      string(link.Status),
						CreatedAt:   link.CreatedAt.Format("2006-01-02 15:04:05"),
					}
					response.Links = append(response.Links, item)
				}
			}
			item := LinkItem{
				ID:          link.ID,
				URL:         link.URL,
				Name:        link.Name,
				Description: link.Description,
				Type:        "custom_download_link",
				Status:      string(link.Status),
				CreatedAt:   link.CreatedAt.Format("2006-01-02 15:04:05"),
			}
			response.Links = append(response.Links, item)
		}
	}

	// 3. 获取所有重定向规则的 source_domain
	redirectRules, _, err := h.redirectService.ListRedirectRules(1, 10000, nil, nil)
	if err != nil {
		log.WithError(err).Error("获取重定向规则列表失败")
	} else {
		for _, rule := range redirectRules {
			// 用 https://source_domain 作为 URL，便于去重和探测
			item := LinkItem{
				ID:          rule.ID,
				URL:         "https://" + rule.SourceDomain,
				Name:        rule.SourceDomain,
				Description: rule.Note,
				Type:        "redirect_rule",
				Status:      string(rule.Status),
				Domain:      rule.SourceDomain,
				CreatedAt:   rule.CreatedAt.Format("2006-01-02 15:04:05"),
			}
			log.WithFields(map[string]interface{}{
				"url":         item.URL,
				"name":        item.Name,
				"description": item.Description,
				"type":        item.Type,
				"status":      item.Status,
				"domain":      item.Domain,
				"created_at":  item.CreatedAt,
			}).Info("重定向规则")
			response.Links = append(response.Links, item)
		}
	}

	// 4. 获取所有 R2 APK 文件
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
					ID:          file.ID,
					URL:         fullURL,
					Name:        file.FileName,
					Description: file.Note,
					Type:        "r2_apk_file",
					Status:      file.Status,
					FilePath:    file.FilePath,
					Domain:      domain.Domain,
					CreatedAt:   file.CreatedAt.Format("2006-01-02 15:04:05"),
				}
				response.Links = append(response.Links, item)
			}
		}
	}

	if c.Query("debug") == "true" {
		c.JSON(http.StatusOK, response)
		return
	}

	// 根据 URL 去重，只保留第一个出现的
	uniqueLinks := make([]LinkItem, 0, len(response.Links))
	seenURLs := make(map[string]bool)

	for _, link := range response.Links {
		if !seenURLs[link.URL] {
			seenURLs[link.URL] = true
			uniqueLinks = append(uniqueLinks, link)
		}
	}

	response.Links = uniqueLinks

	// 过滤掉在探测间隔内已有探测记录的链接（并发处理，最多50并发）
	type filterResult struct {
		link     LinkItem
		included bool // 是否应该包含在结果中
	}

	resultChan := make(chan filterResult, len(response.Links))
	var wg sync.WaitGroup

	// 创建信号量 channel 来控制并发数（最多50个）
	const maxConcurrency = 50
	semaphore := make(chan struct{}, maxConcurrency)

	// 并发处理每个链接
	for _, link := range response.Links {
		wg.Add(1)
		go func(l LinkItem) {
			defer wg.Done()

			// 获取信号量（限制并发数）
			semaphore <- struct{}{}
			defer func() { <-semaphore }() // 释放信号量

			// 获取该链接的探测间隔
			intervalMinutes, err := h.focusProbeLinkService.GetProbeIntervalForURL(l.URL)
			if err != nil {
				log.WithError(err).WithField("url", l.URL).Warn("获取探测间隔失败，使用默认30分钟")
				intervalMinutes = 30
			}

			// 检查是否在指定时间间隔内有探测记录
			hasRecent, err := h.speedProbeService.HasRecentProbeResult(l.URL, intervalMinutes, c.Request.UserAgent())
			if err != nil {
				log.WithError(err).WithField("url", l.URL).Warn("检查探测记录失败，保留该链接")
				resultChan <- filterResult{link: l, included: true}
				return
			}

			// 如果在时间间隔内没有探测记录，才返回该链接
			if !hasRecent {
				resultChan <- filterResult{link: l, included: true}
			} else {
				log.WithFields(map[string]interface{}{
					"url":              l.URL,
					"interval_minutes": intervalMinutes,
				}).Info("链接在探测间隔内已有探测记录，已过滤")
				resultChan <- filterResult{link: l, included: false}
			}
		}(link)
	}

	// 等待所有 goroutine 完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 收集结果
	filteredLinks := make([]LinkItem, 0, len(response.Links))
	for result := range resultChan {
		if result.included {
			filteredLinks = append(filteredLinks, result.link)
		}
	}

	response.Links = filteredLinks

	// 计算总数
	response.Total = len(response.Links)

	log.WithFields(map[string]interface{}{
		"total":         response.Total,
		"before_filter": len(uniqueLinks),
		"filtered_out":  len(uniqueLinks) - response.Total,
	}).Info("获取所有链接成功")

	c.JSON(http.StatusOK, response)
}
