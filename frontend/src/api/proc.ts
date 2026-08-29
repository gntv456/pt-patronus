import { http } from './http'

// === 进程管理（SSH 远程主机）===

export interface ProcRow {
  pid: number
  ppid: number
  user: string
  name: string
  cmd: string
  state: string
  cpu_perc: number
  mem_perc: number
  rss: number // KB
  tty?: string
  time?: string
}

export interface NetConnRow {
  proto: string
  state: string
  local_addr: string
  local_port: string
  peer_addr?: string
  peer_port?: string
  pid: number
  process?: string
}

export interface KillRequest {
  pid: number
  force?: boolean
  confirm?: boolean
}

export const procApi = {
  list: (targetId: number) => http.get<{ processes: ProcRow[] }>(`/ops/proc/${targetId}/list`).then((r) => r.data),
  connections: (targetId: number) =>
    http.get<{ connections: NetConnRow[] }>(`/ops/proc/${targetId}/connections`).then((r) => r.data),
  kill: (targetId: number, data: KillRequest) =>
    http.post<{ ok: boolean; error?: string }>(`/ops/proc/${targetId}/kill`, data).then((r) => r.data),
}
