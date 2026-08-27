import { http } from './http'
import { streamSSE } from './sse'

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
  countries?: string[]
}

// 推荐源/分类筛选项。
export const recommendSources = [
  { key: '', label: '全部' },
  { key: 'douban', label: '豆瓣' },
  { key: 'tmdb', label: 'TMDB' },
  { key: 'bangumi', label: 'Bangumi' },
  { key: 'cunguangji', label: '寸光集' },
]

export const recommendCategories = [
  { key: 'movie', label: '热映电影' },
  { key: 'coming', label: '即将上映' },
  { key: 'tv', label: '剧集' },
  { key: 'anime', label: '动漫' },
  { key: 'duanju', label: '短剧' },
  { key: 'topMovie', label: '电影 Top250' },
  { key: 'topTv', label: '剧集 Top250' },
  { key: 'weekly', label: '一周口碑榜' },
]

// 日历条目（开播日历 / 订阅日历，对齐后端 media.CalendarEpisode）。
export interface CalendarEpisode {
  tmdb_id: string
  title: string
  season?: number
  episode?: number
  air_date: string // YYYY-MM-DD
  overview?: string
  poster_url?: string
}

export const recommendApi = {
  // 热门/趋势信息流：GET /media/recommend?source=&category=&page=&refresh=1
  // 全源失败时后端返回 results=null，此处兜底为 []，避免前端 .length 踩 null。
  feed: (source = '', category = 'movie', page = 1, refresh = false) =>
    http
      .get<{ results: MediaInfo[] }>('/media/recommend', {
        params: { source, category, page, ...(refresh ? { refresh: 1 } : {}) },
      })
      .then((r) => r.data.results || []),
  // 流式首屏：GET /media/recommend/stream（SSE），每源完成推一帧 batch。onBatch 收该源条目，逐批 push。
  feedStream: (
    source = '',
    category = 'movie',
    page = 1,
    refresh = false,
    onBatch: (items: MediaInfo[]) => void,
    signal?: AbortSignal,
  ) => {
    const params = new URLSearchParams({ source, category, page: String(page) })
    if (refresh) params.set('refresh', '1')
    return streamSSE(`/api/v1/media/recommend/stream?${params}`, (event, data) => {
      if (event === 'batch' && data?.results) onBatch(data.results as MediaInfo[])
    }, signal)
  },
  // 开播日历（全量 discover，按日期分组）：GET /media/calendar?kind=tv|movie&days=
  calendar: (kind: 'tv' | 'movie' = 'tv', days = 14) =>
    http
      .get<{ kind: string; days: number; calendar: Record<string, CalendarEpisode[]> }>('/media/calendar', { params: { kind, days } })
      .then((r) => r.data.calendar ?? {}),
  // 订阅日历（我的订阅的单集开播日程）：GET /media/calendar/subscriptions?days=
  subscriptionCalendar: (days = 30) =>
    http
      .get<{ days: number; calendar: Record<string, CalendarEpisode[]> }>('/media/calendar/subscriptions', { params: { days } })
      .then((r) => r.data.calendar ?? {}),
}
