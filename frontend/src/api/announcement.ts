import { http } from './http'

export interface AnnouncementItem {
  title?: string
  date?: string
  content?: string
}

export interface AnnouncementResult {
  site_id: number
  site_name: string
  raw_text?: string
  items: AnnouncementItem[]
  fetched_at: string
  error?: string
}

export const announcementApi = {
  all: (refresh = false) =>
    http
      .get<{ sites: AnnouncementResult[] }>(`/announcements${refresh ? '?refresh=1' : ''}`)
      // 后端 nil 切片序列化为 JSON null（站点无公告/未配置时），归一为 [] 防模板 .length 崩溃
      .then((r) => (r.data.sites || []).map((s) => ({ ...s, items: s.items || [] }))),
  syncOne: (id: number) =>
    http.post<AnnouncementResult>(`/announcements/sync/${id}`).then((r) => ({ ...r.data, items: r.data.items || [] })),
}
