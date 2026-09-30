import { http } from './http'

// 通知 Source（对齐后端 notice.SourceXxx 枚举，缺失会导致通知中心标签/图标退化）。
export type NoticeSource =
  | 'site_anniv'
  | 'new_mail'
  | 'assess_pass'
  | 'assess_fail'
  | 'announce'
  | 'host_offline'
  | 'nas_offline'
  | 'reseed_done'
  | 'dl_done'
  | 'site_offline'
  | 'backup_done'
  | 'backup_import'
  | 'freeleech'
  | 'subscribe_hit'
  | 'workflow'
  | 'rss_hit'
  | 'webhook'
  | 'transfer_done'
  | 'transfer_fail'
  | 'subtitle_fix'
  | 'medal_new'
  | 'medal_available'
  | 'medal_price_drop'
  | 'medal_price_rise'
  | 'medal_out_of_stock'
export type NoticeLevel = 'info' | 'success' | 'warning' | 'danger'

export interface Notice {
  id: number
  source: NoticeSource
  key?: string
  title: string
  body?: string
  level: NoticeLevel
  read: boolean
  created_at: string
}

export const noticeApi = {
  list: (unread = false) =>
    http.get<{ notices: Notice[] }>(`/notices${unread ? '?unread=true' : ''}`).then((r) => r.data.notices ?? []),
  read: (id: number) => http.post(`/notices/${id}/read`).then((r) => r.data),
  readAll: () => http.post('/notices/read-all').then((r) => r.data),
  clear: () => http.post('/notices/clear').then((r) => r.data),
  delete: (id: number) => http.delete(`/notices/${id}`).then((r) => r.data),
}
