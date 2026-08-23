<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { systemApi, type SystemMetrics } from '@/api/system'
import { fmtBytes } from '@/utils/format'
import MetricBar from '@/components/monitor/MetricBar.vue'

// DashboardResourceCard — 区②系统资源：即时采集本机 CPU/内存/磁盘/负载，30s 轮询。
// /system/metrics 需管理员；非 admin 得 403（http.ts 仅对 vip_* 跳登录，普通 403 不跳），
// 组件 catch 后优雅降级提示，不轮询。
const metrics = ref<SystemMetrics | null>(null)
const forbidden = ref(false)
const errorMsg = ref('')
let timer: ReturnType<typeof setInterval> | undefined

async function load() {
  try {
    metrics.value = await systemApi.metrics()
    forbidden.value = false
    errorMsg.value = ''
  } catch (e: unknown) {
    const err = e as { response?: { status?: number } }
    if (err.response?.status === 403) {
      forbidden.value = true
      if (timer) clearInterval(timer) // 非管理员不再轮询
    } else {
      errorMsg.value = (e as Error).message || '采集失败'
    }
  }
}

onMounted(() => {
  void load()
  timer = setInterval(load, 30000)
})
onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})

const disks = computed(() => metrics.value?.disks ?? [])
const diskShown = computed(() => disks.value.slice(0, 3))
const diskMore = computed(() => Math.max(0, disks.value.length - diskShown.value.length))
</script>

<template>
  <div class="dash-card">
    <div class="dc-head">
      <div>
        <h3>系统资源</h3>
        <p v-if="metrics?.hostname">{{ metrics.hostname }} · {{ metrics.os }}</p>
        <p v-else>本机即时采集 · 30s 刷新</p>
      </div>
    </div>

    <div v-if="forbidden" class="dc-empty">需要管理员权限查看系统资源</div>
    <div v-else-if="errorMsg" class="dc-empty">{{ errorMsg }}</div>
    <div v-else-if="!metrics" class="dc-empty">采集中…</div>
    <div v-else class="dc-body">
      <MetricBar label="CPU" :value="metrics.cpu_usage" :detail="`${metrics.cpu_cores ?? '-'} 核`" />
      <MetricBar
        label="内存"
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
      <div v-if="diskMore" class="disk-more">还有 {{ diskMore }} 个分区未显示</div>
      <div v-if="metrics.load1" class="load-row">
        负载 {{ metrics.load1.toFixed(2) }} / {{ (metrics.load5 ?? 0).toFixed(2) }} /
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
