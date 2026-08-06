<script setup lang="ts">
import { toRef } from 'vue'
import DeviceServiceDetail from './DeviceServiceDetail.vue'
import DeviceHostDetail from './DeviceHostDetail.vue'
import DeviceCameraDetail from './DeviceCameraDetail.vue'
import { useDeviceDetail, type PowerAction } from './useDeviceDetail'
import { isSSHAuthError, showSSHAuthFix } from '@/utils/ssh-fix'
import type { MonitorTarget } from '@/api/monitor'
import './device-detail.css'

const visible = defineModel<boolean>('visible', { default: false })
const props = defineProps<{ target?: MonitorTarget | null }>()

const targetRef = toRef(props, 'target')

const {
  latest, loading, powerLoading,
  cpuSeries, memSeries,
  diskUsage, diskTotal, diskUsed,
  hottestTemp, tempEmptyHint,
  isServiceMonitor, resolvedProtocol, canPower,
  status, deviceID, collectTime, targetSubtitle,
  serviceRows, serviceOutput,
  runPower,
} = useDeviceDetail(visible, targetRef)

function onPower(action: PowerAction) {
  return runPower(action)
}
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
      <DeviceServiceDetail
        v-if="target && isServiceMonitor"
        :target="target"
        :latest="latest"
        :device-id="deviceID"
        :status-text="status.text"
        :collect-time="collectTime"
        :service-rows="serviceRows"
        :service-output="serviceOutput"
      />

      <DeviceHostDetail
        v-else-if="target && target.type !== 'camera'"
        :target="target"
        :latest="latest"
        :device-id="deviceID"
        :collect-time="collectTime"
        :cpu-series="cpuSeries"
        :mem-series="memSeries"
        :disk-usage="diskUsage"
        :disk-total="diskTotal"
        :disk-used="diskUsed"
        :hottest-temp="hottestTemp"
        :temp-empty-hint="tempEmptyHint"
        :resolved-protocol="resolvedProtocol"
        :can-power="canPower"
        :power-loading="powerLoading"
        @power="onPower"
      />

      <DeviceCameraDetail
        v-else-if="target"
        :target="target"
        :latest="latest"
      />

      <div v-if="target?.last_error" class="d-error">
        <span>最近错误：{{ target.last_error }}</span>
        <el-button v-if="isSSHAuthError(target.last_error)" size="small" text type="primary" @click="showSSHAuthFix">如何修复？</el-button>
      </div>
    </div>
  </el-drawer>
</template>
