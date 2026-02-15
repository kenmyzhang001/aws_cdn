import request from './request'

export const gameStatsApi = {
  // 获取全部渠道名称（来自 Redis 集合 game_stats:full_channel_names）
  getFullChannelNames() {
    return request.get('/game-stats/full-channel-names')
  },
  // 按时间查询站点日数据（Redis 缓存 allSitesData_日期_小时）
  getSiteDailyData(params = {}) {
    return request.get('/game-stats/site-daily', { params })
  },
  // 渠道分组（用于站点日数据分组统计，与域名/下载包 Group 区分）
  getChannelGroups() {
    return request.get('/game-stats/channel-groups')
  },
  createChannelGroup(data) {
    return request.post('/game-stats/channel-groups', data)
  },
  updateChannelGroup(id, data) {
    return request.put(`/game-stats/channel-groups/${id}`, data)
  },
  deleteChannelGroup(id) {
    return request.delete(`/game-stats/channel-groups/${id}`)
  },
}
