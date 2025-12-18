import request from './request'

export const cloudfrontApi = {
  // 获取 CloudFront 分发列表
  getDistributionList() {
    return request.get('/cloudfront/distributions')
  },

  // 获取 CloudFront 分发详情
  getDistribution(id) {
    return request.get(`/cloudfront/distributions/${id}`)
  },

  // 创建 CloudFront 分发
  createDistribution(data) {
    return request.post('/cloudfront/distributions', data)
  },

  // 更新 CloudFront 分发
  updateDistribution(id, data) {
    return request.put(`/cloudfront/distributions/${id}`, data)
  },

  // 删除 CloudFront 分发
  deleteDistribution(id) {
    return request.delete(`/cloudfront/distributions/${id}`)
  },
}

