import { http } from './http'

// 种子详情页（对齐 backend site.TorrentDetail）。字段 best-effort：站点未抽到的留空。
export interface TorrentDetail {
  title: string
  subtitle?: string
  poster_url?: string
  description?: string // 原始 HTML，前端渲染
  screenshots?: string[]
  mediainfo?: string
  file_list?: string[]
  imdb_id?: string
  douban_id?: string
  tmdb_id?: string
  category?: string
  size?: string
  codec?: string
  source?: string
  resolution?: string
  seeders?: number
  leechers?: number
  download_url?: string
  detail_url: string
}

export const seedbrowseApi = {
  // 抓取并解析单条种子详情页：GET /sites/:id/torrent/detail?url=...
  // 抓取失败由 HTTP 层抛错（502 + error 字段），成功才带 detail。
  detail: (siteId: number, url: string) =>
    http
      .get<{ detail?: TorrentDetail }>(`/sites/${siteId}/torrent/detail?url=${encodeURIComponent(url)}`)
      .then((r) => r.data),
}
