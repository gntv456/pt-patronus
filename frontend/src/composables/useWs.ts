import { ref, readonly } from 'vue'
import type { Torrent } from '@/api/downloader'
import type { Notice } from '@/api/notice'
import { authApi } from '@/api/auth'

// 一条 H&R 危险种子（对齐后端 ws.HRAlert / mobile ws_client.dart）。
export interface HRAlert {
  site: string
  title: string
  status: string
  deadline: string
}

// 单下载器实时种子快照（对齐后端 ws.TorrentSnapshot；下载器抽屉订阅渲染）。
export interface TorrentSnapshot {
  downloader_id: number
  name: string
  error: string
  torrents: Torrent[]
  stats?: { dl_rate: number; up_rate: number } // qb 零请求取；tr 留空
}

// 模块级单例：同一时刻一个 WS 连接服务所有订阅者（对齐 mobile WsClient 单例）。
const connected = ref(false)
const hrAlerts = ref<HRAlert[]>([])
const cookieAlerts = ref<string[]>([])
const torrents = ref<TorrentSnapshot[]>([])
const notices = ref<Notice[]>([]) // 通知中心：WS 实时推送的最新通知（前置，最新在前）

let ws: WebSocket | null = null
let retry = 0
let stopped = false
let reconnectTimer: ReturnType<typeof setTimeout> | null = null

function clearState() {
  hrAlerts.value = []
  cookieAlerts.value = []
  torrents.value = []
  notices.value = []
  connected.value = false
}

function onMessage(raw: MessageEvent) {
  try {
    const j = JSON.parse(raw.data as string) as { type?: string; data?: unknown }
    const list = Array.isArray(j.data) ? j.data : []
    switch (j.type) {
      case 'torrents':
        torrents.value = list as TorrentSnapshot[]
        break
      case 'hr-alert':
        hrAlerts.value = list as HRAlert[]
        break
      case 'cookie-alert':
        cookieAlerts.value = list as string[]
        break
      case 'notice':
        if (j.data) notices.value = [j.data as Notice, ...notices.value].slice(0, 200)
        break
    }
  } catch {
    // 忽略畸形帧
  }
}

function scheduleReconnect() {
  if (stopped) return
  retry++
  const secs = retry > 6 ? 60 : retry * 2 // 指数退避：2,4,6,8,10,12…封顶 60s（与移动端一致）
  reconnectTimer = setTimeout(() => connect(), secs * 1000)
}

function onClose() {
  ws = null
  connected.value = false
  scheduleReconnect()
}

/** 连接（幂等：已连则跳过）。先用 header JWT 换一次性 ticket，再用 ?ticket= 连（JWT 不落 URL）。 */
async function connect() {
  // 主动调用 connect 视为开启新会话：清除上一次 disconnect 残留的 stopped=true，
  // 否则登出再登录后 stopped 永远为 true，WS 再也不连（通知 / H&R / Cookie 告警停推）。
  stopped = false
  if (ws) return // 已连，幂等
  let ticket: string
  try {
    ticket = (await authApi.wsTicket()).ticket
  } catch {
    scheduleReconnect()
    return
  }
  // await 期间若已 disconnect（stopped 被置 true），放弃本次连接，避免登出后又连上。
  if (stopped) return
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  const url = `${proto}://${location.host}/api/v1/ws?ticket=${encodeURIComponent(ticket)}`
  try {
    ws = new WebSocket(url)
    retry = 0
    ws.onopen = () => {
      connected.value = true
    }
    ws.onmessage = onMessage
    ws.onclose = onClose
    ws.onerror = () => {
      // 视同 close：交给 onclose 走重连
      ws?.close()
    }
  } catch {
    ws = null
    scheduleReconnect()
  }
}

/** 关闭并禁用重连（登出/离开鉴权外壳时调）。 */
function disconnect() {
  stopped = true
  if (reconnectTimer) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  if (ws) {
    ws.onclose = null // 阻止 onClose 触发重连
    ws.close()
    ws = null
  }
  clearState()
}

/** 在鉴权外壳挂载后调用，取响应式告警/连接状态。 */
export function useWs() {
  return {
    connected: readonly(connected),
    hrAlerts: readonly(hrAlerts),
    cookieAlerts: readonly(cookieAlerts),
    torrents: readonly(torrents),
    notices: readonly(notices),
    connect,
    disconnect,
  }
}
