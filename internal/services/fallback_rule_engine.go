package services

import (
	"aws_cdn/internal/logger"
	"aws_cdn/internal/models"
	"aws_cdn/internal/redis"
	"context"
	"fmt"
	"time"

	redisv9 "github.com/redis/go-redis/v9"
)

const ruleTriggeredClientIP = "0.0.0.0" // 兜底规则触发的探测结果占位 IP

// FallbackRuleEngine 兜底规则引擎：读取 Redis 日数据，按规则判断，未达标时写入 SpeedProbeResult(failed) 以触发告警
type FallbackRuleEngine struct {
	redisClient *redisv9.Client
	ruleSvc     *FallbackRuleService
	linkSvc     *CustomDownloadLinkService
	probeSvc    *SpeedProbeService
}

// NewFallbackRuleEngine 创建兜底规则引擎
func NewFallbackRuleEngine(redisClient *redisv9.Client, ruleSvc *FallbackRuleService, linkSvc *CustomDownloadLinkService, probeSvc *SpeedProbeService) *FallbackRuleEngine {
	return &FallbackRuleEngine{
		redisClient: redisClient,
		ruleSvc:     ruleSvc,
		linkSvc:     linkSvc,
		probeSvc:    probeSvc,
	}
}

// Run 执行一次规则检查：获取已启用规则，拉取今日/昨日数据，未达标则为该渠道下所有自定义链接写入一条 status=failed 的探测结果
func (e *FallbackRuleEngine) Run(ctx context.Context) error {
	log := logger.GetLogger()
	log.Info("开始执行兜底规则检查")

	if e.redisClient == nil {
		log.Warn("Redis 未配置，跳过兜底规则检查")
		return nil
	}

	rules, err := e.ruleSvc.ListEnabled()
	if err != nil {
		return fmt.Errorf("获取已启用规则失败: %w", err)
	}
	if len(rules) == 0 {
		return nil
	}

	now := time.Now()
	today := now.Format("2006-01-02")
	hour := now.Hour()
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")

	for _, rule := range rules {
		triggered, err := e.evaluateRule(ctx, &rule, today, yesterday, hour)
		if err != nil {
			log.WithError(err).WithField("rule_id", rule.ID).WithField("rule_name", rule.Name).Warn("规则评估失败，跳过")
			continue
		}
		if !triggered {
			log.WithField("rule_id", rule.ID).WithField("rule_name", rule.Name).Info("规则未触发，跳过")
			continue
		}

		// 未达标：为该渠道下所有启用链接写入一条 failed 探测结果
		links, err := e.linkSvc.ListActiveLinksByChannelCode(rule.ChannelCode)
		if err != nil {
			log.WithError(err).WithField("channel_code", rule.ChannelCode).Warn("获取渠道链接失败，跳过写入探测结果")
			continue
		}
		if len(links) == 0 {
			log.WithField("channel_code", rule.ChannelCode).Debug("该渠道无启用自定义链接，跳过写入")
			continue
		}

		msg := fmt.Sprintf("兜底规则触发：%s（%s）未达标", rule.Name, rule.RuleType)
		for _, link := range links {
			result := &models.SpeedProbeResult{
				URL:          link.URL,
				ClientIP:     ruleTriggeredClientIP,
				SpeedKbps:    0,
				Status:       models.SpeedProbeStatusFailed,
				ErrorMessage: msg,
				UserAgent:    "fallback-rule-engine",
			}
			if err := e.probeSvc.ReportProbeResult(result); err != nil {
				log.WithError(err).WithField("url", link.URL).Warn("写入探测结果失败")
			}
		}
		log.WithFields(map[string]interface{}{
			"rule_id":      rule.ID,
			"channel_code": rule.ChannelCode,
			"link_count":   len(links),
		}).Info("兜底规则已触发，已写入探测结果用于告警")
	}
	log.Info("兜底规则检查完成")

	return nil
}

// evaluateRule 评估单条规则，返回 true 表示未达标应触发
func (e *FallbackRuleEngine) evaluateRule(ctx context.Context, rule *models.FallbackRule, today, yesterday string, hour int) (bool, error) {
	switch rule.RuleType {
	case models.FallbackRuleTypeYesterdaySamePeriod:
		return e.evalYesterdaySamePeriod(ctx, rule, today, yesterday, hour)
	case models.FallbackRuleTypeFixedTimeTarget:
		return e.evalFixedTimeTarget(ctx, rule, today, hour)
	case models.FallbackRuleTypeHourlyIncrement:
		return e.evalHourlyIncrement(ctx, rule, today, hour)
	default:
		return false, fmt.Errorf("不支持的规则类型: %s", rule.RuleType)
	}
}

// getChannelRegCount 从 Redis 某日期某小时的站点数据中汇总指定渠道的注册数（匹配 ChannelCode 或 ChannelName）
func (e *FallbackRuleEngine) getChannelRegCount(ctx context.Context, date string, hour int, channelCode string) (int, error) {
	list, err := redis.GetAllSitesData(ctx, e.redisClient, date, hour)
	if err != nil {
		if err == redisv9.Nil {
			return 0, nil
		}
		return 0, err
	}
	var total int
	for _, site := range list {
		for _, stat := range site.Stats {
			if stat.ChannelCode == channelCode || stat.ChannelName == channelCode {
				total += stat.RegCount
			}
		}
	}
	return total, nil
}

// getChannelRegCountCumulative 从 0 点到 targetHour 的累计注册数（按小时 key 累加）
func (e *FallbackRuleEngine) getChannelRegCountCumulative(ctx context.Context, date string, targetHour int, channelCode string) (int, error) {
	var total int
	for h := 0; h <= targetHour; h++ {
		n, err := e.getChannelRegCount(ctx, date, h, channelCode)
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

func (e *FallbackRuleEngine) evalYesterdaySamePeriod(ctx context.Context, rule *models.FallbackRule, today, yesterday string, hour int) (bool, error) {
	p, err := rule.GetParamsYesterdaySamePeriod()
	if err != nil {
		return false, err
	}
	todayCount, err := e.getChannelRegCount(ctx, today, hour, rule.ChannelCode)
	if err != nil {
		return false, err
	}
	yesterdayCount, err := e.getChannelRegCount(ctx, yesterday, hour, rule.ChannelCode)
	if err != nil {
		return false, err
	}
	drop := yesterdayCount - todayCount
	return drop > p.MaxDrop, nil
}

func (e *FallbackRuleEngine) evalFixedTimeTarget(ctx context.Context, rule *models.FallbackRule, today string, currentHour int) (bool, error) {
	p, err := rule.GetParamsFixedTimeTarget()
	if err != nil {
		return false, err
	}
	// 若当前时刻还没到目标时刻，不触发（等下一轮再判）
	if currentHour < p.TargetHour {
		return false, nil
	}
	cum, err := e.getChannelRegCountCumulative(ctx, today, p.TargetHour, rule.ChannelCode)
	if err != nil {
		return false, err
	}
	return cum < p.TargetRegCount, nil
}

func (e *FallbackRuleEngine) evalHourlyIncrement(ctx context.Context, rule *models.FallbackRule, today string, currentHour int) (bool, error) {
	p, err := rule.GetParamsHourlyIncrement()
	if err != nil {
		return false, err
	}
	if currentHour < p.TargetHour {
		return false, nil
	}
	cum, err := e.getChannelRegCountCumulative(ctx, today, p.TargetHour, rule.ChannelCode)
	if err != nil {
		return false, err
	}
	return cum < p.TargetRegCount, nil
}
