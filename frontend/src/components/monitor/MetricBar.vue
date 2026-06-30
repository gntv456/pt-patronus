<script setup lang="ts">
import { computed } from 'vue'

// MetricBar — CPU/内存/磁盘等百分比占用条（绿/黄/红三档，超阈值变色）。
const props = defineProps<{
  label: string
  value?: number // 0-100
  detail?: string // 附加文本（如 "8.0 GB / 16.0 GB"）
  warn?: number // 警戒阈值，默认 80
}>()

const pct = computed(() => Math.min(100, Math.max(0, props.value ?? 0)))
const level = computed(() => {
  const v = pct.value
  const w = props.warn ?? 80
  if (v >= w + 10) return 'danger'
  if (v >= w) return 'warn'
  return 'ok'
})
</script>

<template>
  <div class="metric-bar">
    <div class="mb-head">
      <span class="mb-label">{{ label }}</span>
      <span class="mb-val" :class="level">{{ pct.toFixed(1) }}%</span>
    </div>
    <div class="mb-track">
      <div class="mb-fill" :class="level" :style="{ width: pct + '%' }"></div>
    </div>
    <div v-if="detail" class="mb-detail">{{ detail }}</div>
  </div>
</template>

<style scoped>
.metric-bar {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.mb-head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  font-size: 13px;
}
.mb-label {
  color: var(--text-muted, #9ca3af);
}
.mb-val {
  font-weight: 600;
}
.mb-val.ok {
  color: #11998e;
}
.mb-val.warn {
  color: #f7971e;
}
.mb-val.danger {
  color: #e74c3c;
}
.mb-track {
  height: 8px;
  border-radius: 999px;
  background: rgba(0, 0, 0, 0.14);
  overflow: hidden;
}
.mb-fill {
  height: 100%;
  border-radius: 999px;
  transition: width 0.4s ease;
}
.mb-fill.ok {
  background: linear-gradient(90deg, #11998e, #38ef7d);
}
.mb-fill.warn {
  background: linear-gradient(90deg, #f7971e, #ffd200);
}
.mb-fill.danger {
  background: linear-gradient(90deg, #ff5f6d, #ffc371);
}
.mb-detail {
  font-size: 11px;
  color: var(--text-muted, #9ca3af);
}
</style>
