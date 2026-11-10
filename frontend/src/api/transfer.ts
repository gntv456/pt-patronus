// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

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
  status: 'success' | 'failed' | 'skipped'
  error_msg: string
  files: string
  manual: boolean
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
  path_mappings: TransferPathMapping[]
  failed_retry_minutes: number
  type_folder: boolean
  category_folder: boolean
  movie_rename: string
  tv_rename: string
  scrape: 'missing_only' | 'overwrite' | 'skip'
  refresh_library: boolean
  overwrite_files: boolean
  best_version: boolean
  cloud_upload: CloudUploadConfig
}

// 整理后自动上传网盘（对齐后端 transfer.CloudUpload）。
export interface CloudUploadConfig {
  enabled: boolean
  drive_id: number
  cloud_base: string
  upload_videos: boolean
  strm_output_dir: string
  strm_template: string
  delete_local: boolean
}

export interface TransferPathMapping {
  downloader_id: number
  remote_path: string
  local_path: string
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

export interface TransferPendingItem {
  downloader_id: number
  downloader_name: string
  hash: string
  name: string
  category: string
  save_path: string
  completed_at?: string
}

/** 整理待办汇总：真实待入库 + 失败历史 */
export interface TransferSummary {
  failed: number
  pending: number
  pending_items?: TransferPendingItem[]
  enabled: boolean
  scan_partial?: boolean
}

export const transferApi = {
  historyPage: (status = '', limit = 200) =>
    http
      .get<{ history: TransferHistory[]; total?: number; limit?: number; truncated?: boolean }>('/transfer/history', {
        params: { ...(status ? { status } : {}), limit },
      })
      .then((r) => ({
        history: r.data.history ?? [],
        total: r.data.total,
        limit: r.data.limit ?? limit,
        // 缺字段按「可能被截断」处理：单页封顶 500，别把一页说成全部整理记录。
        truncated: r.data.truncated ?? true,
      })),
  history: (status = '', limit = 200) => transferApi.historyPage(status, limit).then((p) => p.history),
  summary: (withItems = false, limit = 50) =>
    http
      .get<TransferSummary>('/transfer/summary', {
        params: { ...(withItems ? { items: '1' } : {}), limit },
      })
      .then((r) => r.data),
  getConfig: () => http.get<TransferConfig>('/transfer/config').then((r) => r.data),
  setConfig: (cfg: TransferConfig) => http.put('/transfer/config', cfg).then((r) => r.data),
  organize: (downloaderId: number, hash: string) =>
    http
      .post<{ result: OrganizeResult | null }>(
        '/transfer/organize',
        {
          downloader_id: downloaderId,
          hash,
        },
        // 手动整理/重做是同步长任务（识别+硬链+刮削），大库可远超默认 15s：不限时。
        { timeout: 0 },
      )
      .then((r) => r.data.result),
  redo: (id: number) =>
    http
      .post<{ result: OrganizeResult | null }>(`/transfer/redo/${id}`, null, { timeout: 0 })
      .then((r) => r.data.result),
  deleteHistory: (id: number) => http.delete(`/transfer/history/${id}`).then((r) => r.data),
  batchDeleteHistory: (ids: number[]) =>
    http.post<{ deleted: number; failed: number[] }>('/transfer/history/batch-delete', { ids }).then((r) => r.data),
  clearFailedHistory: () => http.post<{ deleted: number }>('/transfer/history/clear-failed').then((r) => r.data),
  sweep: () => http.post<{ ok: boolean; msg: string }>('/transfer/sweep', null).then((r) => r.data),
  // 目录监视配置（重启进程生效）。
  getDirwatchConfig: () => http.get<DirwatchConfig>('/transfer/dirwatch').then((r) => r.data),
  setDirwatchConfig: (cfg: DirwatchConfig) => http.put('/transfer/dirwatch', cfg).then((r) => r.data),
  // 自动分类规则（即时生效）。
  getCategoriesConfig: () => http.get<CategoryConfig>('/transfer/categories').then((r) => r.data),
  setCategoriesConfig: (cfg: CategoryConfig) => http.put('/transfer/categories', cfg).then((r) => r.data),
  // 识别词：整理识别前规整名称（发布组前缀/自定义别名），热生效。
  getIdentifyWords: () =>
    http.get<{ words: IdentifyWord[] }>('/transfer/identify-words').then((r) => r.data.words ?? []),
  setIdentifyWords: (words: IdentifyWord[]) => http.put('/transfer/identify-words', { words }).then((r) => r.data),
  // 目录标签（跨模块目录引用）
  getDirTags: () => http.get<{ tags: DirTag[] }>('/transfer/dir-tags').then((r) => r.data.tags ?? []),
  setDirTags: (tags: DirTag[]) => http.put('/transfer/dir-tags', { tags }).then((r) => r.data),
  // 种子排序规则（订阅/搜索择优权重）
  getSortRules: () => http.get<SortRules>('/transfer/sort-rules').then((r) => r.data),
  setSortRules: (r: SortRules) => http.put('/transfer/sort-rules', r).then((x) => x.data),
  // 目录同步任务（本地目录间 硬链/软链/复制/移动/STRM）
  getDirSync: () => http.get<{ tasks: DirSyncTask[] }>('/transfer/dir-sync').then((r) => r.data.tasks ?? []),
  setDirSync: (tasks: DirSyncTask[]) => http.put('/transfer/dir-sync', { tasks }).then((r) => r.data),
  runDirSync: (name: string) => http.post('/transfer/dir-sync/run', { name }, { timeout: 0 }).then((r) => r.data),
  testIdentifyWords: (name: string) =>
    http
      .post<{ normalized: string; title: string; season: string; episode: string }>('/transfer/identify-words/test', {
        name,
      })
      .then((r) => r.data),
  // 名称识别测试（对标 MSaber）：本地 ParseMeta + 可选媒体源匹配，验证文件名可识别性。
  testRecognize: (name: string, year = 0, media = false) =>
    http
      .post<{
        normalized: string
        title: string
        year: number
        season: number
        episode: number
        resolution: string
        group: string
        is_tv: boolean
        matched?: { title: string; year: number; type: string } | null
        // 媒体源查询失败的原因（与「没匹配上」分开：前者要去看源凭据/网络，后者才补识别词）。
        media_error?: string
      }>('/transfer/recognize/test', { name, year, media })
      .then((r) => r.data),
  // ---- 识别词云端共享（仓库 CRUD + 拉取预览 + 一键合并导入；本地优先不覆盖） ----
  getSharedRepos: () =>
    http.get<{ repos: SharedRepo[] }>('/transfer/identify-words/repos').then((r) => r.data.repos ?? []),
  setSharedRepos: (repos: SharedRepo[]) => http.put('/transfer/identify-words/repos', { repos }).then((r) => r.data),
  previewSharedRepo: (url: string) =>
    http
      .post<{ words: IdentifyWord[]; total: number; new: number }>('/transfer/identify-words/repos/preview', { url })
      .then((r) => r.data),
  importSharedRepo: (url: string) =>
    http.post<{ total: number; added: number }>('/transfer/identify-words/repos/import', { url }).then((r) => r.data),
}

// 共享词库仓库（对标 MSaber 识别词云端协作共享）。
export interface SharedRepo {
  name: string
  url: string
}

// 识别词（对齐后端 transfer.IdentifyWord）。
export interface IdentifyWord {
  kind: 'replace' | 'trim'
  from: string
  to: string
  note?: string
}

// 目录标签（对齐后端 transfer.DirTag）。
export interface DirTag {
  name: string
  kind: 'local' | 'cloud'
  drive_id?: number
  path: string
  note?: string
}

// 种子排序规则（对齐后端 transfer.SortRules）。
export interface SortRules {
  resolution: Record<string, number>
  group_bonus: Record<string, number>
  target_size_gb: number
  seeders_weight: number
  free_bonus: number
  free2x_bonus: number
  recent_days: number
  recent_bonus: number
}

// 目录同步任务（对齐后端 transfer.DirSyncTask）。
export interface DirSyncTask {
  name: string
  src_dir: string
  dst_dir: string
  mode: 'link' | 'softlink' | 'copy' | 'move' | 'strm'
  exts?: string[]
  recursive: boolean
  delete_src?: boolean
  enabled: boolean
}
