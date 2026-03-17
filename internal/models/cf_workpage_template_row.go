package models

import (
	"time"

	"gorm.io/gorm"
)

// CFWorkpageTemplateRow 模版表格行（固定3列：列1标题、列2描述、列3按钮文案，每行一个下载链接；支持“默认自动弹出”）
type CFWorkpageTemplateRow struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	TemplateID  uint           `json:"template_id" gorm:"not null;index"`
	SortOrder   int            `json:"sort_order" gorm:"default:0"` // 行顺序
	Col1Zh      string         `json:"col1_zh" gorm:"type:varchar(255)"`   // 第1列-中文（如：最新优惠）
	Col1My      string         `json:"col1_my" gorm:"type:varchar(255)"`   // 第1列-缅甸文
	Col2Zh      string         `json:"col2_zh" gorm:"type:varchar(512)"`   // 第2列-中文（如：注册送8888）
	Col2My      string         `json:"col2_my" gorm:"type:varchar(512)"`   // 第2列-缅甸文
	Col3Zh      string         `json:"col3_zh" gorm:"type:varchar(128)"`   // 第3列按钮文案-中文（如：立即领取）
	Col3My      string         `json:"col3_my" gorm:"type:varchar(128)"`   // 第3列按钮文案-缅甸文
	DownloadURL string         `json:"download_url" gorm:"type:varchar(1024)"` // 点击按钮时的下载链接
	AutoPopup   bool           `json:"auto_popup" gorm:"default:false"`        // 访问页面时是否自动弹出此包下载（同模版仅允许一个为 true）
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (CFWorkpageTemplateRow) TableName() string {
	return "cf_workpage_template_rows"
}
