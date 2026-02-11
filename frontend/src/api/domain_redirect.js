import request from './request'

export const domainRedirectApi = {
  list(params = {}) {
    return request.get('/domain-redirects', { params })
  },

  get(id) {
    return request.get(`/domain-redirects/${id}`)
  },

  create(data) {
    return request.post('/domain-redirects', data)
  },

  update(id, data) {
    return request.put(`/domain-redirects/${id}`, data)
  },

  delete(id) {
    return request.delete(`/domain-redirects/${id}`)
  },

  ensureDns(id) {
    return request.post(`/domain-redirects/${id}/ensure-dns`)
  },

  /** 创建前检查主域名是否已被占用 */
  checkDomain(domain) {
    return request.get('/domain-redirects/check-domain', { params: { domain: domain || '' } })
  }
}
