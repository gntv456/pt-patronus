<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { computed } from 'vue'

// MetricBar — CPU/内存/磁盘等百分比占用条（绿/黄/红三档，超阈值变色）。
// value 缺失（离线/采集失败快照只带 online）时显示 "-" 而不是 0.0% 绿条——
// 0 值是「空闲」，无读数不是 0，画成 0% 会把故障渲染成健康。
const props = defineProps<{
  label: string
  value?: number // 0-100；undefined = 无读数
  detail?: string // 附加文本（如 "8.0 GB / 16.0 GB"）
  warn?: number // 警戒阈值，默认 80
}>()

const hasValue = computed(() => props.value !== undefined && !Number.isNaN(props.value))
const pct = computed(() => Math.min(100, Math.max(0, props.value ?? 0)))
const level = computed(() => {
  if (!hasValue.value) return 'none'
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
      <span class="mb-val" :class="level">{{ hasValue ? pct.toFixed(1) + '%' : '-' }}</span>
    </div>
    <div class="mb-track">
      <div class="mb-fill" :class="level" :style="{ width: hasValue ? pct + '%' : '0%' }"></div>
    </div>
    <div v-if="detail" class="mb-detail">{{ detail }}</div>
  </div>
</template>

<style scoped>
.metric-bar {
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
}
.mb-head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  font-size: var(--fs-body);
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
  color: var(--pt-danger);
}
.mb-val.none {
  color: var(--text-muted, #9ca3af);
}
.mb-track {
  height: 8px;
  border-radius: var(--radius-pill);
  background: rgba(0, 0, 0, 0.14);
  overflow: hidden;
}
.mb-fill {
  height: 100%;
  border-radius: var(--radius-pill);
  transition: width var(--dur-slow) var(--ease-out-smooth);
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
  font-size: var(--fs-2xs);
  color: var(--text-muted, #9ca3af);
}
</style>
