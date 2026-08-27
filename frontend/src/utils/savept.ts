// PT监护室前端工具函数：从 MoviePilot savept 插件迁移并适配 PTPatronus Go 契约。
// 主要差异：mp 状态从 match_type 派生（后端不冗余 mp_status）；alert type(danger/obituary/
// warning/info/celebration) 映射到 4 个公告分区(critical/success/info/error)。
import type { AlertType, MatchType, SaveptAlert, SaveptSiteCard, SiteHealthStatus } from '@/api/savept'

// ── 状态元信息 ──
export function getStatusMeta(status: SiteHealthStatus) {
  switch (status) {
    case 'healthy':
      return { label: '健康', color: '#22c55e', icon: 'heart-pulse', cardClass: 'healthy' }
    case 'critical':
      return { label: '病危', color: '#ef4444', icon: 'alert-circle', cardClass: 'critical' }
    case 'closed':
      return { label: '死亡', color: '#94a3b8', icon: 'skull', cardClass: 'closed' }
    default:
      return { label: '未知', color: '#94a3b8', icon: '', cardClass: 'unknown' }
  }
}

// 从 match_type 派生本地 MP 站点状态（后端不冗余 mp_status）。
export function getMpStatusMeta(matchType: MatchType) {
  switch (matchType) {
    case 'url_exact':
    case 'domain_exact':
      return { label: '已拥有', chipClass: 'owned', icon: 'check-decagram', color: '#16a34a' }
    case 'name_exact':
    case 'name_fuzzy':
      return { label: '可添加', chipClass: 'available', icon: 'plus-circle', color: '#2563eb' }
    default:
      return { label: '未收录', chipClass: 'unsupported', icon: 'help-circle', color: '#64748b' }
  }
}

export function siteTypeLabel(siteType?: string) {
  switch (siteType) {
    case 'internal':
      return '内站'
    case 'external':
      return '外站'
    default:
      return '未知'
  }
}

// 站点搜索关键词（名称/搜索入口/年份/状态/类型）。
export function buildSiteKeywords(card: SaveptSiteCard) {
  return [card.name, card.search, card.year, card.status_text, siteTypeLabel(card.site_type)]
    .filter(Boolean)
    .join(' ')
    .toLowerCase()
}

// ── 站庆文案 ──
export function getAnniversaryDisplayText(text?: string) {
  const raw = `${text || ''}`.trim()
  if (!raw) return ''
  if (raw.includes('今天站庆') || raw === '0天') return '🎉 今天站庆！'
  return raw.replace(/^emoji\s*站庆\s*/i, '').trim()
}

export function isTodayAnniversaryText(text?: string) {
  return getAnniversaryDisplayText(text) === '🎉 今天站庆！'
}

// ── 站庆日历 ──
export interface AnniversaryEntry {
  card: SaveptSiteCard
  years: number // 站庆周年数（查看年 - 建站年）；建站年未知为 0
  source: 'open_time' | 'countdown' // 开站日期推算 / 站庆倒计时文案补位
}

// key 为 '月-日'（无前导零，如 "3-15"）→ 当日站庆的站点列表。
export type AnniversaryMap = Map<string, AnniversaryEntry[]>

// 解析站庆倒计时文案：'今天站庆'/'0天' → 0，'5天'/'站庆5天'/'距站庆 12 天' → 天数；非倒计时文案返回 null。
export function parseAnnivCountdownDays(text?: string): number | null {
  const display = getAnniversaryDisplayText(text)
  if (!display) return null
  if (display === '🎉 今天站庆！') return 0
  const match = display.match(/(\d+)\s*天/)
  return match ? Number(match[1]) : null
}

// 解析开站日期（data-opentime，'YYYY-MM-DD'），无效返回 null。
export function parseOpenDate(value?: string): { year: number; month: number; day: number } | null {
  const match = `${value || ''}`.trim().match(/^(\d{4})-(\d{1,2})-(\d{1,2})/)
  if (!match) return null
  const year = Number(match[1])
  const month = Number(match[2])
  const day = Number(match[3])
  const date = new Date(year, month - 1, day)
  if (date.getFullYear() !== year || date.getMonth() !== month - 1 || date.getDate() !== day) return null
  return { year, month, day }
}

function isLeapYear(year: number): boolean {
  return new Date(year, 1, 29).getMonth() === 1
}

/**
 * 构建某年的站庆日历（key '月-日'）。优先用开站日期（每年同月日站庆）；
 * 无开站日期但站庆文案是倒计时的，用「今天 + N 天」补位（仅当年有效）。
 * 已关站站点不进日历；建站之前的年份不出现；2/29 建站平年落在 2/28；同名站点去重。
 */
export function buildAnniversaryMap(cards: SaveptSiteCard[], year: number, now = new Date()): AnniversaryMap {
  const map: AnniversaryMap = new Map()
  const put = (key: string, entry: AnniversaryEntry) => {
    const list = map.get(key) ?? []
    if (!list.some((e) => e.card.name === entry.card.name)) list.push(entry)
    map.set(key, list)
  }
  for (const card of cards) {
    if (card.status === 'closed') continue
    const buildYear = Number(card.year)
    const years = Number.isInteger(buildYear) && buildYear >= 1900 && buildYear <= year ? year - buildYear : 0
    const open = parseOpenDate(card.open_time)
    if (open) {
      if (year < open.year) continue
      const day = open.month === 2 && open.day === 29 && !isLeapYear(year) ? 28 : open.day
      put(`${open.month}-${day}`, { card, years, source: 'open_time' })
      continue
    }
    const days = parseAnnivCountdownDays(card.anniversary_text)
    if (days !== null) {
      const date = new Date(now.getFullYear(), now.getMonth(), now.getDate() + days)
      if (date.getFullYear() === year) {
        put(`${date.getMonth() + 1}-${date.getDate()}`, { card, years, source: 'countdown' })
      }
    }
  }
  return map
}

// 站点是否具备可进日历的站庆日期（开站日期或倒计时文案）——用于统计「建站日期未知」的站点数。
export function hasKnownAnniversaryDate(card: SaveptSiteCard): boolean {
  return parseOpenDate(card.open_time) !== null || parseAnnivCountdownDays(card.anniversary_text) !== null
}

// ── 公告分区 ──
type NoticeLevel = 'critical' | 'success' | 'info' | 'error'

// Go alert type → 4 分区 level。
function alertTypeToLevel(type: AlertType): NoticeLevel {
  switch (type) {
    case 'danger':
      return 'critical'
    case 'obituary':
      return 'error'
    case 'warning':
      return 'success'
    case 'info':
    case 'celebration':
      return 'info'
    default:
      return 'info'
  }
}

export interface NoticeSection {
  key: NoticeLevel
  title: string
  items: string[]
}

// 把公告按 4 分区分组。每条 alert 的 content 整体作为一个 item（一行）——
// 站点名由 tokenizeNoticeText 在行内高亮，「|」等分隔符在行内保留，紧凑不分行。
export function buildNoticeSections(alerts: SaveptAlert[] = []): NoticeSection[] {
  const grouped: Record<NoticeLevel, string[]> = { critical: [], success: [], info: [], error: [] }
  for (const alert of alerts) {
    const level = alertTypeToLevel(alert.type)
    const text = `${alert.content || ''}`.trim()
    if (text) grouped[level].push(text)
  }
  return [
    { key: 'critical', title: '⚠️ 病危通知', items: grouped.critical },
    { key: 'success', title: '✅ 恢复公告', items: grouped.success },
    { key: 'info', title: '🎉 站庆预告', items: grouped.info },
    { key: 'error', title: '🕯️ 死亡讣告', items: grouped.error },
  ]
}

// ── 公告正文站点名高亮分词 ──
function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

type NoticeToken = { type: 'text'; text: string } | { type: 'site'; text: string }

function appendNoticeTextToken(tokens: NoticeToken[], text: string) {
  if (!text) return
  const last = tokens[tokens.length - 1]
  if (last?.type === 'text') {
    last.text += text
    return
  }
  tokens.push({ type: 'text', text })
}

function buildNoticeSiteNames(cards: SaveptSiteCard[] = []) {
  return Array.from(
    new Set(cards.map((c) => `${c.name || ''}`.trim()).filter(Boolean)),
  ).sort((a, b) => b.length - a.length)
}

function detectNoticeSiteName(segmentText: string, siteNames: string[]) {
  const raw = `${segmentText || ''}`
  const trimmed = raw.trim()
  if (!trimmed) return null

  const colonMatch = raw.match(/^(.*?[：:]\s*)(.+)$/)
  const prefix = colonMatch ? colonMatch[1] : ''
  const body = colonMatch ? colonMatch[2] : raw
  const trimmedBody = body.trimStart()
  const bodyLeadingWhitespace = body.slice(0, body.length - trimmedBody.length)

  const knownSite = siteNames.find((name) => {
    const sitePattern = new RegExp(`^${escapeRegExp(name)}(?=\\s|$)`)
    return sitePattern.test(trimmedBody)
  })
  if (knownSite) {
    return {
      prefix: `${prefix}${bodyLeadingWhitespace}`,
      siteName: knownSite,
      suffix: trimmedBody.slice(knownSite.length),
    }
  }

  // 启发式：站点名通常在「已抢救/已/还有/抢救/于」等动词前。
  const heuristicMatch = trimmedBody.match(/^(.+?)(?=\s(?:已抢救|已|还有|抢救|于))/)
  if (heuristicMatch?.[1]) {
    const siteName = heuristicMatch[1].trim()
    if (siteName) {
      return {
        prefix: `${prefix}${bodyLeadingWhitespace}`,
        siteName,
        suffix: trimmedBody.slice(heuristicMatch[1].length),
      }
    }
  }
  return null
}

function tokenizeNoticeSegment(segmentText: string, siteNames: string[]): NoticeToken[] {
  const raw = `${segmentText || ''}`
  if (!raw) return []
  const detected = detectNoticeSiteName(raw, siteNames)
  if (!detected) return [{ type: 'text', text: raw }]
  const tokens: NoticeToken[] = []
  appendNoticeTextToken(tokens, detected.prefix)
  tokens.push({ type: 'site', text: detected.siteName })
  appendNoticeTextToken(tokens, detected.suffix)
  return tokens
}

// 把一条公告正文按站点名切成 token，便于在模板里把站点名渲染成高亮 tag。
export function tokenizeNoticeText(text: string, cards: SaveptSiteCard[] = []): NoticeToken[] {
  const content = `${text || ''}`
  if (!content.trim()) return []
  const siteNames = buildNoticeSiteNames(cards)
  const tokens: NoticeToken[] = []
  for (const segment of content.split(/(\s*\|\s*)/)) {
    if (!segment) continue
    if (segment.includes('|')) {
      appendNoticeTextToken(tokens, segment)
      continue
    }
    for (const token of tokenizeNoticeSegment(segment, siteNames)) {
      if (token.type === 'text') appendNoticeTextToken(tokens, token.text)
      else tokens.push(token)
    }
  }
  return tokens
}
