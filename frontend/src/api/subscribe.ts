import { http } from './http'

// 订阅（对标 MoviePilot 订阅）：cron 跨站搜种子→推送默认下载器+通知。
export interface Subscription {
  id: number
  source: string // tmdb | douban | bangumi
  source_id: string
  title: string
  year: number
  type: string // movie | tv
  category: string
  poster_url: string
  resolution: string // 可选：1080p/2160p，空=不过滤
  downloader_id: number // 0=默认下载器
  save_path: string
  enable_rating_guard: boolean | null // 三态：null=跟随全局默认；true/false=订阅自定义
  best_version: boolean | null // 三态：null=跟随全局默认；true/false=订阅自定义
  best_version_full: boolean // 洗版整季包优先模式
  official_only: boolean | null // 三态：null=跟随全局默认；true=仅收官种；false=不过滤
  promo_types: string[] // 接受的促销类型多选（free/2xfree/2x/discount/2xdiscount）；空=不过滤
  site_ids: number[] // 参与站点 id；空=全部活跃站点
  current_priority: number // 电影当前最优 PriOrder（0=未下过）
  episode_priority: string // 剧集按集 PriOrder JSON
  status: string // subscribed | completed | paused
  last_checked_at?: string
  created_at: string
}

export interface SubInput {
  source: string
  source_id: string
  title: string
  year?: number
  type?: string
  category?: string
  poster_url?: string
  resolution?: string
  best_version?: boolean
  official_only?: boolean
  promo_types?: string[]
  site_ids?: number[]
}

// 全局订阅默认设置：未单独设置某项的订阅在检查时回退到此（运行时回退）。
export interface GlobalDefaults {
  resolution: string // 空=不限
  promo_types: string[] // 空=不限
  site_ids: number[] // 空=全部活跃站点
  enable_rating_guard: boolean // 全局二态，默认 false
  best_version: boolean // 全局二态，默认 false
  official_only: boolean // 全局二态，默认 false（开启后订阅只收官种）
}

export const subscribeApi = {
  list: () =>
    http.get<{ subscriptions: Subscription[] }>('/subscriptions').then((r) => r.data.subscriptions ?? []),
  create: (data: SubInput) => http.post<Subscription>('/subscriptions', data).then((r) => r.data),
  update: (id: number, patch: Partial<Pick<Subscription, 'resolution' | 'status' | 'downloader_id' | 'save_path' | 'enable_rating_guard' | 'best_version' | 'best_version_full' | 'official_only' | 'promo_types' | 'site_ids'>>) =>
    http.put<Subscription>(`/subscriptions/${id}`, patch).then((r) => r.data),
  remove: (id: number) => http.delete(`/subscriptions/${id}`).then((r) => r.data),
  check: (id: number) =>
    http.post<{ ok: boolean; pushed: number; error?: string }>(`/subscriptions/${id}/check`).then((r) => r.data),
  // 全局订阅默认设置（未单独设置的订阅回退到此）
  getGlobalDefaults: () => http.get<GlobalDefaults>('/subscribe/settings').then((r) => r.data),
  updateGlobalDefaults: (d: GlobalDefaults) => http.put<GlobalDefaults>('/subscribe/settings', d).then((r) => r.data),
  // 清空订阅指定字段回退全局默认（enable_rating_guard/best_version/resolution/promo_types/site_ids）
  reset: (id: number, fields: string[]) =>
    http.post<Subscription>(`/subscriptions/${id}/reset`, { fields }).then((r) => r.data),
  // 批量操作（对标 MSaber）：op=update(patch 字段级编辑)/check(重新搜索)/delete(取消)。
  // 逐条执行单条失败不影响其余，返回成功/失败汇总与失败明细。
  batch: (ids: number[], op: 'update' | 'check' | 'delete', patch?: SubInput) =>
    http.post<{ total: number; succeeded: number; failed: number; failures: { id: number; error: string }[] }>(
      '/subscriptions/batch', { ids, op, patch },
    ).then((r) => r.data),
}
