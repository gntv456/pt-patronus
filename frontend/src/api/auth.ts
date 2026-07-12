import { http } from './http'

export interface AuthStatus {
  activated: boolean
  authenticated: boolean
  auth_site_on: boolean
  vip_verified: boolean
  demo_mode?: boolean
  oidc_enabled?: boolean // 管理员配置了 OIDC/SSO 且启用
  oidc_display_name?: string // 登录按钮文案（默认 "OIDC / SSO"）
  user?: HxptUser | null // authed 时附当前 user（刷新后恢复顶栏用户名 / VIP 状态）
}

export interface HxptUser {
  id: number
  username: string
  hxpt_uid: string
  hxpt_username: string
  vip_level: number
  is_admin: boolean
}

export interface LoginResult {
  token: string
  user: HxptUser
}

// CaptchaChallenge 对应后端 /auth/hxpt/captcha 返回（账号登录 Step A）。
export interface CaptchaChallenge {
  imagehash: string
  image: string // data:image/png;base64,...；无验证码时为空
  need_captcha: boolean
  challenge_mode: boolean // true=好学开了挑战响应认证，后端复刻挑战算法自动处理；失败可改 cookie 登录
  extra_form?: Record<string, string>
}

export interface VIPStatus {
  verified: boolean
  id?: string
  level: string
  username: string
  message: string
}

export interface HxptLoginParams {
  username: string
  password: string
  imagehash?: string
  imagestring?: string
  two_step_code?: string
  extra_form?: Record<string, string>
  challenge_mode?: boolean // 由 captcha 步探得，回传驱动后端挑战响应分支
}

export const authApi = {
  status: () => http.get<AuthStatus>('/auth/status').then((r) => r.data),

  // 好学账号登录（两步：先 hxptCaptcha 取验证码，再 hxptLogin）
  hxptCaptcha: () => http.post<CaptchaChallenge>('/auth/hxpt/captcha').then((r) => r.data),
  hxptLogin: (p: HxptLoginParams) =>
    http.post<LoginResult>('/auth/hxpt/login', p).then((r) => r.data),

  // 好学 Cookie 登录（最稳，覆盖 2FA / 挑战响应账号）
  hxptCookieLogin: (cookie: string) =>
    http.post<LoginResult>('/auth/hxpt/cookie-login', { cookie }).then((r) => r.data),

  demoLogin: () => http.post<LoginResult>('/auth/demo-login').then((r) => r.data),

  // 登出（bump TokenVersion，使当前 token 失效）
  logout: () => http.post('/auth/logout'),

  // 一次性 WS/RTSP ticket（guarded 内用 header JWT 换取；再用 ?ticket= 连，避免 JWT 落 URL）
  wsTicket: () =>
    http.post<{ ticket: string; expires_in: number }>('/auth/ws-ticket').then((r) => r.data),

  // 好学 VIP 校验（公开端点，供前端探测）
  verifyVip: (cookie: string) =>
    http.post<VIPStatus>('/auth/verify-vip', { cookie }).then((r) => r.data),

  // legacy 激活码（保留兼容）
  activate: (code: string, deviceId = 'default') =>
    http.post('/auth/activate', { code, device_id: deviceId }),
}
