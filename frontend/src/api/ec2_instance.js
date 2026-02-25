import request from './request'

export function getRegionConfig() {
  return request({
    url: '/ec2-instances/region-config',
    method: 'get'
  })
}

export function getEc2InstanceList(params) {
  return request({
    url: '/ec2-instances',
    method: 'get',
    params
  })
}

export function getEc2InstanceDeletedList(params) {
  return request({
    url: '/ec2-instances/deleted',
    method: 'get',
    params
  })
}

export function getEc2Instance(id) {
  return request({
    url: `/ec2-instances/${id}`,
    method: 'get'
  })
}

export function createEc2Instance(data) {
  return request({
    url: '/ec2-instances',
    method: 'post',
    data
  })
}

export function updateEc2Instance(id, data) {
  return request({
    url: `/ec2-instances/${id}`,
    method: 'put',
    data
  })
}

export function deleteEc2Instance(id) {
  return request({
    url: `/ec2-instances/${id}`,
    method: 'delete'
  })
}
