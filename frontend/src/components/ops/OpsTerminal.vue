<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, computed, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { authApi } from '@/api/auth'
import { monitorApi, type MonitorTarget } from '@/api/monitor'
import '@xterm/xterm/css/xterm.css'

const { t } = useI18n()
const targets = ref<MonitorTarget[]>([])
const selectedTarget = ref<number>(0)
const connected = ref(false)
const connecting = ref(false)
const error = ref('')

// 快速命令
const quickCmds = ref<{ name: string; cmd: string }[]>([
  { name: t('ot.qcSysinfo'), cmd: 'uname -a' },
  { name: t('ot.qcDisk'), cmd: 'df -h' },
  { name: t('ot.qcMem'), cmd: 'free -h' },
  { name: 'CPU', cmd: 'top -bn1 | head -5' },
  { name: t('ot.qcNet'), cmd: 'ip addr' },
  { name: t('ot.qcPorts'), cmd: 'ss -tlnp' },
  { name: 'Docker', cmd: 'docker ps' },
  { name: t('ot.qcLogs'), cmd: 'journalctl -u sshd -n 50 --no-pager' },
])
const newCmdName = ref('')
const newCmdText = ref('')
const cmdDialogVisible = ref(false)

// 终端配置
const termConfig = ref({
  fontSize: 14,
  theme: 'dark',
})
const configVisible = ref(false)

const sshTargets = computed(() => targets.value.filter((t) => t.type === 'host'))

let term: Terminal | null = null
let fitAddon: FitAddon | null = null
let ws: WebSocket | null = null
const termContainer = ref<HTMLElement | null>(null)
let resizeObserver: ResizeObserver | null = null

// 从 localStorage 加载自定义快速命令
function loadQuickCmds() {
  try {
    const saved = localStorage.getItem('ops_terminal_quickcmds')
    if (saved) {
      const parsed = JSON.parse(saved)
      if (Array.isArray(parsed) && parsed.length) quickCmds.value = parsed
    }
    const cfg = localStorage.getItem('ops_terminal_config')
    if (cfg) termConfig.value = { ...termConfig.value, ...JSON.parse(cfg) }
  } catch {}
}
function saveQuickCmds() {
  localStorage.setItem('ops_terminal_quickcmds', JSON.stringify(quickCmds.value))
}
function saveConfig() {
  localStorage.setItem('ops_terminal_config', JSON.stringify(termConfig.value))
  if (term) {
    term.options.fontSize = termConfig.value.fontSize
  }
}

function getTheme() {
  return termConfig.value.theme === 'light'
    ? { background: '#fafafa', foreground: '#333', cursor: '#333' }
    : { background: '#1e1e2e', foreground: '#cdd6f4', cursor: '#cdd6f4' }
}

function initTerm() {
  if (!termContainer.value) return
  term?.dispose()
  term = new Terminal({
    fontSize: termConfig.value.fontSize,
    fontFamily: "'Cascadia Code', 'Fira Code', 'JetBrains Mono', Consolas, monospace",
    cursorBlink: true,
    theme: getTheme(),
    scrollback: 5000,
    allowProposedApi: true,
  })
  fitAddon = new FitAddon()
  term.loadAddon(fitAddon)
  term.loadAddon(new WebLinksAddon())
  term.open(termContainer.value)
  fitAddon.fit()

  term.onData((data) => {
    if (ws?.readyState === WebSocket.OPEN) ws.send(data)
  })

  // 窗口尺寸自适应
  resizeObserver?.disconnect()
  resizeObserver = new ResizeObserver(() => {
    if (fitAddon && term) {
      fitAddon.fit()
      sendResize()
    }
  })
  resizeObserver.observe(termContainer.value)
}

function sendResize() {
  if (!term || !ws || ws.readyState !== WebSocket.OPEN) return
  // 后端目前不接受 resize 消息（Shell 固定初始 cols/rows），这里仅 fit 本地渲染
  // 未来可扩展协议：ws.send(JSON.stringify({type:'resize',cols,rows}))
}

async function connect() {
  if (!selectedTarget.value) {
    ElMessage.warning(t('ot.pickHostFirst'))
    return
  }
  if (connecting.value || connected.value) return
  connecting.value = true
  error.value = ''
  term?.clear()

  try {
    const { ticket } = await authApi.wsTicket()
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    const cols = term?.cols || 120
    const rows = term?.rows || 32
    const url = `${proto}://${location.host}/api/v1/monitors/${selectedTarget.value}/terminal?ticket=${encodeURIComponent(ticket)}&cols=${cols}&rows=${rows}`
    ws = new WebSocket(url)

    ws.onopen = () => {
      connected.value = true
      connecting.value = false
      term?.focus()
    }
    ws.onmessage = (ev) => {
      term?.write(typeof ev.data === 'string' ? ev.data : new Uint8Array(ev.data))
    }
    ws.onerror = () => {
      error.value = t('ot.connError')
      connecting.value = false
    }
    ws.onclose = () => {
      connected.value = false
      connecting.value = false
      term?.write('\r\n\x1b[33m[' + t('det.closed') + ']\x1b[0m\r\n')
    }
  } catch (e: any) {
    connecting.value = false
    error.value = e.message
  }
}

function disconnect() {
  ws?.close()
  ws = null
  connected.value = false
}

function sendCmd(cmd: string) {
  if (!ws || ws.readyState !== WebSocket.OPEN) {
    ElMessage.warning(t('ot.notConnected'))
    return
  }
  ws.send(cmd + '\r')
  term?.focus()
}

function sendCtrlC() {
  if (ws?.readyState === WebSocket.OPEN) ws.send('\x03')
}

function addQuickCmd() {
  if (!newCmdName.value || !newCmdText.value) {
    ElMessage.warning(t('ot.cmdRequired'))
    return
  }
  quickCmds.value.push({ name: newCmdName.value, cmd: newCmdText.value })
  saveQuickCmds()
  newCmdName.value = ''
  newCmdText.value = ''
  ElMessage.success(t('common.added'))
}

function removeQuickCmd(i: number) {
  quickCmds.value.splice(i, 1)
  saveQuickCmds()
}

async function onTargetChange() {
  if (connected.value) disconnect()
  await nextTick()
  connect()
}

async function loadTargets() {
  try {
    targets.value = await monitorApi.list()
    if (sshTargets.value.length && !selectedTarget.value) selectedTarget.value = sshTargets.value[0].id
  } catch (e: any) { error.value = e.message }
}

onMounted(async () => {
  loadQuickCmds()
  await loadTargets()
  await nextTick()
  initTerm()
  if (selectedTarget.value) connect()
})

onBeforeUnmount(() => {
  disconnect()
  resizeObserver?.disconnect()
  term?.dispose()
})
</script>

<template>
  <div class="ops-term">
    <!-- 工具栏 -->
    <div class="term-toolbar glass">
      <div class="toolbar-left">
        <label class="target-label">{{ t('ot.host') }}</label>
        <el-select v-model="selectedTarget" :placeholder="t('ot.hostPh')" style="width: 200px" @change="onTargetChange">
          <el-option v-for="t in sshTargets" :key="t.id" :label="t.name" :value="t.id" />
        </el-select>
        <span class="conn-status" :class="{ on: connected, connecting }">
          {{ connected ? '● ' + t('ot.connected') : connecting ? '○ ' + t('tw.connecting') : '○ ' + t('ot.disconnected') }}
        </span>
      </div>
      <div class="toolbar-right">
        <el-button size="small" @click="sendCtrlC" :disabled="!connected">Ctrl+C</el-button>
        <el-button size="small" @click="configVisible = true">{{ t('ot.config') }}</el-button>
        <el-button size="small" @click="cmdDialogVisible = true">{{ t('ot.quickCmds') }}</el-button>
        <el-button v-if="!connected" type="primary" size="small" :loading="connecting" @click="connect">{{ t('ot.connect') }}</el-button>
        <el-button v-else type="danger" size="small" @click="disconnect">{{ t('ot.disconnect') }}</el-button>
      </div>
    </div>

    <div v-if="error" class="term-error">{{ error }}</div>
    <div v-if="!sshTargets.length" class="term-empty glass">
      {{ t('ot.noHosts') }}
    </div>

    <!-- 快速命令条 -->
    <div v-if="sshTargets.length" class="quick-bar glass">
      <span
        v-for="(c, i) in quickCmds" :key="i"
        class="quick-chip" :title="c.cmd"
        @click="sendCmd(c.cmd)"
      >{{ c.name }}</span>
    </div>

    <!-- 终端 -->
    <div v-if="sshTargets.length" class="term-container glass" ref="termContainer"></div>

    <!-- 快速命令管理 -->
    <el-dialog v-model="cmdDialogVisible" :title="t('ot.quickCmds')" width="500px">
      <div class="cmd-list">
        <div v-for="(c, i) in quickCmds" :key="i" class="cmd-item">
          <span class="cmd-item-name">{{ c.name }}</span>
          <code class="cmd-item-text">{{ c.cmd }}</code>
          <el-button size="small" type="danger" link @click="removeQuickCmd(i)">{{ t('common.delete') }}</el-button>
        </div>
      </div>
      <el-divider />
      <div class="cmd-add">
        <el-input v-model="newCmdName" :placeholder="t('common.name')" style="width: 120px" />
        <el-input v-model="newCmdText" :placeholder="t('ot.cmdPh')" style="flex: 1" @keyup.enter="addQuickCmd" />
        <el-button type="primary" @click="addQuickCmd">{{ t('common.add') }}</el-button>
      </div>
    </el-dialog>

    <!-- 配置对话框 -->
    <el-dialog v-model="configVisible" :title="t('ot.termConfig')" width="400px">
      <el-form label-width="80px">
        <el-form-item :label="t('ot.fontSize')">
          <el-slider v-model="termConfig.fontSize" :min="10" :max="24" @change="saveConfig" />
        </el-form-item>
        <el-form-item :label="t('ot.theme')">
          <el-radio-group v-model="termConfig.theme" @change="saveConfig">
            <el-radio value="dark">{{ t('ot.dark') }}</el-radio>
            <el-radio value="light">{{ t('ot.light') }}</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <div class="config-hint">{{ t('ot.configHint') }}</div>
    </el-dialog>
  </div>
</template>

<style scoped>
.ops-term { padding: 0 4px; display: flex; flex-direction: column; height: 100%; }
.term-toolbar { display: flex; align-items: center; justify-content: space-between; padding: 10px 16px; margin-bottom: 10px; }
.toolbar-left { display: flex; align-items: center; gap: 12px; }
.toolbar-right { display: flex; gap: 6px; }
.target-label { font-size: 13px; color: var(--text-secondary, #888); white-space: nowrap; }
.conn-status { font-size: 12px; font-family: monospace; }
.conn-status.on { color: #27ae60; }
.conn-status.connecting { color: #f39c12; }
.term-error { color: #e74c3c; margin-bottom: 8px; padding: 6px 12px; background: rgba(231,76,60,.08); border-radius: 6px; font-size: 13px; }
.term-empty { padding: 28px; text-align: center; color: var(--text-secondary, #888); }
.quick-bar { display: flex; gap: 6px; padding: 8px 14px; margin-bottom: 8px; flex-wrap: wrap; }
.quick-chip { padding: 4px 12px; border-radius: 14px; background: rgba(108,92,231,.12); color: var(--accent, #6c5ce7); font-size: 12px; cursor: pointer; white-space: nowrap; transition: all var(--dur-fast) var(--ease-out-smooth); user-select: none; }
.quick-chip:hover { background: rgba(108,92,231,.25); transform: translateY(-1px); }
.term-container { flex: 1; min-height: 400px; padding: 8px; border-radius: 8px; overflow: hidden; }
.cmd-list { max-height: 300px; overflow-y: auto; }
.cmd-item { display: flex; align-items: center; gap: 10px; padding: 6px 0; border-bottom: 1px solid rgba(128,128,128,.1); }
.cmd-item-name { font-weight: 600; min-width: 80px; font-size: 13px; }
.cmd-item-text { flex: 1; font-size: 12px; color: var(--text-secondary,#888); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.cmd-add { display: flex; gap: 8px; }
.config-hint { font-size: 12px; color: var(--text-secondary,#888); margin-top: 8px; }
</style>
