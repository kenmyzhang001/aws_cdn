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
  },

  /** 模版表格行（3列多行，每行一个下载链接；支持默认自动弹出其中一个包） */
  getRows(templateId) {
    return request.get(`/cf-workpage-templates/${templateId}/rows`)
  },

  saveRows(templateId, rows) {
    return request.put(`/cf-workpage-templates/${templateId}/rows`, { rows })
  }
}
