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
      .get<{ sites: FreeleechResult[] }>(`/freeleech${refresh ? '?refresh=1' : ''}`, { timeout: 60000 })
      // 后端 nil 切片序列化为 JSON null（未开启站免检测/抓空时），归一为 [] 防模板 .length/[0] 崩溃
      .then((r) => (r.data.sites || []).map((s) => ({ ...s, notices: s.notices || [] }))),
  // 流式：每站抓完即推一行 NDJSON，前端逐站追加渲染，不等全部抓完（加载一个显示一个）。
  // 治首次进页面阻塞等全量超过默认 15s timeout 而显示失败的问题（与公告 streamAll 同范式）。
  // fetch 无 axios 整体 timeout（逐站 flush 保活）；signal 用于切页/重载中断。
  streamAll: (
    refresh = false,
    onResult: (r: FreeleechResult) => void,
    signal?: AbortSignal,
  ): Promise<void> => {
    const token = localStorage.getItem('token')
    const url = `/api/v1/freeleech/stream${refresh ? '?refresh=1' : ''}`
    return fetch(url, { headers: token ? { Authorization: `Bearer ${token}` } : {}, signal }).then(async (res) => {
      if (!res.ok) throw new Error(`加载失败 (HTTP ${res.status})`)
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
            const r = JSON.parse(t) as FreeleechResult
            onResult({ ...r, notices: r.notices || [] })
          }
        }
      }
    })
  },
  syncOne: (id: number) =>
    http.post<FreeleechResult>(`/freeleech/sync/${id}`).then((r) => ({ ...r.data, notices: r.data.notices || [] })),
}
