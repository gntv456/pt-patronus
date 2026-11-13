<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { systemApi, type SystemMetrics } from '@/api/system'
import { fmtBytes } from '@/utils/format'
import { registerPolling } from '@/composables/usePolling'
import MetricBar from '@/components/monitor/MetricBar.vue'

// DashboardResourceCard — 区②系统资源：即时采集本机 CPU/内存/磁盘/负载，30s 轮询。
// /system/metrics 需管理员；非 admin 得 403（http.ts 仅对 vip_* 跳登录，普通 403 不跳），
// 组件 catch 后优雅降级提示，不轮询。
const { t } = useI18n()
const metrics = ref<SystemMetrics | null>(null)
const loading = ref(true)
const forbidden = ref(false)
const errorMsg = ref('')
let stopPolling: (() => void) | null = null

async function load() {
  loading.value = true
  try {
    metrics.value = await systemApi.metrics()
    forbidden.value = false
    errorMsg.value = ''
  } catch (e: unknown) {
    const err = e as { response?: { status?: number } }
    if (err.response?.status === 403) {
      forbidden.value = true
      stopPolling?.() // 非管理员不再轮询
    } else {
      errorMsg.value = (e as Error).message || t('drsc.collectFailed')
    }
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void load()
  // 统一轮询调度器：后台标签页暂停、切回补拉（原裸 setInterval 后台照跑）。
  stopPolling = registerPolling(load, 30_000, { runOnShow: true })
})
onBeforeUnmount(() => {
  stopPolling?.()
})

const disks = computed(() => metrics.value?.disks ?? [])
const diskShown = computed(() => disks.value.slice(0, 3))
const diskMore = computed(() => Math.max(0, disks.value.length - diskShown.value.length))
</script>

<template>
  <div class="dash-card">
    <div class="dc-head">
      <div>
        <h3>{{ t('drsc.title') }}</h3>
        <p v-if="metrics?.hostname">{{ metrics.hostname }} · {{ metrics.os }}</p>
        <p v-else>{{ t('drsc.sub') }}</p>
      </div>
    </div>

    <div v-if="loading" class="dc-empty" aria-busy="true"><el-skeleton :rows="4" animated style="width: 100%" /></div>
    <div v-else-if="forbidden" class="dc-empty">{{ t('drsc.forbidden') }}</div>
    <div v-else-if="errorMsg" class="dc-empty">{{ errorMsg }}</div>
    <div v-else-if="!metrics" class="dc-empty">{{ t('drsc.collecting') }}</div>
    <div v-else class="dc-body">
      <MetricBar label="CPU" :value="metrics.cpu_usage" :detail="t('drsc.cores', { n: metrics.cpu_cores ?? '-' })" />
      <MetricBar
        :label="t('dc.memory')"
        :value="metrics.mem_usage"
        :detail="`${fmtBytes(metrics.mem_used ?? 0)} / ${fmtBytes(metrics.mem_total ?? 0)}`"
      />
      <MetricBar
        v-for="d in diskShown"
        :key="d.device"
        :label="d.device"
        :value="d.usage"
        :detail="`${fmtBytes(d.used)} / ${fmtBytes(d.total)}`"
      />
      <div v-if="diskMore" class="disk-more">{{ t('drsc.moreDisks', { n: diskMore }) }}</div>
      <div v-if="metrics.load1" class="load-row">
        {{ t('drsc.load') }} {{ metrics.load1.toFixed(2) }} / {{ (metrics.load5 ?? 0).toFixed(2) }} /
        {{ (metrics.load15 ?? 0).toFixed(2) }}
      </div>
    </div>
  </div>
</template>

<style scoped>
.dash-card {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.dc-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--sp-2);
  margin-bottom: 12px;
}
.dc-head h3 {
  margin: 0;
  font-size: var(--fs-lg);
  line-height: 1.35;
}
.dc-head p {
  margin: 3px 0 0;
  color: var(--text-muted);
  font-size: var(--fs-2xs);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dc-body {
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
  flex: 1;
}
.disk-more {
  font-size: var(--fs-2xs);
  color: var(--text-muted);
}
.load-row {
  font-variant-numeric: tabular-nums;
  margin-top: auto;
  padding-top: 8px;
  border-top: 1px solid var(--divider);
  font-size: var(--fs-2xs);
  color: var(--text-muted);
}
.dc-empty {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 90px;
  color: var(--text-muted);
  font-size: var(--fs-xs);
}
</style>
