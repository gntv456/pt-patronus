// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { http } from './http'

export interface SiteStatItem {
  site_id: number
  site_name: string
  username: string
  user_id: string
  email: string
  level: string
  uploaded: number
  downloaded: number
  ratio: number
  true_uploaded: number
  true_downloaded: number
  true_ratio: number
  seeding: number
  leeching: number
  seeding_size: number
  uploads: number
  unique_groups: number
  perfect_flac: number
  invited: number
  bonus: number
  bonus_per_hour: number
  seeding_bonus: number
  seeding_bonus_per_hour: number
  rescue_seeding_bonus_per_hour: number
  claim_count: number
  max_claim: number
  join_date: string
  last_access_at: string
  updated_at: string
}

export interface SiteHistoryPoint {
  snap_at: string
  level: string
  uploaded: number
  downloaded: number
  ratio: number
  true_uploaded: number
  true_downloaded: number
  true_ratio: number
  seeding: number
  leeching: number
  seeding_size: number
  uploads: number
  unique_groups: number
  perfect_flac: number
  invited: number
  bonus: number
  bonus_per_hour: number
  seeding_bonus: number
  seeding_bonus_per_hour: number
  rescue_seeding_bonus_per_hour: number
  claim_count: number
  max_claim: number
  last_access_at: string
}

// 单站历史快照 + 账面（对齐后端 chain.SiteHistoryResult）。
export interface SiteHistoryResult {
  points: SiteHistoryPoint[]
  days: number
  total: number
  truncated: boolean
}

// 某一天的跨站聚合趋势点（对齐后端 TrendPoint / mobile DashboardTrendPoint）。
export interface DashboardTrendPoint {
  day: string // "2026-06-19"
  uploaded: number
  downloaded: number
  seeding: number
  seeding_size: number
  bonus: number
  ratio: number
}

export interface DashboardOverview {
  totals: {
    uploaded: number
    downloaded: number
    true_uploaded: number
    true_downloaded: number
    true_ratio: number
    seeding: number
    leeching: number
    seeding_size: number
    unique_groups: number
    perfect_flac: number
    invited: number
    bonus: number
    bonus_per_hour: number
    seeding_bonus: number
    seeding_bonus_per_hour: number
    rescue_seeding_bonus_per_hour: number
    claim_count: number
    max_claim: number
  }
  sites: SiteStatItem[]
  site_count: number
  downloader_count: number
  no_data_sites: NoDataSite[]
  // 后端读「每站最新统计」失败时的原因（库锁/表异常）。非空时 no_data_sites 必然为空：
  // 读不到统计不等于站点没数据，两者必须分开渲染，否则用户会白跑一趟重登所有站。
  stat_read_error?: string
}

// 无采集数据的 PT 站点（启用、非论坛、却无任何统计；疑似 cookie 失效/未采集）。
export interface NoDataSite {
  site_id: number
  site_name: string
  platform: string
}

// ---- 工作台聚合（对标 MSaber 天序工作台）----

export interface WorkbenchNode {
  key: 'site' | 'subscribe' | 'download' | 'organize' | 'library' | 'watch'
  label: string
  count: number
  hint: string
}

export interface WorkbenchTodo {
  kind: 'transfer_failed' | 'no_data_site' | 'hr_risk' | string
  title: string
  detail?: string
  link?: string
  level: 'info' | 'warning' | 'danger' | string
}

export interface WorkbenchEvent {
  source: string
  title: string
  level: string
  created_at: string
  progress?: number
}

export interface MediaLibraryStat {
  server_id: number
  server_name: string
  type: string
  movies: number
  series: number
  library_count: number
  synced_at: string
}

export interface Workbench {
  nodes: WorkbenchNode[]
  todos: WorkbenchTodo[]
  events: WorkbenchEvent[]
  summary: {
    sites_active: number
    sites_no_data: number
    subscriptions_new: number
    subscription_hits: number
    downloads_done: number
    organize_failed: number
    cloud_tasks_done: number
    cloud_tasks_pending: number
    uploaded_delta: number
    downloaded_delta: number
    played_today: number
    strm_generated: number
  }
  media_library: { configured: boolean; servers: MediaLibraryStat[]; error?: string }
  health_score: number
  generated_at: string
  /** 某些区块读失败的原因（读不到 ≠ 没问题：待办/体检分已跳过这些块，需明示数据不完整） */
  errors?: string[]
}

export const dashboardApi = {
  overview: () => http.get<DashboardOverview>('/dashboard').then((r) => r.data),
  // 立即抓取所有启用站点统计并返回最新总览（回填新加入站点的 0 数据）。
  sync: () => http.post<DashboardOverview>('/dashboard/sync').then((r) => r.data),
  // 仅刷新单个站点统计（站点明细「刷新数据」按钮）。
  syncOne: (siteId: number) => http.post<DashboardOverview>(`/dashboard/sync/${siteId}`).then((r) => r.data),
  // 单站点历史统计快照（站点明细「用户历史数据」按钮）。
  // 后端单次最多返回 500 条，total/truncated 一起给出：只回数组会把「该区间 800 个快照」
  // 渲染成「只有 500 个」，历史表起点静默缺失。
  history: (siteId: number, days = 30) =>
    http
      .get<SiteHistoryResult>(`/dashboard/history/${siteId}`, { params: { days } })
      .then((r) => ({ ...r.data, points: r.data.points ?? [] })),
  // 按天聚合趋势（每站每日峰值跨站求和）：GET /dashboard/trend?days=N。
  trend: (days = 7) =>
    http
      .get<{ points: DashboardTrendPoint[] }>('/dashboard/trend', { params: { days } })
      .then((r) => r.data.points ?? []),
  // 工作台聚合：今日流转/待办/动态/摘要 + 媒体库态势 + 体检分。
  workbench: () => http.get<Workbench>('/dashboard/workbench').then((r) => r.data),
}
