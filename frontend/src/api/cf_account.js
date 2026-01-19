import request from './request'

export const cfAccountApi = {
  // 获取 Cloudflare 账号列表
  getCFAccountList() {
    return request.get('/cf-accounts')
  },

  // 获取 Cloudflare 账号详情
  getCFAccount(id) {
    return request.get(`/cf-accounts/${id}`)
  },

  // 创建 Cloudflare 账号
  createCFAccount(data) {
    return request.post('/cf-accounts', data)
  },

  // 更新 Cloudflare 账号
  updateCFAccount(id, data) {
    return request.put(`/cf-accounts/${id}`, data)
  },

  // 删除 Cloudflare 账号
  deleteCFAccount(id) {
    return request.delete(`/cf-accounts/${id}`)
  },
}
