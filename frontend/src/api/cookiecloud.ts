import { http } from './http'

// 对应后端 GET /cookiecloud/config（脱敏视图，不回传密文密码）。
export interface CookieCloudConfig {
  enabled: boolean
  mode: 'external' | 'builtin' | string // external=外部服务器；builtin=内置服务端（PTPatronus 自身充当）
  server: string
  uuid: string
  password_configured: boolean
  interval_min: number
  update_mode: string
  direction: string
  last_sync_unix: number
  last_error: string
}

// 管理员提交的配置（password 空串=保留旧密码；regenerate=builtin 重生成凭证）。
export type CookieCloudConfigInput = Omit<
  Partial<CookieCloudConfig>,
  'password_configured' | 'last_sync_unix' | 'last_error'
> & { password?: string; regenerate?: boolean }

// setConfig 响应：builtin 首次保存/重生成时一次性回传明文密码（仅此一次）。
export interface CookieCloudSetResp {
  ok: boolean
  uuid: string
  password?: string
}

export interface CookieCloudSyncReport {
  updated: number
  added: number
  skipped: number
  pushed: number
  errors?: string[]
  fetched_at: string
}

export const cookieCloudApi = {
  getConfig: () => http.get<CookieCloudConfig>('/cookiecloud/config').then((r) => r.data),
  setConfig: (cfg: CookieCloudConfigInput) =>
    http.put<CookieCloudSetResp>('/cookiecloud/config', cfg).then((r) => r.data),
  sync: () => http.post<{ ok: boolean; report: CookieCloudSyncReport }>('/cookiecloud/sync').then((r) => r.data),
  push: () => http.post<{ ok: boolean; report: CookieCloudSyncReport }>('/cookiecloud/push').then((r) => r.data),
}
