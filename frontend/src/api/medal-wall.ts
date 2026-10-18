import { http } from './http'
import { streamSSE } from './sse'

export interface Medal {
  site_id: number
  site_name: string
  medal_id: string
  name: string
  description?: string
  image_small: string
  original_image_url?: string
  sale_begin_time?: number
  sale_end_time?: number
  price: number
  original_price?: number // 原价（溢价时 < price，前端划线展示）
  validity?: string
  bonus_rate?: string
  purchase_status: string // OWNED/AVAILABLE/OUT_OF_STOCK/NOT_ON_SALE/DISCONTINUED/UNKNOWN
  stock: number // -1=无限/未知
  currency: string
  group?: string
  wear_status: boolean
  is_owned: boolean
  actionable?: boolean
  error?: string
}

export interface ActionResult {
  success: boolean
  message: string
}

export const medalWallApi = {
  all: (refresh = false) =>
    http
      .get<{ medals: Medal[] }>(`/medalwall${refresh ? '?refresh=1' : ''}`)
      // 后端失败站可能产出 error 卡片；nil 切片归一为 []
      .then((r) => r.data.medals || []),
  // 流式首屏：GET /medalwall/stream（SSE），每站完成推一帧 batch（该站多张卡），逐批 push。
  allStream: (refresh = false, onCards: (cards: Medal[]) => void, signal?: AbortSignal) =>
    streamSSE(
      `/api/v1/medalwall/stream${refresh ? '?refresh=1' : ''}`,
      (event, data) => {
        if (event === 'batch' && data?.results) onCards(data.results as Medal[])
        // done 帧的 list_error = 站点清单读不出来。不读它就等于把一次查询失败
        // 渲染成「这个号还没勋章」（后端已经如实发了，之前整帧被忽略）。
        else if (event === 'done' && data?.list_error) throw new Error(data.list_error as string)
      },
      signal,
    ),
  buy: (siteId: number, medalId: string) =>
    http.post<ActionResult>('/medalwall/buy', { site_id: siteId, medal_id: medalId }).then((r) => r.data),
  wear: (siteId: number, medalId: string) =>
    http.post<ActionResult>('/medalwall/wear', { site_id: siteId, medal_id: medalId }).then((r) => r.data),
  takeOff: (siteId: number, medalId: string) =>
    http.post<ActionResult>('/medalwall/takeoff', { site_id: siteId, medal_id: medalId }).then((r) => r.data),
}
