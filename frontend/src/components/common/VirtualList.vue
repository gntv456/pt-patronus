<script setup lang="ts">
// VirtualList — 零依赖定高虚拟列表（阶段 1）
//
// 把 O(n) DOM 降到 O(视口)：只渲染可见区间 + overscan 缓冲。
// 不引入 @tanstack/virtual，避免新依赖；范围计算抽成纯函数便于单测。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { computeRange } from './virtualListRange'

const props = withDefaults(
  defineProps<{ items: readonly unknown[]; itemHeight?: number; overscan?: number; height?: number | string }>(),
  { itemHeight: 48, overscan: 6, height: '100%' },
)
const viewport = ref<HTMLElement | null>(null)
const scrollTop = ref(0)
const viewportH = ref(400)
let ro: ResizeObserver | null = null

function onScroll(e: Event) { scrollTop.value = (e.target as HTMLElement).scrollTop }

const range = computed(() =>
  computeRange(scrollTop.value, viewportH.value, props.itemHeight, props.overscan, props.items.length),
)
const visible = computed(() =>
  props.items.slice(range.value.start, range.value.end).map((item, i) => ({ item, index: range.value.start + i })),
)
const totalHeight = computed(() => props.items.length * props.itemHeight)
const heightStyle = computed(() => (typeof props.height === 'number' ? `${props.height}px` : props.height))

onMounted(() => {
  if (!viewport.value) return
  viewportH.value = viewport.value.clientHeight || 400
  if (typeof ResizeObserver !== 'undefined') {
    ro = new ResizeObserver(() => { viewportH.value = viewport.value?.clientHeight || 400 })
    ro.observe(viewport.value)
  }
})
onBeforeUnmount(() => ro?.disconnect())

// 数据变短时把滚动位置夹回合法范围，避免空白区
watch(() => props.items.length, (n) => {
  if (viewport.value && scrollTop.value > n * props.itemHeight) {
    scrollTop.value = Math.max(0, n * props.itemHeight - viewportH.value)
    viewport.value.scrollTop = scrollTop.value
  }
})
</script>

<template>
  <div ref="viewport" class="virtual-list" :style="{ height: heightStyle }" @scroll.passive="onScroll">
    <div :style="{ height: totalHeight + 'px', position: 'relative' }">
      <div :style="{ transform: `translateY(${range.padTop}px)` }">
        <div v-for="row in visible" :key="row.index" :style="{ height: itemHeight + 'px', boxSizing: 'border-box' }">
          <slot :item="row.item" :index="row.index" />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.virtual-list { overflow-y: auto; overflow-x: hidden; -webkit-overflow-scrolling: touch; }
</style>