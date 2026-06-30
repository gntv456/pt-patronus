import { http } from './http'

// 站点搜索命中的单条种子（对齐 backend site.Torrent）。
// 注意：size 是站点已格式化的显示串（如 "4.3 GB"），非字节数，无需再 fmtBytes。
export interface SiteTorrent {
  id: string
  title: string
  subtitle?: string
  detail_url: string
  download_url: string
  size: string
  seeders: number
  leechers: number
  completed: number
  time?: string
  category?: string
  tags?: string[]
  free?: boolean
}

// 单站聚合结果（含错误容错字段：error 非空表示该站搜索失败，不影响其它站）。
export interface SiteSearchResult {
  site_id: number
  site_name: string
  torrents: SiteTorrent[]
  error?: string
}

export const searchApi = {
  // 跨启用站点聚合搜索：POST /search?keyword=...（空 body）。
  // category=movie/tv/music（空=综合）：MTeam 映射 mode、NexusPHP 发 cat[]+后置过滤音乐。
  // 返回每站一组；单站失败在 error 字段里，不致命。
  all: (keyword: string, category = '') =>
    http
      .post<{ results: SiteSearchResult[] }>('/search', null, {
        params: category ? { keyword, category } : { keyword },
      })
      .then((r) => r.data.results),
}
