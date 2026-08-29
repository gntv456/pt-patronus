import axios from 'axios'
import { terr } from './i18nErr'

// 会话令牌封装（收权加固）：登录响应由后端同时种 httpOnly cookie（XSS 读不到，
// 同源请求自动携带），localStorage 的 Bearer 仅作过渡兜底（扩展打开的 Web 页/
// 旧会话）。authenticated 的判定依据两通道任一存在：cookie 无法从 JS 探测，
// 由 auth store 在启动时调 /auth/status 探测后写入 sessionStorage 标记。
export function hasBearerToken(): boolean {
  return !!localStorage.getItem('token')
}
export function clearBearerToken() {
  localStorage.removeItem('token')
}

// 401/403 会话失效的统一处理：清双通道（Bearer + cookie 标记 + user 快照），
// 带上 redirect 便于登录后回原页；redirecting 去重防并发多请求重复跳转。
// 导出供 SSE 等绕过 axios 拦截器的流式请求复用同一语义。
let redirecting = false
export function expireSession() {
  localStorage.removeItem('token')
  localStorage.removeItem('user')
  sessionStorage.removeItem('ptp_cookie_session')
  if (!redirecting && location.pathname !== '/login') {
    redirecting = true
    const redirect = encodeURIComponent(location.pathname + location.search)
    location.href = `/login?redirect=${redirect}`
  }
}

// 统一 HTTP 客户端：注入 JWT；401（含登出后 token_version_mismatch）/403 VIP 准入失败 → 清 token 回登录。
export const http = axios.create({
  baseURL: '/api/v1',
  timeout: 15000,
})

http.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) config.headers.Authorization = `Bearer ${token}`
  // 无 Bearer 时依赖 httpOnly cookie（withCredentials 同源默认携带）
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
    // 401：token 无效/过期/已登出（token_version_mismatch）——统一会话失效处理
    if (status === 401) expireSession()
    // 403 VIP 准入失败（vip_expired/vip_not_verified/not_vip）：清 token，要求重新登录验证
    if (status === 403 && ['vip_expired', 'vip_not_verified', 'vip_unknown', 'not_vip'].includes(code)) {
      expireSession()
    }
    // 错误友好化：改写 err.message 为 i18n 文案，组件 catch 显示 err.message 时即友好。
    // 后端已给 error/message 时优先用，否则按状态码/错误类型兜底（英文界面不再混中文）。
    const backend = err.response?.data?.error || err.response?.data?.message
    if (typeof backend === 'string' && backend) {
      err.message = backend
    } else if (err.code === 'ECONNABORTED' || /timeout/i.test(err.message || '')) {
      err.message = terr('apiErr.timeout', '请求超时，请稍后重试')
    } else if (!err.response) {
      err.message = terr('apiErr.network', '网络连接失败，请检查网络')
    } else if (status === 429) {
      err.message = terr('apiErr.tooMany', '操作过于频繁，请稍后再试')
    } else if (status === 404) {
      err.message = terr('apiErr.notFound', '请求的资源不存在')
    } else if (status && status >= 500) {
      err.message = terr('apiErr.server', '服务暂时不可用，请稍后重试')
    } else if (status && status >= 400) {
      err.message = terr('apiErr.badRequest', '请求参数有误，请检查后重试')
    }
    return Promise.reject(err)
  },
)
