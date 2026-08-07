import { http } from './http'

export interface FileStorage {
  id: number
  name: string
  type: string // webdav | alist | smb | agent | local
  url: string
  username: string
  base_path: string
  agent_id: number
}

export interface FileStorageInput {
  name: string
  type: string
  url: string
  username?: string
  password?: string
  base_path?: string
  agent_id?: number
}

export interface FileEntry {
  name: string
  path: string
  is_dir: boolean
  size: number
  mod_time: string
}

export const fileApi = {
  list: () => http.get<{ storages: FileStorage[] }>('/storages').then((r) => r.data.storages ?? []),
  create: (input: FileStorageInput) => http.post<FileStorage>('/storages', input).then((r) => r.data),
  update: (id: number, input: FileStorageInput) => http.put(`/storages/${id}`, input).then((r) => r.data),
  remove: (id: number) => http.delete(`/storages/${id}`).then((r) => r.data),
  test: (id: number) => http.post<{ ok: boolean; error?: string }>(`/storages/${id}/test`).then((r) => r.data),
  files: (id: number, path = '/') =>
    http.get<{ entries: FileEntry[] }>(`/storages/${id}/files`, { params: { path } }).then((r) => r.data.entries ?? []),
  upload: (id: number, path: string, file: File, onProgress?: (percent: number) => void) => {
    const form = new FormData()
    form.append('path', path)
    form.append('file', file)
    return http
      .post(`/storages/${id}/upload`, form, {
        // 大文件上传不限时：覆盖 http 实例默认 15s，否则慢网/大文件必超时失败。
        timeout: 0,
        onUploadProgress: (ev) => {
          if (onProgress && ev.total) {
            onProgress(Math.round((ev.loaded / ev.total) * 100))
          }
        },
      })
      .then((r) => r.data)
  },
  downloadUrl: (id: number, path: string) => `/api/v1/storages/${id}/download?path=${encodeURIComponent(path)}`,
  mkdir: (id: number, path: string) =>
    http.post(`/storages/${id}/mkdir`, null, { params: { path } }).then((r) => r.data),
  // 删除文件/目录（后端 soft-fail：200 + {ok:false,error}，这里转成异常）。
  removeFile: (id: number, path: string) =>
    http
      .post<{ ok: boolean; error?: string }>(`/storages/${id}/delete`, null, { params: { path } })
      .then((r) => {
        if (!r.data.ok) throw new Error(r.data.error || '删除失败')
      }),
  // 重命名（同目录改名）。
  rename: (id: number, path: string, name: string) =>
    http
      .post<{ ok: boolean; error?: string }>(`/storages/${id}/rename`, null, { params: { path, name } })
      .then((r) => {
        if (!r.data.ok) throw new Error(r.data.error || '重命名失败')
      }),
  // 移动（from 源 → to 目标目录）。
  move: (id: number, from: string, to: string) =>
    http
      .post<{ ok: boolean; error?: string }>(`/storages/${id}/move`, null, { params: { from, to } })
      .then((r) => {
        if (!r.data.ok) throw new Error(r.data.error || '移动失败')
      }),
  // 复制（from 源 → to 目标目录）。
  copy: (id: number, from: string, to: string) =>
    http
      .post<{ ok: boolean; error?: string }>(`/storages/${id}/copy`, null, { params: { from, to } })
      .then((r) => {
        if (!r.data.ok) throw new Error(r.data.error || '复制失败')
      }),
  readFile: (id: number, path: string) =>
    http.get<{ ok: boolean; content?: string; error?: string }>(`/storages/${id}/read`, { params: { path } }).then((r) => {
      if (!r.data.ok) throw new Error(r.data.error || '????')
      return r.data.content || ''
    }),
  writeFile: (id: number, path: string, content: string) =>
    http.post<{ ok: boolean; error?: string }>(`/storages/${id}/write`, { content }, { params: { path } }).then((r) => {
      if (!r.data.ok) throw new Error(r.data.error || '????')
    }),
  archive: (id: number, src: string, dst: string) =>
    http.post<{ ok: boolean; error?: string }>(`/storages/${id}/archive`, null, { params: { src, dst } }).then((r) => {
      if (!r.data.ok) throw new Error(r.data.error || '????')
    }),
  extract: (id: number, src: string, dst: string) =>
    http.post<{ ok: boolean; error?: string }>(`/storages/${id}/extract`, null, { params: { src, dst } }).then((r) => {
      if (!r.data.ok) throw new Error(r.data.error || '????')
    }),
  chmod: (id: number, path: string, mode: number, recursive: boolean) =>
    http.post<{ ok: boolean; error?: string }>(`/storages/${id}/chmod`, { mode, recursive }, { params: { path } }).then((r) => {
      if (!r.data.ok) throw new Error(r.data.error || '??????')
    }),
  dirSize: (id: number, path: string) =>
    http.get<{ ok: boolean; size?: number; size_str?: string; error?: string }>(`/storages/${id}/dirsize`, { params: { path } }).then((r) => {
      if (!r.data.ok) throw new Error(r.data.error || '????')
      return { size: r.data.size || 0, sizeStr: r.data.size_str || '0 B' }
    }),
}
