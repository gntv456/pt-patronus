// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 仪表盘导出图片配置：字段显隐 / 站点筛选 / 数量 / 主题 / 脱敏 / 背景图，持久化到 localStorage。
// 对照重构前旧版(Kotlin) TotalDashboardConfig 的能力子集。配置驱动 dashboardImage.ts 绘制。

export type RankMetric = 'uploaded' | 'downloaded' | 'uploads' | 'seeding' | 'ratio' | 'bonus' | 'bonus_per_hour'
export type ExportTheme = 'dark' | 'light' | 'glass'

export interface DashboardExportConfig {
  fields: {
    uploaded: boolean
    downloaded: boolean
    seeding: boolean
    seedingSize: boolean
    bonus: boolean
    ratio: boolean
    uploadCount: boolean // 总发布数（新）
    bonusPerHour: boolean // 总时魔/h（新）
    earliestJoin: boolean // 最早入站+站点（新）
    ptAge: boolean
  }
  showSites: boolean // 每站明细区
  showIdentity: boolean // 每站明细里的用户名/ID/邮箱身份行
  showRank: boolean // 冠军排行区
  rankMetrics: RankMetric[] // 默认全 7 类
  selectedSiteIds: number[] // 站点白名单，空=全选
  siteLimit: number // 0=全部
  theme: ExportTheme
  blurSite: boolean // 站点名/网址 → ***
  blurUser: boolean // 顶部用户名 → ***
  backgroundImage: string // dataURL，空=无
  backgroundBlur: number // 0-20 px
  backgroundDim: number // 0-0.8 暗色遮罩
}

export const ALL_RANK_METRICS: RankMetric[] = [
  'uploaded',
  'downloaded',
  'uploads',
  'seeding',
  'ratio',
  'bonus',
  'bonus_per_hour',
]

export const defaultConfig: DashboardExportConfig = {
  fields: {
    uploaded: true,
    downloaded: true,
    seeding: true,
    seedingSize: true,
    bonus: true,
    ratio: true,
    uploadCount: true,
    bonusPerHour: true,
    earliestJoin: true,
    ptAge: true,
  },
  showSites: true,
  showIdentity: true,
  showRank: true,
  rankMetrics: [...ALL_RANK_METRICS],
  selectedSiteIds: [],
  siteLimit: 0,
  theme: 'dark',
  blurSite: false,
  blurUser: false,
  backgroundImage: '',
  backgroundBlur: 12,
  backgroundDim: 0.45,
}

const KEY = 'ptpatronus:exportConfig'

// 深拷贝默认配置（避免共享引用）。
function cloneDefault(): DashboardExportConfig {
  return {
    ...defaultConfig,
    fields: { ...defaultConfig.fields },
    rankMetrics: [...defaultConfig.rankMetrics],
    selectedSiteIds: [],
  }
}

// 读取并 merge 默认值容错（旧配置缺新字段时补齐）。
export function loadConfig(): DashboardExportConfig {
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw) return cloneDefault()
    const parsed = JSON.parse(raw) as Partial<DashboardExportConfig>
    return {
      ...cloneDefault(),
      ...parsed,
      fields: { ...defaultConfig.fields, ...(parsed.fields || {}) },
      rankMetrics: Array.isArray(parsed.rankMetrics) ? parsed.rankMetrics : [...defaultConfig.rankMetrics],
      selectedSiteIds: Array.isArray(parsed.selectedSiteIds) ? parsed.selectedSiteIds : [],
    }
  } catch {
    return cloneDefault()
  }
}

// 保存配置；返回 false 表示配额超限（多为背景图过大），调用方提示用户。
export function saveConfig(c: DashboardExportConfig): boolean {
  try {
    localStorage.setItem(KEY, JSON.stringify(c))
    return true
  } catch {
    return false
  }
}
