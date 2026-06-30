<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import MetricBar from './MetricBar.vue'
import MetricChart from './MetricChart.vue'
import { monitorApi, parseMetrics, type MonitorTarget, type Metrics, type MetricSnapshot } from '@/api/monitor'
import { fmtBytes, fmtSpeed } from '@/utils/format'
import { isSSHAuthError, showSSHAuthFix } from '@/utils/ssh-fix'

// DeviceDetail — 设备详情抽屉：历史趋势 + 实时指标 + 磁盘/温度/进程/设备信息。
const visible = defineModel<boolean>('visible', { default: false })
const props = defineProps<{ target?: MonitorTarget | null }>()

const latest = ref<Metrics | null>(null)
const snaps = ref<MetricSnapshot[]>([])
const loading = ref(false)

function seriesOf(pick: (m: Metrics) => number | undefined): number[] {
  return snaps.value.map((s) => pick(parseMetrics(s) ?? ({} as Metrics)) ?? 0)
}
const cpuSeries = computed(() => seriesOf((m) => m.cpu_usage))
const memSeries = computed(() => seriesOf((m) => m.mem_usage))
const diskMax = computed(() => {
  const ds = latest.value?.disks
  return ds && ds.length ? Math.max(...ds.map((d) => d.usage)) : undefined
})

function fmtUptime(sec?: number): string {
  if (!sec) return '-'
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  return d > 0 ? `${d}天${h}时` : `${h}时${m}分`
}

async function load() {
  if (!props.target) return
  loading.value = true
  try {
    const [s, res] = await Promise.all([
      monitorApi.metrics(props.target.id, undefined, 120),
      monitorApi.collect(props.target.id).catch(() => null),
    ])
    snaps.value = s
    latest.value = res?.metrics ?? (s.length ? parseMetrics(s[s.length - 1]) : null)
  } finally {
    loading.value = false
  }
}
watch(visible, (v) => {
  if (v && props.target) load()
})
</script>

<template>
  <el-drawer v-model="visible" :title="target?.name" size="420px" direction="rtl">
    <div v-loading="loading" class="detail">
      <!-- 系统指标（host/nas） -->
      <template v-if="target?.type !== 'camera'">
        <section class="d-sec">
          <div class="d-sec-title">CPU 使用率</div>
          <MetricChart :data="cpuSeries" />
          <MetricBar v-if="latest" label="当前 CPU" :value="latest.cpu_usage" :detail="latest.cpu_cores ? `${latest.cpu_cores} 核` : ''" />
        </section>
        <section class="d-sec">
          <div class="d-sec-title">内存使用率</div>
          <MetricChart :data="memSeries" />
          <MetricBar v-if="latest" label="当前内存" :value="latest.mem_usage" :detail="latest.mem_total ? `${fmtBytes(latest.mem_used ?? 0)} / ${fmtBytes(latest.mem_total)}` : ''" />
        </section>
        <section v-if="latest?.disks?.length" class="d-sec">
          <div class="d-sec-title">磁盘</div>
          <MetricBar v-for="(d, i) in latest.disks" :key="i" :label="d.device" :value="d.usage" :detail="`${fmtBytes(d.used)} / ${fmtBytes(d.total)}`" />
        </section>
        <section v-if="latest && (latest.net_rx_rate !== undefined || latest.load1 !== undefined)" class="d-sec">
          <div class="kv"><span>网络</span><span>↓ {{ fmtSpeed(latest.net_rx_rate ?? 0) }} · ↑ {{ fmtSpeed(latest.net_tx_rate ?? 0) }}</span></div>
          <div v-if="latest.load1 !== undefined" class="kv"><span>负载</span><span>{{ latest.load1.toFixed(2) }} / {{ latest.load5?.toFixed(2) }} / {{ latest.load15?.toFixed(2) }}</span></div>
          <div class="kv"><span>运行时长</span><span>{{ fmtUptime(latest.uptime) }}</span></div>
          <div v-if="latest.process_count" class="kv"><span>进程数</span><span>{{ latest.process_count }}</span></div>
          <div v-if="latest.hostname" class="kv"><span>主机名</span><span>{{ latest.hostname }}</span></div>
          <div v-if="latest.os" class="kv"><span>系统</span><span>{{ latest.os }}</span></div>
        </section>
        <section v-if="latest?.temps?.length" class="d-sec">
          <div class="d-sec-title">温度</div>
          <div v-for="(t, i) in latest.temps" :key="i" class="kv"><span>{{ t.name }}</span><span>{{ t.temp.toFixed(0) }} ℃</span></div>
        </section>
      </template>

      <!-- 摄像头设备信息 -->
      <template v-else>
        <section v-if="latest?.device_info" class="d-sec">
          <div class="d-sec-title">设备信息</div>
          <div class="kv"><span>厂商</span><span>{{ latest.device_info.manufacturer || '-' }}</span></div>
          <div class="kv"><span>型号</span><span>{{ latest.device_info.model || '-' }}</span></div>
          <div class="kv"><span>固件</span><span>{{ latest.device_info.firmware || '-' }}</span></div>
          <div class="kv"><span>序列号</span><span>{{ latest.device_info.serial || '-' }}</span></div>
          <div class="kv"><span>硬件</span><span>{{ latest.device_info.hardware || '-' }}</span></div>
        </section>
        <section class="d-sec">
          <div class="kv"><span>RTSP 地址</span><span class="mono">{{ target?.rtsp_url || '（未获取）' }}</span></div>
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
.detail {
  padding: 0 4px;
}
.d-sec {
  margin-bottom: 20px;
}
.d-sec-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-muted);
  margin-bottom: 8px;
}
.kv {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
  padding: 5px 0;
  border-bottom: 1px solid var(--glass-border);
}
.kv span:first-child {
  color: var(--text-muted);
}
.mono {
  font-family: ui-monospace, monospace;
  font-size: 11px;
  word-break: break-all;
  text-align: right;
  max-width: 60%;
}
.d-error {
  font-size: 12px;
  color: var(--pt-danger);
  background: rgba(231, 76, 60, 0.08);
  padding: 8px 12px;
  border-radius: var(--radius-sm);
  margin-top: 8px;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
</style>
