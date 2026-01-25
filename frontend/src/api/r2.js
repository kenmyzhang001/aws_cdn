import request from './request'

export const r2Api = {
  // 启用 R2
  enableR2(cfAccountId) {
    return request.post(`/cf-accounts/${cfAccountId}/enable-r2`)
  },

  // R2 存储桶管理
  getR2BucketList() {
    return request.get('/r2-buckets')
  },

  getR2Bucket(id) {
    return request.get(`/r2-buckets/${id}`)
  },

  createR2Bucket(data) {
    return request.post('/r2-buckets', data)
  },

  deleteR2Bucket(id) {
    return request.delete(`/r2-buckets/${id}`)
  },

  updateR2BucketNote(id, note) {
    return request.put(`/r2-buckets/${id}/note`, { note })
  },

  updateR2BucketCredentials(id, accessKeyID, secretAccessKey, accountID = '') {
    return request.put(`/r2-buckets/${id}/credentials`, {
      access_key_id: accessKeyID,
      secret_access_key: secretAccessKey,
      account_id: accountID,
    })
  },

  configureCORS(id, corsConfig) {
    return request.put(`/r2-buckets/${id}/cors`, { cors_config: corsConfig })
  },

  // R2 自定义域名管理
  getR2CustomDomainList(r2BucketId) {
    return request.get(`/r2-custom-domains/buckets/${r2BucketId}`)
  },

  addR2CustomDomain(r2BucketId, data) {
    return request.post(`/r2-custom-domains/buckets/${r2BucketId}`, data)
  },

  deleteR2CustomDomain(id) {
    return request.delete(`/r2-custom-domains/${id}`)
  },

  // R2 缓存规则管理
  getR2CacheRuleList(r2CustomDomainId) {
    return request.get(`/r2-cache-rules/domains/${r2CustomDomainId}`)
  },

  createR2CacheRule(r2CustomDomainId, data) {
    return request.post(`/r2-cache-rules/domains/${r2CustomDomainId}`, data)
  },

  deleteR2CacheRule(id) {
    return request.delete(`/r2-cache-rules/${id}`)
  },

  // R2 文件管理
  uploadFile(r2BucketId, formData) {
    return request.post(`/r2-files/buckets/${r2BucketId}/upload`, formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
    })
  },

  // 带进度的文件上传
  uploadFileWithProgress(r2BucketId, formData, onProgress, cancelToken) {
    return request.post(`/r2-files/buckets/${r2BucketId}/upload`, formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
      onUploadProgress: onProgress,
      cancelToken: cancelToken,
      timeout: 600000, // 10分钟超时
    })
  },

  createDirectory(r2BucketId, prefix) {
    return request.post(`/r2-files/buckets/${r2BucketId}/directories`, { prefix })
  },

  listFiles(r2BucketId, prefix = '') {
    return request.get(`/r2-files/buckets/${r2BucketId}`, { params: { prefix } })
  },

  deleteFile(r2BucketId, key) {
    return request.delete(`/r2-files/buckets/${r2BucketId}`, {
      data: { key },
    })
  },

  // APK 文件管理
  listApkFiles(r2BucketId, prefix = '') {
    return request.get(`/r2-files/buckets/${r2BucketId}/apk-files`, { params: { prefix } })
  },

  getApkFileUrls(r2BucketId, filePath) {
    return request.get(`/r2-files/buckets/${r2BucketId}/apk-file-urls`, {
      params: { file_path: filePath },
    })
  },
}
