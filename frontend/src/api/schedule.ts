import { http } from './http'

// 对应后端 GET /schedule/config（聚合快照 + cookie 保活 cron）。
export interface ScheduleConfig {
  snapshot_cron: string
  snapshot_default: string
  keepalive_cron: string
  keepalive_default: string
  hint: string
}

export const scheduleApi = {
  getConfig: () => http.get<ScheduleConfig>('/schedule/config').then((r) => r.data),
  setConfig: (payload: { snapshot_cron?: string; keepalive_cron?: string }) =>
    http.put('/schedule/config', payload).then((r) => r.data),
}
