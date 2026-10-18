import { http } from './http'
import { terr } from './i18nErr'
import { streamErr } from './sse'

export type Mailbox = 'inbox' | 'sent' | 'system' | 'cheater' | 'reports' | 'staff'

export interface AdminActions {
  reply_url?: string
  delete_url?: string
  item_id?: string
}

export interface Message {
  id: number
  site_id: number
  site_name: string
  sender: string
  title: string
  date: string
  url: string
  is_read: boolean
  content?: string
  mailbox: Mailbox
  subtitle?: string
  admin_actions?: AdminActions
}

export interface MessageResult {
  site_id: number
  site_name: string
  messages: Message[]
  error?: string
  // 后端 omitempty：false 时字段缺席。true = 这一站的私信压根没适配（配置事实），
  // 与「抓到了，这站没私信」区分开，别渲染成空信箱。
  unsupported?: boolean
}

export interface ReplyResult {
  success: boolean
  message: string
}

export interface BatchResult {
  success: number
  failed: number
  errors?: string[]
}

export const messageApi = {
  all: (mailbox: Mailbox = 'inbox', refresh = false) =>
    http
      .get<{ sites: MessageResult[] }>(`/messages?mailbox=${mailbox}${refresh ? '&refresh=1' : ''}`, { timeout: 30000 })
      // 后端 nil 切片序列化为 JSON null（站点无消息/未配置时），归一为 [] 防模板 .length 崩溃
      .then((r) => (r.data.sites || []).map((s) => ({ ...s, messages: s.messages || [] }))),
  // 流式：每站抓完即推一行 NDJSON，前端逐站追加渲染，不等全部抓完（加载一个显示一个）。
  streamAll: (
    mailbox: Mailbox = 'inbox',
    refresh = false,
    onResult: (r: MessageResult) => void,
    signal?: AbortSignal,
  ): Promise<void> => {
    const token = localStorage.getItem('token')
    const url = `/api/v1/messages/stream?mailbox=${mailbox}${refresh ? '&refresh=1' : ''}`
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
            // 末尾 done 行带 list_error=站点清单读失败（0 行输出），throw 留痕给视图 catch 弹错
            const r = JSON.parse(t) as MessageResult & { done?: boolean; list_error?: string }
            if (r.done) {
              if (r.list_error) throw new Error(r.list_error)
              continue
            }
            onResult({ ...r, messages: r.messages || [] })
          }
        }
      }
    })
  },
  syncOne: (id: number, mailbox: Mailbox = 'inbox') =>
    http
      .post<MessageResult>(`/messages/sync/${id}?mailbox=${mailbox}`)
      .then((r) => ({ ...r.data, messages: r.data.messages || [] })),
  content: (siteId: number, url: string) =>
    http
      .get<{ content: string }>(`/messages/${siteId}/content?url=${encodeURIComponent(url)}`)
      .then((r) => r.data.content),
  reply: (siteId: number, url: string, content: string) =>
    http.post<ReplyResult>(`/messages/${siteId}/reply`, { url, content }).then((r) => r.data),
  remove: (siteId: number, url: string) =>
    http.post<{ ok: boolean }>(`/messages/${siteId}/delete`, { url }).then((r) => r.data),
  markRead: (siteId: number, ids: string[]) =>
    http.post<BatchResult>(`/messages/${siteId}/markread`, { ids }).then((r) => r.data),
}
