import { http } from './http'

export type SiteHealthStatus = 'healthy' | 'critical' | 'closed' | 'unknown'
export type MatchType = 'none' | 'url_exact' | 'domain_exact' | 'name_exact' | 'name_fuzzy'
export type AlertType = 'danger' | 'obituary' | 'warning' | 'info' | 'celebration'

export interface DetailSource {
  name: string
  url?: string
}

export interface SiteDetail {
  aka?: string[] // 别名
  tags?: string[] // 资源特色标签
  description?: string // 站点介绍
  beginner_tips?: string // 新手提示
  registration?: string // 注册状态（开放/邀请/关闭…）
  difficulty?: string // 入门难度
  logo?: string // LOGO 兜底
  community_labels?: string[] // 社区标签（十二大/传统九大…）
  official_groups?: string[] // 官方发布组（MTeam/MPAD…）
  site_engine?: string // 站点架构（NexusPHP/UNIT3D/mTorrent…）
  users?: number // 站点人数
  torrents?: number // 站点种子数
  peers?: number // 在线 peer 数
  founded?: number // 建站年份
  freeleech?: boolean // 有免费政策
  bonus_points?: boolean // 有魔力/积分系统
  hit_and_run?: boolean // 有 H&R 考核
  ratio_required?: boolean // 有分享率要求
  sources?: DetailSource[] // 资料来源
  language?: string // 站点语言（zh-CN…）
  rank?: number // ptseek 综合排名
  source_category?: string // 来源分类（热门站点/其他站点…）
  research_status?: string // 资料研究状态
  checked_at?: string // 资料核对日期
  content_types?: string[] // 内容类型（电影/剧集/音乐…）
  service_type?: string // pt_tracker/community_forum…
  schema?: string // 平台（nexusphp/unit3d…）
}

export interface SaveptSiteCard {
  name: string
  url: string
  domain: string
  icon?: string
  year: string
  status: SiteHealthStatus
  status_text?: string
  site_type: string
  anniversary_text?: string
  anniversary_flag?: string
  duration_text?: string
  fault_duration?: string
  open_time?: string
  close_time?: string
  search?: string
  detail?: SiteDetail
  match_type: MatchType
  local_site_id?: number
  local_site_name?: string
}

export interface SaveptAlert {
  type: AlertType
  title: string
  content: string
}

export interface SaveptSummary {
  total: number
  healthy: number
  critical: number
  closed: number
  bound: number
  available: number
  internal: number
  external: number
  today_anniv: number
  years: number
  mp_owned: number
  mp_available: number
  mp_unsupported: number
}

export interface SaveptResult {
  summary: SaveptSummary
  alerts: SaveptAlert[]
  cards: SaveptSiteCard[]
  fetched_at: string
  cached: boolean
  stale?: boolean // 降级：抓取失败时返回的缓存快照（可能已过期）
  notice?: string // 降级/提示文案
}

export const saveptApi = {
  get: (refresh = false) => http.get<SaveptResult>(`/savept${refresh ? '?refresh=1' : ''}`).then((r) => r.data),
}
