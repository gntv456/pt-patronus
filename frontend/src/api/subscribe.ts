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
  enable_rating_guard: boolean // 多评分低分保护：与全局 MediaQualityConfig.Enabled 取并集
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
}

export const subscribeApi = {
  list: () =>
    http.get<{ subscriptions: Subscription[] }>('/subscriptions').then((r) => r.data.subscriptions ?? []),
  create: (data: SubInput) => http.post<Subscription>('/subscriptions', data).then((r) => r.data),
  update: (id: number, patch: Partial<Pick<Subscription, 'resolution' | 'status' | 'downloader_id' | 'save_path' | 'enable_rating_guard'>>) =>
    http.put<Subscription>(`/subscriptions/${id}`, patch).then((r) => r.data),
  remove: (id: number) => http.delete(`/subscriptions/${id}`).then((r) => r.data),
  check: (id: number) =>
    http.post<{ ok: boolean; pushed: number; error?: string }>(`/subscriptions/${id}/check`).then((r) => r.data),
}
