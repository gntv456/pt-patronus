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
  filter_groups: string // 绑定过滤规则组名（| 分隔，空=不过滤）
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
  filter_groups?: string
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
  list: () => http.get<{ subscriptions: RssSubscription[] }>('/rss').then((r) => r.data.subscriptions ?? []),
  create: (data: RssInput) => http.post<RssSubscription>('/rss', data).then((r) => r.data),
  update: (id: number, data: RssInput) => http.put<RssSubscription>(`/rss/${id}`, data).then((r) => r.data),
  remove: (id: number) => http.delete(`/rss/${id}`).then((r) => r.data),
  // 成功面必须带扫描账面（scanned/total/truncated）：只回 new_count 时，「源共 500 条只查了
  // 窗口内 200 条」与「整个源查完了」在界面同形。
  check: (id: number) =>
    http
      .post<
        | { ok: true; new_count: number; scanned: number; total: number; truncated: boolean }
        | { ok: false; new_count: number; error?: string }
      >(`/rss/${id}/check`)
      .then((r) => r.data),
  // total 在计数读取失败时不下发（后端按「可能被截断」保守报 truncated=true），
  // 所以这里必须是可选字段，界面不能拿 `?? 0` 把它伪造成「一共 0 条」。
  items: (id: number, limit = 100) =>
    http
      .get<{ items: RssItem[]; total?: number; limit: number; truncated: boolean }>(`/rss/${id}/items`, {
        params: { limit },
      })
      .then((r) => r.data),
}
