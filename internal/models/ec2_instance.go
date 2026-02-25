package models

import (
	"time"

	"gorm.io/gorm"
)

type Ec2Instance struct {
	ID              uint           `json:"id" gorm:"primaryKey"`
	Name            string         `json:"name" gorm:"type:varchar(255);not null"`
	Region          string         `json:"region" gorm:"type:varchar(64);not null;index"`
	AMIID           string         `json:"ami_id" gorm:"type:varchar(64);not null"`
	SecurityGroupID string         `json:"security_group_id" gorm:"type:varchar(64);not null"`
	InstanceType    string         `json:"instance_type" gorm:"type:varchar(32);default:t3.micro"`
	AWSInstanceID   string         `json:"aws_instance_id" gorm:"type:varchar(64);uniqueIndex"`
	State           string         `json:"state" gorm:"type:varchar(32)"`
	Note            string         `json:"note" gorm:"type:text"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
	LifetimeHours   *float64       `json:"lifetime_hours,omitempty" gorm:"type:double"`
}

func (Ec2Instance) TableName() string {
	return "ec2_instances"
}
