import request from './request'

export const speedProbeApi = {
  // 探测结果列表（支持 url, client_ip, user_agent, status, start_time, end_time, speed_min, speed_max）
  getProbeResultList(params) {
    return request.get('/speed-probe/results', { params })
  },
  // 按 IP 查询探测结果
  getProbeResultsByIP(ip, params) {
    return request.get(`/speed-probe/results/${encodeURIComponent(ip)}`, { params })
  },
  // 告警记录列表（支持 url, time_window_from, time_window_to, created_start, created_end, alert_sent, failed_rate_min, failed_rate_max）
  getAlertLogList(params) {
    return request.get('/speed-probe/alerts', { params })
  },
  // 手动触发检查
  triggerCheck(params) {
    return request.post('/speed-probe/check', null, { params })
  },
}
