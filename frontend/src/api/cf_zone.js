import request from './request';

/**
 * 获取 CF 账号下的域名列表（Zone列表）
 * @param {number} cfAccountId - CF账号ID
 * @param {number} page - 页码
 * @param {number} perPage - 每页数量
 * @param {string} name - 域名名称搜索
 * @returns {Promise}
 */
export function getCFAccountZones(cfAccountId, page = 1, perPage = 20, name = '') {
  const params = {
    page,
    per_page: perPage
  };
  
  if (name) {
    params.name = name;
  }
  
  return request({
    url: `/cf-accounts/${cfAccountId}/zones`,
    method: 'get',
    params
  });
}

/**
 * 为域名设置 APK 放行安全规则
 * @param {number} cfAccountId - CF账号ID
 * @param {string} zoneId - Zone ID
 * @param {string} domainName - 域名
 * @returns {Promise}
 */
export function setZoneAPKSecurityRule(cfAccountId, zoneId, domainName) {
  return request({
    url: `/cf-accounts/${cfAccountId}/zones/apk-security`,
    method: 'post',
    data: {
      zone_id: zoneId,
      domain_name: domainName
    }
  });
}

/**
 * 批量添加域名到指定 CF 账号
 * @param {number} cfAccountId - CF账号ID
 * @param {string[]} domains - 域名列表，如 ['example.com', 'foo.com']
 * @returns {Promise<{ message, results, stats }>}
 */
export function addZones(cfAccountId, domains) {
  return request({
    url: `/cf-accounts/${cfAccountId}/zones`,
    method: 'post',
    data: { domains }
  });
}
