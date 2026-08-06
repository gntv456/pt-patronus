<script setup lang="ts">
import type { MonitorTarget, Metrics } from '@/api/monitor'

defineProps<{
  target: MonitorTarget
  latest: Metrics | null
}>()
</script>

<template>
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
    <div class="info-row"><span>RTSP 地址</span><b class="mono">{{ target.rtsp_url || '未获取' }}</b></div>
  </section>
</template>
