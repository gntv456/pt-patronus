<script setup lang="ts">
import { computed, ref } from 'vue'
import { fmtBytes } from '@/utils/format'

type ChartUnit = 'bytes' | 'count' | 'ratio'
type ChartSeries = {
  label: string
  color: string
  values: number[]
}

const props = withDefaults(
  defineProps<{
    days: string[]
    series: ChartSeries[]
    unit?: ChartUnit
  }>(),
  { unit: 'count' },
)

const chart = ref<HTMLDivElement | null>(null)
const hoverIndex = ref<number | null>(null)
const VB_W = 800
const VB_H = 232
const PAD = { left: 54, right: 18, top: 18, bottom: 34 }
const plotW = VB_W - PAD.left - PAD.right
const plotH = VB_H - PAD.top - PAD.bottom

function fmtValue(value: number): string {
  if (props.unit === 'bytes') return fmtBytes(value)
  if (props.unit === 'ratio') return value.toFixed(2)
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}k`
  return Math.round(value).toLocaleString('zh-CN')
}

const geometry = computed(() => {
  const length = Math.max(0, ...props.series.map((item) => item.values.length))
  if (!length) return null
  const values = props.series.flatMap((item) => item.values).filter(Number.isFinite)
  if (!values.length) return null
  let min = Math.min(...values)
  let max = Math.max(...values)
  if (max <= min) max = min + 1
  const padding = (max - min) * 0.08
  min = Math.max(0, min - padding)
  max += padding
  const span = max - min
  const xOf = (index: number) => PAD.left + (length <= 1 ? plotW / 2 : (index / (length - 1)) * plotW)
  const yOf = (value: number) => PAD.top + plotH - ((value - min) / span) * plotH
  const lines = props.series.map((item) => ({
    ...item,
    path: item.values
      .map((value, index) => `${index === 0 ? 'M' : 'L'}${xOf(index).toFixed(2)} ${yOf(value).toFixed(2)}`)
      .join(' '),
  }))
  const yTicks = Array.from({ length: 5 }, (_, index) => {
    const ratio = index / 4
    return { y: PAD.top + ratio * plotH, value: max - ratio * span }
  })
  const labelIndexes = Array.from(new Set([0, Math.floor((length - 1) / 3), Math.floor(((length - 1) * 2) / 3), length - 1]))
  return { length, xOf, yOf, lines, yTicks, labelIndexes }
})

const hoverX = computed(() => {
  if (hoverIndex.value === null || !geometry.value) return 0
  return geometry.value.xOf(hoverIndex.value)
})
const tooltipStyle = computed(() => ({ left: `${(hoverX.value / VB_W) * 100}%` }))

function onMove(event: MouseEvent) {
  if (!chart.value || !geometry.value) return
  const rect = chart.value.getBoundingClientRect()
  const viewX = ((event.clientX - rect.left) / rect.width) * VB_W
  const ratio = Math.max(0, Math.min(1, (viewX - PAD.left) / plotW))
  hoverIndex.value = Math.round(ratio * (geometry.value.length - 1))
}

function shortDay(day: string): string {
  return day.length >= 10 ? day.slice(5) : day
}
</script>

<template>
  <div ref="chart" class="trend-chart" @mousemove="onMove" @mouseleave="hoverIndex = null">
    <svg
      v-if="geometry"
      class="trend-svg"
      :viewBox="`0 0 ${VB_W} ${VB_H}`"
      preserveAspectRatio="none"
      role="img"
      aria-label="账号趋势图"
    >
      <g v-for="tick in geometry.yTicks" :key="tick.y">
        <line :x1="PAD.left" :x2="VB_W - PAD.right" :y1="tick.y" :y2="tick.y" class="grid-line" />
        <text :x="PAD.left - 9" :y="tick.y + 4" text-anchor="end" class="axis-text">{{ fmtValue(tick.value) }}</text>
      </g>
      <path
        v-for="item in geometry.lines"
        :key="item.label"
        :d="item.path"
        fill="none"
        :stroke="item.color"
        stroke-width="2.5"
        stroke-linecap="round"
        stroke-linejoin="round"
        vector-effect="non-scaling-stroke"
      />
      <g v-for="index in geometry.labelIndexes" :key="index">
        <text :x="geometry.xOf(index)" :y="VB_H - 10" text-anchor="middle" class="axis-text">
          {{ shortDay(days[index] || '') }}
        </text>
      </g>
      <g v-if="hoverIndex !== null">
        <line :x1="hoverX" :x2="hoverX" :y1="PAD.top" :y2="PAD.top + plotH" class="hover-rule" />
        <circle
          v-for="item in geometry.lines"
          :key="item.label"
          :cx="hoverX"
          :cy="geometry.yOf(item.values[hoverIndex] ?? 0)"
          r="4"
          :fill="item.color"
          class="hover-dot"
          vector-effect="non-scaling-stroke"
        />
      </g>
    </svg>
    <div v-else class="chart-empty">暂无趋势数据</div>

    <div v-if="hoverIndex !== null && geometry" class="chart-tooltip" :style="tooltipStyle">
      <span>{{ days[hoverIndex] || '-' }}</span>
      <div v-for="item in geometry.lines" :key="item.label">
        <i :style="{ backgroundColor: item.color }" />
        <span>{{ item.label }}</span>
        <b>{{ fmtValue(item.values[hoverIndex] ?? 0) }}</b>
      </div>
    </div>
  </div>
</template>

<style scoped>
.trend-chart {
  position: relative;
  width: 100%;
  height: 250px;
  min-height: 250px;
}
.trend-svg {
  display: block;
  width: 100%;
  height: 100%;
  overflow: visible;
}
.grid-line {
  stroke: var(--divider);
  stroke-width: 1;
  vector-effect: non-scaling-stroke;
}
.axis-text {
  fill: var(--text-muted);
  font-size: 10px;
  letter-spacing: 0;
}
.hover-rule {
  stroke: var(--text-muted);
  stroke-width: 1;
  stroke-dasharray: 4 4;
  opacity: 0.45;
  vector-effect: non-scaling-stroke;
}
.hover-dot {
  stroke: var(--glass-bg-strong);
  stroke-width: 2;
}
.chart-tooltip {
  position: absolute;
  top: 10px;
  z-index: 4;
  min-width: 150px;
  padding: 9px 10px;
  transform: translateX(-50%);
  border: 1px solid var(--divider);
  border-radius: 6px;
  background: var(--glass-bg-strong);
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.18);
  font-size: 12px;
  pointer-events: none;
}
.chart-tooltip > span {
  display: block;
  margin-bottom: 6px;
  color: var(--text-muted);
}
.chart-tooltip div {
  display: grid;
  grid-template-columns: 8px 1fr auto;
  align-items: center;
  gap: 6px;
  margin-top: 4px;
}
.chart-tooltip i {
  width: 7px;
  height: 7px;
  border-radius: 50%;
}
.chart-tooltip b {
  color: var(--text-strong);
}
.chart-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: var(--text-muted);
  font-size: 13px;
}
@media (max-width: 720px) {
  .trend-chart {
    height: 220px;
    min-height: 220px;
  }
  .chart-tooltip {
    transform: translateX(-70%);
  }
}
</style>
