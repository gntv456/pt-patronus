<script setup lang="ts">
import { ref, watch, nextTick } from 'vue'
import type { MonitorTarget } from '@/api/monitor'

const props = defineProps<{
  visible: boolean
  target: MonitorTarget | null
  output: string
  input: string
  connected: boolean
  connecting: boolean
}>()

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'update:input', v: string): void
  (e: 'closed'): void
  (e: 'connect'): void
  (e: 'sendLine'): void
  (e: 'ctrlC'): void
}>()

const screenEl = ref<HTMLElement | null>(null)

// 终端输出增长时自动滚动到底部（原 appendTerminal 的 nextTick 逻辑）。
watch(
  () => props.output,
  () => {
    nextTick(() => {
      if (screenEl.value) screenEl.value.scrollTop = screenEl.value.scrollHeight
    })
  },
)
</script>

<template>
  <el-drawer
    :model-value="visible"
    :title="target ? `${target.name} · 在线终端` : '在线终端'"
    size="min(860px, 96vw)"
    @update:model-value="emit('update:visible', $event)"
    @closed="emit('closed')"
  >
    <div class="terminal-wrap">
      <div class="terminal-head">
        <span :class="{ on: connected }">{{ connected ? '已连接' : connecting ? '连接中' : '未连接' }}</span>
        <div>
          <el-button size="small" :loading="connecting" @click="emit('connect')">重连</el-button>
          <el-button size="small" type="warning" plain @click="emit('ctrlC')">Ctrl+C</el-button>
        </div>
      </div>
      <pre ref="screenEl" class="terminal-screen">{{ output }}</pre>
      <div class="terminal-input">
        <el-input
          :model-value="input"
          placeholder="输入命令，回车发送"
          :disabled="!connected"
          @update:model-value="emit('update:input', $event)"
          @keydown.enter.prevent="emit('sendLine')"
        />
        <el-button type="primary" :disabled="!connected" @click="emit('sendLine')">发送</el-button>
      </div>
    </div>
  </el-drawer>
</template>

<style scoped src="../../views/nas-shared/terminal.css"></style>
