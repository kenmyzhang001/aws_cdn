package services

import (
	"aws_cdn/internal/config"
	"aws_cdn/internal/logger"
	"aws_cdn/internal/models"
	cf "aws_cdn/internal/services/cloudflare"
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"
)

type CFWorkpageSiteService struct {
	db               *gorm.DB
	cfAccountService *CFAccountService
	templateService  *CFWorkpageTemplateService
}

func NewCFWorkpageSiteService(db *gorm.DB, cfAccountService *CFAccountService, templateService *CFWorkpageTemplateService) *CFWorkpageSiteService {
	return &CFWorkpageSiteService{db: db, cfAccountService: cfAccountService, templateService: templateService}
}

// List 列表，支持 cf_account_id、template_id 筛选与分页
func (s *CFWorkpageSiteService) List(cfAccountID, templateID *uint, page, pageSize int) ([]models.CFWorkpageSite, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	var list []models.CFWorkpageSite
	var total int64
	query := s.db.Model(&models.CFWorkpageSite{})
	if cfAccountID != nil {
		query = query.Where("cf_account_id = ?", *cfAccountID)
	}
	if templateID != nil {
		query = query.Where("template_id = ?", *templateID)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("查询总数失败: %w", err)
	}
	offset := (page - 1) * pageSize
	if err := query.Preload("CFAccount").Preload("Template").Order("id DESC").Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("查询列表失败: %w", err)
	}
	return list, total, nil
}

// Get 获取单条
func (s *CFWorkpageSiteService) Get(id uint) (*models.CFWorkpageSite, error) {
	var site models.CFWorkpageSite
	if err := s.db.Preload("CFAccount").Preload("Template").First(&site, id).Error; err != nil {
		return nil, fmt.Errorf("站点不存在: %w", err)
	}
	return &site, nil
}

// Create 创建（仅落库，实际部署 WorkPage 可后续对接 CF API）
func (s *CFWorkpageSiteService) Create(cfAccountID, templateID uint, zoneID, mainDomain, subdomain string) (*models.CFWorkpageSite, error) {
	mainDomain = strings.TrimSpace(strings.ToLower(mainDomain))
	subdomain = strings.TrimSpace(strings.ToLower(subdomain))
	customDomain := mainDomain
	if subdomain != "" {
		// 兼容用户直接填写完整域名（如 www.example.com）
		if strings.Contains(subdomain, ".") {
			customDomain = subdomain
		} else {
			customDomain = subdomain + "." + mainDomain
		}
	}
	site := &models.CFWorkpageSite{
		CFAccountID: cfAccountID,
		TemplateID:  templateID,
		ZoneID:      zoneID,
		MainDomain:  mainDomain,
		Subdomain:   subdomain,
		Status:      "pending",
		CustomDomain: customDomain,
	}
	if err := s.db.Create(site).Error; err != nil {
		return nil, fmt.Errorf("创建站点失败: %w", err)
	}
	return s.Get(site.ID)
}

// Update 更新（仅允许改子域名等，主域名/zone 一般不变）
func (s *CFWorkpageSiteService) Update(id uint, subdomain *string) (*models.CFWorkpageSite, error) {
	site, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if subdomain != nil {
		site.Subdomain = strings.TrimSpace(strings.ToLower(*subdomain))
	}
	// 同步更新 custom_domain
	customDomain := strings.TrimSpace(strings.ToLower(site.MainDomain))
	if site.Subdomain != "" {
		if strings.Contains(site.Subdomain, ".") {
			customDomain = site.Subdomain
		} else {
			customDomain = site.Subdomain + "." + customDomain
		}
	}
	site.CustomDomain = customDomain

	if err := s.db.Model(site).Select("subdomain", "custom_domain").Updates(map[string]any{
		"subdomain":     site.Subdomain,
		"custom_domain": site.CustomDomain,
	}).Error; err != nil {
		return nil, fmt.Errorf("更新站点失败: %w", err)
	}
	return s.Get(id)
}

// Delete 删除
func (s *CFWorkpageSiteService) Delete(id uint) error {
	site, err := s.Get(id)
	if err != nil {
		return err
	}
	// 先尝试删除 Cloudflare Pages 项目（若存在）
	if site.PagesProjectName != "" {
		account, aErr := s.cfAccountService.GetCFAccount(site.CFAccountID)
		if aErr == nil && account.AccountID != "" {
			apiToken := s.cfAccountService.GetAPIToken(account)
			if apiToken != "" {
				if cfSvc, cErr := cf.NewCloudflareService(&config.CloudflareConfig{APIToken: apiToken}); cErr == nil {
					_ = cfSvc.DeletePagesProject(account.AccountID, site.PagesProjectName)
				}
			}
		}
	}
	return s.db.Delete(&models.CFWorkpageSite{}, id).Error
}

func (s *CFWorkpageSiteService) Deploy(id uint) (*models.CFWorkpageSite, error) {
	log := logger.GetLogger()
	site, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	// 标记 deploying
	_ = s.db.Model(&models.CFWorkpageSite{}).Where("id = ?", id).Updates(map[string]any{
		"status":     "deploying",
		"last_error": "",
	}).Error

	account, err := s.cfAccountService.GetCFAccount(site.CFAccountID)
	if err != nil {
		_ = s.db.Model(&models.CFWorkpageSite{}).Where("id = ?", id).Updates(map[string]any{
			"status":     "failed",
			"last_error": err.Error(),
		}).Error
		return nil, err
	}
	apiToken := s.cfAccountService.GetAPIToken(account)
	if apiToken == "" {
		err := fmt.Errorf("该 CF 账号未配置 API Token（需要 Pages Write 权限）")
		_ = s.db.Model(&models.CFWorkpageSite{}).Where("id = ?", id).Updates(map[string]any{
			"status":     "failed",
			"last_error": err.Error(),
		}).Error
		return nil, err
	}
	if account.AccountID == "" {
		err := fmt.Errorf("该 CF 账号未配置 AccountID")
		_ = s.db.Model(&models.CFWorkpageSite{}).Where("id = ?", id).Updates(map[string]any{
			"status":     "failed",
			"last_error": err.Error(),
		}).Error
		return nil, err
	}

	cfSvc, err := cf.NewCloudflareService(&config.CloudflareConfig{APIToken: apiToken})
	if err != nil {
		_ = s.db.Model(&models.CFWorkpageSite{}).Where("id = ?", id).Updates(map[string]any{
			"status":     "failed",
			"last_error": err.Error(),
		}).Error
		return nil, err
	}

	projectName := site.PagesProjectName
	if projectName == "" {
		projectName = fmt.Sprintf("wp-site-%d", site.ID)
	}
	// 确保 project 存在
	if _, err := cfSvc.GetPagesProject(account.AccountID, projectName); err != nil {
		if err.Error() == "project_not_found" {
			if _, cErr := cfSvc.CreatePagesProject(account.AccountID, projectName, "main"); cErr != nil {
				_ = s.db.Model(&models.CFWorkpageSite{}).Where("id = ?", id).Updates(map[string]any{
					"status":     "failed",
					"last_error": cErr.Error(),
				}).Error
				return nil, cErr
			}
		} else {
			_ = s.db.Model(&models.CFWorkpageSite{}).Where("id = ?", id).Updates(map[string]any{
				"status":     "failed",
				"last_error": err.Error(),
			}).Error
			return nil, err
		}
	}

	rows, _ := s.templateService.ListRows(site.TemplateID)
	htmlBytes := []byte(renderWorkpageHTML(site, rows))
	deploy, err := cfSvc.CreatePagesDeployment(account.AccountID, projectName, "main", "Deploy CF-WorkPage site", ".", map[string][]byte{
		"index.html": htmlBytes,
	})
	if err != nil {
		_ = s.db.Model(&models.CFWorkpageSite{}).Where("id = ?", id).Updates(map[string]any{
			"status":     "failed",
			"last_error": err.Error(),
		}).Error
		return nil, err
	}

	// 绑定自定义域名（主域名或子域名），并在同 Zone 下自动创建 CNAME 到 Pages
	customDomain := strings.TrimSpace(site.CustomDomain)
	if customDomain == "" {
		customDomain = strings.TrimSpace(site.MainDomain)
		if site.Subdomain != "" {
			if strings.Contains(site.Subdomain, ".") {
				customDomain = strings.TrimSpace(site.Subdomain)
			} else {
				customDomain = strings.TrimSpace(site.Subdomain) + "." + customDomain
			}
		}
	}
	var domainBindErr error
	if customDomain != "" {
		if _, dErr := cfSvc.AddPagesDomain(account.AccountID, projectName, customDomain); dErr != nil {
			// 域名绑定失败不阻塞 pages.dev 可访问，但记录错误方便排查
			domainBindErr = dErr
			log.WithError(dErr).WithFields(map[string]any{
				"site_id":      site.ID,
				"project_name": projectName,
				"domain":       customDomain,
			}).Warn("绑定 Pages 自定义域名失败")
		} else {
			// 绑定成功后，尝试在同 Zone 内自动创建 CNAME 记录指向 Pages 域名
			if deploy.URL != "" {
				if u, parseErr := url.Parse(deploy.URL); parseErr == nil && u.Host != "" {
					if zoneName, znErr := cfSvc.GetZoneByID(site.ZoneID); znErr == nil {
						zoneName = strings.TrimSuffix(strings.ToLower(zoneName), ".")
						cd := strings.TrimSuffix(strings.ToLower(customDomain), ".")
						host := ""
						if cd == zoneName {
							host = zoneName
						} else if strings.HasSuffix(cd, "."+zoneName) {
							host = cd
						}
						target := strings.TrimSuffix(strings.ToLower(u.Host), ".")
						if host != "" && target != "" {
							if err := cfSvc.CreateCNAMERecord(site.ZoneID, host, target, true); err != nil {
								log.WithError(err).WithFields(map[string]any{
									"site_id": site.ID,
									"zone_id": site.ZoneID,
									"name":    host,
									"value":   target,
								}).Warn("为 Pages 域名创建 CNAME 记录失败")
							}
						}
					}
				}
			}
		}
	}

	deployedAt := time.Now()
	deployURL := ""
	if len(deploy.Aliases) > 0 {
		deployURL = deploy.Aliases[0]
	}
	if deploy.URL != "" {
		deployURL = deploy.URL
	}
	lastErr := ""
	if domainBindErr != nil {
		lastErr = domainBindErr.Error()
	}
	if err := s.db.Model(&models.CFWorkpageSite{}).Where("id = ?", id).Updates(map[string]any{
		"status":               "deployed",
		"pages_project_name":   projectName,
		"deployment_id":        deploy.ID,
		"deployment_url":       deployURL,
		"custom_domain":        customDomain,
		"last_error":           lastErr,
		"deployed_at":          &deployedAt,
		"deployed_index_html":  string(htmlBytes),
	}).Error; err != nil {
		return nil, err
	}
	return s.Get(id)
}

// GetDeployedIndexHTML 返回最近一次成功部署时保存的 index.html 原文
func (s *CFWorkpageSiteService) GetDeployedIndexHTML(id uint) (string, error) {
	var site models.CFWorkpageSite
	if err := s.db.Select("id", "deployed_index_html").First(&site, id).Error; err != nil {
		return "", fmt.Errorf("站点不存在: %w", err)
	}
	return site.DeployedIndexHTML, nil
}

// PreviewHTML 生成部署前预览 HTML（不上传到 Pages）
func (s *CFWorkpageSiteService) PreviewHTML(id uint) (string, error) {
	site, err := s.Get(id)
	if err != nil {
		return "", err
	}
	rows, _ := s.templateService.ListRows(site.TemplateID)
	return renderWorkpageHTML(site, rows), nil
}

func renderWorkpageHTML(site *models.CFWorkpageSite, rows []models.CFWorkpageTemplateRow) string {
	lang := "zh"
	if site.Template != nil && (site.Template.DefaultLang == "my" || site.Template.DefaultLang == "zh") {
		lang = site.Template.DefaultLang
	}
	autoURL := ""
	for _, r := range rows {
		if r.AutoPopup && strings.TrimSpace(r.DownloadURL) != "" {
			autoURL = strings.TrimSpace(r.DownloadURL)
			break
		}
	}

	var b strings.Builder
	b.WriteString("<!doctype html><html><head><meta charset=\"utf-8\" />")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\" />")
	b.WriteString("<title>Download</title>")
	b.WriteString("<style>body{font-family:system-ui,-apple-system,Segoe UI,Roboto,Arial; margin:0; padding:24px; background:#f7f7fb;} .wrap{max-width:920px;margin:0 auto;} table{width:100%; border-collapse:collapse; background:#fff;} td{border:1px solid #e5e7eb; padding:16px; text-align:center; font-size:20px;} a.btn{display:inline-block; padding:10px 18px; background:#2563eb; color:#fff; border-radius:10px; text-decoration:none; font-weight:600;} a.btn:hover{background:#1d4ed8;} .hint{color:#6b7280; font-size:12px; margin-top:10px;}</style>")
	b.WriteString("</head><body><div class=\"wrap\">")
	b.WriteString("<table>")
	for _, r := range rows {
		c1 := r.Col1Zh
		c2 := r.Col2Zh
		c3 := r.Col3Zh
		if lang == "my" {
			if r.Col1My != "" {
				c1 = r.Col1My
			}
			if r.Col2My != "" {
				c2 = r.Col2My
			}
			if r.Col3My != "" {
				c3 = r.Col3My
			}
		}
		if c3 == "" {
			c3 = "立即下载"
		}
		b.WriteString("<tr>")
		b.WriteString("<td>" + html.EscapeString(c1) + "</td>")
		b.WriteString("<td>" + html.EscapeString(c2) + "</td>")
		b.WriteString("<td>")
		if strings.TrimSpace(r.DownloadURL) != "" {
			b.WriteString("<a class=\"btn\" href=\"" + html.EscapeString(strings.TrimSpace(r.DownloadURL)) + "\">" + html.EscapeString(c3) + "</a>")
		} else {
			b.WriteString("<span style=\"color:#9ca3af\">-</span>")
		}
		b.WriteString("</td>")
		b.WriteString("</tr>")
	}
	b.WriteString("</table>")
	b.WriteString("<div class=\"hint\">")
	b.WriteString("Site: " + html.EscapeString(site.MainDomain))
	b.WriteString("</div>")

	if autoURL != "" {
		// 使用 location 跳转避免浏览器弹窗拦截；保持表格仍可手动点击。
		b.WriteString("<script>setTimeout(function(){try{window.location.href=")
		b.WriteString("'" + strings.ReplaceAll(autoURL, "'", "\\'") + "'")
		b.WriteString(";}catch(e){}}\n,800);</script>")
	}
	b.WriteString("</div></body></html>")
	return b.String()
}
