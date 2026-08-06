<script setup lang="ts">
import type { MonitorTarget, Metrics } from '@/api/monitor'
import { extraMs } from './useDeviceDetail'

defineProps<{
  target: MonitorTarget
  latest: Metrics | null
  deviceID: string
  statusText: string
  collectTime: string
  serviceRows: Array<{ label: string; value: string }>
  serviceOutput: { stdout: string; stderr: string; exitError: string }
}>()

function targetTypeName(type: string) {
  const map: Record<string, string> = {
    host: '服务器', nas: 'NAS', camera: '摄像头',
    http: '网站监控', tcp: '端口监控', process: '进程监控', script: '脚本监控',
  }
  return map[type] ?? type ?? '设备'
}
</script>

<template>
  <div class="overview-grid">
    <section class="hero-card hero-primary">
      <span class="hero-label">监控项</span>
      <strong>{{ target.name }}</strong>
      <div class="hero-line">设备ID {{ deviceID }}</div>
      <div class="hero-tags">
        <span>{{ targetTypeName(target.type) }}</span>
      </div>
    </section>

    <section class="hero-card">
      <span class="hero-label">采集状态</span>
      <strong>{{ statusText }}</strong>
      <div class="system-mini">
        <div>
          <span>采集时间</span>
          <b>{{ collectTime }}</b>
        </div>
        <div>
          <span>耗时</span>
          <b>{{ extraMs(latest?.extra || {}, 'response_ms') !== '-' ? extraMs(latest?.extra || {}, 'response_ms') : extraMs(latest?.extra || {}, 'duration_ms') }}</b>
        </div>
      </div>
    </section>
  </div>

  <section class="panel span-2 service-panel">
    <div class="panel-title">
      <span>监控详情</span>
      <b>{{ statusText }}</b>
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
