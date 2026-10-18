import { http } from './http'
import { terr } from './i18nErr'

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

// 目标目录读不出来时该弹什么：后端（文件管理器）是「同名即覆盖」的语义，所以
// 「预检没做成」绝不能被读成「目标没有同名项」——判定与文案收在这里，界面只负责弹。
export function targetReadFailedNotice(mode: 'move' | 'copy', sources: number): { title: string; body: string } {
  const verb = mode === 'move' ? '移动' : '复制'
  return {
    title: '无法确认目标是否有同名文件',
    body: `目标目录读取失败，无法确认是否已存在同名文件。继续${verb}将直接覆盖同名内容且不可恢复（共 ${sources} 项待处理），是否继续？`,
  }
}

export const fileApi = {
  list: () => http.get<{ storages: FileStorage[] }>('/storages').then((r) => r.data.storages ?? []),
  create: (input: FileStorageInput) => http.post<FileStorage>('/storages', input).then((r) => r.data),
  update: (id: number, input: FileStorageInput) => http.put(`/storages/${id}`, input).then((r) => r.data),
  remove: (id: number) => http.delete(`/storages/${id}`).then((r) => r.data),
  test: (id: number) => http.post<{ ok: boolean; error?: string }>(`/storages/${id}/test`).then((r) => r.data),
  files: (id: number, path = '/') =>
    http
      .get<{ entries: FileEntry[]; error?: string }>(`/storages/${id}/files`, { params: { path } })
      // 后端读不到目录时回 200 + {entries: [], error: '中文原因'}（filemanager.go:105）：
      // 状态码与空数组都不说话。丢掉 error 会让「读不到」被下游读成「这个目录是空的」——
      // 移动/复制的同名预检因此放行，而后端是同名静默覆盖（不可逆）。
      .then((r) => {
        if (r.data.error) throw new Error(r.data.error)
        return r.data.entries ?? []
      }),
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
  // 建目录（后端 soft-fail：200 + {ok:false,error}，这里转成异常）。
  mkdir: (id: number, path: string) =>
    http.post<{ ok: boolean; error?: string }>(`/storages/${id}/mkdir`, null, { params: { path } }).then((r) => {
      if (!r.data.ok) throw new Error(r.data.error || terr('common.actionFailed', '操作失败'))
    }),
  // 删除文件/目录（后端 soft-fail：200 + {ok:false,error}，这里转成异常）。
  removeFile: (id: number, path: string) =>
    http.post<{ ok: boolean; error?: string }>(`/storages/${id}/delete`, null, { params: { path } }).then((r) => {
      if (!r.data.ok) throw new Error(r.data.error || terr('apiErr.deleteFailed', '删除失败'))
    }),
  // 重命名（同目录改名）。
  rename: (id: number, path: string, name: string) =>
    http.post<{ ok: boolean; error?: string }>(`/storages/${id}/rename`, null, { params: { path, name } }).then((r) => {
      if (!r.data.ok) throw new Error(r.data.error || terr('apiErr.renameFailed', '重命名失败'))
    }),
  // 移动（from 源 → to 目标目录）。
  move: (id: number, from: string, to: string) =>
    http.post<{ ok: boolean; error?: string }>(`/storages/${id}/move`, null, { params: { from, to } }).then((r) => {
      if (!r.data.ok) throw new Error(r.data.error || terr('apiErr.moveFailed', '移动失败'))
    }),
  // 复制（from 源 → to 目标目录）。
  copy: (id: number, from: string, to: string) =>
    http.post<{ ok: boolean; error?: string }>(`/storages/${id}/copy`, null, { params: { from, to } }).then((r) => {
      if (!r.data.ok) throw new Error(r.data.error || terr('apiErr.copyFailed', '复制失败'))
    }),
  readFile: (id: number, path: string) =>
    http
      .get<{ ok: boolean; content?: string; error?: string }>(`/storages/${id}/read`, { params: { path } })
      .then((r) => {
        if (!r.data.ok) throw new Error(r.data.error || terr('apiErr.readFailed', '读取失败'))
        return r.data.content || ''
      }),
  writeFile: (id: number, path: string, content: string) =>
    http.post<{ ok: boolean; error?: string }>(`/storages/${id}/write`, { content }, { params: { path } }).then((r) => {
      if (!r.data.ok) throw new Error(r.data.error || terr('apiErr.saveFailed', '保存失败'))
    }),
  archive: (id: number, src: string, dst: string) =>
    http.post<{ ok: boolean; error?: string }>(`/storages/${id}/archive`, null, { params: { src, dst } }).then((r) => {
      if (!r.data.ok) throw new Error(r.data.error || terr('apiErr.archiveFailed', '压缩失败'))
    }),
  extract: (id: number, src: string, dst: string) =>
    http.post<{ ok: boolean; error?: string }>(`/storages/${id}/extract`, null, { params: { src, dst } }).then((r) => {
      if (!r.data.ok) throw new Error(r.data.error || terr('apiErr.extractFailed', '解压失败'))
    }),
  chmod: (id: number, path: string, mode: number, recursive: boolean) =>
    http
      .post<{ ok: boolean; error?: string }>(`/storages/${id}/chmod`, { mode, recursive }, { params: { path } })
      .then((r) => {
        if (!r.data.ok) throw new Error(r.data.error || terr('apiErr.chmodFailed', '修改权限失败'))
      }),
  dirSize: (id: number, path: string) =>
    http
      .get<{ ok: boolean; size?: number; size_str?: string; error?: string }>(`/storages/${id}/dirsize`, {
        params: { path },
      })
      .then((r) => {
        if (!r.data.ok) throw new Error(r.data.error || terr('apiErr.calcSizeFailed', '计算大小失败'))
        return { size: r.data.size || 0, sizeStr: r.data.size_str || '0 B' }
      }),
}
