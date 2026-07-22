import { http } from './http'

// 对应后端 GET /schedule/config（聚合站点快照 cron）。
export interface ScheduleConfig {
  snapshot_cron: string
  snapshot_default: string
  hint: string
}

export const scheduleApi = {
  getConfig: () => http.get<ScheduleConfig>('/schedule/config').then((r) => r.data),
  setConfig: (snapshot_cron: string) =>
    http.put('/schedule/config', { snapshot_cron }).then((r) => r.data),
}
