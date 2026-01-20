import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '@/router'

const request = axios.create({
  baseURL: '/api/v1',
  timeout: 30000,
})

// 请求拦截器
request.interceptors.request.use(
  (config) => {
    // 从localStorage获取token并添加到请求头
    const token = localStorage.getItem('token')
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    // 保留 onUploadProgress 和 cancelToken（如果存在）
    // 这些配置会直接传递给 axios
    return config
  },
  (error) => {
    return Promise.reject(error)
  }
)

// 响应拦截器
request.interceptors.response.use(
  (response) => {
    return response.data
  },
  (error) => {
    // 如果是取消请求，直接返回，不显示错误消息
    if (axios.isCancel(error)) {
      return Promise.reject(error)
    }
    
    // 处理401未授权错误
    if (error.response?.status === 401) {
      // 清除本地存储的token
      localStorage.removeItem('token')
      localStorage.removeItem('username')
      ElMessage.error('登录已过期，请重新登录')
      // 跳转到登录页
      router.push('/login')
      return Promise.reject(error)
    }
    
    const message = error.response?.data?.error || error.message || '请求失败'
    ElMessage.error(message)
    return Promise.reject(error)
  }
)

export default request


