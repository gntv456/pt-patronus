import { http } from './http'

// 应用版本号 —— 离线兜底常量（构建期来自 package.json），非权威来源。
// 权威版本在后端 internal/version，经 /system/info 暴露；运行时优先读取，失败才用本兜底，故不必精确同步。
export const APP_VERSION: string = typeof __APP_VERSION__ !== 'undefined' ? __APP_VERSION__ : '0.00.02'

export interface SystemInfo {
  name: string
  // 后端未认证时不返 version（防指纹），故可选；调用方均以 ?. 守卫。
  version?: string
}

export interface UpdateCheckResult {
  hasUpdate: boolean
  currentVersion: string
  latestVersion: string
  releaseName?: string
  releaseNotes?: string
  downloadUrl?: string
  sha256?: string
  image?: string
  instructions?: string
  isForced?: boolean
  isBelowMin?: boolean
}

export interface DoctorFinding {
  id: string
  severity: 'info' | 'warn' | 'error'
  status: 'ok' | 'skipped' | 'degraded' | 'failed' | 'fixed'
  title: string
  detail?: string
  recommendation?: string
  fixable: boolean
  fixed: boolean
  context?: Record<string, any>
}
export interface DoctorReport {
  generated_at: string
  version?: string
  environment?: Record<string, any>
  findings: DoctorFinding[]
  status: 'healthy' | 'degraded' | 'failed'
  summary: { total: number; info: number; warn: number; error: number; fixed: number }
}

// 本机系统资源快照（对齐后端 sysinfo.Metrics；仪表盘「系统资源」卡用）。
// 集合字段（disks/temps）带 omitempty，无数据时后端省略，调用方用 ?? [] 兜底。
export interface DiskMetric {
  device: string
  total: number
  used: number
  usage: number
  type?: string
}
export interface SystemMetrics {
  online?: boolean
  cpu_usage?: number
  cpu_cores?: number
  mem_total?: number
  mem_used?: number
  mem_usage?: number
  swap_total?: number
  swap_used?: number
  net_rx_bytes?: number
  net_tx_bytes?: number
  net_rx_rate?: number
  net_tx_rate?: number
  process_count?: number
  disks?: DiskMetric[]
  load1?: number
  load5?: number
  load15?: number
  uptime?: number
  os?: string
  hostname?: string
  kernel_version?: string
  temps?: { name: string; temp: number }[]
}

export const systemApi = {
  // 本机资源即时快照（管理员）：CPU/内存/磁盘/负载。仪表盘「系统资源」卡用。
  metrics: () => http.get<SystemMetrics>('/system/metrics').then((r) => r.data),
  // 公开端点，免登录：登录页/关于页在认证前读取后端部署态版本。
  info: () => http.get<SystemInfo>('/system/info').then((r) => r.data),
  // 在线更新检查：platform 决定返回哪个平台的产物(docker=服务端自检，给升级命令；
  // android/windows=客户端，给下载地址)。需登录。客户端经本后端中转，不直连官方源。
  checkUpdate: (platform = 'docker') =>
    http.get<UpdateCheckResult>('/system/update/check', { params: { platform } }).then((r) => r.data),
  // 诊断面板（管理员）：跑健康检查 / 尝试修复。
  runDoctor: () => http.get<DoctorReport>('/system/doctor').then((r) => r.data),
  fixDoctor: () => http.post<DoctorReport>('/system/doctor/fix').then((r) => r.data),
}
