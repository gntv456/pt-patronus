<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, watch, nextTick, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { authApi } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'
import type { MonitorTarget } from '@/api/monitor'
import { isSSHHost, targetAddr } from '@/views/nasServerHelpers'
import '@xterm/xterm/css/xterm.css'

const { t: ti } = useI18n()

const props = defineProps<{
  visible: boolean
  target: MonitorTarget | null
}>()

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'closed'): void
}>()

const auth = useAuthStore()
const connected = ref(false)
const connecting = ref(false)
const reconnecting = ref(false)
const termContainer = ref<HTMLElement | null>(null)

let term: Terminal | null = null
let fitAddon: FitAddon | null = null
let ws: WebSocket | null = null
let resizeObserver: ResizeObserver | null = null
let heartbeatTimer: number | null = null
let reconnectTimer: number | null = null
let reconnectAttempts = 0
let intentionalClose = false

function nowTag() {
  return new Date().toLocaleTimeString('zh-CN', { hour12: false })
}

function termURL(id: number, ticket: string) {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  const cols = term?.cols || 120
  const rows = term?.rows || 32
  return `${proto}://${location.host}/api/v1/monitors/${id}/terminal?ticket=${encodeURIComponent(ticket)}&cols=${cols}&rows=${rows}`
}

function ensureTerm() {
  if (!termContainer.value) return
  if (term) return
  term = new Terminal({
    fontSize: 14,
    fontFamily: "'Cascadia Code', 'Fira Code', 'JetBrains Mono', Consolas, monospace",
    cursorBlink: true,
    theme: { background: '#1e1e2e', foreground: '#cdd6f4', cursor: '#cdd6f4' },
    scrollback: 5000,
    allowProposedApi: true,
  })
  fitAddon = new FitAddon()
  term.loadAddon(fitAddon)
  term.loadAddon(new WebLinksAddon())
  term.open(termContainer.value)
  fitAddon.fit()

  // 每次按键直接透传到远端（真实终端体验）
  term.onData((data) => {
    if (ws?.readyState === WebSocket.OPEN) ws.send(data)
  })

  // 容器尺寸自适应
  resizeObserver?.disconnect()
  resizeObserver = new ResizeObserver(() => {
    if (fitAddon && term) fitAddon.fit()
  })
  resizeObserver.observe(termContainer.value)
}

function startHeartbeat() {
  stopHeartbeat()
  heartbeatTimer = window.setInterval(() => {
    if (ws?.readyState === WebSocket.OPEN) ws.send('')
  }, 25000)
}
function stopHeartbeat() {
  if (heartbeatTimer !== null) {
    window.clearInterval(heartbeatTimer)
    heartbeatTimer = null
  }
}

function scheduleReconnect() {
  if (intentionalClose || !props.visible || !props.target) return
  if (reconnectAttempts >= 5) {
    term?.write(`\r\n\x1b[31m[${nowTag()}] {ti('ntd.tooManyReconnects')}\x1b[0m\r\n`)
    reconnecting.value = false
    return
  }
  reconnecting.value = true
  reconnectAttempts++
  const delay = Math.min(1000 * 2 ** reconnectAttempts, 16000)
  term?.write(`\r\n\x1b[33m[${nowTag()}] ${ti('ntd.reconnectIn', { s: Math.round(delay / 1000), n: reconnectAttempts })}...\x1b[0m\r\n`)
  reconnectTimer = window.setTimeout(() => connect(true), delay)
}

async function connect(isReconnect = false) {
  const t = props.target
  if (!t) return
  if (connecting.value || connected.value) return
  if (reconnectTimer !== null) {
    window.clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  connecting.value = true
  intentionalClose = false
  await nextTick()
  ensureTerm()
  if (!isReconnect) {
    term?.clear()
    term?.write(`\x1b[36m[${nowTag()}] ${ti('ntd.connectingTo', { name: t.name, addr: targetAddr(t) })}...\x1b[0m\r\n`)
  }
  try {
    const { ticket } = await authApi.wsTicket()
    ws = new WebSocket(termURL(t.id, ticket))
    ws.onopen = () => {
      connecting.value = false
      connected.value = true
      reconnecting.value = false
      reconnectAttempts = 0
      startHeartbeat()
      term?.write(`\x1b[32m[${nowTag()}] {ti('ntd.connectedHint')}\x1b[0m\r\n\r\n`)
      term?.focus()
      fitAddon?.fit()
    }
    ws.onmessage = (ev) => {
      term?.write(typeof ev.data === 'string' ? ev.data : new Uint8Array(ev.data))
    }
    ws.onerror = () => {
      term?.write(`\r\n\x1b[31m[${nowTag()}] {ti('ot.connError')}\x1b[0m\r\n`)
    }
    ws.onclose = () => {
      connected.value = false
      connecting.value = false
      stopHeartbeat()
      if (!intentionalClose) {
        term?.write(`\r\n\x1b[33m[${nowTag()}] {ti('ntd.disconnected')}\x1b[0m\r\n`)
        scheduleReconnect()
      } else {
        term?.write(`\r\n\x1b[33m[${nowTag()}] {ti('det.closed')}\x1b[0m\r\n`)
      }
    }
  } catch (e: unknown) {
    connecting.value = false
    term?.write(`\r\n\x1b[31m[${nowTag()}] ${ti('ntd.connectFailed', { err: (e as Error).message || ti('ntd.unknownErr') })}\x1b[0m\r\n`)
  }
}

function disconnect() {
  intentionalClose = true
  if (reconnectTimer !== null) {
    window.clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  reconnecting.value = false
  reconnectAttempts = 0
  stopHeartbeat()
  ws?.close()
  ws = null
  connected.value = false
}

function sendCtrlC() {
  if (ws?.readyState === WebSocket.OPEN) ws.send('\x03')
}

// 打开抽屉时校验权限并连接
watch(
  () => props.visible,
  async (v) => {
    if (!v) return
    const t = props.target
    if (!t) return
    if (!auth.isAdmin) {
      ElMessage.warning(ti('ntd.adminOnly'))
      emit('update:visible', false)
      return
    }
    if (!isSSHHost(t)) {
      ElMessage.warning(ti('ntd.sshOnly'))
      emit('update:visible', false)
      return
    }
    reconnectAttempts = 0
    intentionalClose = false
    await nextTick()
    connect(false)
  },
)

// 关闭抽屉 → 清理
watch(
  () => props.visible,
  (v) => {
    if (v) return
    disconnect()
  },
)

onBeforeUnmount(() => {
  disconnect()
  resizeObserver?.disconnect()
  resizeObserver = null
  term?.dispose()
  term = null
})
</script>

<template>
  <el-drawer
    :model-value="visible"
    :title="target ? ti('ntd.titleWith', { name: target.name }) : ti('ntd.title')"
    size="min(860px, 96vw)"
    @update:model-value="emit('update:visible', $event)"
    @closed="emit('closed')"
  >
    <div class="terminal-wrap">
      <div class="terminal-head">
        <span :class="{ on: connected, mid: connecting || reconnecting }">
          {{ connected ? '● ' + ti('ot.connected') : connecting ? '○ ' + ti('tw.connecting') + '…' : reconnecting ? '↻ ' + ti('ntd.reconnecting') + '…' : '○ ' + ti('ot.disconnected') }}
        </span>
        <div>
          <el-button size="small" :loading="connecting" @click="connect(false)">{{ ti('ntd.reconnect') }}</el-button>
          <el-button v-if="connected" size="small" type="danger" plain @click="disconnect">{{ ti('ot.disconnect') }}</el-button>
          <el-button size="small" type="warning" plain :disabled="!connected" @click="sendCtrlC">Ctrl+C</el-button>
        </div>
      </div>
      <div ref="termContainer" class="terminal-xterm"></div>
    </div>
  </el-drawer>
</template>

<style scoped>
.terminal-wrap {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.terminal-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 0 12px;
  font-size: var(--fs-body);
}
.terminal-head .on { color: var(--pt-success); font-weight: 600; }
.terminal-head .mid { color: var(--pt-warn); font-weight: 600; }
.terminal-xterm {
  flex: 1;
  min-height: 420px;
  background: #1e1e2e;
  border-radius: var(--radius-xs);
  padding: 8px;
  overflow: hidden;
}
</style>