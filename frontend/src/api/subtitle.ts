import { http } from './http'

export type SubSource = 'assrt' | 'opensubtitles' | 'subhd' | 'zimuku'
export type LangCode = 'chs' | 'cht' | 'eng' | 'bi' | 'und'

export interface SubtitleResult {
  source: SubSource
  source_id: string
  title: string
  language: string
  lang_code: LangCode
  file_name?: string
  ext?: string
  extra?: string
}

export interface SubtitleConfig {
  assrt_token: string
  assrt_token_configured: boolean
  opensubtitles_key: string
  opensubtitles_key_configured: boolean
  subhd_cookie: string
  subhd_cookie_configured: boolean
  zimuku_cookie: string
  zimuku_cookie_configured: boolean
  target_lang: string // "" | "chs" | "cht"
}

export const subtitleApi = {
  search: (q: string, language = '') =>
    http
      .get<{ results: SubtitleResult[] }>('/subtitle/search', { params: { q, language } })
      .then((r) => r.data.results ?? []),
  // 下载（blob）→ 浏览器另存。返回 { filename, blob }。
  // 超时对齐链层预算：SubtitleChain.Download 给源 30s（解 zip + 转码另算），
  // 用全局 15s 会把「站点慢但会成功」掐成「请求超时，请稍后重试」——
  // 而后端仍在跑，用户以为失败、文件却不会落地（同 #111 首屏超时的量法）。
  download: (sub: Partial<SubtitleResult>) =>
    http
      .post<Blob>('/subtitle/download', sub, { responseType: 'blob', timeout: 45000 })
      .then((r) => ({ blob: r.data, filename: parseFilename(r.headers['content-disposition']) })),
  getConfig: () => http.get<SubtitleConfig>('/subtitle/config').then((r) => r.data),
  setConfig: (cfg: Partial<SubtitleConfig>) => http.put('/subtitle/config', cfg).then((r) => r.data),
}

export const SOURCE_LABEL: Record<SubSource, string> = {
  assrt: 'ASSRT',
  opensubtitles: 'OpenSubtitles',
  subhd: 'SubHD',
  zimuku: '字幕库',
}

export const LANG_LABEL: Record<LangCode, string> = {
  chs: '简体',
  cht: '繁体',
  eng: '英文',
  bi: '双语',
  und: '未知',
}

// 从 Content-Disposition: attachment; filename="x.srt" 解出文件名。
function parseFilename(cd?: string): string {
  if (!cd) return 'subtitle.srt'
  const m = /filename="?([^";]+)"?/.exec(cd)
  return m ? m[1] : 'subtitle.srt'
}
