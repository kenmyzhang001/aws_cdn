import request from './request'

// 获取重点探测链接列表
export const getFocusProbeLinks = (params) => {
  return request.get('/focus-probe-links', { params })
}

// 获取重点探测链接详情
export const getFocusProbeLinkById = (id) => {
  return request.get(`/focus-probe-links/${id}`)
}

// 创建重点探测链接
export const createFocusProbeLink = (data) => {
  return request.post('/focus-probe-links', data)
}

// 更新重点探测链接
export const updateFocusProbeLink = (id, data) => {
  return request.put(`/focus-probe-links/${id}`, data)
}

// 删除重点探测链接
export const deleteFocusProbeLink = (id) => {
  return request.delete(`/focus-probe-links/${id}`)
}

// 批量删除重点探测链接
export const batchDeleteFocusProbeLinks = (ids) => {
  return request.post('/focus-probe-links/batch-delete', { ids })
}

// 批量更新探测间隔
export const batchUpdateProbeInterval = (ids, intervalMinutes, updateAll = false) => {
  return request.post('/focus-probe-links/batch-update-interval', {
    ids,
    interval_minutes: intervalMinutes,
    update_all: updateAll
  })
}

// 切换启用状态
export const toggleFocusProbeLinkEnabled = (id) => {
  return request.post(`/focus-probe-links/${id}/toggle-enabled`)
}

// 获取统计信息
export const getFocusProbeLinkStatistics = () => {
  return request.get('/focus-probe-links/statistics')
}

// 导出链接列表
export const exportFocusProbeLinks = () => {
  return request.get('/focus-probe-links/export')
}

// 检查URL是否已存在
export const checkIfURLExists = (url) => {
  return request.get('/focus-probe-links/check-url', { params: { url } })
}

// 从下载包添加到重点探测
export const addFromDownloadPackage = (packageId, url, name) => {
  return request.post('/focus-probe-links/from-download-package', {
    package_id: packageId,
    url,
    name
  })
}

// 从自定义下载链接添加到重点探测
export const addFromCustomDownloadLink = (linkId, url, name) => {
  return request.post('/focus-probe-links/from-custom-link', {
    link_id: linkId,
    url,
    name
  })
}

// 从R2文件添加到重点探测
export const addFromR2File = (url, name, description) => {
  return request.post('/focus-probe-links/from-r2-file', {
    url,
    name,
    description
  })
}
