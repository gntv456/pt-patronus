import { http } from './http'

export interface SiteDef {
  id: string
  name: string
  schema: string
  dead: boolean
  urls?: string[] // 站点真实地址（首个作添加站点默认 base_url）
  aka?: string[] // 中文名别名（很多站 name 是英文，靠 aka 才能按中文名检索）
  description?: string
  tags?: string[]
}

export interface Site {
  id: number
  name: string
  platform: string
  definition: string
  base_url: string
  username: string
  is_active: boolean
  priority: number
  limit_interval: number
  created_at: string
  game_count?: number // 已适配娱乐项目数（仅 GET /entertainment 返回）
}

export interface SiteInput {
  name: string
  platform: string
  definition: string
  base_url: string
  cookie?: string
  api_key?: string
  passkey?: string
  username?: string
  password?: string
  is_active?: boolean
  priority?: number
  limit_interval?: number
}

export interface TestResult {
  ok: boolean
  level?: string
  error?: string
}

export type SiteConnectivityState = 'connected' | 'disconnected' | 'disabled'

export interface SiteConnectivityStatus {
  site_id: number
  site_name: string
  status: SiteConnectivityState
  level?: string
  error?: string
}

export interface CheckinResult {
  site_id: number
  site_name: string
  success: boolean
  message?: string
  bonus: number
  error?: string
}

export interface CheckinRecord {
  id: number
  site_id: number
  site_name: string
  success: boolean
  message?: string
  bonus: number
  error?: string
  source: string // cron | manual
  created_at: string
}

export const siteApi = {
  list: () => http.get<{ sites: Site[] }>('/sites').then((r) => r.data.sites ?? []),
  definitions: () =>
    http.get<{ definitions: SiteDef[] }>('/sites/definitions').then((r) => r.data.definitions ?? []),
  create: (input: SiteInput) => http.post<Site>('/sites', input).then((r) => r.data),
  update: (id: number, input: SiteInput) => http.put<Site>(`/sites/${id}`, input).then((r) => r.data),
  remove: (id: number) => http.delete(`/sites/${id}`).then((r) => r.data),
  batchDelete: (ids: number[]) =>
    http.post<{ deleted: number; failed: number[] }>('/sites/batch-delete', { ids }).then((r) => r.data),
  test: (id: number) => http.post<TestResult>(`/sites/${id}/test`).then((r) => r.data),
  statuses: () =>
    http
      .post<{ statuses: SiteConnectivityStatus[] }>('/sites/status', undefined, { timeout: 60_000 })
      .then((r) => r.data.statuses ?? []),
  checkin: (id: number) => http.post<CheckinResult>(`/sites/${id}/checkin`).then((r) => r.data),
  checkinAll: () =>
    http.post<{ results: CheckinResult[] }>('/sites/checkin').then((r) => r.data.results ?? []),
  records: (siteId?: number, limit = 100, offset = 0) => {
    const params = new URLSearchParams({ limit: String(limit), offset: String(offset) })
    if (siteId) params.set('site_id', String(siteId))
    return http
      .get<{ records: CheckinRecord[] }>(`/sites/checkin/records?${params.toString()}`)
      .then((r) => r.data.records ?? [])
  },
  search: (id: number, keyword: string) =>
    http.post<{ torrents: unknown[] }>(`/sites/${id}/search?keyword=${encodeURIComponent(keyword)}`).then(
      (r) => r.data.torrents ?? [],
    ),
}
