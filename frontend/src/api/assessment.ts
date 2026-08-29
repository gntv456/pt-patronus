import { http } from './http'
import { streamSSE } from './sse'

export interface AssessmentMetric {
  label: string
  required: number
  current: number
  unit: string
  is_completed: boolean
}

export interface AssessmentCard {
  site_id: number
  site_name: string
  status: string // SAFE | PROGRESS | DANGER | EXEMPT | UNKNOWN
  progress_percent: number
  remaining_days: number
  start_time?: number
  end_time?: number
  metrics: AssessmentMetric[]
  assessment_type: string
  description?: string
  error?: string
}

export const assessmentApi = {
  all: (refresh = false) =>
    http
      .get<{ sites: AssessmentCard[] }>(`/assessment${refresh ? '?refresh=1' : ''}`)
      // 后端 nil 切片序列化为 JSON null（EXEMPT/未配置时 metrics 为 null），归一为 [] 防模板 .length 崩溃
      .then((r) => (r.data.sites || []).map((c) => ({ ...c, metrics: c.metrics || [] }))),
  // 流式首屏：GET /assessment/stream（SSE），每站完成推一帧 batch（单元素数组），逐站 push。
  allStream: (refresh = false, onCard: (card: AssessmentCard) => void, signal?: AbortSignal) =>
    streamSSE(
      `/api/v1/assessment/stream${refresh ? '?refresh=1' : ''}`,
      (event, data) => {
        if (event === 'batch' && data?.results?.length) {
          const card = (data.results as AssessmentCard[])[0]
          onCard({ ...card, metrics: card.metrics || [] })
        }
      },
      signal,
    ),
  syncOne: (id: number) =>
    http.post<AssessmentCard>(`/assessment/sync/${id}`).then((r) => ({ ...r.data, metrics: r.data.metrics || [] })),
}
