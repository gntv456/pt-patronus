import { http } from './http'

// 对应后端 GET /schedule/config（聚合快照 + cookie 保活 cron）。
export interface ScheduleConfig {
  snapshot_cron: string
  snapshot_default: string
  keepalive_cron: string
  keepalive_default: string
  // 后端派生字段：cron 按哪个钟解释、下一次什么时候跑（界面要能核对，不能只给用户一个表达式）
  cron_timezone?: string
  tz_is_utc?: boolean
  server_time?: string
  snapshot_next?: string
  keepalive_next?: string
  hint: string
}

export const scheduleApi = {
  getConfig: () => http.get<ScheduleConfig>('/schedule/config').then((r) => r.data),
  setConfig: (payload: { snapshot_cron?: string; keepalive_cron?: string }) =>
    http.put('/schedule/config', payload).then((r) => r.data),
}
