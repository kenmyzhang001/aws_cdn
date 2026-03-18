import request from './request'

export const workpageSiteApi = {
  list(params = {}) {
    return request.get('/cf-workpage-sites', { params })
  },

  get(id) {
    return request.get(`/cf-workpage-sites/${id}`)
  },

  create(data) {
    return request.post('/cf-workpage-sites', data)
  },

  update(id, data) {
    return request.put(`/cf-workpage-sites/${id}`, data)
  },

  delete(id) {
    return request.delete(`/cf-workpage-sites/${id}`)
  },

  deploy(id) {
    return request.post(`/cf-workpage-sites/${id}/deploy`)
  },

  /** 最近一次成功部署上传的 index.html 原文 */
  getDeployedIndexHtml(id) {
    return request.get(`/cf-workpage-sites/${id}/deployed-index-html`)
  }
}
