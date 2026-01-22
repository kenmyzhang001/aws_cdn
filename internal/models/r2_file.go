package models

import "time"

// R2File R2文件记录
type R2File struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	R2BucketID  uint      `gorm:"not null;index" json:"r2_bucket_id"`
	FilePath    string    `gorm:"type:varchar(1024);not null" json:"file_path"`
	FileName    string    `gorm:"type:varchar(255);not null" json:"file_name"`
	FileSize    *int64    `gorm:"type:bigint" json:"file_size,omitempty"`
	ContentType *string   `gorm:"type:varchar(128)" json:"content_type,omitempty"`
	ETag        *string   `gorm:"type:varchar(128)" json:"etag,omitempty"`
	Note        string    `gorm:"type:text" json:"note"`
	Status      string    `gorm:"type:varchar(20);default:active" json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 指定表名
func (R2File) TableName() string {
	return "r2_files"
}
