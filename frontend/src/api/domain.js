import request from './request'

export const domainApi = {
  // 转入域名
  transferDomain(data) {
    return request.post('/domains', data)
  },

  // 获取域名列表
  getDomainList(params) {
    return request.get('/domains', { params })
  },

  // 获取域名详情
  getDomain(id) {
    return request.get(`/domains/${id}`)
  },

  // 获取 NS 服务器
  getNServers(id) {
    return request.get(`/domains/${id}/ns-servers`)
  },

  // 获取域名状态
  getDomainStatus(id) {
    return request.get(`/domains/${id}/status`)
  },

  // 生成证书
  generateCertificate(id) {
    return request.post(`/domains/${id}/certificate`)
  },

  // 获取证书状态
  getCertificateStatus(id) {
    return request.get(`/domains/${id}/certificate/status`)
  },

  // 删除域名
  deleteDomain(id) {
    return request.delete(`/domains/${id}`)
  },

  // 检查证书配置
  checkCertificate(id) {
    return request.get(`/domains/${id}/certificate/check`)
  },

  // 修复证书配置
  fixCertificate(id) {
    return request.post(`/domains/${id}/certificate/fix`)
  },

  // 获取域名列表（轻量级，用于下拉选择框）
  getDomainListForSelect(params) {
    return request.get('/domains/for-select', { params })
  },
}


