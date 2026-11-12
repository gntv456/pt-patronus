// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { http } from './http'
import { streamSSE } from './sse'

// Backend check-in uses a 30s per-site context and runs batch work in groups of
// eight. Keep these timeouts local to check-in requests so ordinary API calls
// retain the default 15s timeout.
const checkinTimeout = 45_000
const checkinAllTimeout = 5 * 60_000
// 站点定义诊断端点。URL 提常量后，下面那条链折成三行时每行都在 80 列内；
// 内联长 URL 会让 prettier 必须换行，而换出来的 .get<…>('<长 URL>') 那行仍超 80 列，
// 会把本文件的超宽行计数从 15 抬到 16（棘轮只许减不许增）。
const defDiagPath = '/sites/definition-diagnostics'

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
  success?: number // 累计成功次数（供展示成功率）
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

// 站点求种请求（NexusPHP viewsrequests.php）。
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

// 站点定义诊断（后端 internal/site 的 DefinitionDiagnostic）：四类「配置被静默忽略」。
// kind 新增值时前端映射表要同步（utils/siteDefinitionDiag.ts），否则界面会退到「其它」。
export interface DefinitionDiagnostic {
  kind: string
  site?: string
  file?: string
  detail: string
}

export const siteApi = {
  list: () => http.get<{ sites: Site[] }>('/sites').then((r) => r.data.sites ?? []),
  definitions: () => http.get<{ definitions: SiteDef[] }>('/sites/definitions').then((r) => r.data.definitions ?? []),
  // 站点定义诊断（独立端点：只有几十条，不必重读 300+ 条定义）。
  // 读失败不在这里吞掉——交回调用方决定怎么出声（#292/#317 同族教训）。
  // 这里刻意折成三行：本仓的门禁是「80 列只许减不许增」，比 prettier 的 printWidth 更严
  // （整文件在 HEAD 就不过 prettier --check），收成一行会多出一个超宽行、把棘轮顶高。
  definitionDiagnostics: () =>
    http
      .get<{ diagnostics?: DefinitionDiagnostic[] }>(defDiagPath)
      .then((r) => r.data.diagnostics ?? []),
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
        else if (event === 'done') {
          // list_error=站点清单读失败（0 帧 status），throw 留痕给视图 catch 弹错
          if (data?.list_error) throw new Error(data.list_error)
          onDone?.()
        }
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
  // 后端「站点清单读不出来」时会回 {results: [], list_error:'原因'}：
  // 只取 results 就把它讲成「没有启用的站点」（#258），所以整份信封交回调用方判。
  checkinAll: () =>
    http
      .post<{ results?: CheckinResult[]; list_error?: string }>('/sites/checkin', undefined, {
        timeout: checkinAllTimeout,
      })
      .then((r) => ({ results: r.data.results ?? [], listError: r.data.list_error ?? '' })),
  records: (siteId?: number, limit = 100, offset = 0) => {
    const params = new URLSearchParams({ limit: String(limit), offset: String(offset) })
    if (siteId) params.set('site_id', String(siteId))
    return http
      .get<{
        records: CheckinRecord[]
        total?: number
        limit?: number
        offset?: number
        truncated?: boolean
      }>(`/sites/checkin/records?${params.toString()}`)
      .then((r) => ({
        records: r.data.records ?? [],
        total: r.data.total,
        limit: r.data.limit ?? limit,
        offset: r.data.offset ?? offset,
        // 缺字段按「可能被截断」处理：不把这一页说成「签到史到此为止」。
        truncated: r.data.truncated ?? true,
      }))
  },
  search: (id: number, keyword: string) =>
    http
      .post<{ torrents: unknown[] }>(`/sites/${id}/search?keyword=${encodeURIComponent(keyword)}`)
      .then((r) => r.data.torrents ?? []),
}
