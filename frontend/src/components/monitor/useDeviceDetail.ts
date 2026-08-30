import { ref, computed, watch, type Ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { monitorApi, parseMetrics, type MonitorTarget, type Metrics, type MetricSnapshot } from '@/api/monitor'
import { fmtSpeed } from '@/utils/format'
import {
  SERVICE_TYPES,
  formatSwap,
  fmtUptime,
  pct,
  temp,
  extraText,
  extraMs,
  serviceInfoRows,
  humanTempName,
  resolveProtocol,
  targetTypeName,
  protocolName,
  vendorName,
  diskTypeName,
  usageColor,
  donutStyle,
  withNetRate,
} from './deviceDetailFormatters'

// 纯格式化工具原样迁至 deviceDetailFormatters.ts，此处 re-export 保持旧导入路径不变。
export {
  SERVICE_TYPES,
  formatSwap,
  fmtUptime,
  pct,
  temp,
  extraText,
  extraMs,
  serviceInfoRows,
  humanTempName,
  resolveProtocol,
  targetTypeName,
  protocolName,
  vendorName,
  diskTypeName,
  usageColor,
  donutStyle,
  withNetRate,
}

export type PowerAction = 'reboot' | 'shutdown'

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
