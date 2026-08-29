import { http } from './http'
import { terr } from './i18nErr'
import { streamErr } from './sse'

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
      .get<{ sites: AnnouncementResult[] }>(`/announcements${refresh ? '?refresh=1' : ''}`, { timeout: 30000 })
      // 后端 nil 切片序列化为 JSON null（站点无公告/未配置时），归一为 [] 防模板 .length 崩溃
      .then((r) => (r.data.sites || []).map((s) => ({ ...s, items: s.items || [] }))),
  // 流式：每站抓完即推一行 NDJSON，前端逐站追加渲染，不等全部抓完（加载一个显示一个）。
  streamAll: (refresh = false, onResult: (r: AnnouncementResult) => void, signal?: AbortSignal): Promise<void> => {
    const token = localStorage.getItem('token')
    const url = `/api/v1/announcements/stream${refresh ? '?refresh=1' : ''}`
    return fetch(url, { headers: token ? { Authorization: `Bearer ${token}` } : {}, signal }).then(async (res) => {
      if (!res.ok) throw new Error(await streamErr(res))
      const reader = res.body!.getReader()
      const dec = new TextDecoder()
      let buf = ''
      for (;;) {
        const { done, value } = await reader.read()
        if (done) break
        buf += dec.decode(value, { stream: true })
        const lines = buf.split('\n')
        buf = lines.pop() ?? ''
        for (const line of lines) {
          const t = line.trim()
          if (t) {
            const r = JSON.parse(t) as AnnouncementResult
            onResult({ ...r, items: r.items || [] })
          }
        }
      }
    })
  },
  syncOne: (id: number) =>
    http.post<AnnouncementResult>(`/announcements/sync/${id}`).then((r) => ({ ...r.data, items: r.data.items || [] })),
}
