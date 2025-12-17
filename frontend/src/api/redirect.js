import request from './request'

export const redirectApi = {
  // 创建重定向规则
  createRedirectRule(data) {
    return request.post('/redirects', data)
  },

  // 获取重定向规则列表
  getRedirectList(params) {
    return request.get('/redirects', { params })
  },

  // 获取重定向规则详情
  getRedirectRule(id) {
    return request.get(`/redirects/${id}`)
  },

  // 删除重定向规则
  deleteRule(id) {
    return request.delete(`/redirects/${id}`)
  },

  // 添加目标
  addTarget(ruleId, data) {
    return request.post(`/redirects/${ruleId}/targets`, data)
  },

  // 删除目标
  removeTarget(targetId) {
    return request.delete(`/redirects/targets/${targetId}`)
  },

  // 绑定域名到 CloudFront
  bindDomainToCloudFront(ruleId, data) {
    return request.post(`/redirects/${ruleId}/bind-cloudfront`, data)
  },
}

