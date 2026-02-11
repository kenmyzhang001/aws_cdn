import request from './request';

/**
 * 创建 Worker
 * @param {Object} data - Worker 数据
 * @returns {Promise}
 */
export function createWorker(data) {
  return request({
    url: '/cf-workers',
    method: 'post',
    data
  });
}

/**
 * 获取 Worker 列表
 * @param {Object} params - 查询参数
 * @returns {Promise}
 */
export function getWorkerList(params) {
  return request({
    url: '/cf-workers',
    method: 'get',
    params
  });
}

/**
 * 获取 Worker 详情
 * @param {number} id - Worker ID
 * @returns {Promise}
 */
export function getWorker(id) {
  return request({
    url: `/cf-workers/${id}`,
    method: 'get'
  });
}

/**
 * 更新 Worker
 * @param {number} id - Worker ID
 * @param {Object} data - 更新数据
 * @returns {Promise}
 */
export function updateWorker(id, data) {
  return request({
    url: `/cf-workers/${id}`,
    method: 'put',
    data
  });
}

/**
 * 删除 Worker
 * @param {number} id - Worker ID
 * @returns {Promise}
 */
export function deleteWorker(id) {
  return request({
    url: `/cf-workers/${id}`,
    method: 'delete'
  });
}

/**
 * 创建前检查 Worker 域名是否已被占用
 * @param {string} domain - 域名
 * @returns {Promise<{ available: boolean, used_by: string, ref_id: number, ref_name: string }>}
 */
export function checkWorkerDomain(domain) {
  return request({
    url: '/cf-workers/check-domain',
    method: 'get',
    params: { domain: domain || '' }
  });
}
