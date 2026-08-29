<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { MonitorTarget, Metrics } from '@/api/monitor'

const { t } = useI18n()

defineProps<{
  target: MonitorTarget
  latest: Metrics | null
}>()
</script>

<template>
  <section v-if="latest?.device_info" class="panel camera-panel">
    <div class="panel-title"><span>{{ t('dcd.deviceInfo') }}</span></div>
    <div class="info-list">
      <div class="info-row"><span>{{ t('dc.vendor') }}</span><b>{{ latest.device_info.manufacturer || '-' }}</b></div>
      <div class="info-row"><span>{{ t('dc.model') }}</span><b>{{ latest.device_info.model || '-' }}</b></div>
      <div class="info-row"><span>{{ t('dc.firmware') }}</span><b>{{ latest.device_info.firmware || '-' }}</b></div>
      <div class="info-row"><span>{{ t('dcd.serial') }}</span><b>{{ latest.device_info.serial || '-' }}</b></div>
      <div class="info-row"><span>{{ t('dcd.hardware') }}</span><b>{{ latest.device_info.hardware || '-' }}</b></div>
    </div>
  </section>
  <section class="panel camera-panel">
    <div class="panel-title"><span>{{ t('dcd.connInfo') }}</span></div>
    <div class="info-row"><span>{{ t('dcd.rtspUrl') }}</span><b class="mono">{{ target.rtsp_url || t('dcd.notFetched') }}</b></div>
  </section>
</template>
