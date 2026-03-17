package models

import (
	"time"

	"gorm.io/gorm"
)

// CFWorkpageTemplate CF-WorkPage 模版（支持中/缅双语言，可指定落地页默认语言）
type CFWorkpageTemplate struct {
	ID         uint           `json:"id" gorm:"primaryKey"`
	NameZh     string         `json:"name_zh" gorm:"type:varchar(255)"`   // 模版名称-中文
	NameMy     string         `json:"name_my" gorm:"type:varchar(255)"`   // 模版名称-缅甸文
	DefaultLang string        `json:"default_lang" gorm:"type:varchar(8);default:'zh'"` // 落地页默认语言: zh | my
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (CFWorkpageTemplate) TableName() string {
	return "cf_workpage_templates"
}
