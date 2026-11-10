// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import axios from 'axios'
import { terr } from './i18nErr'
import { translateErrcode } from './errcodeI18n'
import { i18n } from '@/i18n'

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

// ---- JWT 滑动续期（对接后端 /auth/refresh）----
// 两层配合：
//   1) 主动：定时器在 token 过期前续期（App 启动时 startTokenRefresher）；
//   2) 被动：401 拦截器先静默续期一次并重放原请求（防定时器窗口漏网，如休眠唤醒）。
// 登出/token_version_mismatch（真失效）不续：code 白名单区分「过期可续」与「失效须重登」。
let refreshing: Promise<boolean> | null = null

// tryRefresh：静默续期一次。成功返回 true 并落位新 token（cookie 由后端同步刷新）。
// 并发 401 共享同一次续期请求（single-flight）；失败/false 不抛出，由调用方决定后续。
export function tryRefresh(): Promise<boolean> {
  if (!refreshing) {
    refreshing = axios
      .post<{ token: string }>('/api/v1/auth/refresh', null, {
        // 续期走裸 axios：不能带过期 Bearer（后端按 cookie/旧 token 验签后重发），
        // 也不能复用 http 实例（会再次进本拦截器造成递归）。
        withCredentials: true,
        timeout: 10000,
        _skipAuth: true,
      } as never)
      .then((r) => {
        if (r.data?.token) {
          localStorage.setItem('token', r.data.token)
          return true
        }
        return false
      })
      .catch(() => false)
      .finally(() => {
        // 下一轮 401 可再次尝试（本次失败可能只是网络抖动）。
        setTimeout(() => (refreshing = null), 3000)
      })
  }
  return refreshing
}

// 请求拦截器跳过标记（refresh 自身不带过期 Bearer）。
declare module 'axios' {
  interface AxiosRequestConfig {
    _skipAuth?: boolean
  }
}

http.interceptors.request.use((config) => {
  if (config._skipAuth) return config
  const token = localStorage.getItem('token')
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

// 定时主动续期：每 30 分钟一次（JWT 默认有效期远大于此；后端滑动续期含绝对寿命钳制，
// 到绝对寿命时 refresh 会 401 → 下次请求走被动路径回登录）。页面不可见时跳过本轮。
export function startTokenRefresher() {
  if ((window as unknown as { __ptpRefresher?: boolean }).__ptpRefresher) return
  ;(window as unknown as { __ptpRefresher?: boolean }).__ptpRefresher = true
  window.setInterval(() => {
    if (document.visibilityState !== 'visible') return
    if (!localStorage.getItem('token') && sessionStorage.getItem('ptp_cookie_session') !== '1') return
    void tryRefresh()
  }, 30 * 60 * 1000)
}


// readErrorEnvelope 把错误响应体读成 {code,message,error} 信封；Blob 先取文本。
// 非 JSON（反代返回的 HTML 错误页等）一律回 null，让调用方走状态码兜底，
// 不把上游 HTML 塞进用户提示里。
async function readErrorEnvelope(data: any): Promise<any | null> {
  if (data instanceof Blob) {
    try {
      const parsed = JSON.parse(await blobText(data))
      return parsed && typeof parsed === 'object' ? parsed : null
    } catch {
      return null
    }
  }
  if (data && typeof data === 'object') return data
  return null
}

// Blob.text() 在部分环境（老 WebKit、jsdom）不存在，退回 FileReader；
// 两个真实 blob 端点的失败原因都要能读出来。
function blobText(b: Blob): Promise<string> {
  if (typeof (b as any).text === 'function') return (b as any).text()
  return new Promise((resolve, reject) => {
    const r = new FileReader()
    r.onload = () => resolve(String(r.result ?? ''))
    r.onerror = () => reject(r.error)
    r.readAsText(b)
  })
}

http.interceptors.response.use(
  (r) => r,
  async (err) => {
    const status = err.response?.status
    // responseType:'blob' 的端点（封面拼图 / 字幕下载）出错时 body 也是 Blob：
    // 直接 err.response.data.code 永远是 undefined，后端写好的中文原因会被换成
    // 「服务暂时不可用」「请求参数有误」这类按状态码兜底的话——等于原因一定丢失。
    // 所以先把 Blob 读成文本再试解析信封。
    const body = await readErrorEnvelope(err.response?.data)
    // 机器可读错误码（后端统一信封 {code, message, error}）：透传到 err.code 供业务分支。
    // 不改变下方文案优先级——code 只做编程判断，message 仍走 error/message 双字段回退。
    const code = body?.code
    if (typeof code === 'string' && code) err.code = code
    // 401 先尝试滑动续期再重放一次（原请求非 refresh 自身；token_version_mismatch 是
    // 真失效——登出/改绑，续期也会被拒，不重放直接走会话失效）。
    if (status === 401 && !err.config?._isRetry && code !== 'token_version_mismatch' && !err.config?._skipAuth) {
      const ok = await tryRefresh()
      if (ok) {
        const cfg = err.config
        cfg._isRetry = true
        const token = localStorage.getItem('token')
        if (token) cfg.headers = { ...cfg.headers, Authorization: `Bearer ${token}` }
        return http.request(cfg)
      }
      expireSession()
    }
    // 401（续期失败/token_version_mismatch）：token 无效/过期/已登出——统一会话失效处理
    else if (status === 401) expireSession()
    // 好学 VIP 准入闸门已退役，不再有 VIP 类 403 需要清会话；403 统一按权限错误处理。
    // 错误友好化：改写 err.message 为 i18n 文案，组件 catch 显示 err.message 时即友好。
    // 后端已给 error/message 时优先用，否则按状态码/错误类型兜底（英文界面不再混中文）。
    const backend = body?.error || body?.message
    if (typeof backend === 'string' && backend) {
      // #355 后端给了文案时按 code 翻成当前界面语言：英文界面下这一档原本直接显示
      // 中文（后端信封总是带 message），i18n 那套 apiErr.* 兜底几乎永远走不到。
      // 查不到 code 对应的译文就原样返回后端文案——那是链层写的唯一线索，不许吞。
      err.message = translateErrcode(code, backend, i18n.global.locale.value as string, body?.specific === true)
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
