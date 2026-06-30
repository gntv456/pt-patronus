import { http } from './http'

// 统一媒体元数据（对齐 backend media.MediaInfo）。
export interface MediaInfo {
  source: string // tmdb | douban | bangumi | musicbrainz
  source_id: string
  title: string
  original_title?: string
  year: number
  type: string // movie | tv | music
  overview?: string
  poster_url: string
  backdrop_url?: string
  release_date?: string
  rating?: number
  genres?: string[]
}

// 推荐源/分类筛选项。
export const recommendSources = [
  { key: '', label: '全部' },
  { key: 'douban', label: '豆瓣' },
  { key: 'tmdb', label: 'TMDB' },
  { key: 'bangumi', label: 'Bangumi' },
  { key: 'xingguangji', label: '星光集' },
]

export const recommendCategories = [
  { key: 'movie', label: '热映电影' },
  { key: 'coming', label: '即将上映' },
  { key: 'tv', label: '剧集' },
  { key: 'anime', label: '动漫' },
  { key: 'duanju', label: '短剧' },
]

export const recommendApi = {
  // 热门/趋势信息流：GET /media/recommend?source=&category=&page=&refresh=1
  // 全源失败时后端返回 results=null，此处兜底为 []，避免前端 .length 踩 null。
  feed: (source = '', category = 'movie', page = 1, refresh = false) =>
    http
      .get<{ results: MediaInfo[] }>('/media/recommend', {
        params: { source, category, page, ...(refresh ? { refresh: 1 } : {}) },
      })
      .then((r) => r.data.results || []),
}
