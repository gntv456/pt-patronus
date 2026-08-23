<script setup lang="ts">
// 零依赖迷你折线（sparkline）：嵌进仪表盘指标卡，紧凑展示该指标近 N 天趋势形态。
// viewBox 100×36 + preserveAspectRatio="none" 横向自适应卡片宽度，
// 折线用 vector-effect="non-scaling-stroke" 保持 2px 描边不被拉伸变粗；
// 下方渐变面积 + hover 列热区出 tooltip（HTML 百分比定位，与容器宽度无关）。
// 延续项目零依赖画图哲学，不引入 ECharts。
import { ref, computed } from 'vue'
import { fmtBytes } from '@/utils/format'

type Unit = 'bytes' | 'count' | 'ratio'

const props = withDefaults(
  defineProps<{
    values: number[]
    color: string
    unit?: Unit
    days?: string[] // 对应日期（长度需 == values.length），用于 tooltip；缺省则不出 tooltip
  }>(),
  { unit: 'count', days: () => [] },
)

const VB_W = 100
const VB_H = 36
const PAD = 4
const hoverIdx = ref<number | null>(null)

const geom = computed(() => {
  const v = props.values
  const n = v.length
  if (!n) return null
  let lo = Math.min(...v)
  let hi = Math.max(...v)
  if (hi <= lo) hi = lo + 1 // 全平线时撑开一档，避免贴顶/贴底看不见
  const span = hi - lo
  const yOf = (val: number) => VB_H - PAD - ((val - lo) / span) * (VB_H - 2 * PAD)
  const xOf = (i: number) => (n === 1 ? VB_W / 2 : (i / (n - 1)) * VB_W)
  const pts = v.map((val, i) => ({ x: xOf(i), y: yOf(val) }))
  const line = pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x.toFixed(2)} ${p.y.toFixed(2)}`).join(' ')
  const area = `${line} L ${pts[n - 1].x.toFixed(2)} ${VB_H} L ${pts[0].x.toFixed(2)} ${VB_H} Z`
  return { n, xOf, line, area }
})

function fmtVal(v: number): string {
  if (props.unit === 'bytes') return fmtBytes(v)
  if (props.unit === 'ratio') return v.toFixed(2)
  if (v >= 1_000_000) return `${(v / 1_000_000).toFixed(1)}M`
  if (v >= 1_000) return `${(v / 1_000).toFixed(1)}k`
  return Math.round(v).toString()
}

// 唯一化渐变 id：同色多实例会共享同一 <defs>，用 color + 随机后缀避免 document 级 id 冲突。
const gradId = `sg-${props.color.replace('#', '')}`

const tipLeft = computed(() => {
  if (hoverIdx.value === null || !geom.value) return '0%'
  return `${geom.value.xOf(hoverIdx.value).toFixed(2)}%`
})
const tipEdge = computed(() => {
  if (hoverIdx.value === null || !geom.value) return false
  return geom.value.xOf(hoverIdx.value) > 82
})
function fmtDay(d: string): string {
  return d && d.length >= 10 ? d.slice(5) : d
}
</script>

<template>
  <div class="spark" @mouseleave="hoverIdx = null">
    <svg v-if="geom" :viewBox="`0 0 ${VB_W} ${VB_H}`" preserveAspectRatio="none" class="spark-svg" role="img" aria-label="近期趋势">
      <defs>
        <linearGradient :id="gradId" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" :stop-color="color" stop-opacity="0.32" />
          <stop offset="100%" :stop-color="color" stop-opacity="0.02" />
        </linearGradient>
      </defs>
      <path :d="geom.area" :fill="`url(#${gradId})`" />
      <path
        :d="geom.line"
        fill="none"
        :stroke="color"
        stroke-width="2"
        stroke-linejoin="round"
        stroke-linecap="round"
        vector-effect="non-scaling-stroke"
      />
      <line
        v-if="hoverIdx !== null"
        :x1="geom.xOf(hoverIdx)"
        :x2="geom.xOf(hoverIdx)"
        :y1="0"
        :y2="VB_H"
        class="hover-line"
        vector-effect="non-scaling-stroke"
      />
    </svg>
    <div v-else class="spark-empty text-muted">—</div>

    <!-- 列热区：HTML 百分比定位，宽度随容器自适应；仅当提供了 days 才启用 hover -->
    <div v-if="geom && days.length === geom.n" class="hot">
      <div
        v-for="(d, i) in days"
        :key="i"
        class="hot-col"
        :style="{ width: `${100 / geom.n}%` }"
        @mouseenter="hoverIdx = i"
      />
    </div>

    <div
      v-if="hoverIdx !== null && values[hoverIdx] !== undefined"
      class="tip"
      :class="{ edge: tipEdge }"
      :style="{ left: tipLeft }"
    >
      <span class="tip-day">{{ fmtDay(days[hoverIdx] || '') }}</span>
      <b :style="{ color }">{{ fmtVal(values[hoverIdx]) }}</b>
    </div>
  </div>
</template>

<style scoped>
.spark { position: relative; height: 38px; margin-top: 10px; }
.spark-svg { width: 100%; height: 100%; display: block; overflow: visible; }
.spark-empty { height: 38px; display: flex; align-items: center; justify-content: center; font-size: var(--fs-md); }
.hover-line { stroke: rgba(255, 255, 255, 0.28); stroke-width: 1; stroke-dasharray: 3 3; }

.hot { position: absolute; inset: 0; display: flex; }
.hot-col { height: 100%; }

.tip {
  position: absolute; top: -4px; transform: translateX(-50%);
  z-index: 5; padding: 3px 7px; white-space: nowrap;
  background: rgba(20, 22, 38, 0.96); border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: var(--radius-xs); box-shadow: var(--shadow-md);
  font-size: var(--fs-2xs); display: flex; align-items: center; gap: var(--sp-1);
  pointer-events: none;
}
.tip.edge { transform: translateX(-90%); }
.tip-day { color: var(--text-muted); }
.tip b { font-weight: 600; }
</style>
