import { http } from './http'

// 辅种候选任务（对齐后端 models.CrossSeedTask）。
export interface ReseedTask {
  id: number
  downloader_id: number
  info_hash: string
  source_site: string
  source_name: string
  candidate_site: string
  candidate_url: string
  save_path: string
  status: 'pending' | 'added' | 'skipped' | 'failed'
  match_score: number
  created_at: string
  resolved_at: string | null
}

export interface ReseedConfig {
  iyuu_token_masked: string
  configured: boolean
  torznab_url?: string
  torznab_key_masked?: string
  torznab_configured?: boolean
}

// 辅种设置（per-downloader，对齐后端 models.CrossSeedSetting）。
export interface ReseedSetting {
  downloader_id: number
  save_path: string
  pause_after_add: boolean
  auto_reseed: boolean
  auto_reseed_add: boolean
}

// 未辅种检查结果（对齐后端 chain.MissingSummary）。
export interface MissingSample {
  hash: string
  name: string
}
export interface MissingSeed {
  site_id: number
  site_name: string
  count: number // -1 = 该站未映射到 IYUU 目录，无法判定
  samples: MissingSample[]
}
export interface MissingSummary {
  downloader_id: number
  total: number
  checked: number
  targets: number
  missing: MissingSeed[]
}

export const reseedApi = {
  getConfig: () => http.get<ReseedConfig>('/keepseed/config').then((r) => r.data),
  setConfig: (body: { iyuu_token?: string; torznab_url?: string; torznab_apikey?: string; set_torznab?: boolean }) =>
    http.put('/keepseed/config', body).then((r) => r.data),
  // 触发某下载器的 IYUU 精确辅种，返回新增 pending 候选。
  reseed: (downloaderId: number) =>
    http
      .post<{ tasks: ReseedTask[]; count: number }>('/keepseed/reseed', { downloader_id: downloaderId })
      .then((r) => r.data),
  // 列候选任务（status 空则全部）。后端单次最多返回 500 条，total/truncated 一起给出：
  // 只回 tasks 会让界面把「最近 500 条」当成「全部候选」，清完后还谎报「没有待处理候选」。
  tasks: (status = '', siteId = 0) =>
    http
      .get<{ tasks: ReseedTask[]; total?: number; truncated?: boolean }>('/keepseed/tasks', {
        params: {
          ...(status ? { status } : {}),
          ...(siteId > 0 ? { site_id: siteId } : {}),
        },
      })
      .then((r) => ({
        tasks: r.data.tasks ?? [],
        total: typeof r.data.total === 'number' ? r.data.total : null,
        truncated: !!r.data.truncated,
      })),
  // 处理一个候选：add（添加辅种）/ skip。
  resolve: (id: number, action: 'add' | 'skip') =>
    http.post(`/keepseed/tasks/${id}/resolve`, { action }).then((r) => r.data),
  // 取/存该下载器的辅种设置（自动辅种 / 自动添加 / 保存路径 / 添加后暂停）。
  getSetting: (downloaderId: number) =>
    http.get<ReseedSetting>('/keepseed/setting', { params: { downloader_id: downloaderId } }).then((r) => r.data),
  setSetting: (s: ReseedSetting) => http.put('/keepseed/setting', s).then((r) => r.data),
  // 未辅种检查：对某下载器做种，检查目标站是否已辅种（IYUU hash 跨站匹配）。
  missing: (downloaderId: number, targetSiteIds: number[]) =>
    http
      .post<MissingSummary>('/keepseed/missing', {
        downloader_id: downloaderId,
        target_site_ids: targetSiteIds,
      })
      .then((r) => r.data),
}
