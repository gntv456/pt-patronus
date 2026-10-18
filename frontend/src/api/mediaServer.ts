import { http } from './http'

export type MediaServerType = 'emby' | 'jellyfin' | 'plex' | 'ugos' | 'fnos' | 'zspace'

export interface MediaServer {
  id: number
  name: string
  type: MediaServerType
  url: string
  username: string
  is_default: boolean
  refresh_on_download: boolean
  notify_on_play: boolean
  direct_link_enabled: boolean
  direct_link_base: string
  created_at: string
  updated_at: string
}

export interface MediaLibrary {
  id: string
  name: string
  type: string
}

export interface TestResult {
  ok: boolean
  error?: string
  info?: string
}

// 观影报告（PlaybackReport）
export interface PlayItem {
  item_id: string
  name: string
  type: string
  year: number
  user_id: string
  user_name: string
  last_played: string
  play_count: number
  duration: number
}
export interface PlaybackUser {
  id: string
  name: string
  play_count: number
}
export interface PlaybackReport {
  server: string
  generated_at: string
  days: number
  users: PlaybackUser[]
  recent_plays: PlayItem[]
  top_items: PlayItem[]
  total_plays: number
}

// 重复媒体检测
export interface DetailedItem {
  item_id: string
  name: string
  type: string
  year: number
  tmdb_id: string
  imdb_id: string
  path: string
}
export interface DuplicateGroup {
  key: string
  title: string
  year: number
  type: string
  count: number
  items: DetailedItem[]
}

export const mediaServerApi = {
  list: () => http.get<{ servers: MediaServer[] }>('/mediaserver').then((r) => r.data.servers ?? []),
  create: (s: Partial<MediaServer> & { token?: string }) =>
    http.post<MediaServer>('/mediaserver', s).then((r) => r.data),
  update: (id: number, s: Partial<MediaServer> & { token?: string }) =>
    http.put<MediaServer>(`/mediaserver/${id}`, s).then((r) => r.data),
  remove: (id: number) => http.delete(`/mediaserver/${id}`).then((r) => r.data),
  test: (id: number) => http.post<TestResult>(`/mediaserver/${id}/test`).then((r) => r.data),
  libraries: (id: number) =>
    http.get<{ libraries: MediaLibrary[] }>(`/mediaserver/${id}/libraries`).then((r) => r.data.libraries ?? []),
  scan: (id: number, libraryId?: string) =>
    http
      .post<{ ok: boolean; scanned?: number; error?: string }>(`/mediaserver/${id}/scan`, {
        library_id: libraryId ?? '',
      })
      .then((r) => r.data),
  refresh: (id: number, libraryId: string) =>
    http
      .post<{ ok: boolean; error?: string }>(`/mediaserver/${id}/refresh`, { library_id: libraryId })
      .then((r) => r.data),
  notify: (id: number, title: string, body: string) =>
    http.post(`/mediaserver/${id}/notify`, { title, body }).then((r) => r.data),
  // 封面拼图直链（<img src> 用，带 Authorization 不便 → 由同源代理透传 token；这里仅给 URL 工具）
  coverURL: (id: number, libraryId: string, cols = 3, rows = 3) =>
    `/api/v1/mediaserver/${id}/cover?library_id=${encodeURIComponent(libraryId)}&cols=${cols}&rows=${rows}`,
  // 封面拼图（带 JWT 拉取为 blob，前端 objectURL 渲染；<img src> 无法带 Authorization 头）。
  coverBlob: (id: number, libraryId: string, cols = 3, rows = 3) =>
    http
      .get<Blob>(`/mediaserver/${id}/cover`, {
        params: { library_id: libraryId, cols, rows },
        responseType: 'blob',
      })
      .then((r) => r.data),
  // 观影报告（近 N 天播放统计）
  playbackReport: (id: number, days = 30) =>
    http.get<PlaybackReport>(`/mediaserver/${id}/playback-report`, { params: { days } }).then((r) => r.data),
  // 重复媒体检测（某库）
  duplicates: (id: number, libraryId: string) =>
    http
      .get<{ groups: DuplicateGroup[] }>(`/mediaserver/${id}/duplicates`, {
        params: { library_id: libraryId },
      })
      .then((r) => r.data.groups ?? []),
  // 媒体健康中心：全服务器巡检（未匹配/缺集/缺字幕/可升级洗版）；
  // subtitles=1 开启本地字幕扫描，upgradeable=1 开启洗版检测（同 TMDB 高低版本）
  health: (subtitles = false, upgradeable = false) =>
    http
      .get<HealthReport>('/mediaserver/health', {
        params: { ...(subtitles ? { subtitles: '1' } : {}), ...(upgradeable ? { upgradeable: '1' } : {}) },
        timeout: 0,
      })
      .then((r) => r.data),
  // 一键补齐缺失字幕（body 传健康报告；空 body 服务端现算全库）。
  // 只回受理账面：queued=0 表示本轮一部都没排进队列（也不会来通知），必须当终态播报。
  fixSubtitles: (report?: HealthReport, lang = 'chs') =>
    http
      .post<SubtitleFixAccepted>('/mediaserver/fix-subtitles', report ?? {}, { params: { lang }, timeout: 0 })
      .then((r) => r.data),
}

// 字幕补齐的受理回执（后台执行，最终结果见通知）。
export interface SubtitleFixAccepted {
  queued: number
  running: boolean
}

// 媒体健康报告（对齐后端 chain.HealthReport）。
export interface HealthIssue {
  server_id: number
  library: string
  item_id: string
  title: string
  type: 'unmatched' | 'missing_episodes' | 'missing_subtitle' | 'upgradeable'
  path?: string
  detail: string
}

export interface HealthReport {
  issues: HealthIssue[]
  unmatched: number
  missing_episodes: number
  missing_subtitle: number
  upgradeable: number
  checked_at: string
  // 未覆盖到的服务器/库（连不上、读不出）。非空时「0 问题」不代表健康。
  errors?: string[]
}
