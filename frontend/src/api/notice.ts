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
  | 'camera_offline'
  | 'reseed_done'
  | 'dl_done'
  | 'site_offline'
  | 'backup_done'
  | 'backup_import'
  | 'freeleech'
  | 'subscribe_hit'
  | 'subscribe_list_fail'
  | 'rss_list_fail'
  | 'rss_check_fail'
  | 'strm_list_fail'
  | 'brushflow_list_fail'
  | 'contract_list_fail'
  | 'dirsync_list_fail'
  | 'brushflow'
  | 'cloud'
  | 'contract'
  | 'mediaserver'
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

/// 收件箱一页：除了数组还带取数账面（后端有单页上限，默认 200、封顶 1000）。
export interface NoticeListResponse {
  notices: Notice[]
  /** 该筛选下的通知总数；后端计数读不出时省略这个字段 */
  total?: number
  /** 服务端生效的 limit（请求值被归一后的真值） */
  limit: number
  truncated: boolean
  /** 未读总数（与本页筛选无关）；本页装不下全部未读时给角标兜底，读不出则省略 */
  unread_total?: number
}

export const noticeApi = {
  // 缺 truncated 字段按「可能被截断」处理：宁可多说一句，不把一页说成整个收件箱。
  listPage: (unread = false) =>
    http
      .get<NoticeListResponse>(`/notices${unread ? '?unread=true' : ''}`)
      .then((r) => ({ ...r.data, truncated: r.data.truncated ?? true })),
  list: (unread = false) => noticeApi.listPage(unread).then((p) => p.notices ?? []),
  read: (id: number) => http.post(`/notices/${id}/read`).then((r) => r.data),
  readAll: () => http.post('/notices/read-all').then((r) => r.data),
  clear: () => http.post('/notices/clear').then((r) => r.data),
  delete: (id: number) => http.delete(`/notices/${id}`).then((r) => r.data),
}
