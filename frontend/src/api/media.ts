import { http } from './http'

export interface CreditPerson {
  name: string
  role?: string
  avatar?: string
}

export interface MediaInfo {
  source: string
  source_id: string
  title: string
  year: number
  type: string // movie | tv | music
  overview?: string
  poster_url: string
  backdrop_url?: string
  rating?: number
  genres?: string[]
  // 富字段（Detail 填充）：
  runtime?: number // 分钟
  release_date?: string
  countries?: string[]
  directors?: string[]
  cast?: CreditPerson[]
  recommendations?: MediaInfo[]
}

export const mediaApi = {
  search: (keyword: string, year = 0) =>
    http.get<{ results: MediaInfo[] }>(`/media/search`, { params: { keyword, year } }).then((r) => r.data.results),
  detail: (source: string, id: string) =>
    http.get<{ result: MediaInfo }>(`/media/detail`, { params: { source, id } }).then((r) => r.data.result),
  poster: (keyword: string, year = 0) =>
    http.get<{ poster_url: string }>(`/media/poster`, { params: { keyword, year } }).then((r) => r.data.poster_url),
  nfo: (info: Partial<MediaInfo>) => http.post<{ nfo: string }>('/media/nfo', { info }).then((r) => r.data.nfo),
}

// —— 媒体源设置（TMDB/豆瓣/Bangumi 凭据，UI 可热改，回显时 secret 脱敏）——
export interface MediaSourceConfig {
  tmdb_api_key: string // 脱敏
  tmdb_api_key_configured: boolean
  tmdb_api_domain: string // 明文（非敏感）
  douban_secret: string // 脱敏
  douban_secret_configured: boolean
  douban_app_key: string // 脱敏
  douban_app_key_configured: boolean
  bangumi_token: string // 脱敏
  bangumi_token_configured: boolean
}

// 提交体：各字段空串=不修改（便于只改部分项而不必重输全部 secret）。
export type MediaSourceConfigInput = Partial<{
  tmdb_api_key: string
  tmdb_api_domain: string
  douban_secret: string
  douban_app_key: string
  bangumi_token: string
}>

export const mediaConfigApi = {
  get: () => http.get<MediaSourceConfig>('/media/config').then((r) => r.data),
  set: (cfg: MediaSourceConfigInput) => http.put<{ ok: boolean }>('/media/config', cfg).then((r) => r.data),
}
