<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, computed } from 'vue'
import { systemApi, type SystemMetrics } from '@/api/system'
import { humanTempName } from '@/components/monitor/useDeviceDetail'
import GlassIcon from '@/components/GlassIcon.vue'

const metrics = ref<SystemMetrics>({})
const loading = ref(false)
const error = ref('')
const history = ref<SystemMetrics[]>([])
const maxHistory = 60
let timer: number | undefined

const cpuHistory = computed(() => history.value.map((m) => m.cpu_usage ?? 0))
const memHistory = computed(() => history.value.map((m) => m.mem_usage ?? 0))
const netRxHistory = computed(() => history.value.map((m) => m.net_rx_rate ?? 0))
const netTxHistory = computed(() => history.value.map((m) => m.net_tx_rate ?? 0))

function fmtBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return (bytes / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1) + ' ' + units[i]
}

function fmtUptime(seconds: number): string {
  if (!seconds) return '-'
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (d > 0) return `${d}天 ${h}小时`
  if (h > 0) return `${h}小时 ${m}分钟`
  return `${m}分钟`
}

async function refresh() {
  loading.value = true
  try {
    const m = await systemApi.metrics()
    metrics.value = m
    history.value.push(m)
    if (history.value.length > maxHistory) history.value.shift()
    error.value = ''
  } catch (e: any) {
    error.value = e.message || '采集失败'
  } finally {
    loading.value = false
  }
}

// 迷你折线图 SVG path
function sparkline(data: number[], max = 100): string {
  if (data.length < 2) return ''
  const w = 100
  const h = 30
  const step = w / (maxHistory - 1)
  return data
    .map((v, i) => {
      const x = i * step
      const y = h - (Math.min(v, max) / max) * h
      return `${i === 0 ? 'M' : 'L'}${x.toFixed(1)},${y.toFixed(1)}`
    })
    .join(' ')
}

onMounted(() => {
  refresh()
  timer = window.setInterval(refresh, 3000)
})
onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <div class="ops-monitor">
    <div class="monitor-scope glass">
      <span class="scope-dot"></span>
      <span class="scope-label">本机监控</span>
      <span class="text-muted small">（采集后端服务器所在主机的实时指标）</span>
    </div>
    <!-- 顶部指标卡片 -->
    <div class="metric-grid">
      <!-- CPU -->
      <div class="metric-card glass">
        <div class="metric-header">
          <GlassIcon name="dashboard" from="#6c5ce7" to="#4285f4" :size="28" />
          <span class="metric-label">CPU</span>
          <span class="metric-value">{{ (metrics.cpu_usage ?? 0).toFixed(1) }}%</span>
        </div>
        <div class="metric-bar">
          <div class="metric-bar-fill" :style="{ width: (metrics.cpu_usage ?? 0) + '%' }" />
        </div>
        <svg class="sparkline" viewBox="0 0 100 30" preserveAspectRatio="none">
          <path :d="sparkline(cpuHistory)" fill="none" stroke="#6c5ce7" stroke-width="1.5" />
        </svg>
        <div class="metric-sub">{{ metrics.cpu_cores ?? '-' }} 核心</div>
      </div>

      <!-- 内存 -->
      <div class="metric-card glass">
        <div class="metric-header">
          <GlassIcon name="dashboard" from="#11998e" to="#38ef7d" :size="28" />
          <span class="metric-label">内存</span>
          <span class="metric-value">{{ (metrics.mem_usage ?? 0).toFixed(1) }}%</span>
        </div>
        <div class="metric-bar">
          <div class="metric-bar-fill green" :style="{ width: (metrics.mem_usage ?? 0) + '%' }" />
        </div>
        <svg class="sparkline" viewBox="0 0 100 30" preserveAspectRatio="none">
          <path :d="sparkline(memHistory)" fill="none" stroke="#38ef7d" stroke-width="1.5" />
        </svg>
        <div class="metric-sub">{{ fmtBytes(metrics.mem_used ?? 0) }} / {{ fmtBytes(metrics.mem_total ?? 0) }}</div>
      </div>

      <!-- 负载 -->
      <div class="metric-card glass">
        <div class="metric-header">
          <GlassIcon name="dashboard" from="#f7971e" to="#ff6a00" :size="28" />
          <span class="metric-label">负载</span>
          <span class="metric-value">{{ (metrics.load1 ?? 0).toFixed(2) }}</span>
        </div>
        <div class="load-row">
          <span class="pt-chip">1m: {{ (metrics.load1 ?? 0).toFixed(2) }}</span>
          <span class="pt-chip">5m: {{ (metrics.load5 ?? 0).toFixed(2) }}</span>
          <span class="pt-chip">15m: {{ (metrics.load15 ?? 0).toFixed(2) }}</span>
        </div>
        <div class="metric-sub">进程数: {{ metrics.process_count ?? '-' }}</div>
      </div>

      <!-- 网络 -->
      <div class="metric-card glass">
        <div class="metric-header">
          <GlassIcon name="dashboard" from="#4cc9f0" to="#4361ee" :size="28" />
          <span class="metric-label">网络</span>
        </div>
        <div class="net-row">
          <div class="net-item">
            <span class="net-arrow down">↓</span>
            <span>{{ fmtBytes(metrics.net_rx_rate ?? 0) }}/s</span>
          </div>
          <div class="net-item">
            <span class="net-arrow up">↑</span>
            <span>{{ fmtBytes(metrics.net_tx_rate ?? 0) }}/s</span>
          </div>
        </div>
        <svg class="sparkline" viewBox="0 0 100 30" preserveAspectRatio="none">
          <path :d="sparkline(netRxHistory, Math.max(...netRxHistory, 1))" fill="none" stroke="#4cc9f0" stroke-width="1.5" />
          <path :d="sparkline(netTxHistory, Math.max(...netTxHistory, 1))" fill="none" stroke="#4361ee" stroke-width="1.5" />
        </svg>
      </div>

      <!-- 运行时间 -->
      <div class="metric-card glass">
        <div class="metric-header">
          <GlassIcon name="dashboard" from="#8e44ad" to="#3498db" :size="28" />
          <span class="metric-label">运行</span>
          <span class="metric-value small">{{ fmtUptime(metrics.uptime ?? 0) }}</span>
        </div>
        <div class="metric-sub">{{ metrics.hostname ?? '-' }}</div>
        <div class="metric-sub">{{ metrics.os ?? '-' }}</div>
      </div>
    </div>

    <!-- 磁盘 -->
    <div v-if="metrics.disks?.length" class="disk-section glass">
      <h3 class="section-title">磁盘</h3>
      <div class="disk-list">
        <div v-for="disk in metrics.disks" :key="disk.device" class="disk-item">
          <div class="disk-info">
            <span class="disk-path">{{ disk.device }}</span>
            <span class="disk-usage">{{ fmtBytes(disk.used) }} / {{ fmtBytes(disk.total) }}</span>
          </div>
          <div class="disk-bar">
            <div
              class="disk-bar-fill"
              :class="{ danger: disk.usage > 90, warn: disk.usage > 75 && disk.usage <= 90 }"
              :style="{ width: disk.usage + '%' }"
            />
          </div>
          <span class="disk-pct" :class="{ danger: disk.usage > 90 }">{{ disk.usage.toFixed(1) }}%</span>
        </div>
      </div>
    </div>

    <!-- 温度 -->
    <div v-if="metrics.temps?.length" class="temp-section glass">
      <h3 class="section-title">温度</h3>
      <div class="temp-list">
        <span v-for="t in metrics.temps" :key="t.name" class="temp-chip" :class="{ hot: t.temp > 70, warm: t.temp > 50 && t.temp <= 70 }">
          {{ humanTempName(t.name) }}: {{ t.temp.toFixed(0) }}°C
        </span>
      </div>
    </div>

    <div v-if="error" class="monitor-error">{{ error }}</div>
  </div>
</template>

<style scoped>
.ops-monitor {
  display: flex;
  flex-direction: column;
  gap: var(--sp-4);
}
.metric-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: var(--sp-3);
}
.metric-card {
  padding: var(--sp-4);
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
  border-radius: var(--radius-md);
}
.metric-header {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
}
.metric-label {
  font-size: var(--fs-md);
  color: var(--text-muted);
  font-weight: 500;
}
.metric-value {
  margin-left: auto;
  font-size: var(--fs-xl);
  font-weight: 700;
  color: var(--text-strong);
}
.metric-value.small {
  font-size: var(--fs-lg);
}
.metric-bar {
  height: 6px;
  background: var(--surface-muted);
  border-radius: var(--radius-pill);
  overflow: hidden;
}
.metric-bar-fill {
  height: 100%;
  background: var(--grad-primary);
  border-radius: var(--radius-pill);
  transition: width 0.5s ease;
}
.metric-bar-fill.green {
  background: var(--grad-success);
}
.sparkline {
  width: 100%;
  height: 30px;
  opacity: 0.7;
}
.metric-sub {
  font-size: var(--fs-xs);
  color: var(--text-muted);
}
.load-row, .net-row {
  display: flex;
  gap: var(--sp-1);
  flex-wrap: wrap;
}
.net-row {
  justify-content: space-between;
  font-size: var(--fs-sm);
}
.net-item {
  display: flex;
  align-items: center;
  gap: 4px;
}
.net-arrow {
  font-weight: 700;
}
.net-arrow.down { color: #4cc9f0; }
.net-arrow.up { color: #4361ee; }

.disk-section, .temp-section {
  padding: var(--sp-4);
  border-radius: var(--radius-md);
}
.section-title {
  font-size: var(--fs-md);
  margin: 0 0 var(--sp-3);
  color: var(--text-strong);
}
.disk-list {
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
}
.disk-item {
  display: grid;
  grid-template-columns: 1fr 120px 50px;
  align-items: center;
  gap: var(--sp-3);
}
.disk-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.disk-path {
  font-size: var(--fs-sm);
  font-weight: 500;
  color: var(--text-strong);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.disk-usage {
  font-size: var(--fs-xs);
  color: var(--text-muted);
}
.disk-bar {
  height: 8px;
  background: var(--surface-muted);
  border-radius: var(--radius-pill);
  overflow: hidden;
}
.disk-bar-fill {
  height: 100%;
  background: var(--grad-success);
  border-radius: var(--radius-pill);
  transition: width 0.5s ease;
}
.disk-bar-fill.warn { background: var(--pt-warn); }
.disk-bar-fill.danger { background: var(--pt-danger); }
.disk-pct {
  font-size: var(--fs-sm);
  font-weight: 600;
  text-align: right;
  color: var(--text-strong);
}
.disk-pct.danger { color: var(--pt-danger); }

.temp-list {
  display: flex;
  gap: var(--sp-2);
  flex-wrap: wrap;
}
.temp-chip {
  padding: 4px 12px;
  border-radius: var(--radius-pill);
  font-size: var(--fs-sm);
  background: rgba(46, 204, 113, 0.15);
  color: var(--pt-success);
}
.temp-chip.warm { background: rgba(243, 156, 18, 0.15); color: var(--pt-warn); }
.temp-chip.hot { background: rgba(231, 76, 60, 0.15); color: var(--pt-danger); }

.monitor-error {
  color: var(--pt-danger);
  font-size: var(--fs-sm);
  padding: var(--sp-2);
}

@media (max-width: 760px) {
  .disk-item {
    grid-template-columns: 1fr 40px;
  }
  .disk-bar { display: none; }
}
.monitor-scope { display: flex; align-items: center; gap: 8px; padding: 8px 14px; margin-bottom: 12px; border-radius: 8px; font-size: 13px; }
.scope-dot { width: 8px; height: 8px; border-radius: 50%; background: #27ae60; box-shadow: 0 0 6px #27ae60; }
.scope-label { font-weight: 700; }
</style>
