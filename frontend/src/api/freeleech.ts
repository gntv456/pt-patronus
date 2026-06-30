import { http } from './http'

// 站免状态（对齐后端 site.FreeleechStatus）
export type FreeleechStatus = 'active' | 'upcoming' | 'none' | 'failed'

export interface SiteFreeleech {
  site_id: number
  site_name: string
  status: FreeleechStatus
  title?: string
  content?: string
  link?: string
  color?: string
  error?: string
  fetched_at: string
}

export interface FreeleechResult {
  site_id: number
  site_name: string
  notices: SiteFreeleech[]
  error?: string
}

export const freeleechApi = {
  all: (refresh = false) =>
    http
      .get<{ sites: FreeleechResult[] }>(`/freeleech${refresh ? '?refresh=1' : ''}`)
      // 后端 nil 切片序列化为 JSON null（未开启站免检测/抓空时），归一为 [] 防模板 .length/[0] 崩溃
      .then((r) => (r.data.sites || []).map((s) => ({ ...s, notices: s.notices || [] }))),
  syncOne: (id: number) =>
    http.post<FreeleechResult>(`/freeleech/sync/${id}`).then((r) => ({ ...r.data, notices: r.data.notices || [] })),
}
