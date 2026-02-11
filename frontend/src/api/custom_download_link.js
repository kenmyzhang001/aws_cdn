import request from './request';

/**
 * 获取自定义下载链接列表
 * @param {Object} params - 查询参数
 * @param {number} params.page - 页码
 * @param {number} params.page_size - 每页数量
 * @param {number} [params.group_id] - 分组ID（可选）
 * @param {string} [params.search] - 搜索关键词（可选）
 * @param {string} [params.status] - 状态筛选（可选）
 * @returns {Promise}
 */
export function getCustomDownloadLinks(params) {
  return request({
    url: '/custom-download-links',
    method: 'get',
    params
  });
}

/**
 * 获取单个自定义下载链接
 * @param {number} id - 链接ID
 * @returns {Promise}
 */
export function getCustomDownloadLink(id) {
  return request({
    url: `/custom-download-links/${id}`,
    method: 'get'
  });
}

/**
 * 创建单个自定义下载链接
 * @param {Object} data - 链接数据
 * @param {string} data.url - 下载链接URL
 * @param {string} [data.name] - 链接名称（可选）
 * @param {string} [data.description] - 链接描述（可选）
 * @param {number} [data.group_id] - 所属分组ID（可选）
 * @param {string} data.channel_code - 渠道（必填，从 ListFullChannelNames 选择）
 * @param {string} [data.status] - 状态（可选）
 * @returns {Promise}
 */
export function createCustomDownloadLink(data) {
  return request({
    url: '/custom-download-links',
    method: 'post',
    data
  });
}

/**
 * 批量创建自定义下载链接
 * @param {Object} data - 链接数据
 * @param {string} data.urls - 链接列表（支持换行符或逗号分隔）
 * @param {string} data.channel_code - 渠道（必填）
 * @param {number} [data.group_id] - 所属分组ID（可选）
 * @returns {Promise}
 */
export function batchCreateCustomDownloadLinks(data) {
  return request({
    url: '/custom-download-links/batch',
    method: 'post',
    data
  });
}

/**
 * 更新自定义下载链接
 * @param {number} id - 链接ID
 * @param {Object} data - 更新数据
 * @param {string} [data.url] - 下载链接URL（可选）
 * @param {string} [data.name] - 链接名称（可选）
 * @param {string} [data.description] - 链接描述（可选）
 * @param {number} [data.group_id] - 所属分组ID（可选）
 * @param {string} [data.status] - 状态（可选）
 * @returns {Promise}
 */
export function updateCustomDownloadLink(id, data) {
  return request({
    url: `/custom-download-links/${id}`,
    method: 'put',
    data
  });
}

/**
 * 删除自定义下载链接
 * @param {number} id - 链接ID
 * @returns {Promise}
 */
export function deleteCustomDownloadLink(id) {
  return request({
    url: `/custom-download-links/${id}`,
    method: 'delete'
  });
}

/**
 * 批量删除自定义下载链接
 * @param {Array<number>} ids - 链接ID列表
 * @returns {Promise}
 */
export function batchDeleteCustomDownloadLinks(ids) {
  return request({
    url: '/custom-download-links/batch-delete',
    method: 'post',
    data: { ids }
  });
}

/**
 * 增加点击次数
 * @param {number} id - 链接ID
 * @returns {Promise}
 */
export function incrementClickCount(id) {
  return request({
    url: `/custom-download-links/${id}/click`,
    method: 'post'
  });
}
