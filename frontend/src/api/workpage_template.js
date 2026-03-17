import request from './request'

export const workpageTemplateApi = {
  list(params = {}) {
    return request.get('/cf-workpage-templates', { params })
  },

  get(id) {
    return request.get(`/cf-workpage-templates/${id}`)
  },

  create(data) {
    return request.post('/cf-workpage-templates', data)
  },

  update(id, data) {
    return request.put(`/cf-workpage-templates/${id}`, data)
  },

  delete(id) {
    return request.delete(`/cf-workpage-templates/${id}`)
  }
}
