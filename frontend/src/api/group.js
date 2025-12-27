import request from './request'

export const groupApi = {
  // 获取分组列表
  getGroupList() {
    return request.get('/groups')
  },

  // 获取分组列表（带统计信息，用于优化页面加载）
  getGroupListWithStats() {
    return request.get('/groups/with-stats')
  },

  // 获取分组详情
  getGroup(id) {
    return request.get(`/groups/${id}`)
  },

  // 创建分组
  createGroup(data) {
    return request.post('/groups', data)
  },

  // 更新分组
  updateGroup(id, data) {
    return request.put(`/groups/${id}`, data)
  },

  // 删除分组
  deleteGroup(id) {
    return request.delete(`/groups/${id}`)
  },
}

