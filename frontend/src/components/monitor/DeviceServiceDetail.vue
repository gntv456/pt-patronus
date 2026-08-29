<script setup lang="ts">
import type { MonitorTarget, Metrics } from '@/api/monitor'
import { useI18n } from 'vue-i18n'
import { extraMs } from './useDeviceDetail'

defineProps<{
  target: MonitorTarget
  latest: Metrics | null
  deviceId: string
  statusText: string
  collectTime: string
  serviceRows: Array<{ label: string; value: string }>
  serviceOutput: { stdout: string; stderr: string; exitError: string }
}>()

const { t } = useI18n()

function targetTypeName(type: string) {
  const map: Record<string, string> = {
    host: t('dsd.tHost'), nas: 'NAS', camera: t('dsd.tCamera'),
    http: t('dsd.tHttp'), tcp: t('dsd.tTcp'), process: t('dsd.tProcess'), script: t('dsd.tScript'),
  }
  return map[type] ?? type ?? t('dsd.tDevice')
}
</script>

<template>
  <div class="overview-grid">
    <section class="hero-card hero-primary">
      <span class="hero-label">{{ t('dsd.monitoredItem') }}</span>
      <strong>{{ target.name }}</strong>
      <div class="hero-line">{{ t('dsd.deviceId') }} {{ deviceId }}</div>
      <div class="hero-tags">
        <span>{{ targetTypeName(target.type) }}</span>
      </div>
    </section>

    <section class="hero-card">
      <span class="hero-label">{{ t('dsd.collectStatus') }}</span>
      <strong>{{ statusText }}</strong>
      <div class="system-mini">
        <div>
          <span>{{ t('dsd.collectTime') }}</span>
          <b>{{ collectTime }}</b>
        </div>
        <div>
          <span>{{ t('dc.elapsed') }}</span>
          <b>{{ extraMs(latest?.extra || {}, 'response_ms') !== '-' ? extraMs(latest?.extra || {}, 'response_ms') : extraMs(latest?.extra || {}, 'duration_ms') }}</b>
        </div>
      </div>
    </section>
  </div>

  <section class="panel span-2 service-panel">
    <div class="panel-title">
      <span>{{ t('dsd.detail') }}</span>
      <b>{{ statusText }}</b>
    </div>
    <div class="info-list">
      <div v-for="row in serviceRows" :key="row.label" class="info-row">
        <span>{{ row.label }}</span><b>{{ row.value }}</b>
      </div>
      <div class="info-row"><span>{{ t('dsd.lastError') }}</span><b>{{ target.last_error || '-' }}</b></div>
    </div>
  </section>

  <section v-if="serviceOutput.stdout || serviceOutput.stderr || serviceOutput.exitError" class="panel span-2">
    <div class="panel-title"><span>{{ t('dsd.cmdOutput') }}</span></div>
    <pre v-if="serviceOutput.stdout" class="service-output">{{ serviceOutput.stdout }}</pre>
    <pre v-if="serviceOutput.stderr" class="service-output err">{{ serviceOutput.stderr }}</pre>
    <p v-if="serviceOutput.exitError" class="cmd-error">{{ serviceOutput.exitError }}</p>
  </section>
</template>
