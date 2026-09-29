<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { computed } from 'vue'
import { Handle, Position } from '@vue-flow/core'
import type { NodeInfo } from '@/api/workflow'

// WorkflowNode — Vue Flow 自定义节点（统一 type:'wf'，按 data 渲染）。
// 条件节点渲染两个输出桩（true/false）；其余节点单输出桩。
const { t } = useI18n()

const props = defineProps<{
  id: string
  data: {
    type: string
    name: string
    params?: Record<string, any>
    info?: NodeInfo
    branch?: boolean // 条件节点：两个输出桩
  }
  selected?: boolean
}>()

// 类别配色
const catColor: Record<string, string> = {
  '\u8d44\u6e90': '#4285f4',
  '\u8fc7\u6ee4': '#f39c12',
  '\u4e0b\u8f7d': '#2ecc71',
  '\u901a\u77e5': '#e100ff',
  '\u6570\u636e': '#6c5ce7',
  '\u7ad9\u70b9': '#00b894',
  '\u903b\u8f91': '#e74c3c',
  '\u6d41\u7a0b': '#0f9b0f',
  '\u5a92\u4f53': '#e84393',
  '\u6587\u4ef6': '#0984e3',
}
const color = computed(() => (props.data.info && catColor[props.data.info.category]) || 'var(--pt-primary)')

// 输出端口标签：condition 固定 真/假；switch 来自 branches 参数；其余单输出。
const outputs = computed<string[]>(() => {
  if (props.data.type === 'condition') return ['true', 'false']
  if (props.data.type === 'switch') {
    const raw = (props.data.params && String(props.data.params.branches)) || ''
    return raw
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
  }
  return []
})
</script>

<template>
  <div class="wf-node" :class="{ selected }" :style="{ '--c': color }">
    <Handle type="target" :position="Position.Top" class="handle" />
    <div class="head">
      <span class="dot" :style="{ background: color }"></span>
      <span class="name">{{ data.name }}</span>
    </div>
    <div class="meta">{{ data.info?.name || data.type }}</div>

    <!-- 多输出桩（condition=真/假；switch=branches 标签） -->
    <template v-if="outputs.length">
      <div class="branches">
        <span v-for="o in outputs" :key="o" class="branch-tag">{{
          o === 'true' ? t('wn.true') : o === 'false' ? t('wn.false') : o
        }}</span>
      </div>
      <Handle
        v-for="(o, i) in outputs"
        :id="o"
        :key="o"
        type="source"
        :position="Position.Bottom"
        class="handle"
        :style="{ left: ((i + 1) * 100) / (outputs.length + 1) + '%' }"
      />
    </template>
    <template v-else>
      <Handle type="source" :position="Position.Bottom" class="handle" />
    </template>
  </div>
</template>

<style scoped>
.wf-node {
  width: 160px;
  padding: 10px 12px;
  border-radius: var(--radius-sm);
  background: var(--el-bg-color, #fff);
  border: 2px solid var(--c);
  box-shadow: var(--shadow-md);
  font-size: var(--fs-body);
  cursor: grab;
  transition:
    transform var(--dur-fast) var(--ease-out-spring),
    box-shadow var(--dur-base) var(--ease-out-smooth);
}
.wf-node:hover {
  transform: translateY(-1px);
  box-shadow: var(--shadow-lg);
}
.wf-node:active {
  cursor: grabbing;
  transform: scale(0.98);
}
.wf-node.selected {
  box-shadow:
    0 0 0 3px color-mix(in srgb, var(--c) 40%, transparent),
    0 4px 12px rgba(0, 0, 0, 0.18);
}
.head {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  font-weight: 700;
}
.dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex: none;
}
.meta {
  margin-top: 2px;
  font-size: var(--fs-2xs);
  color: var(--text-muted);
}
.branches {
  display: flex;
  justify-content: space-around;
  margin-top: 6px;
}
.branch-tag {
  font-size: var(--fs-2xs);
  padding: 1px 8px;
  border-radius: var(--radius-pill);
  font-weight: 700;
  background: var(--pt-primary-bg);
  color: var(--pt-primary);
}
.handle {
  width: 11px;
  height: 11px;
  background: var(--c);
  border: 2px solid #fff;
}
</style>
