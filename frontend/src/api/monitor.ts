import { http } from './http'

export interface MonitorTarget {
  id: number
  name: string
  type: string // host | nas | camera
  host: string
  port: number
  vendor: string
  protocol: string
  username?: string
  has_private_key?: boolean
  rtsp_url: string
  interval_sec: number
  timeout_sec: number
  enabled: boolean
  thresholds: string
  last_online: boolean | null
  last_error: string
  last_checked_at: string | null
  created_at: string
  updated_at: string
}

export interface TargetInput {
  name: string
  type: string
  host?: string
  port?: number
  vendor?: string
  protocol?: string
  rtsp_url?: string
  interval_sec?: number
  timeout_sec?: number
  username?: string
  password?: string
  private_key?: string
  thresholds?: string
  enabled?: boolean
}

export interface DiskMetric {
  device: string
  total: number
  used: number
  usage: number
  type?: string
}
export interface TempMetric {
  name: string
  temp: number
}
export interface DeviceInfo {
  manufacturer: string
  model: string
  firmware: string
  serial: string
  hardware: string
}

export interface Metrics {
  online: boolean
  cpu_usage?: number
  cpu_cores?: number
  mem_total?: number
  mem_used?: number
  mem_usage?: number
  swap_total?: number
  swap_used?: number
  disks?: DiskMetric[]
  net_rx_bytes?: number
  net_tx_bytes?: number
  net_rx_rate?: number
  net_tx_rate?: number
  load1?: number
  load5?: number
  load15?: number
  temps?: TempMetric[]
  process_count?: number
  uptime?: number
  os?: string
  hostname?: string
  kernel_version?: string
  device_info?: DeviceInfo
  extra?: Record<string, unknown>
}

export interface MetricSnapshot {
  id: number
  target_id: number
  snap_at: string
  online: boolean
  metrics: string // JSON（Metrics 序列化）
  created_at: string
}

export interface MonitorAlert {
  id: number
  target_id: number
  level: string // info | warning | danger
  metric: string
  value: string
  message: string
  read: boolean
  created_at: string
  resolved_at: string | null
}

/** 解析 MetricSnapshot.metrics JSON */
export function parseMetrics(s: MetricSnapshot): Metrics | null {
  try {
    return JSON.parse(s.metrics) as Metrics
  } catch {
    return null
  }
}

export const monitorApi = {
  list: () => http.get<{ targets: MonitorTarget[] }>('/monitors').then((r) => r.data.targets),
  create: (input: TargetInput) => http.post<MonitorTarget>('/monitors', input).then((r) => r.data),
  update: (id: number, input: TargetInput) =>
    http.put<MonitorTarget>(`/monitors/${id}`, input).then((r) => r.data),
  remove: (id: number) => http.delete(`/monitors/${id}`).then((r) => r.data),
  probe: (id: number) =>
    http.post<{ ok: boolean; online: boolean; error?: string }>(`/monitors/${id}/probe`).then((r) => r.data),
  collect: (id: number) =>
    http
      .post<{ ok: boolean; metrics?: Metrics; error?: string }>(`/monitors/${id}/collect`)
      .then((r) => r.data),
  power: (id: number, action: 'reboot') =>
    http.post<{ ok: boolean; error?: string }>(`/monitors/${id}/power`, { action }).then((r) => r.data),
  metrics: (id: number, since?: string, limit?: number) =>
    http
      .get<{ snapshots: MetricSnapshot[] }>(`/monitors/${id}/metrics`, { params: { since, limit } })
      .then((r) => r.data.snapshots),
  alerts: (targetId?: number, unread = false) =>
    http
      .get<{ alerts: MonitorAlert[] }>('/monitor/alerts', { params: { target_id: targetId, unread } })
      .then((r) => r.data.alerts),
  readAlert: (id: number) => http.post(`/monitor/alerts/${id}/read`).then((r) => r.data),
  /** 拼接 RTSP→WS 流地址（前端 jessibuca 消费；?ticket=<一次性> 鉴权，url 由服务端按 id 从 DB 取） */
  streamURL: (id: number, ticket: string) => {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    return `${proto}://${location.host}/api/v1/monitor/cameras/${id}/stream?ticket=${encodeURIComponent(ticket)}`
  },
}

// Agent — push 架构的被监控 agent（per-agent secret 鉴权，1:1 关联 MonitorTarget；明文 secret 不回传）。
export interface Agent {
  id: number
  name: string
  secret_hint: string
  enabled: boolean
  target_id: number
  online: boolean
  version: string
  hostname: string
  ip: string
  last_seen_at: string | null
  created_at: string
}

export const agentApi = {
  list: () => http.get<{ agents: Agent[] }>('/monitors/agents').then((r) => r.data.agents),
  create: (name: string) =>
    http.post<{ agent: Agent; plain_secret: string }>('/monitors/agents', { name }).then((r) => r.data),
  remove: (id: number) => http.delete(`/monitors/agents/${id}`).then((r) => r.data),
  rotate: (id: number) =>
    http.post<{ plain_secret: string }>(`/monitors/agents/${id}/rotate`).then((r) => r.data),
}
