import request from './request'

/**
 * 兜底规则列表（分页）
 */
export function getFallbackRules(params) {
  return request({
    url: '/fallback-rules',
    method: 'get',
    params,
  })
}

/**
 * 获取单条兜底规则
 */
export function getFallbackRule(id) {
  return request({
    url: `/fallback-rules/${id}`,
    method: 'get',
  })
}

/**
 * 创建兜底规则
 * @param {Object} data
 * @param {string} data.channel_code - 渠道
 * @param {string} data.name - 规则名称
 * @param {string} data.rule_type - yesterday_same_period | fixed_time_target | hourly_increment
 * @param {string} data.params_json - JSON 字符串，如 {"max_drop":10} / {"target_hour":10,"target_reg_count":100} / {"start_hour":0,"target_hour":10,"target_reg_count":100}
 * @param {boolean} [data.enabled]
 */
export function createFallbackRule(data) {
  return request({
    url: '/fallback-rules',
    method: 'post',
    data,
  })
}

/**
 * 更新兜底规则
 */
export function updateFallbackRule(id, data) {
  return request({
    url: `/fallback-rules/${id}`,
    method: 'put',
    data,
  })
}

/**
 * 删除兜底规则
 */
export function deleteFallbackRule(id) {
  return request({
    url: `/fallback-rules/${id}`,
    method: 'delete',
  })
}
