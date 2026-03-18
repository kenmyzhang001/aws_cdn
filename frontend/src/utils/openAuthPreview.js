import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '@/router'

/**
 * 带登录态打开需鉴权的 HTML 预览（新窗口无法自动带 Authorization，故先请求再 Blob 打开）
 * @param {string} path 如 /cf-workpage-templates/1/preview（不含 /api/v1）
 */
export async function openAuthPreview(path) {
  const token = localStorage.getItem('token')
  if (!token) {
    ElMessage.warning('请先登录')
    router.push('/login')
    return
  }
  const url = path.startsWith('/api/v1') ? path : `/api/v1${path.startsWith('/') ? path : '/' + path}`
  try {
    const res = await axios.get(url, {
      responseType: 'text',
      headers: { Authorization: `Bearer ${token}` },
      transformResponse: [(data) => data],
    })
    const html = typeof res.data === 'string' ? res.data : String(res.data ?? '')
    if (html.trimStart().startsWith('{') && html.includes('"error"')) {
      try {
        const j = JSON.parse(html)
        ElMessage.error(j.error || '预览失败')
      } catch {
        ElMessage.error('预览失败')
      }
      return
    }
    const blob = new Blob([html], { type: 'text/html;charset=utf-8' })
    const href = URL.createObjectURL(blob)
    const w = window.open(href, '_blank')
    if (!w) {
      ElMessage.warning('请允许浏览器弹出窗口')
      URL.revokeObjectURL(href)
      return
    }
    setTimeout(() => URL.revokeObjectURL(href), 120000)
  } catch (e) {
    if (e.response?.status === 401) {
      localStorage.removeItem('token')
      localStorage.removeItem('username')
      ElMessage.error('登录已过期，请重新登录')
      router.push('/login')
      return
    }
    const d = e.response?.data
    let msg = '预览加载失败'
    if (typeof d === 'string' && d.trim().startsWith('{')) {
      try {
        msg = JSON.parse(d).error || msg
      } catch {
        msg = d.slice(0, 200)
      }
    } else if (d?.error) {
      msg = d.error
    }
    ElMessage.error(msg)
  }
}
