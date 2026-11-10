// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { ref, shallowRef, readonly } from 'vue'
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
// shallowRef：每帧整体替换快照，避免 ref 深层代理把上千个 torrent 对象逐个包成 Proxy（每帧开销）。
// 全仓无 torrents/notices 的原地写入（已 grep 验证），故浅层响应式安全。
const torrents = shallowRef<TorrentSnapshot[]>([])
const notices = shallowRef<Notice[]>([]) // 通知中心：WS 实时推送的最新通知（前置，最新在前）
// 长任务进度（W4 任务中心）：kind → 最新进度帧。finished 帧短暂保留后清除（顶栏指示器消费）。
export interface TaskProgress {
  kind: string
  done: number
  total: number
  site?: string
  finished: boolean
}
const taskProgress = shallowRef<Record<string, TaskProgress>>({})
const taskTimers: Record<string, ReturnType<typeof setTimeout>> = {}

let ws: WebSocket | null = null
let retry = 0
let stopped = false
let reconnectTimer: ReturnType<typeof setTimeout> | null = null

// 增量协议状态（?proto=inc 协商成功后用）。
let lastSeq = 0 // 最近一次 torrents_full/diff 的 seq
let dirty = false // 丢帧标记：true 时暂停 apply，等下个 torrents_full 重同步

// ── 帧合并（阶段 1）：同一 rAF 内的多帧 WS 消息只触发一轮响应式更新 ──
// 高频推送时避免「一帧消息 → 一次组件树重算」。队列按序执行，
// 故 applyDiff 读到的始终是上一帧 apply 之后的最新快照（不会丢增量）。
const frameQueue: Array<() => void> = []
let rafId: number | null = null
const raf: (cb: FrameRequestCallback) => number =
  typeof requestAnimationFrame === 'function'
    ? requestAnimationFrame
    : (cb) => setTimeout(() => cb(performance.now()), 16) as unknown as number
const cancelRaf: (id: number) => void =
  typeof cancelAnimationFrame === 'function'
    ? cancelAnimationFrame
    : (id) => clearTimeout(id)

function scheduleApply(apply: () => void) {
  frameQueue.push(apply)
  if (rafId === null) rafId = raf(flushFrames)
}

function flushFrames() {
  rafId = null
  // 快照后清空：flush 期间新入队的更新留到下一帧，避免长任务越积越长
  const batch = frameQueue.splice(0, frameQueue.length)
  for (const apply of batch) apply()
}
function clearState() {
  frameQueue.length = 0
  if (rafId !== null) {
    cancelRaf(rafId)
    rafId = null
  }
  hrAlerts.value = []
  cookieAlerts.value = []
  torrents.value = []
  notices.value = []
  taskProgress.value = {}
  connected.value = false
  lastSeq = 0
  dirty = false
}

// applyDiff 把一轮增量合并到当前快照，返回新数组（触发 Vue 响应式）。纯函数，便于单测。
export function applyDiff(
  cur: TorrentSnapshot[],
  diff: TorrentDiff,
): TorrentSnapshot[] {
  // 只克隆本轮有变更的下载器快照，未命中的复用原引用（避免每轮 WS 推送全量浅拷贝 + torrents 重建）。
  // cur 已在 onMessage 的 torrents/torrents_full 路径归一过 torrents（null → []），
  // 此处无需再处理。
  // added 路径仍需归一：diff.added 来自后端 torrents_diff 帧，出错下载器的 torrents 可能为 null。
  const byId = new Map<number, TorrentSnapshot>()
  for (const s of cur) byId.set(s.downloader_id, s)
  for (const id of diff.removed ?? []) byId.delete(id)
  for (const s of diff.added ?? [])
    byId.set(s.downloader_id, { ...s, torrents: [...(s.torrents ?? [])] })
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
    const j = JSON.parse(raw.data as string) as {
      type?: string; seq?: number; data?: unknown
    }
    const list = Array.isArray(j.data) ? j.data : []
    switch (j.type) {
      case 'torrents': // legacy 全量（未协商增量的旧前端，或后端回落）
        scheduleApply(() => {
          torrents.value = (list as TorrentSnapshot[]).map(
            (s) => ({ ...s, torrents: s.torrents ?? [] }),
          )
        })
        lastSeq = 0
        dirty = false
        break
      case 'torrents_full': // inc 首帧 / 周期重同步
        scheduleApply(() => {
          torrents.value = (list as TorrentSnapshot[]).map(
            (s) => ({ ...s, torrents: s.torrents ?? [] }),
          )
        })
        lastSeq = j.seq ?? 0
        dirty = false
        break
      case 'torrents_diff': {
        // inc 稳态增量
        if (dirty) break // 丢帧中：等下个 full 重同步
        const seq = j.seq ?? 0
        if (seq !== lastSeq + 1) {
          dirty = true
          requestResync()
          break
        }
        lastSeq = seq
        const diff = (j.data ?? {}) as TorrentDiff
        scheduleApply(() => {
          if (dirty) return // flush 时已被标记丢帧：跳过本轮增量
          torrents.value = applyDiff(torrents.value, diff)
        })
        break
      }
      case 'hr-alert':
        scheduleApply(() => { hrAlerts.value = list as HRAlert[] })
        break
      case 'cookie-alert':
        scheduleApply(() => { cookieAlerts.value = list as string[] })
        break
      case 'notice':
        if (j.data) {
          const n = j.data as Notice
          scheduleApply(() => {
            notices.value = [n, ...notices.value].slice(0, 200)
          })
        }
        break
      case 'task-progress': {
        const tp = j.data as TaskProgress
        if (!tp?.kind) break
        scheduleApply(() => {
          taskProgress.value = { ...taskProgress.value, [tp.kind]: tp }
        })
        if (taskTimers[tp.kind]) clearTimeout(taskTimers[tp.kind])
        if (tp.finished) {
          taskTimers[tp.kind] = setTimeout(() => {
            const next = { ...taskProgress.value }
            delete next[tp.kind]
            scheduleApply(() => { taskProgress.value = next })
          }, 2500)
        }
        break
      }
    }
  } catch {
    // 忽略畸形帧
  }
}

// requestResync 丢帧后的主动重同步：断开当前连接并立即重连（跳过退避）——
// Serve 会推携带当前 seq 的 torrents_full 首帧，比等 30 帧周期 full 快一个数量级。
// 防抖：1s 内多次丢帧只触发一次（丢帧风暴时不疯狂断连）。
let resyncTimer: ReturnType<typeof setTimeout> | null = null
function requestResync() {
  if (resyncTimer || !ws) return
  resyncTimer = setTimeout(() => {
    resyncTimer = null
    retry = 0 // 跳过指数退避：这是主动重同步而非网络故障
    ws?.close()
  }, 1000)
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
  // 断线即弃增量状态：重连后 Serve 会推 torrents_full 首帧重新对齐 seq，
  // 若沿用旧 lastSeq/dirty，重连首帧到达前的窗口里 diff 判定会用过期基准。
  lastSeq = 0
  dirty = false
  // 会话已失效（Bearer 过期/被登出）时不要无限重连：ws-ticket 换不到新 ticket，
  // 每 3s 打一轮 401 只会刷日志并拖慢 http 拦截器里的会话跳转；http 拦截器已对
  // 401/403 统一清态跳登录，这里停止重连即可。其余关闭（网络抖动/服务器重启）照常退避。
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
  } catch (e: unknown) {
    // 会话失效（401/403 VIP）：换不到 ticket 就别进入重连循环——
    // http 拦截器已对 401/403 触发 expireSession() 跳登录，这里停止即可。
    const status = (e as { response?: { status?: number } })?.response?.status
    if (status === 401 || status === 403) {
      stopped = true
      return
    }
    scheduleReconnect()
    return
  }
  // await 期间若已 disconnect（stopped 被置 true），放弃本次连接，避免登出后又连上。
  if (stopped) return
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  const q = encodeURIComponent(ticket)
  const url = `${proto}://${location.host}/api/v1/ws?ticket=${q}&proto=inc`
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
  // 清掉所有进度指示器的保留定时器，避免 clearState 后定时器仍触发改写 taskProgress。
  Object.values(taskTimers).forEach(clearTimeout)
  for (const k of Object.keys(taskTimers)) delete taskTimers[k]
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
    taskProgress: readonly(taskProgress),
    /** 前端本地长任务（如 dashboard 逐站循环）镜像进度进来：与后端 WS 帧同构。 */
    mirrorTaskProgress(tp: TaskProgress) {
      scheduleApply(() => {
        taskProgress.value = { ...taskProgress.value, [tp.kind]: tp }
      })
      if (taskTimers[tp.kind]) clearTimeout(taskTimers[tp.kind])
      if (tp.finished) {
        taskTimers[tp.kind] = setTimeout(() => {
          const next = { ...taskProgress.value }
          delete next[tp.kind]
          scheduleApply(() => { taskProgress.value = next })
        }, 2500)
      }
    },
    connect,
    disconnect,
  }
}
