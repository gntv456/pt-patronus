<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { computed } from 'vue'
import GlassIcon from '@/components/GlassIcon.vue'
import MetricBar from './MetricBar.vue'
import { fmtBytes, fmtSpeed } from '@/utils/format'
import { isSSHAuthError, showSSHAuthFix } from '@/utils/ssh-fix'
import type { MonitorTarget, Metrics } from '@/api/monitor'

// DeviceCard — 单设备玻璃卡片：状态灯 + 关键指标 + 操作。
const { t } = useI18n()
const props = defineProps<{
  target: MonitorTarget
  metrics?: Metrics | null
}>()
const emit = defineEmits<{
  edit: [MonitorTarget]
  detail: [MonitorTarget]
  probe: [MonitorTarget]
  remove: [MonitorTarget]
  preview: [MonitorTarget]
  terminal: [MonitorTarget]
}>()

const serviceTypes = ['http', 'tcp', 'process', 'script']
const isServiceMonitor = computed(() => serviceTypes.includes(props.target.type))
const icon = computed(() => {
  if (props.target.type === 'camera') return 'camera'
  if (props.target.type === 'nas') return 'nas'
  if (props.target.type === 'http') return 'sites'
  if (props.target.type === 'tcp') return 'settings'
  if (props.target.type === 'process') return 'workflow'
  if (props.target.type === 'script') return 'history'
  return 'server'
})
const color = computed<[string, string]>(() =>
  props.target.type === 'camera' ? ['#ff5f6d', '#ffc371']
    : props.target.type === 'nas' ? ['#f7971e', '#ffd200']
      : props.target.type === 'http' ? ['#00b894', '#0984e3']
        : props.target.type === 'tcp' ? ['#34495e', '#16a085']
          : props.target.type === 'process' ? ['#8e44ad', '#3498db']
            : props.target.type === 'script' ? ['#d35400', '#f1c40f']
              : ['#0f2027', '#2c5364'],
)
const status = computed(() => {
  if (props.target.last_online === null) return { cls: 'unknown', text: t('dc.notChecked') }
  if (props.target.last_online) return { cls: 'online', text: t('dc.online') }
  return { cls: 'offline', text: t('dc.offline') }
})
// 磁盘总量须去重后再累加（btrfs子卷/ZFS dataset 报相同整盘容量会乘N倍）。
const diskStats = computed(() => {
  const seen = new Set<number>(); const devs = new Set<string>(); let t = 0, u = 0
  for (const d of props.metrics?.disks ?? []) {
    if (d.total <= 0 || devs.has(d.device || "") || seen.has(d.total)) continue
    devs.add(d.device || ""); seen.add(d.total); t += d.total; u += d.used || 0
  }
  return { total: t, used: u, usage: t ? (u / t) * 100 : undefined, empty: !t }
})
const diskUsage = computed(() => diskStats.value.usage)
const diskDetail = computed(() => { const s = diskStats.value; return s.empty ? "" : t('dc.diskDetail', { used: fmtBytes(s.used), total: fmtBytes(s.total) }) })
const lastCheck = computed(() =>
  props.target.last_checked_at ? new Date(props.target.last_checked_at).toLocaleTimeString() : '-',
)
const vendorLabel = computed(() => {
  switch (props.target.vendor) {
    case 'synology':
      return t('dc.synology')
    case 'truenas':
    case 'freenas':
      return 'TrueNAS'
    case 'snmp':
      return 'SNMP'
    default:
      return props.target.vendor
  }
})
const canTerminal = computed(() => props.target.type === 'host' && (props.target.protocol === 'ssh' || (!props.target.protocol && !!props.target.host)))
const extra = computed<Record<string, unknown>>(() => props.metrics?.extra || {})

function extraText(key: string, fallback = '-'): string {
  const value = extra.value[key]
  if (value === undefined || value === null || value === '') return fallback
  return String(value)
}

function extraMs(key = 'response_ms'): string {
  const raw = Number(extra.value[key] || 0)
  return raw > 0 ? `${raw}ms` : '-'
}

const serviceRows = computed(() => {
  if (props.target.type === 'http') {
    return [
      { label: t('dc.statusCode'), value: extraText('status_code') },
      { label: t('dc.response'), value: extraMs() },
      { label: t('dc.method'), value: extraText('method', 'GET') },
    ]
  }
  if (props.target.type === 'tcp') {
    return [
      { label: t('dc.address'), value: `${extraText('host', props.target.host)}:${extraText('port', String(props.target.port || '-'))}` },
      { label: t('dc.response'), value: extraMs() },
    ]
  }
  if (props.target.type === 'process') {
    return [
      { label: t('dc.process'), value: extraText('process_name') },
      { label: t('dc.count'), value: extraText('count', '0') },
    ]
  }
  if (props.target.type === 'script') {
    return [
      { label: t('dc.elapsed'), value: extraMs('duration_ms') },
      { label: t('dc.output'), value: extraText('stdout', t('dc.noOutput')).slice(0, 36) },
    ]
  }
  return []
})
</script>

<template>
  <div class="device-card glass glass-hover">
    <div class="dc-head">
      <GlassIcon :name="icon" :from="color[0]" :to="color[1]" :size="40" />
      <div class="dc-title">
        <div class="dc-name">{{ target.name }}</div>
        <div class="dc-sub">
          {{ target.host || t('dc.localhost') }}<span v-if="vendorLabel"> · {{ vendorLabel }}</span>
        </div>
      </div>
      <span class="dc-status" :class="status.cls"><i class="dot"></i>{{ status.text }}</span>
    </div>

    <div class="dc-body">
      <template v-if="isServiceMonitor && metrics">
        <div v-for="row in serviceRows" :key="row.label" class="dc-info"><span>{{ row.label }}</span>{{ row.value }}</div>
      </template>
      <template v-else-if="target.type !== 'camera' && metrics">
        <MetricBar label="CPU" :value="metrics.cpu_usage" />
        <MetricBar
          :label="t('dc.memory')"
          :value="metrics.mem_usage"
          :detail="metrics.mem_total ? `${fmtBytes(metrics.mem_used ?? 0)} / ${fmtBytes(metrics.mem_total)}` : ''"
        />
        <MetricBar v-if="diskUsage !== undefined" :label="t('dc.disk')" :value="diskUsage" :detail="diskDetail" />
      </template>
      <template v-else-if="target.type === 'camera' && metrics?.device_info">
        <div class="dc-info"><span>{{ t('dc.vendor') }}</span>{{ metrics.device_info.manufacturer || '-' }}</div>
        <div class="dc-info"><span>{{ t('dc.model') }}</span>{{ metrics.device_info.model || '-' }}</div>
        <div class="dc-info"><span>{{ t('dc.firmware') }}</span>{{ metrics.device_info.firmware || '-' }}</div>
      </template>
      <div v-else class="dc-nodata">
        <span>{{ target.last_error || t('dc.noDataHint') }}</span>
        <el-button v-if="isSSHAuthError(target.last_error)" size="small" text type="primary" @click="showSSHAuthFix">{{ t('dd.howToFix') }}</el-button>
      </div>

      <div v-if="metrics?.net_rx_rate !== undefined && target.type !== 'camera'" class="dc-net">
        ↓ {{ fmtSpeed(metrics.net_rx_rate ?? 0) }} · ↑ {{ fmtSpeed(metrics.net_tx_rate ?? 0) }}
      </div>
    </div>

    <div class="dc-foot">
      <span class="dc-time">{{ lastCheck }}</span>
      <div class="dc-actions">
        <el-button size="small" text @click="emit('probe', target)">{{ t('dc.probe') }}</el-button>
        <el-button v-if="canTerminal" size="small" text @click="emit('terminal', target)">{{ t('dc.terminal') }}</el-button>
        <el-button v-if="target.type === 'camera'" size="small" text @click="emit('preview', target)">{{ t('dc.preview') }}</el-button>
        <el-button size="small" text @click="emit('detail', target)">{{ t('dc.detail') }}</el-button>
        <el-button size="small" text @click="emit('edit', target)">{{ t('common.edit') }}</el-button>
        <el-button size="small" text type="danger" @click="emit('remove', target)">{{ t('common.delete') }}</el-button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.device-card {
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
}
.dc-head {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
}
.dc-title {
  flex: 1;
  min-width: 0;
}
.dc-name {
  font-weight: 700;
  font-size: var(--fs-lg);
  color: var(--text-strong);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dc-sub {
  font-size: var(--fs-xs);
  color: var(--text-muted);
}
.dc-status {
  display: inline-flex;
  align-items: center;
  gap: var(--sp-1);
  font-size: var(--fs-xs);
  font-weight: 600;
  padding: 3px 8px;
  border-radius: var(--radius-pill);
}
.dc-status .dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
}
.dc-status.online {
  color: #11998e;
  background: rgba(17, 153, 142, 0.12);
}
.dc-status.online .dot {
  background: #11998e;
  box-shadow: 0 0 6px #11998e;
}
.dc-status.offline {
  color: var(--pt-danger);
  background: var(--pt-danger-bg);
}
.dc-status.offline .dot {
  background: #e74c3c;
}
.dc-status.unknown {
  color: var(--text-muted);
  background: rgba(156, 163, 175, 0.15);
}
.dc-status.unknown .dot {
  background: #9ca3af;
}
.dc-body {
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
}
.dc-info {
  display: flex;
  justify-content: space-between;
  font-size: var(--fs-body);
}
.dc-info span {
  color: var(--text-muted);
}
.dc-nodata {
  font-size: var(--fs-body);
  color: var(--text-muted);
  padding: 8px 0;
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  flex-wrap: wrap;
}
/* last_error 含多行排查指引（enrichSSHDialError 生成）：卡片上折叠为单行摘要（首行），
   完整内容在详情抽屉（.d-error 已按换行展示） */
.dc-nodata > span:first-child {
  flex: 1 1 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dc-net {
  font-size: var(--fs-xs);
  color: var(--text-muted);
}
.dc-foot {
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-top: 1px solid var(--glass-border);
  padding-top: 8px;
}
.dc-time {
  font-size: var(--fs-2xs);
  color: var(--text-muted);
}
.dc-actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--sp-1);
}
</style>
