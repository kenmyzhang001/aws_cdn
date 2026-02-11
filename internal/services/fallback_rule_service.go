package services

import (
	"aws_cdn/internal/models"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
)

type FallbackRuleService struct {
	db *gorm.DB
}

func NewFallbackRuleService(db *gorm.DB) *FallbackRuleService {
	return &FallbackRuleService{db: db}
}

// Create 创建兜底规则
func (s *FallbackRuleService) Create(rule *models.FallbackRule) error {
	if rule.ChannelCode == "" {
		return fmt.Errorf("渠道不能为空")
	}
	if rule.RuleType == "" {
		return fmt.Errorf("规则类型不能为空")
	}
	if err := s.validateParams(rule.RuleType, rule.ParamsJSON); err != nil {
		return err
	}
	return s.db.Create(rule).Error
}

// Update 更新兜底规则
func (s *FallbackRuleService) Update(id uint, updates map[string]interface{}) error {
	if params, ok := updates["params_json"]; ok {
		ruleType := updates["rule_type"]
		var rt string
		switch v := ruleType.(type) {
		case string:
			rt = v
		default:
			var r models.FallbackRule
			if err := s.db.Select("rule_type").First(&r, id).Error; err != nil {
				return err
			}
			rt = string(r.RuleType)
		}
		if rt != "" {
			if ps, ok := params.(string); ok && ps != "" {
				if err := s.validateParams(models.FallbackRuleType(rt), ps); err != nil {
					return err
				}
			}
		}
	}
	return s.db.Model(&models.FallbackRule{}).Where("id = ?", id).Updates(updates).Error
}

func (s *FallbackRuleService) validateParams(ruleType models.FallbackRuleType, paramsJSON string) error {
	if paramsJSON == "" {
		return fmt.Errorf("规则参数不能为空")
	}
	switch ruleType {
	case models.FallbackRuleTypeYesterdaySamePeriod:
		var p models.ParamsYesterdaySamePeriod
		if err := json.Unmarshal([]byte(paramsJSON), &p); err != nil {
			return fmt.Errorf("昨日同时段参数无效: %w", err)
		}
	case models.FallbackRuleTypeFixedTimeTarget:
		var p models.ParamsFixedTimeTarget
		if err := json.Unmarshal([]byte(paramsJSON), &p); err != nil {
			return fmt.Errorf("指定时刻目标参数无效: %w", err)
		}
		if p.TargetHour < 0 || p.TargetHour > 23 {
			return fmt.Errorf("target_hour 需在 0-23")
		}
	case models.FallbackRuleTypeHourlyIncrement:
		var p models.ParamsHourlyIncrement
		if err := json.Unmarshal([]byte(paramsJSON), &p); err != nil {
			return fmt.Errorf("每小时增量参数无效: %w", err)
		}
		if p.StartHour < 0 || p.StartHour > 23 || p.TargetHour < 0 || p.TargetHour > 23 {
			return fmt.Errorf("start_hour/target_hour 需在 0-23")
		}
	default:
		return fmt.Errorf("不支持的规则类型: %s", ruleType)
	}
	return nil
}

// Get 获取单条规则
func (s *FallbackRuleService) Get(id uint) (*models.FallbackRule, error) {
	var rule models.FallbackRule
	if err := s.db.First(&rule, id).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

// List 分页列表，支持按渠道、类型、启用状态筛选
func (s *FallbackRuleService) List(page, pageSize int, channelCode *string, ruleType *models.FallbackRuleType, enabled *bool) ([]models.FallbackRule, int64, error) {
	var list []models.FallbackRule
	var total int64
	query := s.db.Model(&models.FallbackRule{})
	if channelCode != nil && *channelCode != "" {
		query = query.Where("channel_code = ?", *channelCode)
	}
	if ruleType != nil && *ruleType != "" {
		query = query.Where("rule_type = ?", *ruleType)
	}
	if enabled != nil {
		query = query.Where("enabled = ?", *enabled)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Order("id DESC").Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// ListEnabled 获取所有已启用的规则（供规则引擎使用）
func (s *FallbackRuleService) ListEnabled() ([]models.FallbackRule, error) {
	var list []models.FallbackRule
	if err := s.db.Where("enabled = ?", true).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// Delete 删除（软删除）
func (s *FallbackRuleService) Delete(id uint) error {
	return s.db.Delete(&models.FallbackRule{}, id).Error
}
