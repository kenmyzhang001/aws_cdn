package models

import (
	"time"

	"gorm.io/gorm"
)

// User 用户模型
type User struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	Username  string         `json:"username" gorm:"uniqueIndex;not null"`
	Email     string         `json:"email" gorm:"uniqueIndex;not null"`
	Password  string         `json:"-" gorm:"not null"` // bcrypt 哈希
	IsActive  bool           `json:"is_active" gorm:"default:true"`

	// 谷歌验证码 (TOTP)
	TwoFactorSecret    string `json:"-" gorm:"column:two_factor_secret"`
	IsTwoFactorEnabled bool   `json:"is_two_factor_enabled" gorm:"default:false"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (User) TableName() string {
	return "users"
}

