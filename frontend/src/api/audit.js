import request from './request'

export const auditApi = {
  // 获取审计日志列表
  getAuditLogList(params) {
    return request.get('/audit-logs', { params })
  },
}




