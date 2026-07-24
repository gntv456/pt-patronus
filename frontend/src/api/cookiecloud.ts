import { http } from './http'

// 对应后端 GET /cookiecloud/config（脱敏视图，不回传密文密码）。
export interface CookieCloudConfig {
  enabled: boolean
  server: string
  uuid: string
  password_configured: boolean
  interval_min: number
  update_mode: string
  direction: string
  last_sync_unix: number
  last_error: string
}

// 管理员提交的配置（password 空串=保留旧密码）。
export type CookieCloudConfigInput = Omit<
  Partial<CookieCloudConfig>,
  'password_configured' | 'last_sync_unix' | 'last_error'
> & { password?: string }

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
  setConfig: (cfg: CookieCloudConfigInput) => http.put('/cookiecloud/config', cfg).then((r) => r.data),
  sync: () => http.post<{ ok: boolean; report: CookieCloudSyncReport }>('/cookiecloud/sync').then((r) => r.data),
  push: () => http.post<{ ok: boolean; report: CookieCloudSyncReport }>('/cookiecloud/push').then((r) => r.data),
}
