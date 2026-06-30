<script setup lang="ts">
import { computed } from 'vue'

// MetricChart — 零依赖 SVG 折线趋势图（与 Dashboard CSS 柱状图同思路，不引图表库）。
const props = defineProps<{
  data: number[] // 0-100 序列
  height?: number
  max?: number // Y 轴上限，默认 100
}>()

const W = 240
const H = computed(() => props.height ?? 60)
const path = computed(() => {
  const d = props.data
  if (!d || d.length < 2) return ''
  const max = props.max ?? 100
  const stepX = W / (d.length - 1)
  return d
    .map((v, i) => {
      const x = i * stepX
      const y = H.value - (Math.min(max, Math.max(0, v)) / max) * H.value
      return `${i === 0 ? 'M' : 'L'}${x.toFixed(1)},${y.toFixed(1)}`
    })
    .join(' ')
})
const area = computed(() => (path.value ? `${path.value} L${W},${H.value} L0,${H.value} Z` : ''))
</script>

<template>
  <svg :viewBox="`0 0 ${W} ${H}`" class="metric-chart" preserveAspectRatio="none">
    <defs>
      <linearGradient id="mc-area" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0%" stop-color="rgba(108, 92, 231, 0.35)" />
        <stop offset="100%" stop-color="rgba(108, 92, 231, 0)" />
      </linearGradient>
      <linearGradient id="mc-line" x1="0" y1="0" x2="1" y2="0">
        <stop offset="0%" stop-color="#4285f4" />
        <stop offset="100%" stop-color="#6c5ce7" />
      </linearGradient>
    </defs>
    <path v-if="area" :d="area" fill="url(#mc-area)" />
    <path v-if="path" :d="path" fill="none" stroke="url(#mc-line)" stroke-width="2" stroke-linejoin="round" />
    <text v-if="!path" x="50%" y="50%" text-anchor="middle" dy=".3em" class="mc-empty">暂无数据</text>
  </svg>
</template>

<style scoped>
.metric-chart {
  width: 100%;
  display: block;
}
.mc-empty {
  fill: var(--text-muted, #9ca3af);
  font-size: 12px;
}
</style>
