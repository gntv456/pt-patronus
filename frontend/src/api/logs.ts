import { http } from './http'

export interface LogEntry {
  time: string
  level: string // DEBUG/INFO/WARN/ERROR
  message: string
  attrs?: { key: string; value: string }[]
}

export const logsApi = {
  list: (level = '', n = 500) =>
    http
      .get<{ entries: LogEntry[] }>(`/system/logs?${level ? `level=${level}&` : ''}n=${n}`)
      .then((r) => r.data.entries || []),
  clear: () => http.delete('/system/logs').then((r) => r.data),
}
