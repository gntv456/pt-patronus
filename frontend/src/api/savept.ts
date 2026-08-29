import { http } from './http'

export type SiteHealthStatus = 'healthy' | 'critical' | 'closed' | 'unknown'
export type MatchType = 'none' | 'url_exact' | 'domain_exact' | 'name_exact' | 'name_fuzzy'
export type AlertType = 'danger' | 'obituary' | 'warning' | 'info' | 'celebration'

export interface SaveptSiteCard {
  name: string
  url: string
  domain: string
  icon?: string
  year: string
  status: SiteHealthStatus
  status_text?: string
  site_type: string
  anniversary_text?: string
  anniversary_flag?: string
  duration_text?: string
  fault_duration?: string
  open_time?: string
  close_time?: string
  search?: string
  match_type: MatchType
  local_site_id?: number
  local_site_name?: string
}

export interface SaveptAlert {
  type: AlertType
  title: string
  content: string
}

export interface SaveptSummary {
  total: number
  healthy: number
  critical: number
  closed: number
  bound: number
  available: number
  internal: number
  external: number
  today_anniv: number
  years: number
  mp_owned: number
  mp_available: number
  mp_unsupported: number
}

export interface SaveptResult {
  summary: SaveptSummary
  alerts: SaveptAlert[]
  cards: SaveptSiteCard[]
  fetched_at: string
  cached: boolean
  stale?: boolean // 降级：抓取失败时返回的缓存快照（可能已过期）
  notice?: string // 降级/提示文案
}

export const saveptApi = {
  get: (refresh = false) => http.get<SaveptResult>(`/savept${refresh ? '?refresh=1' : ''}`).then((r) => r.data),
}
