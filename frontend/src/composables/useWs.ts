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

// 单下载器本轮增量（对齐后端 ws.DownloaderDiff）。
export interface DownloaderDiff {
  downloader_id: number
  name?: string // 仅变化时带
  error?: string // 后端始终带（空串=无错），覆盖式
  stats?: { dl_rate: number; up_rate: number }
  added?: Torrent[] // 新 hash
  changed?: Torrent[] // 字段变化（整条替换）
  removed?: string[] // 删除的 hash
}

// 本轮所有下载器增量（对齐后端 ws.TorrentDiff）。
export interface TorrentDiff {
  added?: TorrentSnapshot[] // 新增整个下载器
  removed?: number[] // 删除的 downloader_id
  changed?: DownloaderDiff[]
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

// 增量协议状态（?proto=inc 协商成功后用）。
let lastSeq = 0 // 最近一次 torrents_full/diff 的 seq
let dirty = false // 丢帧标记：true 时暂停 apply，等下个 torrents_full 重同步

function clearState() {
  hrAlerts.value = []
  cookieAlerts.value = []
  torrents.value = []
  notices.value = []
  connected.value = false
  lastSeq = 0
  dirty = false
}

// applyDiff 把一轮增量合并到当前快照，返回新数组（触发 Vue 响应式）。纯函数，便于单测。
export function applyDiff(cur: TorrentSnapshot[], diff: TorrentDiff): TorrentSnapshot[] {
  // 只克隆本轮有变更的下载器快照，未命中的复用原引用（避免每轮 WS 推送全量浅拷贝 + torrents 重建）。
  const byId = new Map<number, TorrentSnapshot>()
  for (const s of cur) byId.set(s.downloader_id, s)
  for (const id of diff.removed ?? []) byId.delete(id)
  for (const s of diff.added ?? []) byId.set(s.downloader_id, { ...s, torrents: [...s.torrents] })
  for (const dd of diff.changed ?? []) {
    const prev = byId.get(dd.downloader_id)
    if (!prev) continue
    // 仅克隆命中的快照（torrents 数组需重建以应用增删改），未变快照保持原引用。
    const s: TorrentSnapshot = { ...prev, torrents: [...prev.torrents] }
    if (dd.name !== undefined) s.name = dd.name
    if ('error' in dd) s.error = dd.error ?? ''
    if (dd.stats !== undefined) s.stats = dd.stats
    const tmap = new Map<string, Torrent>(s.torrents.map((t) => [t.hash, t]))
    for (const h of dd.removed ?? []) tmap.delete(h)
    for (const t of dd.added ?? []) tmap.set(t.hash, t)
    for (const t of dd.changed ?? []) tmap.set(t.hash, t) // 整条替换
    s.torrents = [...tmap.values()]
    byId.set(dd.downloader_id, s)
  }
  return [...byId.values()]
}

function onMessage(raw: MessageEvent) {
  try {
    const j = JSON.parse(raw.data as string) as { type?: string; seq?: number; data?: unknown }
    const list = Array.isArray(j.data) ? j.data : []
    switch (j.type) {
      case 'torrents': // legacy 全量（未协商增量的旧前端，或后端回落）
        torrents.value = list as TorrentSnapshot[]
        lastSeq = 0
        dirty = false
        break
      case 'torrents_full': // inc 首帧 / 周期重同步
        torrents.value = list as TorrentSnapshot[]
        lastSeq = j.seq ?? 0
        dirty = false
        break
      case 'torrents_diff': { // inc 稳态增量
        if (dirty) break // 丢帧中：等下个 full 重同步
        const seq = j.seq ?? 0
        if (seq !== lastSeq + 1) {
          dirty = true // 不连号=丢帧
          break
        }
        lastSeq = seq
        torrents.value = applyDiff(torrents.value, (j.data ?? {}) as TorrentDiff)
        break
      }
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
  const url = `${proto}://${location.host}/api/v1/ws?ticket=${encodeURIComponent(ticket)}&proto=inc`
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
