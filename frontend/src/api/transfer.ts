import { http } from './http'

// 整理历史（对齐后端 models.TransferHistory）。
export interface TransferHistory {
  id: number
  src: string
  dest: string
  mode: string // link | move | copy | softlink
  type: string // movie | tv
  title: string
  year: number
  source: string
  source_id: string
  seasons: string
  episodes: string
  poster_url: string
  downloader_id: number
  download_hash: string
  status: 'success' | 'failed'
  error_msg: string
  files: string
  created_at: string
}

// 整理配置（对齐后端 transfer.Config）。
export interface TransferConfig {
  enabled: boolean
  mode: 'link' | 'softlink' | 'move' | 'copy'
  movie_dir: string
  tv_dir: string
  categories: string[]
  skip_tags: string[]
  organize_tag: string
  type_folder: boolean
  category_folder: boolean
  movie_rename: string
  tv_rename: string
  scrape: 'missing_only' | 'overwrite' | 'skip'
  refresh_library: boolean
  overwrite_files: boolean
  best_version: boolean
}

// 自动分类规则（对齐后端 transfer.CategoryConfig）。
export interface CategoryRule {
  genres?: string[]
  countries?: string[]
  year?: string
}
export interface NamedCategoryRule {
  name: string
  rule: CategoryRule
}
export interface CategoryConfig {
  movie: NamedCategoryRule[]
  tv: NamedCategoryRule[]
}

export interface OrganizeResult {
  Success: boolean
  ErrMsg: string
  Src: string
  DestDir: string
  Mode: string
  Files: string[]
  Type: string
  Seasons: string
  Episodes: string
}

export interface DirwatchConfig {
  enabled: boolean
  dirs: { path: string; library_path: string }[]
  monitor_mode: 'auto' | 'compatibility'
  debounce_ms: number
  poll_interval_sec: number
}

export const transferApi = {
  history: (status = '', limit = 200) =>
    http
      .get<{ history: TransferHistory[] }>('/transfer/history', {
        params: { ...(status ? { status } : {}), limit },
      })
      .then((r) => r.data.history ?? []),
  getConfig: () => http.get<TransferConfig>('/transfer/config').then((r) => r.data),
  setConfig: (cfg: TransferConfig) => http.put('/transfer/config', cfg).then((r) => r.data),
  organize: (downloaderId: number, hash: string) =>
    http.post<{ result: OrganizeResult | null }>('/transfer/organize', {
      downloader_id: downloaderId,
      hash,
    }).then((r) => r.data.result),
  redo: (id: number) =>
    http.post<{ result: OrganizeResult | null }>(`/transfer/redo/${id}`).then((r) => r.data.result),
  deleteHistory: (id: number) => http.delete(`/transfer/history/${id}`).then((r) => r.data),
  batchDeleteHistory: (ids: number[]) =>
    http.post<{ deleted: number; failed: number[] }>('/transfer/history/batch-delete', { ids }).then((r) => r.data),
  clearFailedHistory: () =>
    http.post<{ deleted: number }>('/transfer/history/clear-failed').then((r) => r.data),
  sweep: () => http.post('/transfer/sweep').then((r) => r.data),
  // 目录监视配置（重启进程生效）。
  getDirwatchConfig: () => http.get<DirwatchConfig>('/transfer/dirwatch').then((r) => r.data),
  setDirwatchConfig: (cfg: DirwatchConfig) => http.put('/transfer/dirwatch', cfg).then((r) => r.data),
  // 自动分类规则（即时生效）。
  getCategoriesConfig: () => http.get<CategoryConfig>('/transfer/categories').then((r) => r.data),
  setCategoriesConfig: (cfg: CategoryConfig) => http.put('/transfer/categories', cfg).then((r) => r.data),
}
