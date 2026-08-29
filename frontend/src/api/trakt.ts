import { http } from './http'

// 对应后端 GET /trakt/config（脱敏视图）。
export interface TraktConfig {
  enabled: boolean
  client_id: string
  client_secret_configured: boolean
  redirect_uri: string
  sync_movies: boolean
  sync_shows: boolean
  resolution: string
  downloader_id: number
  authorized: boolean
  last_sync_at: number
  last_sync_count: number
  last_error: string
  pending_user_code: string
  pending_verification_url: string
  pending_expires_at: number
}

// 管理员提交的配置（client_secret 空串=保留旧值）。
export type TraktConfigInput = Omit<
  Partial<TraktConfig>,
  | 'client_secret_configured'
  | 'authorized'
  | 'last_sync_at'
  | 'last_sync_count'
  | 'last_error'
  | 'pending_user_code'
  | 'pending_verification_url'
  | 'pending_expires_at'
> & { client_secret?: string }

export interface TraktDeviceCode {
  user_code: string
  verification_url: string
  expires_in: number
  interval: number
  device_url: string
}

export interface TraktDeviceStatus {
  authorized: boolean
  user_code?: string
  verification_url?: string
  error?: string
}

export const traktApi = {
  getConfig: () => http.get<TraktConfig>('/trakt/config').then((r) => r.data),
  setConfig: (cfg: TraktConfigInput) => http.put('/trakt/config', cfg).then((r) => r.data),
  deviceCode: () => http.post<TraktDeviceCode>('/trakt/device-code').then((r) => r.data),
  deviceStatus: () => http.get<TraktDeviceStatus>('/trakt/device-status').then((r) => r.data),
  sync: () => http.post<{ ok: boolean; created: number }>('/trakt/sync').then((r) => r.data),
}
