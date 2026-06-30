import { http } from './http'

// RSS 订阅：feed 抓取→过滤→去重→(可选)下载→通知。

export interface RssSubscription {
  id: number
  name: string
  url: string
  site_id: number // 0=匿名 RSS；非 0=用站点 cookie 鉴权抓取
  enabled: boolean
  interval_min: number
  title_include: string
  title_exclude: string
  min_size: number // 字节，0 不过滤
  max_size: number
  resolution: string
  auto_download: boolean
  downloader_id: number // 0=默认下载器
  save_path: string
  category: string
  tags: string
  last_checked_at?: string
  created_at: string
  updated_at: string
}

export interface RssInput {
  name: string
  url: string
  site_id?: number
  enabled?: boolean
  interval_min?: number
  title_include?: string
  title_exclude?: string
  min_size?: number
  max_size?: number
  resolution?: string
  auto_download?: boolean
  downloader_id?: number
  save_path?: string
  category?: string
  tags?: string
}

export interface RssItem {
  id: number
  subscription_id: number
  item_key: string
  title: string
  link: string
  torrent_url: string
  size: number
  pub_date?: string
  downloaded: boolean
  created_at: string
}

export const rssApi = {
  list: () => http.get<{ subscriptions: RssSubscription[] }>('/rss').then((r) => r.data.subscriptions),
  create: (data: RssInput) => http.post<RssSubscription>('/rss', data).then((r) => r.data),
  update: (id: number, data: RssInput) => http.put<RssSubscription>(`/rss/${id}`, data).then((r) => r.data),
  remove: (id: number) => http.delete(`/rss/${id}`).then((r) => r.data),
  check: (id: number) =>
    http.post<{ ok: boolean; new_count: number; error?: string }>(`/rss/${id}/check`).then((r) => r.data),
  items: (id: number, limit = 100) =>
    http.get<{ items: RssItem[] }>(`/rss/${id}/items`, { params: { limit } }).then((r) => r.data.items),
}
