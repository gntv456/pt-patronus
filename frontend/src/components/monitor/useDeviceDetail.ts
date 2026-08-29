import { ref, computed, watch, type Ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { monitorApi, parseMetrics, type MonitorTarget, type Metrics, type MetricSnapshot } from '@/api/monitor'
import { fmtBytes, fmtSpeed } from '@/utils/format'

export type PowerAction = 'reboot' | 'shutdown'

export const SERVICE_TYPES = ['http', 'tcp', 'process', 'script']

/** Swap：有容量显示已用/总量；0 或 omitempty 缺省且已有内存 → 未配置（云 VPS 常见）。 */
export function formatSwap(m?: Metrics | null): string {
  if (!m) return '-'
  if (m.swap_total) return `${fmtBytes(m.swap_used ?? 0)} / ${fmtBytes(m.swap_total)}`
  if (m.mem_total) return '未配置'
  return '-'
}

export function fmtUptime(sec?: number): string {
  if (!sec) return '-'
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  if (d > 0) return `${d}天${h}时`
  if (h > 0) return `${h}时${m}分`
  return `${m}分`
}

export function pct(v?: number): string {
  return v === undefined || Number.isNaN(v) ? '-' : `${v.toFixed(v < 10 ? 1 : 0)}%`
}

export function temp(v?: number): string {
  return v === undefined || Number.isNaN(v) ? '-' : `${v.toFixed(0)} ℃`
}

export function extraText(extra: Record<string, unknown>, key: string, fallback = '-'): string {
  const value = extra[key]
  if (value === undefined || value === null || value === '') return fallback
  return String(value)
}

export function extraMs(extra: Record<string, unknown>, key = 'response_ms'): string {
  const raw = Number(extra[key] || 0)
  return raw > 0 ? `${raw} ms` : '-'
}

export function serviceInfoRows(
  target: MonitorTarget,
  extra: Record<string, unknown>,
): Array<{ label: string; value: string }> {
  const t = target
  if (t.type === 'http') {
    return [
      { label: 'URL', value: extraText(extra, 'url', t.host || '-') },
      { label: '请求方法', value: extraText(extra, 'method', 'GET') },
      { label: '状态码', value: extraText(extra, 'status_code') },
      { label: '状态文本', value: extraText(extra, 'status') },
      { label: '响应耗时', value: extraMs(extra) },
    ]
  }
  if (t.type === 'tcp') {
    return [
      { label: '地址', value: extraText(extra, 'host', t.host || '-') },
      { label: '端口', value: extraText(extra, 'port', String(t.port || '-')) },
      { label: '连接耗时', value: extraMs(extra) },
    ]
  }
  if (t.type === 'process') {
    return [
      { label: 'SSH 主机', value: t.host || '-' },
      { label: '进程名称', value: extraText(extra, 'process_name') },
      { label: '当前数量', value: extraText(extra, 'count', '0') },
      { label: '最少数量', value: extraText(extra, 'min_count', '1') },
    ]
  }
  if (t.type === 'script') {
    return [
      { label: 'SSH 主机', value: t.host || '-' },
      { label: '执行耗时', value: extraMs(extra, 'duration_ms') },
      { label: '命令', value: extraText(extra, 'command') },
    ]
  }
  return []
}

export function humanTempName(raw: string): string {
  const name = (raw || '').trim()
  const key = name.toLowerCase()
  if (!key) return '温度传感器'
  if (key === 'acpitz') return 'ACPI 温区'
  if (key === 'nvme_composite') return 'NVMe 综合温度'
  const nvmeSensor = key.match(/^nvme_sensor_(\d+)$/)
  if (nvmeSensor) return `NVMe 传感器 ${nvmeSensor[1]}`
  const nvme = key.match(/^nvme(?:_temp)?_?(\d+)$/)
  if (nvme) return `NVMe 温度 ${nvme[1]}`
  const pkg = key.match(/^coretemp_package_id_(\d+)$/)
  if (pkg) return pkg[1] === '0' ? 'CPU 封装' : `CPU 封装 ${pkg[1]}`
  const core = key.match(/^coretemp_core_(\d+)$/)
  if (core) return `CPU 核心 ${core[1]}`
  if (key === 'k10temp_tctl') return 'AMD CPU 控制温度'
  if (key === 'k10temp_tccd1') return 'AMD CCD1 温度'
  if (key.startsWith('k10temp_tccd')) return `AMD CCD${key.replace('k10temp_tccd', '')} 温度`
  const nvidia = key.match(/^nvidia_gpu_(\d+)$/)
  if (nvidia) return nvidia[1] === '0' ? 'NVIDIA GPU' : `NVIDIA GPU ${nvidia[1]}`
  if (key.startsWith('lhm_')) return name.slice(4).replace(/_/g, ' ') + ' (LHM)'
  if (key.startsWith('ohm_')) return name.slice(4).replace(/_/g, ' ') + ' (OHM)'
  if (key.includes('cpu')) return name.replace(/_/g, ' ').replace(/\bcpu\b/i, 'CPU')
  return name.replace(/_/g, ' ')
}

export function resolveProtocol(t: MonitorTarget): string {
  if (t.protocol) return t.protocol
  if (t.type === 'nas') return t.host ? 'api' : 'local'
  if (t.type === 'camera') return 'onvif'
  return t.host ? 'ssh' : 'local'
}

export function targetTypeName(raw: string): string {
  switch (raw) {
    case 'host':
      return '服务器'
    case 'nas':
      return 'NAS'
    case 'camera':
      return '摄像头'
    case 'http':
      return '网站监控'
    case 'tcp':
      return '端口监控'
    case 'process':
      return '进程监控'
    case 'script':
      return '脚本监控'
    default:
      return raw || '设备'
  }
}

export function protocolName(raw: string): string {
  switch (raw) {
    case 'local':
      return '本机'
    case 'ssh':
      return 'SSH 远程'
    case 'agent':
      return 'Agent 上报'
    case 'api':
      return '群晖 / TrueNAS API'
    case 'snmp':
      return '通用 SNMP'
    case 'onvif':
      return 'ONVIF'
    case 'rtsp':
      return 'RTSP 直连'
    case 'http':
      return 'HTTP/HTTPS'
    case 'tcp':
      return 'TCP'
    default:
      return raw
  }
}

export function vendorName(raw: string): string {
  switch (raw) {
    case 'synology':
      return '群晖'
    case 'truenas':
    case 'freenas':
      return 'TrueNAS'
    case 'snmp':
      return 'SNMP'
    default:
      return raw
  }
}

export function diskTypeName(raw?: string): string {
  switch (raw) {
    case 'pool':
      return '存储池'
    case 'partition':
      return '分区'
    default:
      return raw || '磁盘'
  }
}

export function usageColor(v?: number): string {
  if ((v ?? 0) >= 90) return '#ef4444'
  if ((v ?? 0) >= 80) return '#f59e0b'
  return '#14b8a6'
}

export function donutStyle(v?: number) {
  const p = Math.min(100, Math.max(0, v ?? 0))
  return {
    background: `conic-gradient(${usageColor(p)} ${p * 3.6}deg, rgba(148, 163, 184, 0.18) 0deg)`,
  }
}

/** 若快照未带 net_*_rate，用最近两条累计字节差补算（bytes/s）。 */
export function withNetRate(snapsList: MetricSnapshot[], m: Metrics | null): Metrics | null {
  if (!m) return null
  // 快照已带速率字段（含 0=空闲）则直接用；缺字段时再按累计字节补算
  if (m.net_rx_rate !== undefined || m.net_tx_rate !== undefined) return m
  if (snapsList.length < 2) return m
  const a = parseMetrics(snapsList[snapsList.length - 2])
  const b = parseMetrics(snapsList[snapsList.length - 1])
  if (!a || !b) return m
  const t0 = new Date(snapsList[snapsList.length - 2].snap_at).getTime()
  const t1 = new Date(snapsList[snapsList.length - 1].snap_at).getTime()
  const dt = (t1 - t0) / 1000
  if (dt <= 0) return m
  const out = { ...m }
  if (
    m.net_rx_rate === undefined &&
    b.net_rx_bytes !== undefined &&
    a.net_rx_bytes !== undefined &&
    b.net_rx_bytes >= a.net_rx_bytes
  ) {
    out.net_rx_rate = Math.round((b.net_rx_bytes - a.net_rx_bytes) / dt)
  }
  if (
    m.net_tx_rate === undefined &&
    b.net_tx_bytes !== undefined &&
    a.net_tx_bytes !== undefined &&
    b.net_tx_bytes >= a.net_tx_bytes
  ) {
    out.net_tx_rate = Math.round((b.net_tx_bytes - a.net_tx_bytes) / dt)
  }
  return out
}

export interface DeviceDetailState {
  latest: Ref<Metrics | null>
  snaps: Ref<MetricSnapshot[]>
  loading: Ref<boolean>
  powerLoading: Ref<PowerAction | ''>
  cpuSeries: Ref<number[]>
  memSeries: Ref<number[]>
  diskStats: Ref<{ total: number; used: number; usage: number | undefined }>
  diskUsage: Ref<number | undefined>
  diskTotal: Ref<number>
  diskUsed: Ref<number>
  hottestTemp: Ref<number | undefined>
  tempEmptyHint: Ref<string>
  isServiceMonitor: Ref<boolean>
  resolvedProtocol: Ref<string>
  canPower: Ref<boolean>
  status: Ref<{ text: string; cls: string }>
  deviceID: Ref<string>
  collectTime: Ref<string>
  targetSubtitle: Ref<string>
  extra: Ref<Record<string, unknown>>
  serviceRows: Ref<Array<{ label: string; value: string }>>
  serviceOutput: Ref<{ stdout: string; stderr: string; exitError: string }>
  load: () => Promise<void>
  runPower: (action: PowerAction) => Promise<void>
}

export function useDeviceDetail(
  visible: Ref<boolean>,
  target: Ref<MonitorTarget | null | undefined>,
): DeviceDetailState {
  const latest = ref<Metrics | null>(null)
  const snaps = ref<MetricSnapshot[]>([])
  const loading = ref(false)
  const powerLoading = ref<PowerAction | ''>('')

  function seriesOf(pick: (m: Metrics) => number | undefined): number[] {
    return snaps.value.map((s) => pick(parseMetrics(s) ?? ({} as Metrics)) ?? 0)
  }

  const cpuSeries = computed(() => seriesOf((m) => m.cpu_usage))
  const memSeries = computed(() => seriesOf((m) => m.mem_usage))
  // 磁盘总量须去重后再累加（btrfs子卷/ZFS dataset 报相同整盘容量会乘N倍）。
  const diskStats = computed(() => {
    const seen = new Set<number>()
    const devs = new Set<string>()
    let t = 0,
      u = 0
    for (const d of latest.value?.disks ?? []) {
      if (d.total <= 0 || devs.has(d.device || '') || seen.has(d.total)) continue
      devs.add(d.device || '')
      seen.add(d.total)
      t += d.total
      u += d.used || 0
    }
    return { total: t, used: u, usage: t ? (u / t) * 100 : undefined }
  })
  const diskUsage = computed(() => diskStats.value.usage)
  const diskTotal = computed(() => diskStats.value.total)
  const diskUsed = computed(() => diskStats.value.used)
  const hottestTemp = computed(() => {
    const ts = latest.value?.temps
    return ts && ts.length ? Math.max(...ts.map((t) => t.temp)) : undefined
  })
  // 空温度提示：区分云 VPS / Windows 本机（ACPI 需管理员或 LHM）/ 其它。
  const tempEmptyHint = computed(() => {
    if (!latest.value) return '等待采集…'
    const p = target.value ? resolveProtocol(target.value) : ''
    if (p === 'ssh' || p === 'agent') return '主机未暴露温度传感器'
    if (p === 'local') {
      const os = (latest.value.os || '').toLowerCase()
      if (os.includes('windows')) {
        return 'Win 需管理员或安装 LibreHardwareMonitor'
      }
    }
    return '未读到温度传感器'
  })
  const isServiceMonitor = computed(() => !!target.value && SERVICE_TYPES.includes(target.value.type))
  const resolvedProtocol = computed(() => (target.value ? resolveProtocol(target.value) : ''))
  const canPower = computed(() => {
    const t = target.value
    if (!t || t.type === 'camera') return false
    const p = resolveProtocol(t)
    if (t.type === 'host') return p === 'local' || p === 'ssh'
    if (t.type === 'nas') return p === 'local' || p === 'api'
    return false
  })
  const status = computed(() => {
    const online = target.value?.last_online
    if (online === true) return { text: '在线', cls: 'online' }
    if (online === false) return { text: '离线', cls: 'offline' }
    return { text: '未知', cls: 'unknown' }
  })
  const deviceID = computed(() => (target.value ? `#${target.value.id.toString().padStart(4, '0')}` : '-'))
  const collectTime = computed(() =>
    target.value?.last_checked_at
      ? new Date(target.value.last_checked_at).toLocaleString('zh-CN', { hour12: false })
      : '-',
  )
  const targetSubtitle = computed(() => {
    const t = target.value
    if (!t) return ''
    const parts = [targetTypeName(t.type), protocolName(resolvedProtocol.value)]
    const vendor = vendorName(t.vendor)
    if (vendor) parts.push(vendor)
    return parts.filter(Boolean).join(' · ')
  })
  const extra = computed<Record<string, unknown>>(() => latest.value?.extra || {})
  const serviceRows = computed(() => (target.value ? serviceInfoRows(target.value, extra.value) : []))
  const serviceOutput = computed(() => {
    const stdout = extraText(extra.value, 'stdout', '')
    const stderr = extraText(extra.value, 'stderr', '')
    const exitError = extraText(extra.value, 'exit_error', '')
    return { stdout, stderr, exitError }
  })

  async function load() {
    if (!target.value) return
    loading.value = true
    try {
      // 先 collect 写入含网速的新快照，再拉历史——避免 Promise.all 竞态：
      // collect 返回值若未带回 net_*_rate，详情会显示 0 B/s，而库里快照其实已有速率。
      await monitorApi.collect(target.value.id).catch(() => null)
      const s = await monitorApi.metrics(target.value.id, undefined, 120)
      snaps.value = s
      const raw = s.length ? parseMetrics(s[s.length - 1]) : null
      latest.value = withNetRate(s, raw)
    } finally {
      loading.value = false
    }
  }

  async function runPower(action: PowerAction) {
    const t = target.value
    if (!t) return
    const isShutdown = action === 'shutdown'
    const title = isShutdown ? '确认关机' : '确认重启'
    const detail = isShutdown
      ? '关机后可能需要手动开机，或通过 NAS/云厂商控制台恢复。'
      : '设备会短暂离线，稍后可回到本页刷新状态。'
    try {
      await ElMessageBox.confirm(`确定要${isShutdown ? '关闭' : '重启'}「${t.name}」吗？${detail}`, title, {
        type: 'warning',
        confirmButtonText: isShutdown ? '关机' : '重启',
        cancelButtonText: '取消',
      })
    } catch {
      return
    }

    powerLoading.value = action
    try {
      const r = await monitorApi.power(t.id, action)
      if (!r.ok) throw new Error(r.error || '电源操作失败')
      ElMessage.success(isShutdown ? '关机命令已发送' : '重启命令已发送')
    } catch (e) {
      ElMessage.error((e as Error).message || '电源操作失败')
    } finally {
      powerLoading.value = ''
    }
  }

  watch(visible, (v) => {
    if (v && target.value) load()
  })
  watch(
    () => target.value?.id,
    () => {
      if (visible.value && target.value) load()
    },
  )

  return {
    latest,
    snaps,
    loading,
    powerLoading,
    cpuSeries,
    memSeries,
    diskStats,
    diskUsage,
    diskTotal,
    diskUsed,
    hottestTemp,
    tempEmptyHint,
    isServiceMonitor,
    resolvedProtocol,
    canPower,
    status,
    deviceID,
    collectTime,
    targetSubtitle,
    extra,
    serviceRows,
    serviceOutput,
    load,
    runPower,
  }
}
