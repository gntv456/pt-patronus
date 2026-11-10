// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { http } from './http'

export interface AuthStatus {
  activated: boolean
  authenticated: boolean
  auth_site_on: boolean
  local_login?: boolean // 本地密码登录恒可用（好学站点关闭后的主路径）
  vip_verified: boolean
  demo_mode?: boolean
  oidc_enabled?: boolean // 管理员配置了 OIDC/SSO 且启用
  oidc_display_name?: string // 登录按钮文案（默认 "OIDC / SSO"）
  user?: HxptUser | null // authed 时附当前 user（刷新后恢复顶栏用户名 / VIP 状态）
  has_local_password?: boolean // 升级引导：已登录账号是否已设本地密码（false=存量好学账号）
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

export const authApi = {
  status: () => http.get<AuthStatus>('/auth/status').then((r) => r.data),

  // JWT 滑动续期（后端复用旧 token 首发时间、绝对寿命钳制；同步刷新 httpOnly cookie）。
  // http 拦截器在 401 时先尝试一次续期再重放原请求（见 http.ts）。
  refresh: () => http.post<LoginResult>('/auth/refresh').then((r) => r.data),

  // 本地密码登录（好学站点关闭后的主路径；首次登录自动建号，实例已有主人时拒绝第二账号）
  localLogin: (username: string, password: string) =>
    http
      .post<LoginResult>('/auth/local/login', { username, password })
      .then((r) => r.data),

  // 修改本地密码（guarded；需校验旧密码）
  changePassword: (oldPassword: string, newPassword: string) =>
    http.post('/auth/local/password', {
      old_password: oldPassword,
      new_password: newPassword,
    }),

  demoLogin: () => http.post<LoginResult>('/auth/demo-login').then((r) => r.data),

  // 登出（bump TokenVersion，使当前 token 失效）
  logout: () => http.post('/auth/logout'),

  // 一次性 WS/RTSP ticket（guarded 内用 header JWT 换取；再用 ?ticket= 连，避免 JWT 落 URL）
  wsTicket: () => http.post<{ ticket: string; expires_in: number }>('/auth/ws-ticket').then((r) => r.data),

  // legacy 激活码（保留兼容）
  activate: (code: string, deviceId = 'default') => http.post('/auth/activate', { code, device_id: deviceId }),

  // 用户界面偏好（跨浏览器同步，服务端按域存储；value 为 JSON 串）
  getPreferences: (key: string) =>
    http.get<{ key: string; value: string }>(`/auth/preferences/${key}`).then((r) => r.data),
  setPreferences: (key: string, value: unknown) =>
    http.put(`/auth/preferences/${key}`, { value: JSON.stringify(value ?? {}) }).then((r) => r.data),
}
