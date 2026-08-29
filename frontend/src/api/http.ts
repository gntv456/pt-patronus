import axios from 'axios'

// 统一 HTTP 客户端：注入 JWT；401（含登出后 token_version_mismatch）/403 VIP 准入失败 → 清 token 回登录。
export const http = axios.create({
  baseURL: '/api/v1',
  timeout: 15000,
})

http.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

http.interceptors.response.use(
  (r) => r,
  (err) => {
    const status = err.response?.status
    // 机器可读错误码（后端统一信封 {code, message, error}）：透传到 err.code 供业务分支。
    // 不改变下方文案优先级——code 只做编程判断，message 仍走 error/message 双字段回退。
    const code = err.response?.data?.code
    if (typeof code === 'string' && code) err.code = code
    // 401：token 无效/过期/已登出（token_version_mismatch）
    if (status === 401) {
      localStorage.removeItem('token')
      if (location.pathname !== '/login') location.href = '/login'
    }
    // 403 VIP 准入失败（vip_expired/vip_not_verified/not_vip）：清 token，要求重新登录验证
    if (status === 403 && ['vip_expired', 'vip_not_verified', 'vip_unknown', 'not_vip'].includes(code)) {
      localStorage.removeItem('token')
      if (location.pathname !== '/login') location.href = '/login'
    }
    // 错误友好化：改写 err.message 为中文文案，组件 catch 显示 err.message 时即友好。
    // 后端已给中文 error/message 时优先用，否则按状态码/错误类型兜底。
    const backend = err.response?.data?.error || err.response?.data?.message
    if (typeof backend === 'string' && backend) {
      err.message = backend
    } else if (err.code === 'ECONNABORTED' || /timeout/i.test(err.message || '')) {
      err.message = '请求超时，请稍后重试'
    } else if (!err.response) {
      err.message = '网络连接失败，请检查网络'
    } else if (status === 429) {
      err.message = '操作过于频繁，请稍后再试'
    } else if (status === 404) {
      err.message = '请求的资源不存在'
    } else if (status && status >= 500) {
      err.message = '服务暂时不可用，请稍后重试'
    } else if (status && status >= 400) {
      err.message = '请求参数有误，请检查后重试'
    }
    return Promise.reject(err)
  },
)
