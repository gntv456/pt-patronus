<script setup lang="ts">
import { computed } from 'vue'

// DashboardRingChart — 手撸 SVG 多段环形图（零依赖，对齐项目画图哲学）。
// 仪表盘「站点健康分布」「契约状态分布」复用。全 0 时显示空底环 + 占位。
export interface RingSegment {
  label: string
  value: number
  color: string
}

const props = withDefaults(
  defineProps<{
    items: RingSegment[]
    size?: number
    thickness?: number
    centerValue?: string
    centerLabel?: string
    legend?: boolean
  }>(),
  { size: 128, thickness: 14, legend: true },
)

const total = computed(() => props.items.reduce((s, i) => s + (i.value || 0), 0))
const radius = computed(() => (props.size - props.thickness) / 2)
const circumference = computed(() => 2 * Math.PI * radius.value)

// 每段：dasharray=[段长, 周长-段长]，dashoffset 累计前置段长（负值顺延起点）。
const segments = computed(() => {
  if (total.value <= 0) return []
  let acc = 0
  return props.items.map((it) => {
    const len = ((it.value || 0) / total.value) * circumference.value
    const seg = { color: it.color, len, gap: circumference.value - len, offset: -acc }
    acc += len
    return seg
  })
})
const legPct = (v: number) => (total.value > 0 ? Math.round((v / total.value) * 100) : 0)
</script>

<template>
  <div class="ring-chart">
    <div class="ring-wrap" :style="{ width: size + 'px', height: size + 'px' }">
      <svg :width="size" :height="size" :viewBox="`0 0 ${size} ${size}`">
        <g :transform="`rotate(-90 ${size / 2} ${size / 2})`">
          <circle :cx="size / 2" :cy="size / 2" :r="radius" fill="none" :stroke-width="thickness" class="ring-track" />
          <circle
            v-for="(s, i) in segments"
            :key="i"
            :cx="size / 2"
            :cy="size / 2"
            :r="radius"
            fill="none"
            :stroke="s.color"
            :stroke-width="thickness"
            :stroke-dasharray="`${s.len} ${s.gap}`"
            :stroke-dashoffset="s.offset"
          />
        </g>
      </svg>
      <div class="ring-center">
        <strong v-if="centerValue">{{ centerValue }}</strong>
        <small v-if="centerLabel">{{ centerLabel }}</small>
        <small v-else-if="total === 0">暂无数据</small>
      </div>
    </div>
    <ul v-if="legend" class="ring-legend">
      <li v-for="(it, i) in items" :key="i">
        <i :style="{ background: it.color }" />
        <span class="leg-label">{{ it.label }}</span>
        <b class="leg-val">{{ it.value }}</b>
        <small class="leg-pct" v-if="total > 0">{{ legPct(it.value) }}%</small>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.ring-chart {
  display: flex;
  align-items: center;
  gap: 18px;
}
.ring-wrap {
  position: relative;
  flex: none;
}
.ring-track {
  stroke: rgba(148, 163, 184, 0.2);
}
.ring-center {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 2px;
  pointer-events: none;
}
.ring-center strong {
  font-size: 20px;
  font-weight: 700;
  color: var(--text-strong);
}
.ring-center small {
  font-size: 11px;
  color: var(--text-muted);
}
.ring-legend {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 7px;
  min-width: 0;
}
.ring-legend li {
  display: grid;
  grid-template-columns: 12px minmax(0, 1fr) auto auto;
  align-items: center;
  gap: 8px;
  font-size: 12.5px;
}
.ring-legend i {
  width: 10px;
  height: 10px;
  border-radius: 3px;
}
.leg-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-strong);
}
.leg-val {
  font-weight: 600;
  color: var(--text-strong);
}
.leg-pct {
  color: var(--text-muted);
  font-size: 11px;
}
@media (max-width: 640px) {
  .ring-chart {
    flex-direction: column;
    gap: 12px;
  }
}
</style>
