// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { http } from './http'
import { terr } from './i18nErr'

export type CloudDriveType =
  '115' | '115_open' | 'quark' | '123' | 'aliyun_open' | 'onedrive' | 'baidu' | 'tianyi' | 'mobile'

export interface CloudDrive {
  id: number
  name: string
  type: CloudDriveType
  root_path: string
  is_default: boolean
  created_at: string
  updated_at: string
}

export interface CloudFile {
  name: string
  path: string
  is_dir: boolean
  size: number
  mod_time: string
  pick_code?: string
}

export interface CloudQuota {
  used: number
  total: number
}

export interface StrmRule {
  id: number
  name: string
  drive_id: number
  cloud_path: string
  output_dir: string
  mount_root: string
  template: string
  recursive: boolean
  keep_ext: boolean
  enabled: boolean
  /** 输出目录绑定的目录标签（local 型）；非空时优先于 output_dir */
  output_tag?: string
  /** 扩展名白名单（逗号分隔，如 mkv,mp4；空=全部媒体） */
  include_exts?: string
  /** 文件名排除关键词（逗号分隔，如 sample,trailer） */
  exclude_keywords?: string
  /** 内网基址：模板 {direct} 优先用（空=回退全局 APP_BASE_URL） */
  local_base_url?: string
  /** 外网基址：模板 {directPublic} 用（空=同 {direct}） */
  public_base_url?: string
  /** 增量模式：本地 .strm 已存在即跳过 */
  incremental?: boolean
  created_at: string
  updated_at: string
}

export interface StrmRecord {
  id: number
  drive_id: number
  cloud_path: string
  strm_path: string
  size: number
  subscription_id: number
  created_at: string
}

export interface StrmResult {
  generated: number
  skipped: number
  errors: string[]
}

export interface TestResult {
  ok: boolean
  error?: string
  info?: string
}

// 输入：token 明文（创建必填，编辑空串=保留）；recursive/keep_ext/enabled 用指针语义（更新时空=不改）。
export type DriveInput = Partial<CloudDrive> & { token?: string }
export type RuleInput = Partial<StrmRule> & { token?: never }

export const cloudApi = {
  // ---- 网盘 ----
  listDrives: () => http.get<{ drives: CloudDrive[] }>('/cloud/drives').then((r) => r.data.drives ?? []),
  createDrive: (d: DriveInput) => http.post<CloudDrive>('/cloud/drives', d).then((r) => r.data),
  updateDrive: (id: number, d: DriveInput) => http.put<CloudDrive>(`/cloud/drives/${id}`, d).then((r) => r.data),
  removeDrive: (id: number) => http.delete(`/cloud/drives/${id}`).then((r) => r.data),
  test: (id: number) => http.post<TestResult>(`/cloud/drives/${id}/test`).then((r) => r.data),
  quota: (id: number) => http.get<{ quota: CloudQuota }>(`/cloud/drives/${id}/quota`).then((r) => r.data.quota),
  browse: (id: number, path: string) =>
    http.get<{ files: CloudFile[] }>(`/cloud/drives/${id}/list`, { params: { path } }).then((r) => r.data.files ?? []),
  // ---- STRM 规则 ----
  listRules: () => http.get<{ rules: StrmRule[] }>('/cloud/rules').then((r) => r.data.rules ?? []),
  createRule: (r: RuleInput) => http.post<StrmRule>('/cloud/rules', r).then((res) => res.data),
  updateRule: (id: number, r: RuleInput) => http.put<StrmRule>(`/cloud/rules/${id}`, r).then((res) => res.data),
  removeRule: (id: number) => http.delete(`/cloud/rules/${id}`).then((r) => r.data),
  generateRule: (id: number) =>
    // 递归生成 STRM 可能很久：不限时，覆盖 http 实例默认 15s。
    http.post<StrmResult>(`/cloud/rules/${id}/generate`, null, { timeout: 0 }).then((r) => r.data),
  // 即时生成 STRM（文件页选中目录直接生成，不持久化为规则）。
  generate: (opts: {
    drive_id: number
    cloud_path: string
    output_dir: string
    output_tag?: string
    mount_root: string
    template?: string
    recursive?: boolean
    keep_ext?: boolean
    include_exts?: string
    exclude_keywords?: string
    local_base_url?: string
    public_base_url?: string
  }) =>
    // 即时/递归生成 STRM 可能很久：不限时，覆盖 http 实例默认 15s。
    http.post<StrmResult>('/cloud/generate', opts, { timeout: 0 }).then((r) => r.data),
  // 全部启用规则一次性生成：后端回跨规则汇总（哪条规则整条失败也在里面），
  // 前端据此决定是成功、无规则、还是「N/M 条规则失败」——不能只回一句「已触发」。
  generateAll: () =>
    http
      .post<{ rules: number; generated: number; skipped: number; file_errors: number; failed?: string[] }>(
        '/cloud/generate-all',
        null,
        { timeout: 0 },
      )
      .then((r) => r.data),
  syncSubscriptions: () => http.post('/cloud/sync-subscriptions').then((r) => r.data),
  // ---- STRM 记录 ----
  listRecords: (driveId?: number) =>
    http
      .get<{ records: StrmRecord[] }>('/cloud/records', {
        params: driveId ? { drive_id: driveId } : {},
      })
      .then((r) => r.data.records ?? []),
  removeRecord: (id: number) => http.delete(`/cloud/records/${id}`).then((r) => r.data),
  // ---- 云端任务队列（上传/STRM/目录树导入） ----
  // 队列有单页上限（默认 50、封顶 200）：total/truncated 要与清单同批返回，否则积压中的队列
  // 被画成「就这些」。计数读不出时后端不下发 total 且保守报 truncated=true，所以 total 可选。
  listTasks: (status = '', limit = 50) =>
    http
      .get<{ tasks: CloudTask[]; total?: number; limit: number; truncated: boolean }>('/cloud/tasks', {
        params: { ...(status ? { status } : {}), limit },
      })
      .then((r) => r.data),
  // 重试任务（后端 soft-fail：200 + {ok:false,error}，这里转成异常，否则界面一律报「已重新排队」）
  retryTask: (id: number) =>
    http.post(`/cloud/tasks/${id}/retry`).then((r) => {
      if (!r.data.ok) throw new Error(r.data.error || terr('common.actionFailed', '操作失败'))
      return r.data
    }),
  removeTask: (id: number) => http.delete(`/cloud/tasks/${id}`).then((r) => r.data),
  // 115 目录树批量导入生成 STRM（异步任务，入队后由队列执行）。
  importTree: (opts: { drive_id: number; tree: string; base?: string; output_dir: string; template?: string }) =>
    http.post('/cloud/import-tree', opts, { timeout: 0 }).then((r) => r.data),
  // ---- 云端文件管理器 ----
  fileOp: (
    op: 'mkdir' | 'move' | 'copy' | 'rename' | 'delete' | 'offline' | 'share-save',
    body: { drive_id: number; path: string; dst_dir?: string; new_name?: string; url?: string; pwd?: string },
  ) => http.post(`/cloud/files/${op}`, body).then((r) => r.data),
  // 云端整理（识别→归位分类目录）
  organize: (opts: {
    drive_id: number
    src_dir: string
    dst_movie_dir?: string
    dst_tv_dir?: string
    rename?: boolean
  }) => http.post('/cloud/organize', opts, { timeout: 0 }).then((r) => r.data),
  // STRM 修复（按当前配置批量重写）
  strmRepair: (driveId?: number) =>
    http
      .post<{
        repaired: number
        skipped: number
        failed?: number
        errors?: string[]
        // 规则表这次读不出来：被保守跳过的那些 .strm 仍然是坏的（含在 skipped 里）
        rules_unavailable?: boolean
        rules_skipped?: number
      }>('/cloud/strm-repair', { drive_id: driveId ?? 0 }, { timeout: 0 })
      .then((r) => r.data),

  // 115 开放平台设备码授权：发起（返回扫码 URL）→ 轮询 → token 直接建盘
  start115Auth: (appId: string) =>
    http
      .post<{ uid: string; time: string; qrcode: string; sign: string; verifier: string }>('/cloud/115open/auth', {
        app_id: appId,
      })
      .then((r) => r.data),
  poll115Auth: (p: { uid: string; time: string; sign: string; verifier: string }) =>
    http.post<{ confirmed: boolean; status: number; token: string }>('/cloud/115open/poll', p).then((r) => r.data),
}

// 云端任务（对齐后端 models.CloudTask）。
export interface CloudTask {
  id: number
  drive_id: number
  kind: 'upload' | 'strm' | 'import_tree'
  status: 'pending' | 'running' | 'success' | 'failed'
  cloud_path: string
  local_path: string
  size: number
  progress: number
  error?: string
  attempts: number
  created_at: string
  updated_at: string
}

// 标签 map 的值经 terr() 解析（i18n key，中文兜底）——英文界面下不再是中文标签。
export const TYPE_LABEL: Record<CloudDriveType, string> = {
  '115_open': terr('cloud.type115Open', '115网盘（开放平台）'),
  '115': terr('cloud.type115Cookie', '115网盘（Cookie）'),
  aliyun_open: terr('cloud.typeAliyunOpen', '阿里云盘（开放平台）'),
  quark: terr('cloud.typeQuark', '夸克网盘'),
  '123': terr('cloud.type123', '123云盘'),
  onedrive: terr('cloud.typeOnedrive', 'OneDrive'),
  baidu: terr('cloud.typeBaidu', '百度网盘'),
  tianyi: terr('cloud.typeTianyi', '天翼云盘'),
  mobile: terr('cloud.typeMobile', '移动云盘'),
}

export const TASK_KIND_LABEL: Record<string, string> = {
  upload: terr('cloud.taskUpload', '上传'),
  strm: terr('cloud.taskStrm', 'STRM'),
  import_tree: terr('cloud.taskImportTree', '目录树导入'),
}

export const TASK_STATUS_LABEL: Record<string, string> = {
  pending: terr('cloud.statusPending', '排队中'),
  running: terr('cloud.statusRunning', '执行中'),
  success: terr('cloud.statusSuccess', '成功'),
  failed: terr('cloud.statusFailed', '失败'),
}
