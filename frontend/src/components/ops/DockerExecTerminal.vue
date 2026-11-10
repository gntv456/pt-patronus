<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, watch, onBeforeUnmount, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { authApi } from '@/api/auth'
import { dockerApi } from '@/api/docker'
import '@xterm/xterm/css/xterm.css'

const { t } = useI18n()
const props = defineProps<{ visible: boolean; container: string }>()
const emit = defineEmits<{ 'update:visible': [val: boolean] }>()
const termContainer = ref<HTMLElement | null>(null)
let term: Terminal | null = null
let fit: FitAddon | null = null
let ws: WebSocket | null = null

watch(
  () => props.visible,
  async (v) => {
    if (!v) {
      cleanup()
      return
    }
    await nextTick()
    term?.dispose()
    term = new Terminal({ fontSize: 13, cursorBlink: true, theme: { background: '#1e1e2e', foreground: '#cdd6f4' } })
    fit = new FitAddon()
    term.loadAddon(fit)
    if (termContainer.value) {
      term.open(termContainer.value)
      fit.fit()
      term.focus()
      // ws 可能尚未建立或正在关闭：send 前判 OPEN，避免 InvalidStateError 冒泡
      term.onData((d) => {
        if (ws && ws.readyState === WebSocket.OPEN) ws.send(d)
      })
    }
    try {
      const { ticket } = await authApi.wsTicket()
      const url = dockerApi.localExecURL(props.container) + '&ticket=' + encodeURIComponent(ticket)
      ws = new WebSocket(url)
      ws.onopen = () => term?.write('\x1b[32m[' + t('det.connected') + ']\x1b[0m\r\n')
      ws.onmessage = (ev) => term?.write(typeof ev.data === 'string' ? ev.data : new Uint8Array(ev.data))
      ws.onclose = () => term?.write('\r\n\x1b[33m[' + t('det.closed') + ']\x1b[0m\r\n')
      ws.onerror = () => term?.write('\r\n\x1b[31m[' + t('det.error') + ']\x1b[0m\r\n')
    } catch (e: any) {
      ElMessage.error(e.message)
    }
  },
)

function cleanup() {
  ws?.close()
  ws = null
  term?.dispose()
  term = null
}
onBeforeUnmount(() => cleanup())
</script>

<template>
  <el-drawer
    :model-value="visible"
    @update:model-value="emit('update:visible', $event)"
    :title="t('det.title', { name: container })"
    size="70%"
    direction="rtl"
    @close="cleanup"
  >
    <div ref="termContainer" class="exec-term-area"></div>
  </el-drawer>
</template>

<style scoped>
.exec-term-area {
  height: 100%;
  padding: 8px;
  background: #1e1e2e;
  border-radius: var(--radius-2xs);
}
</style>
