import type { DashboardExportConfig, ExportTheme, RankMetric } from './dashboardExportConfig'

export interface DashboardShareSite {
  site_id: number
  site_name: string
  username: string
  user_id: string
  email: string
  level: string
  uploaded: number
  downloaded: number
  ratio: number
  seeding: number
  leeching: number
  seeding_size: number
  uploads: number
  bonus: number
  bonus_per_hour: number
  join_date: string
  updated_at: string
  base_url: string // 拼 favicon.ico 用
}

export interface DashboardShareData {
  username: string
  siteCount: number
  downloaderCount: number
  totals: {
    uploaded: number
    downloaded: number
    seeding: number
    seeding_size: number
    bonus: number
  }
  sites: DashboardShareSite[] // 全量，按 config 过滤/排序/截断
}

// 通用：导出主文件与拆分模块共享的布局/派生类型
export interface NumField {
  key: string
  label: string
  color: string
  value: string
}

export interface RankDef {
  key: RankMetric
  label: string
  color: string
  pick: (s: DashboardShareSite) => number
  fmt: (v: number) => string
}

export interface Champion {
  def: RankDef
  site: DashboardShareSite
  value: number
}

export interface ThemeColors {
  text: string
  sub: string
  muted: string
  divider: string
  levelBg: string
  levelText: string
  base: string // 兜底底色
}

export const FONT = "'PingFang SC', 'Microsoft YaHei', 'Hiragino Sans GB', system-ui, -apple-system, sans-serif"
export const W = 760
export const PAD = 40
