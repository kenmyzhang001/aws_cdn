package services

import (
	"aws_cdn/internal/models"
	"fmt"
	"html"
	"strings"

	"gorm.io/gorm"
)

type CFWorkpageTemplateService struct {
	db *gorm.DB
}

func NewCFWorkpageTemplateService(db *gorm.DB) *CFWorkpageTemplateService {
	return &CFWorkpageTemplateService{db: db}
}

// List 列表，支持关键词筛选（name_zh/name_my）与分页
func (s *CFWorkpageTemplateService) List(keyword string, page, pageSize int) ([]models.CFWorkpageTemplate, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	var list []models.CFWorkpageTemplate
	var total int64
	query := s.db.Model(&models.CFWorkpageTemplate{})
	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("name_zh LIKE ? OR name_my LIKE ?", like, like)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("查询总数失败: %w", err)
	}
	offset := (page - 1) * pageSize
	if err := query.Order("id DESC").Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("查询列表失败: %w", err)
	}
	return list, total, nil
}

// Get 获取单条
func (s *CFWorkpageTemplateService) Get(id uint) (*models.CFWorkpageTemplate, error) {
	var t models.CFWorkpageTemplate
	if err := s.db.First(&t, id).Error; err != nil {
		return nil, fmt.Errorf("模版不存在: %w", err)
	}
	return &t, nil
}

// Create 创建
func (s *CFWorkpageTemplateService) Create(nameZh, nameMy, defaultLang string) (*models.CFWorkpageTemplate, error) {
	if defaultLang != "zh" && defaultLang != "my" {
		defaultLang = "zh"
	}
	t := &models.CFWorkpageTemplate{
		NameZh:      strings.TrimSpace(nameZh),
		NameMy:      strings.TrimSpace(nameMy),
		DefaultLang: defaultLang,
	}
	if err := s.db.Create(t).Error; err != nil {
		return nil, fmt.Errorf("创建模版失败: %w", err)
	}
	return t, nil
}

// Update 更新
func (s *CFWorkpageTemplateService) Update(id uint, nameZh, nameMy, defaultLang *string) (*models.CFWorkpageTemplate, error) {
	t, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if nameZh != nil {
		t.NameZh = strings.TrimSpace(*nameZh)
	}
	if nameMy != nil {
		t.NameMy = strings.TrimSpace(*nameMy)
	}
	if defaultLang != nil && (*defaultLang == "zh" || *defaultLang == "my") {
		t.DefaultLang = *defaultLang
	}
	if err := s.db.Save(t).Error; err != nil {
		return nil, fmt.Errorf("更新模版失败: %w", err)
	}
	return t, nil
}

// Delete 删除（会级联删除模版下的所有表格行）
func (s *CFWorkpageTemplateService) Delete(id uint) error {
	var count int64
	if err := s.db.Model(&models.CFWorkpageSite{}).Where("template_id = ?", id).Count(&count).Error; err != nil {
		return fmt.Errorf("检查关联站点失败: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("该模版已被 %d 个站点使用，请先解除关联再删除", count)
	}
	_ = s.db.Where("template_id = ?", id).Delete(&models.CFWorkpageTemplateRow{})
	return s.db.Delete(&models.CFWorkpageTemplate{}, id).Error
}

// ListRows 获取模版下所有表格行（按 sort_order 排序）
func (s *CFWorkpageTemplateService) ListRows(templateID uint) ([]models.CFWorkpageTemplateRow, error) {
	var rows []models.CFWorkpageTemplateRow
	if err := s.db.Where("template_id = ?", templateID).Order("sort_order ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("查询表格行失败: %w", err)
	}
	return rows, nil
}

// SaveRows 批量保存模版表格行：
// - 传入的行如果已存在（按 id 匹配）则执行更新
// - 传入的行如果不存在则插入
// - 不在传入列表中的旧行会被删除
// - 若有多行 auto_popup=true，仅保留第一行为 true，其余强制为 false
func (s *CFWorkpageTemplateService) SaveRows(templateID uint, rows []models.CFWorkpageTemplateRow) ([]models.CFWorkpageTemplateRow, error) {
	if _, err := s.Get(templateID); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		// 若前端传空数组，视为清空所有行
		if err := s.db.Where("template_id = ?", templateID).Delete(&models.CFWorkpageTemplateRow{}).Error; err != nil {
			return nil, fmt.Errorf("清空原表格行失败: %w", err)
		}
		return []models.CFWorkpageTemplateRow{}, nil
	}

	tx := s.db.Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("开启事务失败: %w", tx.Error)
	}

	// 1. 规范 auto_popup：只保留第一条为 true
	hasAuto := false
	for i := range rows {
		if rows[i].AutoPopup {
			if hasAuto {
				rows[i].AutoPopup = false
			} else {
				hasAuto = true
			}
		}
	}

	// 如果本次提交中有 auto_popup=true，先把库里该模板下所有行的 auto_popup 清零
	if hasAuto {
		if err := tx.Model(&models.CFWorkpageTemplateRow{}).
			Where("template_id = ?", templateID).
			Update("auto_popup", false).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("重置自动弹出标记失败: %w", err)
		}
	}

	// 2. 记录前端传来的行 id 集合，用于删除“已被移除的行”
	var keepIDs []uint
	for i := range rows {
		if rows[i].ID != 0 {
			keepIDs = append(keepIDs, rows[i].ID)
		}
	}

	// 删除该模板下但不在 keepIDs 列表中的旧行（即前端已经删除的行）
	if len(keepIDs) > 0 {
		if err := tx.Where("template_id = ? AND id NOT IN ?", templateID, keepIDs).
			Delete(&models.CFWorkpageTemplateRow{}).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("删除已移除的表格行失败: %w", err)
		}
	} else {
		// 所有旧行都被移除
		if err := tx.Where("template_id = ?", templateID).
			Delete(&models.CFWorkpageTemplateRow{}).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("清空原表格行失败: %w", err)
		}
	}

	// 3. 按当前顺序更新/插入各行，并写入 sort_order & template_id
	for i := range rows {
		rows[i].TemplateID = templateID
		rows[i].SortOrder = i

		if rows[i].ID == 0 {
			// 新增行
			if err := tx.Create(&rows[i]).Error; err != nil {
				tx.Rollback()
				return nil, fmt.Errorf("创建表格行失败: %w", err)
			}
			continue
		}

		// 已存在行：按 id + template_id 精确更新
		updateData := map[string]any{
			"sort_order":   rows[i].SortOrder,
			"col1_zh":      rows[i].Col1Zh,
			"col1_my":      rows[i].Col1My,
			"col2_zh":      rows[i].Col2Zh,
			"col2_my":      rows[i].Col2My,
			"col3_zh":      rows[i].Col3Zh,
			"col3_my":      rows[i].Col3My,
			"download_url": rows[i].DownloadURL,
			"auto_popup":   rows[i].AutoPopup,
		}
		if err := tx.Model(&models.CFWorkpageTemplateRow{}).
			Where("id = ? AND template_id = ?", rows[i].ID, templateID).
			Updates(updateData).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("更新表格行失败: %w", err)
		}
	}

	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("提交事务失败: %w", err)
	}

	return s.ListRows(templateID)
}

// PreviewHTML 生成模版预览 HTML（不依赖具体站点）
func (s *CFWorkpageTemplateService) PreviewHTML(id uint) (string, error) {
	tpl, err := s.Get(id)
	if err != nil {
		return "", err
	}
	rows, _ := s.ListRows(id)
	lang := "zh"
	if tpl.DefaultLang == "my" {
		lang = "my"
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
	b.WriteString("<title>CF-WorkPage 模版预览</title>")
	b.WriteString("<style>body{font-family:system-ui,-apple-system,Segoe UI,Roboto,Arial; margin:0; padding:clamp(12px,4vw,24px); background:#f7f7fb;} .wrap{max-width:920px;margin:0 auto; overflow-x:auto; -webkit-overflow-scrolling:touch;} h1{font-size:clamp(16px,4.2vw,20px);margin-bottom:clamp(10px,2.8vw,16px); white-space:nowrap;} table{width:max-content; min-width:100%; border-collapse:collapse; background:#fff;} td{border:1px solid #e5e7eb; padding:clamp(10px,2.6vw,16px); text-align:center; font-size:clamp(14px,3.8vw,20px); white-space:nowrap;} a.btn{display:inline-block; padding:clamp(8px,2.4vw,10px) clamp(12px,3.2vw,18px); background:#2563eb; color:#fff; border-radius:10px; text-decoration:none; font-weight:600; white-space:nowrap;} a.btn:hover{background:#1d4ed8;} .hint{color:#6b7280; font-size:12px; margin-top:10px; white-space:nowrap;}</style>")
	b.WriteString("</head><body><div class=\"wrap\">")
	title := tpl.NameZh
	if lang == "my" && tpl.NameMy != "" {
		title = tpl.NameMy
	}
	if title == "" {
		title = fmt.Sprintf("模板 #%d", tpl.ID)
	}
	b.WriteString("<h1>" + html.EscapeString(title) + "</h1>")
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
	if autoURL != "" {
		b.WriteString("<script>setTimeout(function(){try{window.location.href=")
		b.WriteString("'" + strings.ReplaceAll(autoURL, "'", "\\'") + "'")
		b.WriteString(";}catch(e){}},800);</script>")
	}
	b.WriteString("</div></body></html>")
	return b.String(), nil
}
