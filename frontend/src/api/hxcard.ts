import { http } from './http'

// 对应后端 GET/PUT /hxcard/settings（secret 掩码回显，不回传明文）。
export interface HxCardSettings {
  enabled: boolean
  endpoint: string
  secret: string // 掩码；空=未配置
}

// PUT /hxcard/settings 请求体。secret：空=保持不变；"-"=清除。
export interface HxCardSettingsInput {
  enabled: boolean
  endpoint: string
  secret: string
}

// GET /hxcard/preview：将推送的聚合数据（不含站点明细以外的任何敏感字段）。
export interface HxCardPreview {
  client_version: string
  totals: { uploaded: number; downloaded: number; seeding_size: number; bonus: number }
  site_count: number
  sites: Array<{
    site: string
    uploaded: number
    downloaded: number
    seeding_size: number
    bonus: number
    seeding?: number
    ratio?: number
    level?: string
    updated_at?: string
  }>
}

export function getHxCardSettings() {
  return http.get<{ data: HxCardSettings }>('/hxcard/settings')
}

export function setHxCardSettings(input: HxCardSettingsInput) {
  return http.put<{ data: HxCardSettings }>('/hxcard/settings', input)
}

export function pushHxCard() {
  return http.post<{ success: boolean; message: string }>('/hxcard/push')
}

export function previewHxCard() {
  return http.get<{ data: HxCardPreview }>('/hxcard/preview')
}
