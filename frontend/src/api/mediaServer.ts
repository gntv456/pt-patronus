import { http } from './http'

export type MediaServerType = 'emby' | 'jellyfin' | 'plex'

export interface MediaServer {
  id: number
  name: string
  type: MediaServerType
  url: string
  username: string
  is_default: boolean
  refresh_on_download: boolean
  notify_on_play: boolean
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

export const mediaServerApi = {
  list: () =>
    http.get<{ servers: MediaServer[] }>('/mediaserver').then((r) => r.data.servers ?? []),
  create: (s: Partial<MediaServer> & { token?: string }) =>
    http.post<MediaServer>('/mediaserver', s).then((r) => r.data),
  update: (id: number, s: Partial<MediaServer> & { token?: string }) =>
    http.put<MediaServer>(`/mediaserver/${id}`, s).then((r) => r.data),
  remove: (id: number) => http.delete(`/mediaserver/${id}`).then((r) => r.data),
  test: (id: number) =>
    http.post<TestResult>(`/mediaserver/${id}/test`).then((r) => r.data),
  libraries: (id: number) =>
    http.get<{ libraries: MediaLibrary[] }>(`/mediaserver/${id}/libraries`).then((r) => r.data.libraries ?? []),
  scan: (id: number, libraryId?: string) =>
    http.post(`/mediaserver/${id}/scan`, { library_id: libraryId ?? '' }).then((r) => r.data),
  refresh: (id: number, libraryId: string) =>
    http.post(`/mediaserver/${id}/refresh`, { library_id: libraryId }).then((r) => r.data),
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
}
