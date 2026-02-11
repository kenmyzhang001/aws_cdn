package models

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// FallbackRuleType 兜底规则类型
type FallbackRuleType string

const (
	// FallbackRuleTypeYesterdaySamePeriod 与昨日同时段对比，少 N 即告警
	FallbackRuleTypeYesterdaySamePeriod FallbackRuleType = "yesterday_same_period"
	// FallbackRuleTypeFixedTimeTarget 指定时刻目标（如 9/10/11 点应达到的注册数）
	FallbackRuleTypeFixedTimeTarget FallbackRuleType = "fixed_time_target"
	// FallbackRuleTypeHourlyIncrement 从某时刻起每小时至少增加 N，或到某时刻累计应达到 K
	FallbackRuleTypeHourlyIncrement FallbackRuleType = "hourly_increment"
)

// FallbackRule 兜底规则模型（按渠道）
type FallbackRule struct {
	ID          uint             `json:"id" gorm:"primaryKey"`
	ChannelCode string           `json:"channel_code" gorm:"type:varchar(255);not null;index:idx_channel_code"` // 渠道，与 DailyStats.ChannelCode/ChannelName 可匹配
	Name        string           `json:"name" gorm:"type:varchar(255);not null"`                                 // 规则名称
	RuleType    FallbackRuleType `json:"rule_type" gorm:"type:varchar(50);not null"`                            // 规则类型
	ParamsJSON  string           `json:"params_json" gorm:"type:text"`                                          // 规则参数 JSON
	Enabled     bool             `json:"enabled" gorm:"default:true;index:idx_enabled"`                         // 是否启用
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
	DeletedAt   gorm.DeletedAt   `json:"-" gorm:"index"`
}

// TableName 指定表名
func (FallbackRule) TableName() string {
	return "fallback_rules"
}

// ParamsYesterdaySamePeriod 昨日同时段对比参数
type ParamsYesterdaySamePeriod struct {
	MaxDrop int `json:"max_drop"` // 允许比昨天少的上限，超过则告警（例如 10 表示少超过 10 就告警）
}

// ParamsFixedTimeTarget 指定时刻目标参数
type ParamsFixedTimeTarget struct {
	TargetHour    int `json:"target_hour"`    // 目标时刻 0-23
	TargetRegCount int `json:"target_reg_count"` // 到该时刻累计注册数应达到
}

// ParamsHourlyIncrement 每小时增量/到点累计参数
type ParamsHourlyIncrement struct {
	StartHour       int `json:"start_hour"`        // 从哪一时刻开始（0-23）
	TargetHour      int `json:"target_hour"`       // 到哪一时刻检查（0-23）
	TargetRegCount  int `json:"target_reg_count"`  // 到 target_hour 时累计注册数应达到
	HourlyMinGrowth int `json:"hourly_min_growth"` // 可选：每小时至少增加（0 表示不校验每小时增量，只校验累计）
}

// GetParamsYesterdaySamePeriod 解析昨日同时段参数
func (r *FallbackRule) GetParamsYesterdaySamePeriod() (*ParamsYesterdaySamePeriod, error) {
	var p ParamsYesterdaySamePeriod
	if err := json.Unmarshal([]byte(r.ParamsJSON), &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// GetParamsFixedTimeTarget 解析指定时刻目标参数
func (r *FallbackRule) GetParamsFixedTimeTarget() (*ParamsFixedTimeTarget, error) {
	var p ParamsFixedTimeTarget
	if err := json.Unmarshal([]byte(r.ParamsJSON), &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// GetParamsHourlyIncrement 解析每小时增量参数
func (r *FallbackRule) GetParamsHourlyIncrement() (*ParamsHourlyIncrement, error) {
	var p ParamsHourlyIncrement
	if err := json.Unmarshal([]byte(r.ParamsJSON), &p); err != nil {
		return nil, err
	}
	return &p, nil
}
