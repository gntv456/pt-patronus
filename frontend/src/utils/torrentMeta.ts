/**
 * Lightweight torrent title meta for MoviePilot-style result cards.
 * Pure frontend heuristics - no filter engine dependency.
 */

const TAG_RULES: Array<{ re: RegExp; label: string; tone?: string }> = [
  { re: /\b(2160p|4k|x2160|uhd)\b/i, label: '4K', tone: 'res' },
  { re: /\b(1080[pi]|x1080)\b/i, label: '1080p', tone: 'res' },
  { re: /\b(720[pi]|x720)\b/i, label: '720p', tone: 'res' },
  { re: /\bWEB-?DL\b/i, label: 'WEB-DL', tone: 'src' },
  { re: /\bWEB-?Rip\b/i, label: 'WEBRip', tone: 'src' },
  { re: /\bBlu-?Ray\b|\bBluRay\b/i, label: 'BluRay', tone: 'src' },
  { re: /\bREMUX\b/i, label: 'REMUX', tone: 'src' },
  { re: /\bHDTV\b/i, label: 'HDTV', tone: 'src' },
  { re: /\bUHD\b/i, label: 'UHD', tone: 'src' },
  { re: /\b([Hx]\.?265|HEVC)\b/i, label: 'H265', tone: 'codec' },
  { re: /\b([Hx]\.?264|AVC)\b/i, label: 'H264', tone: 'codec' },
  { re: /\bAV1\b/i, label: 'AV1', tone: 'codec' },
  { re: /\bHDR10\+?/i, label: 'HDR10', tone: 'hdr' },
  { re: /\b(DoVi|Dolby\s*Vision)\b|\bDV\b/i, label: 'DoVi', tone: 'hdr' },
  { re: /\bAtmos\b/i, label: 'Atmos', tone: 'audio' },
  { re: /\bDTS(?:-?HD)?(?:\.?MA)?\b/i, label: 'DTS', tone: 'audio' },
  { re: /\bTrueHD\b/i, label: 'TrueHD', tone: 'audio' },
  { re: /\bHDR\b/i, label: 'HDR', tone: 'hdr' },
  { re: /中字|简繁|简体|繁体|国语|双语|CHS|CHT|\bGB\b|\bBIG5\b/i, label: '中字', tone: 'lang' },
  { re: /\bDIY\b/i, label: 'DIY', tone: 'misc' },
  { re: /\biTunes\b/i, label: 'iTunes', tone: 'src' },
  { re: /\b(Netflix|NF)\b/i, label: 'NF', tone: 'src' },
  { re: /\bDisney\+?|\bDSNP\b/i, label: 'Disney+', tone: 'src' },
  { re: /\bAMZN\b|\bAmazon\b/i, label: 'Amazon', tone: 'src' },
  { re: /\bIMAX\b/i, label: 'IMAX', tone: 'misc' },
  { re: /\b60fps\b/i, label: '60fps', tone: 'misc' },
]

const STRIP_FOR_GROUP =
  /\b(2160p|1080[pi]|720[pi]|4k|uhd|web-?dl|web-?rip|bluray|blu-?ray|remux|hdtv|hdr10\+?|hdr|dovi|dolby\s*vision|\bdv\b|atmos|truehd|dts(?:-?hd)?(?:\.?ma)?|[hx]\.?26[45]|hevc|avc|av1|10bit|8bit|aac|ddp?5\.1|flac|lpcm|itunes|netflix|\bnf\b|amzn|dsnp|imax|60fps|complete|repack|proper|internal)\b/gi

export type TorrentTag = { label: string; tone: string }

export function extractTorrentTags(title: string, extra: string[] = []): TorrentTag[] {
  const text = `${title} ${extra.join(' ')}`
  const out: TorrentTag[] = []
  const seen = new Set<string>()
  for (const rule of TAG_RULES) {
    if (!rule.re.test(text)) continue
    const key = rule.label.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    out.push({ label: rule.label, tone: rule.tone || 'misc' })
  }
  for (const raw of extra) {
    const label = raw.trim()
    // 副标题/长描述不进标签（会和卡片副标题重复，且 chip 过长）
    if (!label || label.length > 20) continue
    if (/\s{2,}/.test(label) || (label.length > 12 && /[\u4e00-\u9fff].*[\u4e00-\u9fff]/.test(label) && label.includes('|'))) continue
    const key = label.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    out.push({ label, tone: 'misc' })
  }
  return out.slice(0, 8)
}

/** Group key for "more sources" - strip tech tokens so same release collapses. */
export function torrentGroupKey(title: string): string {
  return title
    .toLowerCase()
    .replace(STRIP_FOR_GROUP, ' ')
    .replace(/[\[\](){}【】（）]/g, ' ')
    .replace(/[._\-]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, 80)
}

export function promoLabel(
  free?: boolean,
  tags?: string[],
): { text: string; tone: 'free' | 'half' | 'double' } | null {
  const bag = (tags || []).join(' ').toLowerCase()
  if (/2x\s*free|双倍免费|2x免费/.test(bag)) return { text: '2X 免费', tone: 'free' }
  if (/2x\s*50|50%.*2x|2x.*50/.test(bag)) return { text: '2X 50%', tone: 'double' }
  if (/50%|半价/.test(bag)) return { text: '50%', tone: 'half' }
  if (free || /free|免费/.test(bag)) return { text: '免费', tone: 'free' }
  return null
}

/** Best-effort relative time for PT site date strings. */
export function relativeTime(raw?: string): string {
  if (!raw) return ''
  const s = raw.trim()
  if (!s) return ''
  if (/前|ago|昨天|今天|刚刚|分钟|小时|天|周|月|年/.test(s)) return s

  const normalized = s.replace(/年|\/|\./g, '-').replace(/月/g, '-').replace(/日/g, '').replace(/T/, ' ')
  const d = new Date(normalized)
  if (Number.isNaN(d.getTime())) return s

  const diffSec = (Date.now() - d.getTime()) / 1000
  if (diffSec < 60) return '刚刚'
  if (diffSec < 3600) return `${Math.floor(diffSec / 60)} 分钟前`
  if (diffSec < 86400) return `${Math.floor(diffSec / 3600)} 小时前`
  if (diffSec < 86400 * 30) return `${Math.floor(diffSec / 86400)} 天前`
  if (diffSec < 86400 * 365) return `${Math.floor(diffSec / (86400 * 30))} 个月前`
  return `${Math.floor(diffSec / (86400 * 365))} 年前`
}


/** MoviePilot-style promo text from upload/download volume factors. */
export function volumeFactorLabel(
  upload?: number,
  download?: number,
  free?: boolean,
  tags?: string[],
): { text: string; tone: 'free' | 'half' | 'double' | 'plain' } | null {
  if (upload != null && download != null) {
    const key = `${upload.toFixed(2)} ${download.toFixed(2)}`
    const map: Record<string, { text: string; tone: 'free' | 'half' | 'double' | 'plain' }> = {
      '1.00 0.00': { text: '免费', tone: 'free' },
      '2.00 0.00': { text: '2X免费', tone: 'free' },
      '4.00 0.00': { text: '4X免费', tone: 'free' },
      '2.00 1.00': { text: '2X', tone: 'double' },
      '4.00 1.00': { text: '4X', tone: 'double' },
      '1.00 0.50': { text: '50%', tone: 'half' },
      '2.00 0.50': { text: '2X 50%', tone: 'double' },
      '1.00 0.70': { text: '70%', tone: 'half' },
      '1.00 0.30': { text: '30%', tone: 'half' },
      '1.00 0.25': { text: '25%', tone: 'half' },
      '1.00 0.75': { text: '75%', tone: 'half' },
    }
    if (key === '1.00 1.00') return null
    if (map[key]) return map[key]
    if (download === 0) return { text: '免费', tone: 'free' }
    if (download < 1) return { text: `${Math.round(download * 100)}%`, tone: 'half' }
    if (upload > 1) return { text: `${upload}X`, tone: 'double' }
  }
  return promoLabel(free, tags)
}

export function cardGroupKey(t: {
  title: string
  size?: string
  tags?: string[]
}): string {
  const tags = extractTorrentTags(t.title, t.tags || [])
  const res = tags.find((x) => x.tone === 'res')?.label || ''
  const codec = tags.find((x) => x.tone === 'codec')?.label || ''
  const src = tags.find((x) => x.tone === 'src')?.label || ''
  // MoviePilot groups by name+pix+edition+team+season+size; we approximate with size+tech tokens.
  return `${(t.size || '').toLowerCase()}|${res}|${codec}|${src}|${torrentGroupKey(t.title)}`
}

/** Format site size: keep '4.3 GB', convert bare byte numbers to human string. */
export function formatTorrentSize(size?: string | number | null): string {
  if (size === null || size === undefined || size === '') return ''
  if (typeof size === 'number') {
    if (!Number.isFinite(size) || size <= 0) return ''
    return formatBytes(size)
  }
  const s = String(size).trim()
  if (!s) return ''
  // Already human: 7.05 GB / 709.84MB / 1.2TiB
  if (/[a-zA-Z\u4e00-\u9fff]/.test(s) && !/^\d+(\.\d+)?$/.test(s)) return s
  // Pure integer/float bytes
  if (/^\d+(\.\d+)?$/.test(s)) {
    const n = Number(s)
    if (!Number.isFinite(n) || n <= 0) return ''
    // Heuristic: values < 1024 without unit are ambiguous; still show as bytes
    return formatBytes(n)
  }
  return s
}

function formatBytes(n: number): string {
  const u = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let v = n
  let i = 0
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024
    i++
  }
  const digits = v >= 100 || i === 0 ? 0 : v >= 10 ? 1 : 2
  return `${v.toFixed(digits)} ${u[i]}`
}

// —— 搜索筛选：体积/时间反解析 + 一次性 meta 派生（Search 表格 & SiteTorrentFindDialog 共用）——

const SIZE_UNITS: Record<string, number> = {
  b: 1, kb: 1024, mb: 1024 ** 2, gb: 1024 ** 3, tb: 1024 ** 4, pb: 1024 ** 5,
  kib: 1024, mib: 1024 ** 2, gib: 1024 ** 3, tib: 1024 ** 4, pib: 1024 ** 5,
  byte: 1, bytes: 1,
}

/** Parse human size ("4.3 GB"/"709.84MB"/"1.2TiB") → bytes; 0 if unparseable. Inverse of formatTorrentSize. */
export function parseSizeToBytes(size?: string | number | null): number {
  if (size == null || size === '') return 0
  if (typeof size === 'number') return Number.isFinite(size) && size > 0 ? size : 0
  const s = String(size).trim()
  const m = s.match(/^([\d.]+)\s*([a-z]+)$/i)
  if (m) {
    const n = Number(m[1])
    const unit = SIZE_UNITS[m[2].toLowerCase()]
    if (Number.isFinite(n) && n > 0 && unit) return Math.round(n * unit)
    return 0
  }
  // bare number → assume bytes
  const n = Number(s)
  return Number.isFinite(n) && n > 0 ? n : 0
}

/** Best-effort pubdate → epoch ms. Prefers ISO (pub_date); falls back to site text (time, 相对文案). 0 if unknown. */
export function parsePubDateToMs(raw?: string | null): number {
  if (!raw) return 0
  const s = String(raw).trim()
  if (!s) return 0
  const iso = s.replace(/[年\/.]/g, '-').replace(/月/g, '-').replace(/日/g, '').replace('T', ' ')
  const d = new Date(iso)
  const t = d.getTime()
  if (!Number.isNaN(t)) return t
  // 中文相对："3天前"/"2小时前" 等
  const rel = s.match(/(\d+)\s*(秒|分钟|小时|天|周|月|年)/)
  if (rel) {
    const n = Number(rel[1])
    const unitMs: Record<string, number> = {
      秒: 1000, 分钟: 60_000, 小时: 3_600_000, 天: 86_400_000,
      周: 604_800_000, 月: 2_592_000_000, 年: 31_536_000_000,
    }
    const ms = unitMs[rel[2]]
    if (ms) return Date.now() - n * ms
  }
  return 0
}

/** Derived filterable meta for a torrent row (search table & find dialog share this). */
export interface EnrichedMeta {
  tags: TorrentTag[]
  res: string
  codec: string
  src: string
  hdr: string
  cnSub: boolean
  promo: { text: string; tone: string } | null
  sizeBytes: number
  pubMs: number
}

/** Enrich any torrent-like row with filterable meta in one pass (avoids repeated regex). */
export function enrichTorrent<T extends {
  title?: string
  subtitle?: string
  size?: string | number
  free?: boolean
  uploadvolumefactor?: number
  downloadvolumefactor?: number
  tags?: string[] | null
  labels?: string[] | null
  category?: string
  pub_date?: string
  time?: string
}>(row: T): T & EnrichedMeta {
  const tagSeed = [...(row.tags || []), ...(row.labels || []), row.category || ''].filter(Boolean) as string[]
  const tags = extractTorrentTags(`${row.title || ''} ${row.subtitle || ''}`, tagSeed)
  const pick = (tone: string) => tags.find((t) => t.tone === tone)?.label || ''
  const promo = volumeFactorLabel(
    row.uploadvolumefactor,
    row.downloadvolumefactor,
    row.free,
    [...(row.tags || []), ...(row.labels || [])],
  )
  return {
    ...row,
    tags,
    res: pick('res'),
    codec: pick('codec'),
    src: pick('src'),
    hdr: pick('hdr'),
    cnSub: tags.some((t) => t.tone === 'lang'),
    promo: promo ?? null,
    sizeBytes: parseSizeToBytes(row.size),
    pubMs: parsePubDateToMs(row.pub_date || row.time),
  }
}
