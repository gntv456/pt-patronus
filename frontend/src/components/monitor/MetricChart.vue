<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

// MetricChart — 零依赖 SVG 折线趋势图（与 Dashboard CSS 柱状图同思路，不引图表库）。
// null/undefined 表示该时刻无读数（离线/采集失败/手动探活快照），画成断口而不是 0 值折线——
// 否则设备掉线时段会被画成「CPU 跌到 0%」的假断崖。
const props = defineProps<{
  data: (number | null | undefined)[] // 0-100 序列；空洞为 null
  height?: number
  max?: number // Y 轴上限，默认 100
}>()

const W = 240
const H = computed(() => props.height ?? 60)
const points = computed(() => {
  const d = props.data ?? []
  const max = props.max ?? 100
  const stepX = d.length > 1 ? W / (d.length - 1) : 0
  return d.map((v, i) => {
    if (v === null || v === undefined || Number.isNaN(v)) return null
    const y = H.value - (Math.min(max, Math.max(0, v)) / max) * H.value
    return { x: i * stepX, y }
  })
})
const path = computed(() => {
  let out = ''
  let run = 0 // 连续有效点数：0 表示上一段已断，需重新 M 起点
  for (const p of points.value) {
    if (!p) {
      run = 0
      continue
    }
    out += `${run === 0 ? 'M' : 'L'}${p.x.toFixed(1)},${p.y.toFixed(1)} `
    run++
  }
  return out.trim()
})
// 孤立单点（两侧都断）画小圆点；否则 [50,null,60] 这类全孤立点的图会整片空白。
const dots = computed(() => {
  const pts = points.value
  const out: Array<{ x: number; y: number }> = []
  for (let i = 0; i < pts.length; i++) {
    const p = pts[i]
    if (!p) continue
    const prev = i > 0 ? pts[i - 1] : null
    const next = i < pts.length - 1 ? pts[i + 1] : null
    if (!prev && !next) out.push(p)
  }
  return out
})
const area = computed(() => (path.value ? `${path.value} L${W},${H.value} L0,${H.value} Z` : ''))
</script>

<template>
  <svg :viewBox="`0 0 ${W} ${H}`" class="metric-chart" preserveAspectRatio="none">
    <defs>
      <linearGradient id="mc-area" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0%" stop-color="var(--pt-primary-bg)" />
        <stop offset="100%" stop-color="var(--pt-primary-bg)" />
      </linearGradient>
      <linearGradient id="mc-line" x1="0" y1="0" x2="1" y2="0">
        <stop offset="0%" style="stop-color: var(--pt-primary-2)" />
        <stop offset="100%" style="stop-color: var(--pt-primary)" />
      </linearGradient>
    </defs>
    <path v-if="area" :d="area" fill="url(#mc-area)" />
    <path v-if="path" :d="path" fill="none" stroke="url(#mc-line)" stroke-width="2" stroke-linejoin="round" />
    <circle v-for="(p, i) in dots" :key="i" :cx="p.x.toFixed(1)" :cy="p.y.toFixed(1)" r="1.6" class="mc-dot" />
    <text v-if="!path && !dots.length" x="50%" y="50%" text-anchor="middle" dy=".3em" class="mc-empty">
      {{ t('common.noData') }}
    </text>
  </svg>
</template>

<style scoped>
.metric-chart {
  width: 100%;
  display: block;
}
.mc-dot {
  fill: var(--pt-primary);
}
.mc-empty {
  fill: var(--text-muted, #9ca3af);
  font-size: var(--fs-xs);
}
</style>
