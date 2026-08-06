// NasServer 在线终端（WebSocket 交互）—— 从 NasServer.vue 抽出的状态簇。
// 依赖注入：auth（管理员判断）。
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { authApi } from '@/api/auth'
import { nowTime, isSSHHost, targetAddr } from './nasServerHelpers'
import type { MonitorTarget } from '@/api/monitor'

export function useTerminal(auth: { isAdmin: boolean }) {
  const terminalVisible = ref(false)
  const terminalTarget = ref<MonitorTarget | null>(null)
  const terminalOutput = ref('')
  const terminalInput = ref('')
  const terminalConnected = ref(false)
  const terminalConnecting = ref(false)
  let terminalSocket: WebSocket | null = null

  function terminalURL(id: number, ticket: string) {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    return `${proto}://${location.host}/api/v1/monitors/${id}/terminal?ticket=${encodeURIComponent(ticket)}&cols=120&rows=32`
  }

  function appendTerminal(text: string) {
    terminalOutput.value = (terminalOutput.value + text).slice(-80000)
  }

  function closeTerminalSocket() {
    terminalSocket?.close()
    terminalSocket = null
    terminalConnected.value = false
    terminalConnecting.value = false
  }

  async function openTerminal(t: MonitorTarget) {
    if (!auth.isAdmin) {
      ElMessage.warning('只有管理员可以打开终端')
      return
    }
    if (!isSSHHost(t)) {
      ElMessage.warning('目前仅 SSH 主机支持在线终端')
      return
    }
    terminalTarget.value = t
    terminalVisible.value = true
    terminalOutput.value = ''
    terminalInput.value = ''
    await connectTerminal()
  }

  async function connectTerminal() {
    const t = terminalTarget.value
    if (!t) return
    closeTerminalSocket()
    terminalConnecting.value = true
    appendTerminal(`[${nowTime()}] 正在连接 ${t.name} (${targetAddr(t)})...\r\n`)
    try {
      const ticket = (await authApi.wsTicket()).ticket
      const ws = new WebSocket(terminalURL(t.id, ticket))
      terminalSocket = ws
      ws.onopen = () => {
        terminalConnected.value = true
        terminalConnecting.value = false
        appendTerminal(`[${nowTime()}] 已连接，输入命令后回车执行。\r\n\r\n`)
      }
      ws.onmessage = (ev) => appendTerminal(String(ev.data || ''))
      ws.onerror = () => {
        appendTerminal(`\r\n[${nowTime()}] 终端连接错误\r\n`)
      }
      ws.onclose = () => {
        terminalConnected.value = false
        terminalConnecting.value = false
        appendTerminal(`\r\n[${nowTime()}] 连接已关闭\r\n`)
      }
    } catch (e: unknown) {
      terminalConnecting.value = false
      appendTerminal(`[${nowTime()}] 连接失败：${(e as Error).message || '未知错误'}\r\n`)
    }
  }

  function sendTerminalLine() {
    if (!terminalSocket || terminalSocket.readyState !== WebSocket.OPEN) {
      ElMessage.warning('终端未连接')
      return
    }
    terminalSocket.send(`${terminalInput.value}\r`)
    terminalInput.value = ''
  }

  function sendTerminalCtrlC() {
    if (terminalSocket?.readyState === WebSocket.OPEN) terminalSocket.send('\x03')
  }

  return {
    terminalVisible,
    terminalTarget,
    terminalOutput,
    terminalInput,
    terminalConnected,
    terminalConnecting,
    openTerminal,
    closeTerminalSocket,
    connectTerminal,
    sendTerminalLine,
    sendTerminalCtrlC,
  }
}
