<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, onMounted, onBeforeUnmount, computed } from 'vue'
import { systemApi, type SystemMetrics } from '@/api/system'
import { humanTempName } from '@/components/monitor/useDeviceDetail'
import { fmtNumOr } from '@/utils/format'
import GlassIcon from '@/components/GlassIcon.vue'
import { registerPolling } from '@/composables/usePolling'

const { t } = useI18n()
const metrics = ref<SystemMetrics>({})
const loading = ref(false)
const error = ref('')
const history = ref<SystemMetrics[]>([])
const maxHistory = 60
let stopPolling: (() => void) | undefined

const cpuHistory = computed(() => history.value.map((m) => m.cpu_usage ?? 0))
const memHistory = computed(() => history.value.map((m) => m.mem_usage ?? 0))
const netRxHistory = computed(() => history.value.map((m) => m.net_rx_rate ?? 0))
const netTxHistory = computed(() => history.value.map((m) => m.net_tx_rate ?? 0))

// 未采到读数时必须显示「没有读数」，不许画成 0.0% / 0 B / 0 B/s：那等于对着一台
// 什么都没查到的机器宣布「服务器很闲」。负载与进程数一直是 —，这里与它们对齐。
function pctOr(v?: number): string {
  return v === undefined || v === null ? '—' : `${v.toFixed(1)}%`
}

function bytesOr(v?: number): string {
  return v === undefined || v === null ? '—' : fmtBytes(v)
}

function uptimeOr(v?: number): string {
  return v === undefined || v === null ? '—' : fmtUptime(v)
}

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
  if (d > 0) return t('opsmonitor.uptimeDH', { d, h })
  if (h > 0) return t('opsmonitor.uptimeHM', { h, m })
  return t('opsmonitor.uptimeM', { m })
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
    error.value = e.message || t('opsmonitor.collectFailed')
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
  // 统一调度器（阶段 1）：3s 轮询合并进单一 tick，页面隐藏自动暂停、切回补一次
  stopPolling = registerPolling(refresh, 3000, { immediate: true, runOnShow: true })
})
onBeforeUnmount(() => stopPolling?.())
</script>

<template>
  <div class="ops-monitor">
    <div class="monitor-scope glass">
      <span class="scope-dot"></span>
      <span class="scope-label">{{ t('opsmonitor.localScope') }}</span>
      <span class="text-muted small">{{ t('opsmonitor.localScopeDesc') }}</span>
    </div>
    <!-- 顶部指标卡片 -->
    <div class="metric-grid">
      <!-- CPU -->
      <div class="metric-card glass">
        <div class="metric-header">
          <GlassIcon name="dashboard" :size="28" />
          <span class="metric-label">CPU</span>
          <span class="metric-value">{{ pctOr(metrics.cpu_usage) }}</span>
        </div>
        <div class="metric-bar">
          <div class="metric-bar-fill" :style="{ width: (metrics.cpu_usage ?? 0) + '%' }" />
        </div>
        <svg class="sparkline" viewBox="0 0 100 30" preserveAspectRatio="none">
          <path :d="sparkline(cpuHistory)" fill="none" stroke="var(--pt-primary)" stroke-width="1.5" />
        </svg>
        <div class="metric-sub">{{ metrics.cpu_cores ?? '-' }} {{ t('opsmonitor.cores') }}</div>
      </div>

      <!-- 内存 -->
      <div class="metric-card glass">
        <div class="metric-header">
          <GlassIcon name="dashboard" from="#11998e" to="#38ef7d" :size="28" />
          <span class="metric-label">{{ t('opsmonitor.mem') }}</span>
          <span class="metric-value">{{ pctOr(metrics.mem_usage) }}</span>
        </div>
        <div class="metric-bar">
          <div class="metric-bar-fill green" :style="{ width: (metrics.mem_usage ?? 0) + '%' }" />
        </div>
        <svg class="sparkline" viewBox="0 0 100 30" preserveAspectRatio="none">
          <path :d="sparkline(memHistory)" fill="none" stroke="#38ef7d" stroke-width="1.5" />
        </svg>
        <div class="metric-sub">{{ bytesOr(metrics.mem_used) }} / {{ bytesOr(metrics.mem_total) }}</div>
      </div>

      <!-- 负载 -->
      <div class="metric-card glass">
        <div class="metric-header">
          <GlassIcon name="dashboard" from="#f7971e" to="#ff6a00" :size="28" />
          <span class="metric-label">{{ t('opsmonitor.load') }}</span>
          <span class="metric-value">{{ fmtNumOr(metrics.load1) }}</span>
        </div>
        <div class="load-row">
          <span class="pt-chip">1m: {{ fmtNumOr(metrics.load1) }}</span>
          <span class="pt-chip">5m: {{ fmtNumOr(metrics.load5) }}</span>
          <span class="pt-chip">15m: {{ fmtNumOr(metrics.load15) }}</span>
        </div>
        <div class="metric-sub">{{ t('opsmonitor.procs') }}: {{ metrics.process_count ?? '-' }}</div>
      </div>

      <!-- 网络 -->
      <div class="metric-card glass">
        <div class="metric-header">
          <GlassIcon name="dashboard" from="#4cc9f0" to="#4361ee" :size="28" />
          <span class="metric-label">{{ t('opsmonitor.net') }}</span>
        </div>
        <div class="net-row">
          <div class="net-item">
            <span class="net-arrow down">↓</span>
            <span>{{ bytesOr(metrics.net_rx_rate) }}/s</span>
          </div>
          <div class="net-item">
            <span class="net-arrow up">↑</span>
            <span>{{ bytesOr(metrics.net_tx_rate) }}/s</span>
          </div>
        </div>
        <svg class="sparkline" viewBox="0 0 100 30" preserveAspectRatio="none">
          <path
            :d="sparkline(netRxHistory, Math.max(...netRxHistory, 1))"
            fill="none"
            stroke="#4cc9f0"
            stroke-width="1.5"
          />
          <path
            :d="sparkline(netTxHistory, Math.max(...netTxHistory, 1))"
            fill="none"
            stroke="#4361ee"
            stroke-width="1.5"
          />
        </svg>
      </div>

      <!-- 运行时间 -->
      <div class="metric-card glass">
        <div class="metric-header">
          <GlassIcon name="dashboard" from="#8e44ad" to="#3498db" :size="28" />
          <span class="metric-label">{{ t('opsmonitor.uptime') }}</span>
          <span class="metric-value small">{{ uptimeOr(metrics.uptime) }}</span>
        </div>
        <div class="metric-sub">{{ metrics.hostname ?? '-' }}</div>
        <div class="metric-sub">{{ metrics.os ?? '-' }}</div>
      </div>
    </div>

    <!-- 磁盘 -->
    <div v-if="metrics.disks?.length" class="disk-section glass">
      <h3 class="section-title">{{ t('opsmonitor.disk') }}</h3>
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
          <span class="disk-pct" :class="{ danger: disk.usage > 90 }"
            >{{ disk.usage.toFixed(1) }}%</span>
        </div>
      </div>
    </div>

    <!-- 温度 -->
    <div v-if="metrics.temps?.length" class="temp-section glass">
      <h3 class="section-title">{{ t('opsmonitor.temp') }}</h3>
      <div class="temp-list">
        <span
          v-for="t in metrics.temps"
          :key="t.name"
          class="temp-chip"
          :class="{ hot: t.temp > 70, warm: t.temp > 50 && t.temp <= 70 }"
        >
          {{ humanTempName(t.name) }}: {{ t.temp.toFixed(0) }}°C
        </span>
      </div>
    </div>

    <div v-if="error" class="monitor-error">{{ error }}</div>
  </div>
</template>

<style scoped src="./OpsMonitor.css"></style>
