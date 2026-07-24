<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { Refresh, SwitchButton } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import MetricBar from './MetricBar.vue'
import MetricChart from './MetricChart.vue'
import { monitorApi, parseMetrics, type MonitorTarget, type Metrics, type MetricSnapshot } from '@/api/monitor'
import { fmtBytes, fmtSpeed } from '@/utils/format'
import { isSSHAuthError, showSSHAuthFix } from '@/utils/ssh-fix'

type PowerAction = 'reboot' | 'shutdown'

const visible = defineModel<boolean>('visible', { default: false })
const props = defineProps<{ target?: MonitorTarget | null }>()

const latest = ref<Metrics | null>(null)
const snaps = ref<MetricSnapshot[]>([])
const loading = ref(false)
const powerLoading = ref<PowerAction | ''>('')

function seriesOf(pick: (m: Metrics) => number | undefined): number[] {
  return snaps.value.map((s) => pick(parseMetrics(s) ?? ({} as Metrics)) ?? 0)
}

const cpuSeries = computed(() => seriesOf((m) => m.cpu_usage))
const memSeries = computed(() => seriesOf((m) => m.mem_usage))
// 存储卡百分比须与「已用/总量」同源：用 sum(used)/sum(total)，不能用 max(单盘 usage)——
// 否则多盘时百分比取最满那块，与累加出的总量对不上（曾出现 78% 配 87.7TB/1.1PB 的矛盾）。
const diskUsage = computed(() => {
  const ds = latest.value?.disks
  if (!ds || !ds.length) return undefined
  const total = ds.reduce((n, d) => n + (d.total || 0), 0)
  if (!total) return undefined
  const used = ds.reduce((n, d) => n + (d.used || 0), 0)
  return (used / total) * 100
})
const diskTotal = computed(() => (latest.value?.disks ?? []).reduce((n, d) => n + (d.total || 0), 0))
const diskUsed = computed(() => (latest.value?.disks ?? []).reduce((n, d) => n + (d.used || 0), 0))
const hottestTemp = computed(() => {
  const ts = latest.value?.temps
  return ts && ts.length ? Math.max(...ts.map((t) => t.temp)) : undefined
})
// 空温度提示：区分云 VPS / Windows 本机（ACPI 需管理员或 LHM）/ 其它。
const tempEmptyHint = computed(() => {
  if (!latest.value) return '等待采集…'
  const p = props.target ? resolveProtocol(props.target) : ''
  if (p === 'ssh' || p === 'agent') return '主机未暴露温度传感器'
  if (p === 'local') {
    const os = (latest.value.os || '').toLowerCase()
    if (os.includes('windows')) {
      return 'Win 需管理员或安装 LibreHardwareMonitor'
    }
  }
  return '未读到温度传感器'
})
const serviceTypes = ['http', 'tcp', 'process', 'script']
const isServiceMonitor = computed(() => !!props.target && serviceTypes.includes(props.target.type))
const resolvedProtocol = computed(() => (props.target ? resolveProtocol(props.target) : ''))
const canPower = computed(() => {
  const t = props.target
  if (!t || t.type === 'camera') return false
  const p = resolveProtocol(t)
  if (t.type === 'host') return p === 'local' || p === 'ssh'
  if (t.type === 'nas') return p === 'local' || p === 'api'
  return false
})
const status = computed(() => {
  const online = props.target?.last_online
  if (online === true) return { text: '在线', cls: 'online' }
  if (online === false) return { text: '离线', cls: 'offline' }
  return { text: '未知', cls: 'unknown' }
})
const deviceID = computed(() => (props.target ? `#${props.target.id.toString().padStart(4, '0')}` : '-'))
const collectTime = computed(() =>
  props.target?.last_checked_at ? new Date(props.target.last_checked_at).toLocaleString('zh-CN', { hour12: false }) : '-',
)
const targetSubtitle = computed(() => {
  const t = props.target
  if (!t) return ''
  const parts = [targetTypeName(t.type), protocolName(resolvedProtocol.value)]
  const vendor = vendorName(t.vendor)
  if (vendor) parts.push(vendor)
  return parts.filter(Boolean).join(' · ')
})
const extra = computed<Record<string, unknown>>(() => latest.value?.extra || {})
const serviceRows = computed(() => serviceInfoRows())
const serviceOutput = computed(() => {
  const stdout = extraText('stdout', '')
  const stderr = extraText('stderr', '')
  const exitError = extraText('exit_error', '')
  return { stdout, stderr, exitError }
})

/** Swap：有容量显示已用/总量；0 或 omitempty 缺省且已有内存 → 未配置（云 VPS 常见）。 */
function formatSwap(m?: Metrics | null): string {
  if (!m) return '-'
  if (m.swap_total) return `${fmtBytes(m.swap_used ?? 0)} / ${fmtBytes(m.swap_total)}`
  if (m.mem_total) return '未配置'
  return '-'
}

function fmtUptime(sec?: number): string {
  if (!sec) return '-'
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  if (d > 0) return `${d}天${h}时`
  if (h > 0) return `${h}时${m}分`
  return `${m}分`
}

function pct(v?: number): string {
  return v === undefined || Number.isNaN(v) ? '-' : `${v.toFixed(v < 10 ? 1 : 0)}%`
}

function temp(v?: number): string {
  return v === undefined || Number.isNaN(v) ? '-' : `${v.toFixed(0)} ℃`
}

function extraText(key: string, fallback = '-'): string {
  const value = extra.value[key]
  if (value === undefined || value === null || value === '') return fallback
  return String(value)
}

function extraMs(key = 'response_ms'): string {
  const raw = Number(extra.value[key] || 0)
  return raw > 0 ? `${raw} ms` : '-'
}

function serviceInfoRows(): Array<{ label: string; value: string }> {
  const t = props.target
  if (!t) return []
  if (t.type === 'http') {
    return [
      { label: 'URL', value: extraText('url', t.host || '-') },
      { label: '请求方法', value: extraText('method', 'GET') },
      { label: '状态码', value: extraText('status_code') },
      { label: '状态文本', value: extraText('status') },
      { label: '响应耗时', value: extraMs() },
    ]
  }
  if (t.type === 'tcp') {
    return [
      { label: '地址', value: extraText('host', t.host || '-') },
      { label: '端口', value: extraText('port', String(t.port || '-')) },
      { label: '连接耗时', value: extraMs() },
    ]
  }
  if (t.type === 'process') {
    return [
      { label: 'SSH 主机', value: t.host || '-' },
      { label: '进程名称', value: extraText('process_name') },
      { label: '当前数量', value: extraText('count', '0') },
      { label: '最少数量', value: extraText('min_count', '1') },
    ]
  }
  if (t.type === 'script') {
    return [
      { label: 'SSH 主机', value: t.host || '-' },
      { label: '执行耗时', value: extraMs('duration_ms') },
      { label: '命令', value: extraText('command') },
    ]
  }
  return []
}

function humanTempName(raw: string): string {
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

function resolveProtocol(t: MonitorTarget): string {
  if (t.protocol) return t.protocol
  if (t.type === 'nas') return t.host ? 'api' : 'local'
  if (t.type === 'camera') return 'onvif'
  return t.host ? 'ssh' : 'local'
}

function targetTypeName(raw: string): string {
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

function protocolName(raw: string): string {
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

function vendorName(raw: string): string {
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

function diskTypeName(raw?: string): string {
  switch (raw) {
    case 'pool':
      return '存储池'
    case 'partition':
      return '分区'
    default:
      return raw || '磁盘'
  }
}

function usageColor(v?: number): string {
  if ((v ?? 0) >= 90) return '#ef4444'
  if ((v ?? 0) >= 80) return '#f59e0b'
  return '#14b8a6'
}

function donutStyle(v?: number) {
  const pct = Math.min(100, Math.max(0, v ?? 0))
  return {
    background: `conic-gradient(${usageColor(pct)} ${pct * 3.6}deg, rgba(148, 163, 184, 0.18) 0deg)`,
  }
}

/** 若快照未带 net_*_rate，用最近两条累计字节差补算（bytes/s）。 */
function withNetRate(snapsList: MetricSnapshot[], m: Metrics | null): Metrics | null {
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
  if (m.net_rx_rate === undefined && b.net_rx_bytes !== undefined && a.net_rx_bytes !== undefined && b.net_rx_bytes >= a.net_rx_bytes) {
    out.net_rx_rate = Math.round((b.net_rx_bytes - a.net_rx_bytes) / dt)
  }
  if (m.net_tx_rate === undefined && b.net_tx_bytes !== undefined && a.net_tx_bytes !== undefined && b.net_tx_bytes >= a.net_tx_bytes) {
    out.net_tx_rate = Math.round((b.net_tx_bytes - a.net_tx_bytes) / dt)
  }
  return out
}

async function load() {
  if (!props.target) return
  loading.value = true
  try {
    // 先 collect 写入含网速的新快照，再拉历史——避免 Promise.all 竞态：
    // collect 返回值若未带回 net_*_rate，详情会显示 0 B/s，而库里快照其实已有速率。
    await monitorApi.collect(props.target.id).catch(() => null)
    const s = await monitorApi.metrics(props.target.id, undefined, 120)
    snaps.value = s
    const raw = s.length ? parseMetrics(s[s.length - 1]) : null
    latest.value = withNetRate(s, raw)
  } finally {
    loading.value = false
  }
}

async function runPower(action: PowerAction) {
  const t = props.target
  if (!t) return
  const isShutdown = action === 'shutdown'
  const title = isShutdown ? '确认关机' : '确认重启'
  const detail = isShutdown ? '关机后可能需要手动开机，或通过 NAS/云厂商控制台恢复。' : '设备会短暂离线，稍后可回到本页刷新状态。'
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
  if (v && props.target) load()
})
watch(
  () => props.target?.id,
  () => {
    if (visible.value && props.target) load()
  },
)
</script>

<template>
  <el-drawer v-model="visible" size="min(900px, 94vw)" direction="rtl" class="device-detail-drawer">
    <template #header>
      <div class="detail-header">
        <div class="dh-main">
          <div class="dh-title">{{ target?.name || '设备详情' }}</div>
          <div class="dh-sub">{{ targetSubtitle }}</div>
        </div>
        <div class="dh-actions">
          <span class="status-pill" :class="status.cls"><i></i>{{ status.text }}</span>
        </div>
      </div>
    </template>

    <div v-loading="loading" class="detail">
      <template v-if="target && isServiceMonitor">
        <div class="overview-grid">
          <section class="hero-card hero-primary">
            <span class="hero-label">监控项</span>
            <strong>{{ target.name }}</strong>
            <div class="hero-line">设备ID {{ deviceID }}</div>
            <div class="hero-tags">
              <span>{{ targetTypeName(target.type) }}</span>
              <span>{{ protocolName(resolvedProtocol) }}</span>
            </div>
          </section>

          <section class="hero-card">
            <span class="hero-label">采集状态</span>
            <strong>{{ status.text }}</strong>
            <div class="system-mini">
              <div>
                <span>采集时间</span>
                <b>{{ collectTime }}</b>
              </div>
              <div>
                <span>耗时</span>
                <b>{{ extraMs('response_ms') !== '-' ? extraMs('response_ms') : extraMs('duration_ms') }}</b>
              </div>
            </div>
          </section>
        </div>

        <section class="panel span-2 service-panel">
          <div class="panel-title">
            <span>监控详情</span>
            <b>{{ status.text }}</b>
          </div>
          <div class="info-list">
            <div v-for="row in serviceRows" :key="row.label" class="info-row">
              <span>{{ row.label }}</span><b>{{ row.value }}</b>
            </div>
            <div class="info-row"><span>最后错误</span><b>{{ target.last_error || '-' }}</b></div>
          </div>
        </section>

        <section v-if="serviceOutput.stdout || serviceOutput.stderr || serviceOutput.exitError" class="panel span-2">
          <div class="panel-title"><span>命令输出</span></div>
          <pre v-if="serviceOutput.stdout" class="service-output">{{ serviceOutput.stdout }}</pre>
          <pre v-if="serviceOutput.stderr" class="service-output err">{{ serviceOutput.stderr }}</pre>
          <p v-if="serviceOutput.exitError" class="cmd-error">{{ serviceOutput.exitError }}</p>
        </section>
      </template>

      <template v-else-if="target?.type !== 'camera'">
        <div class="overview-grid">
          <section class="hero-card hero-primary">
            <span class="hero-label">设备名称</span>
            <strong>{{ target?.name || '-' }}</strong>
            <div class="hero-line">设备ID {{ deviceID }}</div>
            <div class="hero-tags">
              <span>{{ target?.host || '本机' }}</span>
              <span>{{ protocolName(resolvedProtocol) }}</span>
            </div>
          </section>

          <section class="hero-card">
            <span class="hero-label">系统信息</span>
            <strong>{{ latest?.os || '暂无系统信息' }}</strong>
            <div class="system-mini">
              <div>
                <span>采集时间</span>
                <b>{{ collectTime }}</b>
              </div>
              <div>
                <span>运行时长</span>
                <b>{{ fmtUptime(latest?.uptime) }}</b>
              </div>
            </div>
          </section>
        </div>

        <section v-if="canPower" class="power-panel">
          <div class="power-copy">
            <strong>电源操作</strong>
            <span>重启或关机前会再次确认，关机后可能需要手动开机。</span>
          </div>
          <div class="power-actions">
            <el-button
              size="large"
              :icon="Refresh"
              :loading="powerLoading === 'reboot'"
              :disabled="!!powerLoading"
              @click="runPower('reboot')"
            >
              重启设备
            </el-button>
            <el-button
              size="large"
              type="danger"
              plain
              :icon="SwitchButton"
              :loading="powerLoading === 'shutdown'"
              :disabled="!!powerLoading"
              @click="runPower('shutdown')"
            >
              关机
            </el-button>
          </div>
        </section>

        <div class="stat-grid">
          <section class="stat-card">
            <span>CPU</span>
            <strong>{{ pct(latest?.cpu_usage) }}</strong>
            <small>{{ latest?.cpu_cores ? `${latest.cpu_cores} 核心` : '核心数未知' }}</small>
          </section>
          <section class="stat-card">
            <span>内存</span>
            <strong>{{ pct(latest?.mem_usage) }}</strong>
            <small>{{ latest?.mem_total ? `${fmtBytes(latest.mem_used ?? 0)} / ${fmtBytes(latest.mem_total)}` : '容量未知' }}</small>
          </section>
          <section class="stat-card">
            <span>存储</span>
            <strong>{{ pct(diskUsage) }}</strong>
            <small>{{ diskTotal ? `${fmtBytes(diskUsed)} / ${fmtBytes(diskTotal)}` : '暂无磁盘数据' }}</small>
          </section>
          <section class="stat-card" :class="{ muted: hottestTemp === undefined }">
            <span>最高温度</span>
            <strong>{{ hottestTemp === undefined ? 'N/A' : temp(hottestTemp) }}</strong>
            <small>{{ latest?.temps?.length ? `${latest.temps.length} 个传感器` : tempEmptyHint }}</small>
          </section>
        </div>

        <section v-if="latest?.temps?.length" class="panel temp-panel">
          <div class="panel-title">
            <span>温度传感器</span>
            <b>最高 {{ temp(hottestTemp) }}</b>
          </div>
          <div class="temp-grid">
            <div v-for="(t, i) in latest.temps" :key="i" class="temp-row">
              <span :title="t.name">{{ humanTempName(t.name) }}</span>
              <b>{{ t.temp.toFixed(0) }} ℃</b>
            </div>
          </div>
        </section>

        <div class="detail-grid">
          <section class="panel">
            <div class="panel-title">
              <span>CPU 趋势</span>
              <b>{{ pct(latest?.cpu_usage) }}</b>
            </div>
            <MetricChart :data="cpuSeries" :height="74" />
            <MetricBar v-if="latest" label="当前 CPU" :value="latest.cpu_usage" :detail="latest.cpu_cores ? `${latest.cpu_cores} 核心` : ''" />
          </section>

          <section class="panel">
            <div class="panel-title">
              <span>内存趋势</span>
              <b>{{ pct(latest?.mem_usage) }}</b>
            </div>
            <MetricChart :data="memSeries" :height="74" />
            <MetricBar
              v-if="latest"
              label="当前内存"
              :value="latest.mem_usage"
              :detail="latest.mem_total ? `${fmtBytes(latest.mem_used ?? 0)} / ${fmtBytes(latest.mem_total)}` : ''"
            />
          </section>

          <section v-if="latest?.disks?.length" class="panel span-2">
            <div class="panel-title">
              <span>存储空间</span>
              <b>{{ latest.disks.length }} 个{{ target?.type === 'nas' ? '存储项' : '分区' }}</b>
            </div>
            <div class="storage-list">
              <div v-for="(d, i) in latest.disks" :key="i" class="storage-row">
                <div class="donut" :style="donutStyle(d.usage)">
                  <span>{{ pct(d.usage) }}</span>
                </div>
                <div class="storage-main">
                  <div class="storage-name">{{ d.device || `磁盘 ${i + 1}` }}</div>
                  <div class="storage-meta">
                    <span>{{ diskTypeName(d.type) }}</span>
                    <span>{{ fmtBytes(d.used) }} / {{ fmtBytes(d.total) }}</span>
                  </div>
                  <div class="storage-track">
                    <i :style="{ width: `${Math.min(100, Math.max(0, d.usage || 0))}%`, background: usageColor(d.usage) }"></i>
                  </div>
                </div>
              </div>
            </div>
          </section>

          <section class="panel">
            <div class="panel-title"><span>硬件信息</span></div>
            <div class="info-list">
              <div class="info-row"><span>CPU 核心</span><b>{{ latest?.cpu_cores ? `${latest.cpu_cores} 核` : '-' }}</b></div>
              <div class="info-row"><span>物理内存</span><b>{{ latest?.mem_total ? fmtBytes(latest.mem_total) : '-' }}</b></div>
              <div class="info-row"><span>Swap</span><b>{{ formatSwap(latest) }}</b></div>
              <div class="info-row"><span>进程数</span><b>{{ latest?.process_count || '-' }}</b></div>
              <div class="info-row"><span>主机名</span><b>{{ latest?.hostname || '-' }}</b></div>
              <div class="info-row"><span>内核版本</span><b>{{ latest?.kernel_version || '-' }}</b></div>
            </div>
          </section>

          <section class="panel">
            <div class="panel-title"><span>网络状态</span></div>
            <div class="info-list">
              <div class="info-row"><span>下行速率</span><b>↓ {{ fmtSpeed(latest?.net_rx_rate ?? 0) }}</b></div>
              <div class="info-row"><span>上行速率</span><b>↑ {{ fmtSpeed(latest?.net_tx_rate ?? 0) }}</b></div>
              <div class="info-row"><span>负载</span><b>{{ latest?.load1 !== undefined ? `${latest.load1.toFixed(2)} / ${latest.load5?.toFixed(2) ?? '-'} / ${latest.load15?.toFixed(2) ?? '-'}` : '-' }}</b></div>
              <div class="info-row"><span>地址</span><b>{{ target?.host || '本机' }}</b></div>
              <div class="info-row"><span>端口</span><b>{{ target?.port || '-' }}</b></div>
              <div class="info-row"><span>采集方式</span><b>{{ protocolName(resolvedProtocol) }}</b></div>
            </div>
          </section>
        </div>
      </template>

      <template v-else>
        <section v-if="latest?.device_info" class="panel camera-panel">
          <div class="panel-title"><span>设备信息</span></div>
          <div class="info-list">
            <div class="info-row"><span>厂商</span><b>{{ latest.device_info.manufacturer || '-' }}</b></div>
            <div class="info-row"><span>型号</span><b>{{ latest.device_info.model || '-' }}</b></div>
            <div class="info-row"><span>固件</span><b>{{ latest.device_info.firmware || '-' }}</b></div>
            <div class="info-row"><span>序列号</span><b>{{ latest.device_info.serial || '-' }}</b></div>
            <div class="info-row"><span>硬件</span><b>{{ latest.device_info.hardware || '-' }}</b></div>
          </div>
        </section>
        <section class="panel camera-panel">
          <div class="panel-title"><span>连接信息</span></div>
          <div class="info-row"><span>RTSP 地址</span><b class="mono">{{ target?.rtsp_url || '未获取' }}</b></div>
        </section>
      </template>

      <div v-if="target?.last_error" class="d-error">
        <span>最近错误：{{ target.last_error }}</span>
        <el-button v-if="isSSHAuthError(target.last_error)" size="small" text type="primary" @click="showSSHAuthFix">如何修复？</el-button>
      </div>
    </div>
  </el-drawer>
</template>

<style scoped>
.detail-header {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.dh-main {
  min-width: 0;
}
.dh-title {
  font-size: 18px;
  font-weight: 700;
  color: var(--text-strong);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dh-sub {
  margin-top: 3px;
  font-size: 12px;
  color: var(--text-muted);
}
.dh-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: flex-end;
}
.status-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 26px;
  padding: 0 10px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
  white-space: nowrap;
}
.status-pill i {
  width: 7px;
  height: 7px;
  border-radius: 50%;
}
.status-pill.online {
  color: #11998e;
  background: rgba(17, 153, 142, 0.12);
}
.status-pill.online i {
  background: #11998e;
}
.status-pill.offline {
  color: #ef4444;
  background: rgba(239, 68, 68, 0.12);
}
.status-pill.offline i {
  background: #ef4444;
}
.status-pill.unknown {
  color: var(--text-muted);
  background: rgba(148, 163, 184, 0.16);
}
.status-pill.unknown i {
  background: #94a3b8;
}
.detail {
  padding: 0 2px 18px;
}
.overview-grid,
.stat-grid,
.detail-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}
.overview-grid {
  margin-bottom: 12px;
}
.stat-grid {
  grid-template-columns: repeat(4, minmax(0, 1fr));
  margin-bottom: 12px;
}
.detail-grid {
  align-items: start;
}
.hero-card,
.stat-card,
.panel {
  border: 1px solid var(--glass-border);
  background: color-mix(in srgb, var(--glass-bg) 86%, transparent);
  border-radius: 8px;
  box-shadow: 0 10px 28px rgba(15, 23, 42, 0.08);
}
.hero-card {
  min-height: 148px;
  padding: 18px;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  overflow: hidden;
}
.hero-primary {
  color: #fff;
  border-color: rgba(20, 184, 166, 0.25);
  background:
    linear-gradient(135deg, rgba(15, 118, 110, 0.92), rgba(37, 99, 235, 0.82)),
    var(--glass-bg);
}
.hero-label {
  font-size: 12px;
  color: var(--text-muted);
}
.hero-primary .hero-label {
  color: rgba(255, 255, 255, 0.72);
}
.hero-card strong {
  margin-top: 10px;
  font-size: 24px;
  line-height: 1.2;
  color: var(--text-strong);
  word-break: break-word;
}
.hero-primary strong {
  color: #fff;
}
.hero-line {
  margin-top: 10px;
  font-size: 13px;
  color: var(--text-muted);
}
.hero-primary .hero-line {
  color: rgba(255, 255, 255, 0.78);
}
.hero-tags {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 12px;
}
.hero-tags span {
  max-width: 100%;
  padding: 4px 8px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.18);
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.system-mini {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  margin-top: 16px;
}
.system-mini span {
  display: block;
  font-size: 12px;
  color: var(--text-muted);
}
.system-mini b {
  display: block;
  margin-top: 3px;
  font-size: 13px;
  color: var(--text-strong);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.power-panel {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 12px;
  padding: 12px 14px;
  border: 1px solid rgba(239, 68, 68, 0.18);
  border-radius: 8px;
  background: rgba(239, 68, 68, 0.05);
}
.power-copy {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.power-copy strong {
  color: var(--text-strong);
  font-size: 14px;
}
.power-copy span {
  color: var(--text-muted);
  font-size: 12px;
  line-height: 1.5;
}
.power-actions {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  gap: 10px;
}
.stat-card.muted strong {
  color: var(--text-muted);
  font-size: 22px;
}
.stat-card {
  min-height: 94px;
  padding: 14px;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
}
.stat-card span,
.stat-card small {
  color: var(--text-muted);
  font-size: 12px;
}
.stat-card strong {
  font-size: 24px;
  line-height: 1.1;
  color: var(--text-strong);
}
.panel {
  padding: 16px;
}
.temp-panel {
  margin-bottom: 12px;
}
.span-2 {
  grid-column: span 2;
}
.panel-title {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 12px;
  margin-bottom: 12px;
}
.panel-title span {
  font-size: 14px;
  font-weight: 700;
  color: var(--text-strong);
}
.panel-title b {
  color: var(--text-muted);
  font-size: 12px;
  font-weight: 700;
}
.storage-list {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}
.storage-row {
  display: flex;
  gap: 14px;
  align-items: center;
  min-width: 0;
  padding: 10px 0;
  border-bottom: 1px solid var(--glass-border);
}
.storage-row:nth-last-child(-n + 2) {
  border-bottom: 0;
}
.donut {
  width: 64px;
  height: 64px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  flex: 0 0 auto;
  position: relative;
}
.donut::after {
  content: '';
  position: absolute;
  inset: 9px;
  border-radius: 50%;
  background: var(--app-bg, #fff);
}
.donut span {
  position: relative;
  z-index: 1;
  font-size: 12px;
  font-weight: 800;
  color: var(--text-strong);
}
.storage-main {
  flex: 1;
  min-width: 0;
}
.storage-name {
  font-size: 14px;
  font-weight: 700;
  color: var(--text-strong);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.storage-meta {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  margin-top: 5px;
  font-size: 12px;
  color: var(--text-muted);
}
.storage-track {
  height: 7px;
  margin-top: 8px;
  border-radius: 999px;
  background: rgba(148, 163, 184, 0.18);
  overflow: hidden;
}
.storage-track i {
  display: block;
  height: 100%;
  border-radius: inherit;
}
.info-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.info-row {
  display: flex;
  justify-content: space-between;
  gap: 14px;
  min-height: 30px;
  padding: 6px 0;
  border-bottom: 1px solid var(--glass-border);
  font-size: 13px;
}
.info-row:last-child {
  border-bottom: 0;
}
.info-row span {
  color: var(--text-muted);
  white-space: nowrap;
}
.info-row b {
  color: var(--text-strong);
  font-weight: 600;
  text-align: right;
  min-width: 0;
  overflow-wrap: anywhere;
}
.temp-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px 14px;
}
.temp-row {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  min-width: 0;
  padding: 7px 0;
  border-bottom: 1px solid var(--glass-border);
  font-size: 13px;
}
.temp-row span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-muted);
}
.temp-row b {
  flex: 0 0 auto;
  color: var(--text-strong);
}
.camera-panel + .camera-panel {
  margin-top: 12px;
}
.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
}
.service-panel {
  margin-bottom: 12px;
}
.service-output {
  max-height: 260px;
  overflow: auto;
  margin: 0 0 8px;
  padding: 10px 12px;
  border-radius: 8px;
  background: rgba(15, 23, 42, 0.06);
  color: var(--text-strong);
  font-size: 12px;
  line-height: 1.55;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.service-output.err,
.cmd-error {
  color: var(--pt-danger, #e74c3c);
  background: rgba(231, 76, 60, 0.08);
}
.cmd-error {
  margin: 0;
  padding: 10px 12px;
  border-radius: 8px;
  font-size: 12px;
}
.d-error {
  margin-top: 12px;
  padding: 10px 12px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  color: var(--pt-danger);
  background: rgba(231, 76, 60, 0.08);
  font-size: 12px;
}

@media (max-width: 760px) {
  .detail-header {
    align-items: flex-start;
    flex-direction: column;
  }
  .dh-actions {
    width: 100%;
    justify-content: flex-start;
  }
  .power-panel {
    align-items: flex-start;
    flex-direction: column;
  }
  .power-actions {
    width: 100%;
  }
  .power-actions :deep(.el-button) {
    flex: 1;
  }
  .overview-grid,
  .detail-grid,
  .storage-list {
    grid-template-columns: 1fr;
  }
  .stat-grid,
  .temp-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .span-2 {
    grid-column: span 1;
  }
  .storage-row:nth-last-child(-n + 2) {
    border-bottom: 1px solid var(--glass-border);
  }
  .storage-row:last-child {
    border-bottom: 0;
  }
}

@media (max-width: 520px) {
  .stat-grid,
  .system-mini,
  .temp-grid {
    grid-template-columns: 1fr;
  }
  .hero-card strong,
  .stat-card strong {
    font-size: 22px;
  }
}
</style>
