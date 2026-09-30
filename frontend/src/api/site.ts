import { http } from './http'
import { streamSSE } from './sse'

// Backend check-in uses a 30s per-site context and runs batch work in groups of
// eight. Keep these timeouts local to check-in requests so ordinary API calls
// retain the default 15s timeout.
const checkinTimeout = 45_000
const checkinAllTimeout = 5 * 60_000

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
  checked_at?: string // 上次检测时刻（后端 site_connectivity 缓存；ISO 字符串）
  success?: number // 累计成功次数（对标 MP SiteStatistic，供展示成功率）
  fail?: number // 累计失败次数
  avg_seconds?: number // 平均成功响应耗时（秒）
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

// 站点求种请求（对标 MSaber「站点请求」；NexusPHP viewsrequests.php）。
export interface SiteRequestItem {
  id: string
  title: string
  category?: string
  comments: number
  votes: number
  filled: boolean
  created_by?: string
  time_added?: string
  detail_url?: string
  fetched_at: string
}

export interface SiteRequestResult {
  site_id: number
  site_name: string
  requests: SiteRequestItem[]
  error?: string
}

export const siteApi = {
  list: () => http.get<{ sites: Site[] }>('/sites').then((r) => r.data.sites ?? []),
  definitions: () => http.get<{ definitions: SiteDef[] }>('/sites/definitions').then((r) => r.data.definitions ?? []),
  create: (input: SiteInput) => http.post<Site>('/sites', input).then((r) => r.data),
  update: (id: number, input: SiteInput) => http.put<Site>(`/sites/${id}`, input).then((r) => r.data),
  // 站点硬删不可恢复（凭据随之失），后端要求 confirm_risk 二次确认（UI 已有弹窗）。
  remove: (id: number) => http.delete(`/sites/${id}`, { data: { confirm_risk: true } }).then((r) => r.data),
  batchDelete: (ids: number[]) =>
    http
      .post<{ deleted: number; failed: number[] }>('/sites/batch-delete', { ids, confirm_risk: true })
      .then((r) => r.data),
  test: (id: number) => http.post<TestResult>(`/sites/${id}/test`).then((r) => r.data),
  // GET 返回上次检测缓存（瞬时、不发网络请求），进页面瞬时回显「上次检测结果」。
  statuses: () => http.get<{ statuses: SiteConnectivityStatus[] }>('/sites/status').then((r) => r.data.statuses ?? []),
  // 流式逐站检测（SSE）：每站完成推一帧 event:status，结束推 event:done。
  // 实现「检测到一个更新一个」；signal 用于切页中断。返回 streamSSE 的 Promise（done 后 resolve）。
  statusesStream: (onStatus: (s: SiteConnectivityStatus) => void, onDone?: () => void, signal?: AbortSignal) =>
    streamSSE(
      '/api/v1/sites/status/stream',
      (event, data) => {
        if (event === 'status') onStatus(data as SiteConnectivityStatus)
        else if (event === 'done') onDone?.()
      },
      signal,
    ),
  checkin: (id: number) =>
    http.post<CheckinResult>(`/sites/${id}/checkin`, undefined, { timeout: checkinTimeout }).then((r) => r.data),
  // 站点求种请求列表（filled=1 已满足段；siteId 指定单站，空=全部活跃站）
  siteRequests: (filled = false, siteId?: number) =>
    http
      .get<{ sites: SiteRequestResult[] }>('/site-requests', {
        params: { ...(filled ? { filled: '1' } : {}), ...(siteId ? { site_id: siteId } : {}) },
        timeout: 0,
      })
      .then((r) => r.data.sites ?? []),
  checkinAll: () =>
    http
      .post<{ results: CheckinResult[] }>('/sites/checkin', undefined, { timeout: checkinAllTimeout })
      .then((r) => r.data.results ?? []),
  records: (siteId?: number, limit = 100, offset = 0) => {
    const params = new URLSearchParams({ limit: String(limit), offset: String(offset) })
    if (siteId) params.set('site_id', String(siteId))
    return http
      .get<{ records: CheckinRecord[] }>(`/sites/checkin/records?${params.toString()}`)
      .then((r) => r.data.records ?? [])
  },
  search: (id: number, keyword: string) =>
    http
      .post<{ torrents: unknown[] }>(`/sites/${id}/search?keyword=${encodeURIComponent(keyword)}`)
      .then((r) => r.data.torrents ?? []),
}
