import { http } from './http'

export type CloudDriveType = '115' | 'quark' | '123'

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
  listDrives: () =>
    http.get<{ drives: CloudDrive[] }>('/cloud/drives').then((r) => r.data.drives ?? []),
  createDrive: (d: DriveInput) => http.post<CloudDrive>('/cloud/drives', d).then((r) => r.data),
  updateDrive: (id: number, d: DriveInput) =>
    http.put<CloudDrive>(`/cloud/drives/${id}`, d).then((r) => r.data),
  removeDrive: (id: number) => http.delete(`/cloud/drives/${id}`).then((r) => r.data),
  test: (id: number) => http.post<TestResult>(`/cloud/drives/${id}/test`).then((r) => r.data),
  quota: (id: number) =>
    http.get<{ quota: CloudQuota }>(`/cloud/drives/${id}/quota`).then((r) => r.data.quota),
  browse: (id: number, path: string) =>
    http
      .get<{ files: CloudFile[] }>(`/cloud/drives/${id}/list`, { params: { path } })
      .then((r) => r.data.files ?? []),
  // ---- STRM 规则 ----
  listRules: () => http.get<{ rules: StrmRule[] }>('/cloud/rules').then((r) => r.data.rules ?? []),
  createRule: (r: RuleInput) => http.post<StrmRule>('/cloud/rules', r).then((res) => res.data),
  updateRule: (id: number, r: RuleInput) =>
    http.put<StrmRule>(`/cloud/rules/${id}`, r).then((res) => res.data),
  removeRule: (id: number) => http.delete(`/cloud/rules/${id}`).then((r) => r.data),
  generateRule: (id: number) =>
    http.post<StrmResult>(`/cloud/rules/${id}/generate`).then((r) => r.data),
  generateAll: () => http.post('/cloud/generate-all').then((r) => r.data),
  syncSubscriptions: () => http.post('/cloud/sync-subscriptions').then((r) => r.data),
  // ---- STRM 记录 ----
  listRecords: (driveId?: number) =>
    http
      .get<{ records: StrmRecord[] }>('/cloud/records', {
        params: driveId ? { drive_id: driveId } : {},
      })
      .then((r) => r.data.records ?? []),
  removeRecord: (id: number) => http.delete(`/cloud/records/${id}`).then((r) => r.data),
}

export const TYPE_LABEL: Record<CloudDriveType, string> = {
  '115': '115网盘',
  quark: '夸克网盘',
  '123': '123云盘',
}
