<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// Canvas 版迷你折线（阶段 1）：替换 SVG path + gradient defs + hover line，
// 单个 <canvas> 完成绘制，减少 DOM 节点与 <defs> id 冲突。API 与原 SVG 版完全一致。
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { fmtBytes } from '@/utils/format'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
type Unit = 'bytes' | 'count' | 'ratio'
const props = withDefaults(
  defineProps<{
    values: number[]; color: string; unit?: Unit; days?: string[]
  }>(),
  { unit: 'count', days: () => [] },
)

const canvasRef = ref<HTMLCanvasElement | null>(null)
const wrapRef = ref<HTMLElement | null>(null)
const hoverIdx = ref<number | null>(null)
let ro: ResizeObserver | null = null

const geom = computed(() => {
  const n = props.values.length
  if (!n) return null
  return { n, xOf: (i: number) => (n === 1 ? 50 : (i / (n - 1)) * 100) }
})

function resolveColor(c: string): string {
  if (c.startsWith('var(')) {
    const name = c.slice(4, -1).trim()
    const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
    return v || '#6c5ce7'
  }
  return c
}

function draw() {
  const cv = canvasRef.value
  if (!cv) return
  const rect = cv.getBoundingClientRect()
  const w = Math.max(1, Math.round(rect.width))
  const h = Math.max(1, Math.round(rect.height))
  const dpr = window.devicePixelRatio || 1
  cv.width = Math.round(w * dpr)
  cv.height = Math.round(h * dpr)
  const ctx = cv.getContext('2d')
  if (!ctx) return
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  ctx.clearRect(0, 0, w, h)
  const v = props.values
  if (!v.length) return
  let lo = Math.min(...v)
  let hi = Math.max(...v)
  if (hi <= lo) hi = lo + 1
  const span = hi - lo
  const pad = 4
  const xOf = (i: number) => (v.length === 1 ? w / 2 : (i / (v.length - 1)) * w)
  const yOf = (val: number) => h - pad - ((val - lo) / span) * (h - 2 * pad)
  const color = resolveColor(props.color)
  // 面积渐变（用 globalAlpha 兼容 hex / rgb / CSS 变量，无需解析颜色格式）
  const grad = ctx.createLinearGradient(0, 0, 0, h)
  grad.addColorStop(0, color)
  grad.addColorStop(1, 'transparent')
  ctx.globalAlpha = 0.3
  ctx.fillStyle = grad
  ctx.beginPath()
  ctx.moveTo(xOf(0), yOf(v[0]))
  v.forEach((val, i) => { if (i) ctx.lineTo(xOf(i), yOf(val)) })
  ctx.lineTo(xOf(v.length - 1), h)
  ctx.lineTo(xOf(0), h)
  ctx.closePath()
  ctx.fill()
  ctx.globalAlpha = 1
  // 折线
  ctx.beginPath()
  ctx.moveTo(xOf(0), yOf(v[0]))
  v.forEach((val, i) => { if (i) ctx.lineTo(xOf(i), yOf(val)) })
  ctx.strokeStyle = color
  ctx.lineWidth = 2
  ctx.lineJoin = 'round'
  ctx.lineCap = 'round'
  ctx.stroke()
  // hover 竖线
  if (hoverIdx.value !== null && v[hoverIdx.value] !== undefined) {
    ctx.beginPath()
    ctx.moveTo(xOf(hoverIdx.value), 0)
    ctx.lineTo(xOf(hoverIdx.value), h)
    ctx.strokeStyle = 'rgba(255,255,255,0.28)'
    ctx.lineWidth = 1
    ctx.setLineDash([3, 3])
    ctx.stroke()
    ctx.setLineDash([])
  }
}

function fmtVal(v: number): string {
  if (props.unit === 'bytes') return fmtBytes(v)
  if (props.unit === 'ratio') return v.toFixed(2)
  if (v >= 1_000_000) return `${(v / 1_000_000).toFixed(1)}M`
  if (v >= 1_000) return `${(v / 1_000).toFixed(1)}k`
  return Math.round(v).toString()
}
function fmtDay(d: string): string {
  return d && d.length >= 10 ? d.slice(5) : d
}
const tipLeft = computed(() => {
  if (hoverIdx.value === null || !geom.value) return '0%'
  return `${geom.value.xOf(hoverIdx.value).toFixed(2)}%`
})
const tipEdge = computed(() => {
  if (hoverIdx.value === null || !geom.value) return false
  return geom.value.xOf(hoverIdx.value) > 82
})

watch(
  [() => props.values, () => props.color, hoverIdx],
  () => void nextTick(draw),
  { deep: true, flush: 'post' },
)
onMounted(() => {
  draw()
  if (typeof ResizeObserver !== 'undefined' && wrapRef.value) {
    ro = new ResizeObserver(() => draw())
    ro.observe(wrapRef.value)
  }
})
onBeforeUnmount(() => ro?.disconnect())
</script>

<template>
  <div ref="wrapRef" class="spark" @mouseleave="hoverIdx = null">
    <canvas v-if="geom" ref="canvasRef" class="spark-svg" role="img"
      :aria-label="t('spark.trend')" />
    <div v-else class="spark-empty text-muted">—</div>
    <div v-if="geom && days.length === geom.n" class="hot">
      <div v-for="(d, i) in days" :key="i" class="hot-col"
        :style="{ width: `${100 / geom.n}%` }" @mouseenter="hoverIdx = i" />
    </div>
    <div v-if="hoverIdx !== null && values[hoverIdx] !== undefined" class="tip"
      :class="{ edge: tipEdge }" :style="{ left: tipLeft }">
      <span class="tip-day">{{ fmtDay(days[hoverIdx] || '') }}</span>
      <b :style="{ color }">{{ fmtVal(values[hoverIdx]) }}</b>
    </div>
  </div>
</template>

<style scoped>
.spark { position: relative; height: 38px; margin-top: 10px; }
.spark-svg { width: 100%; height: 100%; display: block; }
.spark-empty { height: 38px; display: flex; align-items: center; justify-content: center;
  font-size: var(--fs-md); }
.hot { position: absolute; inset: 0; display: flex; }
.hot-col { height: 100%; }
.tip { position: absolute; top: -4px; transform: translateX(-50%); z-index: 5;
  padding: 3px 7px; white-space: nowrap; background: rgba(20,22,38,.96);
  border: 1px solid rgba(255,255,255,.1); border-radius: var(--radius-xs);
  box-shadow: var(--shadow-md); font-size: var(--fs-2xs); display: flex;
  align-items: center; gap: var(--sp-1); pointer-events: none; }
.tip.edge { transform: translateX(-90%); }
.tip-day { color: var(--text-muted); }
.tip b { font-weight: 600; }
</style>