// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { http } from './http'
import { streamSSE } from './sse'

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
  // Align TorrentInfo promo fields when backend fills them.
  downloadvolumefactor?: number
  uploadvolumefactor?: number
  pub_date?: string
  labels?: string[]
}

// 单站聚合结果（含错误容错字段：error 非空表示该站搜索失败，不影响其它站）。
export interface SiteSearchResult {
  site_id: number
  site_name: string
  // 失败站 Go 侧 nil slice → JSON null
  torrents: SiteTorrent[] | null
  error?: string
}

export const searchApi = {
  // 跨启用站点聚合搜索：POST /search?keyword=...（空 body）。
  // category=movie/tv/music（空=综合）：MTeam 映射 mode、NexusPHP 发 cat[]+后置过滤音乐。
  // siteIds 非空时仅搜索指定启用站点子集（搜索页「站点筛选」用；空=全部启用）。
  // 返回每站一组；单站失败在 error 字段里，不致命。
  //
  // 超时单独放宽到 60s：后端 SearchSites 每站 15s 超时 + 8 并发扇出，N 个启用站
  // 总耗时上限约 ceil(N/8)×15s（N>8 时必然 >15s）。http 实例默认 15s 在此场景必超时
  // （表现为「请求超时，请稍后重试」），60s 可覆盖 ~30 个启用站。
  all: (keyword: string, category = '', siteIds: number[] = []) => {
    const params: Record<string, string> = { keyword }
    if (category) params.category = category
    if (siteIds.length) params.site_ids = siteIds.join(',')
    return http
      .post<{ results: SiteSearchResult[] }>('/search', null, { params, timeout: 60000 })
      .then((r) => r.data.results ?? [])
  },
  // 流式跨站搜索：GET /search/stream（SSE），每站完成推一帧 batch（results 为单元素数组），
  // 逐站 push 到 results，实现「搜出一个显示一个」。signal 用于切换关键词时中断上一流。
  allStream: (
    keyword: string,
    category = '',
    siteIds: number[] = [],
    onSite: (r: SiteSearchResult) => void,
    signal?: AbortSignal,
  ) => {
    const params = new URLSearchParams({ keyword })
    if (category) params.set('category', category)
    if (siteIds.length) params.set('site_ids', siteIds.join(','))
    return streamSSE(
      `/api/v1/search/stream?${params}`,
      (event, data) => {
        if (event === 'batch' && data?.results?.length) onSite((data.results as SiteSearchResult[])[0])
        // done 帧带 list_error = 站点清单读失败：抛错让调用方 catch 留痕（不再「0 站全绿」）
        if (event === 'done' && data?.list_error) throw new Error(data.list_error)
      },
      signal,
    )
  },
}
