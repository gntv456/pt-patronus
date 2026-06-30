// 零依赖：Canvas 2D 绘制「PTPatronus 仪表盘」成就海报，config 驱动（字段显隐/主题/脱敏/背景图/冠军 7 类）。
// 参考 PT-depiler UserDataTimeline + 旧版 Kotlin DashboardExporter。@2x 渲染，不引 html2canvas/konva。
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

const FONT = "'PingFang SC', 'Microsoft YaHei', 'Hiragino Sans GB', system-ui, -apple-system, sans-serif"
const W = 760
const PAD = 40

// ---------- 基础 helper ----------
function fmtBytes(n: number): string {
  if (!n) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let i = 0
  let v = n
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(i === 0 ? 0 : 2)} ${u[i]}`
}

function roundRect(ctx: CanvasRenderingContext2D, x: number, y: number, w: number, h: number, r: number): void {
  const rr = Math.min(r, w / 2, h / 2)
  ctx.beginPath()
  ctx.moveTo(x + rr, y)
  ctx.arcTo(x + w, y, x + w, y + h, rr)
  ctx.arcTo(x + w, y + h, x, y + h, rr)
  ctx.arcTo(x, y + h, x, y, rr)
  ctx.arcTo(x, y, x + w, y, rr)
  ctx.closePath()
}

function divider(ctx: CanvasRenderingContext2D, x1: number, x2: number, y: number, color: string): void {
  ctx.strokeStyle = color
  ctx.lineWidth = 1
  ctx.beginPath()
  ctx.moveTo(x1, y + 0.5)
  ctx.lineTo(x2, y + 0.5)
  ctx.stroke()
}

function fmtShort(s: string): string {
  const d = new Date(s)
  if (isNaN(d.getTime())) return ''
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

function nowStamp(): string {
  const s = new Date()
  const p = (n: number) => String(n).padStart(2, '0')
  return `${s.getFullYear()}-${p(s.getMonth() + 1)}-${p(s.getDate())} ${p(s.getHours())}:${p(s.getMinutes())}`
}

function sum<T>(arr: T[], f: (t: T) => number): number {
  return arr.reduce((a, t) => a + (f(t) || 0), 0)
}

// 最早 join_date 的站点（总览「最早入站」+ PT 龄共用）。
function earliestSite(sites: DashboardShareSite[]): { name: string; date: string } | null {
  let min = Infinity
  let name = ''
  for (const s of sites) {
    const t = new Date(s.join_date).getTime()
    if (!isNaN(t) && t < min) {
      min = t
      name = s.site_name
    }
  }
  if (!isFinite(min)) return null
  const d = new Date(min)
  const p = (n: number) => String(n).padStart(2, '0')
  return { name, date: `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}` }
}

function ptYears(sites: DashboardShareSite[]): number | null {
  let min = Infinity
  for (const s of sites) {
    const t = new Date(s.join_date).getTime()
    if (!isNaN(t) && t < min) min = t
  }
  if (!isFinite(min)) return null
  return (Date.now() - min) / (365 * 24 * 3600 * 1000)
}

// ---------- 图像加载 ----------
function loadFavicon(baseUrl: string): Promise<HTMLImageElement | null> {
  return new Promise((resolve) => {
    if (!baseUrl) return resolve(null)
    const img = new Image()
    img.crossOrigin = 'anonymous' // 防 canvas taint；CORS 不通 → onerror 回退色块
    const timer = setTimeout(() => resolve(null), 3000)
    img.onload = () => {
      clearTimeout(timer)
      resolve(img)
    }
    img.onerror = () => {
      clearTimeout(timer)
      resolve(null)
    }
    try {
      img.src = baseUrl.replace(/\/$/, '') + '/favicon.ico'
    } catch {
      clearTimeout(timer)
      resolve(null)
    }
  })
}

// 用户本地上传背景图（dataURL，同源不 taint）。
function loadImage(dataUrl: string): Promise<HTMLImageElement | null> {
  return new Promise((resolve) => {
    const img = new Image()
    img.onload = () => resolve(img)
    img.onerror = () => resolve(null)
    img.src = dataUrl
  })
}

function hueOf(name: string): number {
  let h = 0
  for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) % 360
  return h
}

function drawAvatarFallback(ctx: CanvasRenderingContext2D, cx: number, cy: number, r: number, name: string): void {
  ctx.save()
  ctx.beginPath()
  ctx.arc(cx, cy, r, 0, Math.PI * 2)
  ctx.fillStyle = `hsl(${hueOf(name)}, 58%, 46%)`
  ctx.fill()
  ctx.fillStyle = '#fff'
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  ctx.font = `700 ${Math.round(r * 1.05)}px ${FONT}`
  ctx.fillText((name.trim()[0] || '?').toUpperCase(), cx, cy + 1)
  ctx.restore()
}

function drawAvatar(
  ctx: CanvasRenderingContext2D,
  cx: number,
  cy: number,
  r: number,
  name: string,
  img: HTMLImageElement | null,
): void {
  if (img && img.width > 0 && img.height > 0) {
    ctx.save()
    ctx.beginPath()
    ctx.arc(cx, cy, r, 0, Math.PI * 2)
    ctx.closePath()
    ctx.clip()
    const s = Math.max((r * 2) / img.width, (r * 2) / img.height)
    const dw = img.width * s
    const dh = img.height * s
    ctx.drawImage(img, cx - dw / 2, cy - dh / 2, dw, dh)
    ctx.restore()
  } else {
    drawAvatarFallback(ctx, cx, cy, r, name)
  }
}

// ---------- 主题色 ----------
interface ThemeColors {
  text: string
  sub: string
  muted: string
  divider: string
  levelBg: string
  levelText: string
  base: string // 兜底底色
}

function themeColors(theme: ExportTheme, hasBg: boolean): ThemeColors {
  if (hasBg || theme === 'glass') {
    // 有背景图：白字 + 暗遮罩保证可读
    return {
      text: '#ffffff',
      sub: 'rgba(255,255,255,0.72)',
      muted: 'rgba(255,255,255,0.5)',
      divider: 'rgba(255,255,255,0.16)',
      levelBg: 'rgba(120,160,255,0.28)',
      levelText: '#cdd9ff',
      base: '#0f0f1e',
    }
  }
  if (theme === 'light') {
    return {
      text: '#2c2c2c',
      sub: '#5a6072',
      muted: '#9aa0b4',
      divider: 'rgba(0,0,0,0.1)',
      levelBg: 'rgba(80,120,200,0.14)',
      levelText: '#3a6fd8',
      base: '#ffffff',
    }
  }
  return {
    text: '#ffffff',
    sub: '#9aa0b4',
    muted: '#6b7088',
    divider: 'rgba(255,255,255,0.1)',
    levelBg: 'rgba(120,160,255,0.18)',
    levelText: '#6ea8fe',
    base: '#0f0f1e',
  }
}

function drawBackground(
  ctx: CanvasRenderingContext2D,
  w: number,
  h: number,
  theme: ExportTheme,
  bgImg: HTMLImageElement | null,
  blur: number,
  dim: number,
  base: string,
): void {
  ctx.fillStyle = base
  ctx.fillRect(0, 0, w, h) // 兜底，防 blur 边缘漏白
  if (bgImg && bgImg.width > 0) {
    ctx.save()
    if (blur > 0) {
      try {
        ctx.filter = `blur(${blur}px)`
      } catch {
        /* 不支持 filter 则清晰绘制 */
      }
    }
    const s = Math.max(w / bgImg.width, h / bgImg.height) * 1.12 // 放大 12% 防 blur 边缘
    const dw = bgImg.width * s
    const dh = bgImg.height * s
    ctx.drawImage(bgImg, (w - dw) / 2, (h - dh) / 2, dw, dh)
    ctx.restore()
    ctx.filter = 'none'
    if (dim > 0) {
      ctx.fillStyle = `rgba(0,0,0,${dim})`
      ctx.fillRect(0, 0, w, h)
    }
    return
  }
  const bg = ctx.createLinearGradient(0, 0, w, h)
  if (theme === 'light') {
    bg.addColorStop(0, '#F5F5F5')
    bg.addColorStop(1, '#FFFFFF')
  } else {
    bg.addColorStop(0, '#1a1a2e')
    bg.addColorStop(1, '#0f0f1e')
  }
  ctx.fillStyle = bg
  ctx.fillRect(0, 0, w, h)
}

// ---------- 数据派生 ----------
interface NumField {
  key: string
  label: string
  color: string
  value: string
}

function buildNumericFields(d: DashboardShareData, config: DashboardExportConfig): NumField[] {
  const totalRatio = d.totals.downloaded > 0 ? d.totals.uploaded / d.totals.downloaded : 0
  const all: NumField[] = [
    { key: 'uploaded', label: '总上传', color: '#2ecc71', value: fmtBytes(d.totals.uploaded) },
    { key: 'downloaded', label: '总下载', color: '#4285f4', value: fmtBytes(d.totals.downloaded) },
    { key: 'seeding', label: '总做种', color: '#38ef7d', value: `${d.totals.seeding}` },
    { key: 'seedingSize', label: '做种体积', color: '#43cea2', value: fmtBytes(d.totals.seeding_size) },
    { key: 'bonus', label: '总魔力', color: '#f7971e', value: `${Math.round(d.totals.bonus)}` },
    { key: 'ratio', label: '总分享率', color: '#e74c3c', value: totalRatio.toFixed(2) },
    { key: 'uploadCount', label: '总发布数', color: '#9b59b6', value: `${sum(d.sites, (s) => s.uploads)}` },
    { key: 'bonusPerHour', label: '总时魔/h', color: '#e67e22', value: sum(d.sites, (s) => s.bonus_per_hour).toFixed(1) },
  ]
  const f = config.fields as Record<string, boolean>
  return all.filter((m) => f[m.key])
}

interface RankDef {
  key: RankMetric
  label: string
  color: string
  pick: (s: DashboardShareSite) => number
  fmt: (v: number) => string
}

const RANK_DEFS: RankDef[] = [
  { key: 'uploaded', label: '上传', color: '#2ecc71', pick: (s) => s.uploaded, fmt: (v) => fmtBytes(v) },
  { key: 'downloaded', label: '下载', color: '#4285f4', pick: (s) => s.downloaded, fmt: (v) => fmtBytes(v) },
  { key: 'uploads', label: '发布', color: '#9b59b6', pick: (s) => s.uploads, fmt: (v) => `${v}` },
  { key: 'seeding', label: '做种', color: '#38ef7d', pick: (s) => s.seeding, fmt: (v) => `${v}` },
  { key: 'ratio', label: '分享率', color: '#e74c3c', pick: (s) => s.ratio, fmt: (v) => v.toFixed(2) },
  { key: 'bonus', label: '魔力', color: '#f7971e', pick: (s) => s.bonus, fmt: (v) => `${Math.round(v)}` },
  { key: 'bonus_per_hour', label: '时魔', color: '#e67e22', pick: (s) => s.bonus_per_hour, fmt: (v) => v.toFixed(1) },
]

interface Champion {
  def: RankDef
  site: DashboardShareSite
  value: number
}

function rankChampions(sites: DashboardShareSite[], metrics: RankMetric[]): Champion[] {
  const out: Champion[] = []
  for (const key of metrics) {
    const def = RANK_DEFS.find((d) => d.key === key)
    if (!def) continue
    let best: DashboardShareSite | null = null
    for (const s of sites) {
      if (best === null || def.pick(s) > def.pick(best)) best = s
    }
    if (best && def.pick(best) > 0) out.push({ def, site: best, value: def.pick(best) })
  }
  return out
}

// 按 config 过滤/排序/截断出要画明细的站点。
function computeViewSites(d: DashboardShareData, config: DashboardExportConfig): DashboardShareSite[] {
  let v = d.sites
  if (config.selectedSiteIds.length) v = v.filter((s) => config.selectedSiteIds.includes(s.site_id))
  v = [...v].sort((a, b) => b.uploaded - a.uploaded)
  if (config.siteLimit > 0) v = v.slice(0, config.siteLimit)
  return v
}

// ---------- 布局（绝对坐标，绘制共用，杜绝漂移） ----------
function layout(d: DashboardShareData, config: DashboardExportConfig) {
  const HEADER_H = 84
  const gridRowH = 34
  const rankRowH = 36
  const siteRowH = 28
  const earliestH = 22
  const SITE_ROW_H = 116 + (config.showIdentity ? 18 : 0) // +18 容纳身份行（用户名/ID/邮箱）
  const FOOTER_H = 56

  const numeric = buildNumericFields(d, config)
  const gridRows = Math.ceil(numeric.length / 2)
  const champions = config.showRank ? rankChampions(d.sites, config.rankMetrics) : []
  const rankRows = Math.ceil(champions.length / 2)
  const earliest = config.fields.earliestJoin ? earliestSite(d.sites) : null
  const viewSites = config.showSites ? computeViewSites(d, config) : []

  let y = HEADER_H
  const headDividerY = y
  y += 20
  const siteRowY = y + 20 // 总站点/PT龄 baseline
  y += siteRowH
  let earliestY = 0
  if (earliest) {
    y += 4
    earliestY = y + 16
    y += earliestH
  }
  let gridY = 0
  if (numeric.length) {
    y += 8
    gridY = y
    y += gridRowH * gridRows
    y += 12
  }
  let rankDividerY = 0
  let rankTitleY = 0
  let rankGridY = 0
  if (champions.length) {
    y += 8
    rankDividerY = y
    y += 16
    rankTitleY = y + 14
    y += 30 // 标题行 + 与网格的间距，避免「各项冠军」与第一行内容重叠
    rankGridY = y
    y += rankRowH * rankRows
  }
  let siteDividerY = 0
  let siteTitleY = 0
  let siteGridY = 0
  if (viewSites.length) {
    y += 8
    siteDividerY = y
    y += 24
    siteTitleY = y + 14
    y += 30
    siteGridY = y
    y += Math.ceil(viewSites.length / 2) * SITE_ROW_H
    y += 8
  }
  const totalH = y + FOOTER_H
  const footerY = totalH - 30

  return {
    numeric,
    gridRowH,
    gridY,
    champions,
    rankRowH,
    rankDividerY,
    rankTitleY,
    rankGridY,
    earliest,
    earliestY,
    siteRowY,
    viewSites,
    SITE_ROW_H,
    siteDividerY,
    siteTitleY,
    siteGridY,
    headDividerY,
    totalH,
    footerY,
  }
}

// ---------- 主绘制 ----------
export async function exportDashboardImage(d: DashboardShareData, config: DashboardExportConfig): Promise<void> {
  const maskName = (s: string): string => (config.blurSite ? '***' : s)
  const maskUser = (s: string): string => (config.blurUser ? '***' : s)

  const L = layout(d, config)
  const favicons = config.blurSite ? [] : await Promise.all(L.viewSites.map((s) => loadFavicon(s.base_url)))
  const bgImg = config.backgroundImage ? await loadImage(config.backgroundImage) : null
  const hasBg = !!bgImg
  const th = themeColors(config.theme, hasBg)

  const scale = 2
  const c = document.createElement('canvas')
  c.width = W * scale
  c.height = L.totalH * scale
  const ctx = c.getContext('2d')
  if (!ctx) return
  ctx.scale(scale, scale)
  ctx.textBaseline = 'alphabetic'

  // 1. 背景
  drawBackground(ctx, W, L.totalH, config.theme, bgImg, config.backgroundBlur, config.backgroundDim, th.base)

  const when = nowStamp()

  // 2. 顶部
  ctx.fillStyle = th.text
  ctx.font = `700 26px ${FONT}`
  ctx.textAlign = 'left'
  ctx.fillText(maskUser(d.username || 'PT 玩家'), PAD, 48)
  ctx.fillStyle = th.sub
  ctx.font = `400 14px ${FONT}`
  ctx.fillText('PTPatronus · 仪表盘成就', PAD, 70)
  ctx.fillStyle = th.sub
  ctx.font = `400 13px ${FONT}`
  ctx.textAlign = 'right'
  ctx.fillText(`生成于 ${when}`, W - PAD, 36)
  ctx.fillText(`${d.siteCount} 站点 · ${d.downloaderCount} 下载器`, W - PAD, 56)
  ctx.textAlign = 'left'

  divider(ctx, PAD, W - PAD, L.headDividerY, th.divider)

  // 3. 总览：总站点（标签 + 突出大号数字）+ PT 龄
  const yrs = config.fields.ptAge ? ptYears(d.sites) : null
  ctx.textAlign = 'left'
  let sx = PAD
  ctx.fillStyle = th.sub
  ctx.font = `600 16px ${FONT}`
  ctx.fillText('总站点', sx, L.siteRowY)
  sx += ctx.measureText('总站点').width + 8
  ctx.fillStyle = th.text
  ctx.font = `700 24px ${FONT}`
  ctx.fillText(`${d.siteCount}`, sx, L.siteRowY)
  if (yrs !== null) {
    sx += ctx.measureText(`${d.siteCount}`).width + 12
    ctx.fillStyle = th.sub
    ctx.font = `600 16px ${FONT}`
    ctx.fillText(`· PT 龄 ${yrs.toFixed(1)} 年`, sx, L.siteRowY)
  }

  if (L.earliest) {
    ctx.fillStyle = th.sub
    ctx.font = `400 13px ${FONT}`
    ctx.fillText(`最早入站 ${L.earliest.date} · ${maskName(L.earliest.name)}`, PAD, L.earliestY)
  }

  if (L.numeric.length) {
    const colW = (W - PAD * 2 - 20) / 2
    L.numeric.forEach((m, i) => {
      const col = i % 2
      const row = Math.floor(i / 2)
      const mx = PAD + col * (colW + 20)
      const my = L.gridY + row * L.gridRowH
      ctx.fillStyle = m.color
      roundRect(ctx, mx, my + 5, 4, 16, 2)
      ctx.fill()
      ctx.fillStyle = th.sub
      ctx.font = `400 13px ${FONT}`
      ctx.fillText(m.label, mx + 12, my + 17)
      ctx.fillStyle = th.text
      ctx.font = `700 17px ${FONT}`
      ctx.fillText(m.value, mx + 12, my + 33)
    })
  }

  // 4. 冠军排行（7 类各冠军站点）
  if (L.champions.length) {
    divider(ctx, PAD, W - PAD, L.rankDividerY, th.divider)
    ctx.fillStyle = th.text
    ctx.font = `600 14px ${FONT}`
    ctx.fillText('各项冠军', PAD, L.rankTitleY)
    const colW = (W - PAD * 2 - 20) / 2
    L.champions.forEach((r, i) => {
      const col = i % 2
      const row = Math.floor(i / 2)
      const mx = PAD + col * (colW + 20)
      const my = L.rankGridY + row * L.rankRowH
      ctx.beginPath()
      ctx.arc(mx + 8, my + 14, 6, 0, Math.PI * 2)
      ctx.fillStyle = r.def.color
      ctx.fill()
      ctx.fillStyle = th.sub
      ctx.font = `400 12px ${FONT}`
      ctx.textAlign = 'left'
      ctx.fillText(`${r.def.label}冠军`, mx + 22, my + 12)
      ctx.fillStyle = th.text
      ctx.font = `600 14px ${FONT}`
      ctx.fillText(r.def.fmt(r.value), mx + 22, my + 29)
      ctx.fillStyle = th.muted
      ctx.font = `400 12px ${FONT}`
      ctx.textAlign = 'right'
      ctx.fillText(maskName(r.site.site_name), mx + colW, my + 29)
      ctx.textAlign = 'left'
    })
  }

  // 5. 每站明细（两列）
  if (L.viewSites.length) {
    divider(ctx, PAD, W - PAD, L.siteDividerY, th.divider)
    ctx.fillStyle = th.text
    ctx.font = `600 15px ${FONT}`
    ctx.textAlign = 'center'
    ctx.fillText(`各站点（${L.viewSites.length}）`, W / 2, L.siteTitleY)
    ctx.textAlign = 'left'

    const innerW = W - PAD * 2
    const colGap = 20
    const cellW = (innerW - colGap) / 2
    const ar = 18
    const identityH = config.showIdentity ? 18 : 0
    L.viewSites.forEach((s, i) => {
      const col = i % 2
      const row = Math.floor(i / 2)
      const cx = PAD + col * (cellW + colGap)
      const cy = L.siteGridY + row * L.SITE_ROW_H
      const displayName = maskName(s.site_name)
      drawAvatar(ctx, cx + ar, cy + ar, ar, displayName, config.blurSite ? null : favicons[i])
      const tx = cx + ar * 2 + 12
      ctx.fillStyle = th.text
      ctx.font = `600 15px ${FONT}`
      ctx.fillText(displayName, tx, cy + 14)
      if (s.level) {
        const nameW = ctx.measureText(displayName).width
        const lvX = tx + nameW + 10
        ctx.font = `400 11px ${FONT}`
        const lvW = ctx.measureText(s.level).width
        roundRect(ctx, lvX, cy + 2, lvW + 12, 16, 8)
        ctx.fillStyle = th.levelBg
        ctx.fill()
        ctx.fillStyle = th.levelText
        ctx.fillText(s.level, lvX + 6, cy + 14)
      }
      // 身份行：用户名 / ID / 邮箱（受 blurUser 脱敏 → ***）；空值跳过，best-effort
      if (identityH > 0) {
        const parts: string[] = []
        if (s.username) parts.push(maskUser(s.username))
        if (s.user_id) parts.push(`ID ${maskUser(s.user_id)}`)
        if (s.email) parts.push(maskUser(s.email))
        if (parts.length) {
          ctx.fillStyle = th.muted
          ctx.font = `400 11px ${FONT}`
          ctx.fillText(parts.join('   ·   '), tx, cy + 31)
        }
      }
      ctx.fillStyle = th.sub
      ctx.font = `400 12px ${FONT}`
      ctx.fillText(`↑${fmtBytes(s.uploaded)}   ↓${fmtBytes(s.downloaded)}   分享 ${s.ratio > 0 ? s.ratio.toFixed(2) : '-'}`, tx, cy + 34 + identityH)
      ctx.fillText(`做种 ${s.seeding || 0}   下载中 ${s.leeching || 0}   体积 ${fmtBytes(s.seeding_size)}`, tx, cy + 52 + identityH)
      const bh = s.bonus_per_hour > 0 ? `   时魔 ${s.bonus_per_hour.toFixed(1)}/h` : ''
      ctx.fillText(`发布 ${s.uploads > 0 ? s.uploads : '-'}   魔力 ${s.bonus > 0 ? Math.round(s.bonus) : 0}${bh}`, tx, cy + 70 + identityH)
      const joinStr = s.join_date ? `加入 ${s.join_date}` : ''
      const updStr = s.updated_at ? `更新 ${fmtShort(s.updated_at)}` : ''
      const meta = [joinStr, updStr].filter(Boolean).join('   ')
      if (meta) {
        ctx.fillStyle = th.muted
        ctx.fillText(meta, tx, cy + 90 + identityH)
      }
    })
  }

  // 6. 页脚
  divider(ctx, PAD, W - PAD, L.footerY - 18, th.divider)
  ctx.fillStyle = th.muted
  ctx.font = `400 13px ${FONT}`
  ctx.textAlign = 'left'
  ctx.fillText('PTPatronus', PAD, L.footerY)
  ctx.textAlign = 'right'
  ctx.fillText(`客户端导出 · ${when}`, W - PAD, L.footerY)
  ctx.textAlign = 'left'

  c.toBlob((blob) => {
    if (!blob) return
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    const safeName = (d.username || 'dashboard').replace(/[^\w一-龥]/g, '')
    a.download = `ptpatronus-${safeName}-${when.replace(/[: ]/g, '-').slice(0, 16)}.png`
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
  }, 'image/png')
}
