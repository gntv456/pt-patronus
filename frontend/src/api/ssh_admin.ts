import { http } from './http'

// === SSH 管理（SSH 远程主机）===

export interface SshServiceStatus {
  active: boolean
  state: string
  enabled: boolean
  service_name: string
  port: string
  listen_addr: string
  version: string
}

export interface SshConfigRequest {
  port?: string
  listen_addr?: string
  confirm?: boolean
}

export interface SshSessionRow {
  user: string
  tty: string
  from: string
  login: string
  idle: string
  pid: number
  what: string
}

export interface SshLogRow {
  time: string
  type: string // Accepted/Failed/Disconnected
  user: string
  from: string
  port: string
  method: string // password/publickey
}

export const sshApi = {
  status: (targetId: number) =>
    http.get<SshServiceStatus>(`/ops/ssh/${targetId}/status`).then((r) => r.data),
  action: (targetId: number, action: 'start' | 'stop' | 'restart' | 'reload') =>
    http.post<{ ok: boolean; error?: string }>(`/ops/ssh/${targetId}/action`, { action }).then((r) => r.data),
  toggleEnable: (targetId: number, enable: boolean) =>
    http.post<{ ok: boolean; error?: string }>(`/ops/ssh/${targetId}/enable`, null, { params: { enable } }).then((r) => r.data),
  config: (targetId: number, data: SshConfigRequest) =>
    http.post<{ ok: boolean; error?: string }>(`/ops/ssh/${targetId}/config`, data).then((r) => r.data),
  sessions: (targetId: number) =>
    http.get<{ sessions: SshSessionRow[] }>(`/ops/ssh/${targetId}/sessions`).then((r) => r.data),
  killSession: (targetId: number, pid: number) =>
    http.delete<{ ok: boolean; error?: string }>(`/ops/ssh/${targetId}/sessions/${pid}`).then((r) => r.data),
  logs: (targetId: number, lines = 100) =>
    http.get<{ logs: SshLogRow[] }>(`/ops/ssh/${targetId}/logs`, { params: { lines } }).then((r) => r.data),
}
