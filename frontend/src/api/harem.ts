import { http } from './http'
import { streamSSE } from './sse'

export interface InviteeInfo {
  user_id?: string
  username: string
  email?: string
  uploaded: number
  downloaded: number
  ratio?: string
  status: string
  enabled: boolean
}

export interface PrescriptionItem {
  site_id: number
  site_name: string
  remain: number
  permanent_count: number
  temporary_count: number
  can_buy: number
  can_invite: boolean
  harem_count: number
  dead_count: number
  invitees: InviteeInfo[]
  error?: string
}

export interface HaremOverview {
  total_sites: number
  total_invitees: number
  total_perm_invites: number
  total_temp_invites: number
  total_low_ratio: number
  total_banned: number
  total_no_data: number
}

export interface HaremResult {
  overview: HaremOverview
  prescriptions: PrescriptionItem[]
}

export interface PrescriptionExport {
  total_remain: number
  total_can_buy: number
  failed_count: number
  text: string
}

export const haremApi = {
  all: (refresh = false) => http.get<HaremResult>(`/harem${refresh ? '?refresh=1' : ''}`).then((r) => r.data),
  // 流式首屏：GET /harem/stream（SSE）。onItem 收每站药单（渐进显示）；onOverview 收 done 帧的全量汇总。
  allStream: (
    refresh = false,
    onItem: (item: PrescriptionItem) => void,
    onOverview: (o: HaremOverview) => void,
    signal?: AbortSignal,
  ) =>
    streamSSE(
      `/api/v1/harem/stream${refresh ? '?refresh=1' : ''}`,
      (event, data) => {
        if (event === 'batch' && data?.results?.length) onItem((data.results as PrescriptionItem[])[0])
        else if (event === 'done' && data?.overview) onOverview(data.overview as HaremOverview)
      },
      signal,
    ),
  syncOne: (id: number) => http.post<PrescriptionItem>(`/harem/sync/${id}`).then((r) => r.data),
  export: (refresh = false) =>
    http.get<PrescriptionExport>(`/harem/export${refresh ? '?refresh=1' : ''}`).then((r) => r.data),
}
