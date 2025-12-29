import request from './request'

export const downloadPackageApi = {
  // 获取下载包列表
  getDownloadPackageList(params) {
    return request.get('/download-packages', { params })
  },

  // 获取指定域名下的所有下载包
  getDownloadPackagesByDomain(domainId) {
    return request.get('/download-packages/by-domain', {
      params: { domain_id: domainId },
    })
  },

  // 获取下载包详情
  getDownloadPackage(id) {
    return request.get(`/download-packages/${id}`)
  },

  // 创建下载包（上传文件）
  createDownloadPackage(formData) {
    return request.post('/download-packages', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
      timeout: 300000, // 5分钟超时
    })
  },

  // 删除下载包
  deleteDownloadPackage(id) {
    return request.delete(`/download-packages/${id}`)
  },

  // 检查下载包状态
  checkDownloadPackage(id) {
    return request.get(`/download-packages/${id}/check`)
  },

  // 修复下载包
  fixDownloadPackage(id) {
    return request.post(`/download-packages/${id}/fix`)
  },

  // 更新下载包备注
  updateDownloadPackageNote(id, note) {
    return request.put(`/download-packages/${id}/note`, { note })
  },
}

