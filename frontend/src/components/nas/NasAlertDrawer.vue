<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// 告警历史抽屉：标题栏 + 清空按钮 + AlertList。
// 从 NasServer.vue 迁出；动作经 emits 回传宿主。
import { useI18n } from 'vue-i18n'
import AlertList from '@/components/monitor/AlertList.vue'
import type { MonitorAlert } from '@/api/monitor'

const { t } = useI18n()

defineProps<{
  visible: boolean
  alerts: MonitorAlert[]
}>()

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'read', alert: MonitorAlert): void
  (e: 'ack', alert: MonitorAlert): void
  (e: 'silence', alert: MonitorAlert): void
  (e: 'del', alert: MonitorAlert): void
  (e: 'clear'): void
}>()
</script>

<template>
  <el-drawer
    :model-value="visible"
    size="420px"
    :with-header="false"
    :body-style="{ padding: '0', display: 'flex', flexDirection: 'column' }"
    @update:model-value="emit('update:visible', $event)"
  >
    <div class="alert-drawer">
      <div class="alert-drawer-head">
        <span class="alert-drawer-title">{{ t('nas.alertHistory') }}</span>
        <el-button size="small" type="danger" plain :disabled="!alerts.length" @click="emit('clear')">{{
          t('nas.clearHistory')
        }}</el-button>
      </div>
      <div class="alert-drawer-body">
        <AlertList :alerts="alerts" @read="emit('read', $event)" @ack="emit('ack', $event)" @silence="emit('silence', $event)" @del="emit('del', $event)" />
      </div>
    </div>
  </el-drawer>
</template>
