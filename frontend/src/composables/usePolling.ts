// usePolling — 统一轮询调度器（阶段 1）
//
// 替代散落在组件里的 15 处 setInterval：
//   - 单一 1s tick 驱动所有任务，避免 N 个独立定时器互相错开造成持续唤醒；
//   - 页面隐藏时整体暂停（不推进计数，恢复后不补积压）；
//   - 可选 runOnShow：切回页面时立即补一次；
//   - usePolling() 在组件卸载时自动注销，registerPolling() 供手动生命周期使用。
import { onUnmounted } from 'vue'

const TICK_MS = 1000

type PollTask = { fn: () => void; every: number; nextAt: number; runOnShow: boolean }

const tasks = new Map<number, PollTask>()
let timer: ReturnType<typeof setInterval> | null = null
let nextId = 1
let tickCount = 0
let visibilityBound = false
// 只有真实收到 visibilitychange 才暂停。jsdom / 测试环境的 document.hidden 不可靠，
// 直接在 tick 里读 document.hidden 会把所有轮询误停（见阶段 1 验证事故）。
let paused = false

function tick() {
  if (paused) return // 隐藏时整体暂停，且不推进计数
  tickCount++
  for (const task of tasks.values()) {
    if (tickCount >= task.nextAt) {
      task.nextAt = tickCount + task.every
      try { task.fn() } catch (e) { console.error('[usePolling] task failed', e) }
    }
  }
}

function onVisibilityChange() {
  if (typeof document === 'undefined') return
  paused = document.hidden
  if (paused) return
  for (const task of tasks.values()) {
    if (task.runOnShow) {
      try { task.fn() } catch (e) { console.error('[usePolling] on-show task failed', e) }
    }
  }
}

function ensureTimer() {
  if (timer !== null) return
  timer = setInterval(tick, TICK_MS)
  if (!visibilityBound && typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', onVisibilityChange)
    visibilityBound = true
  }
}

function maybeStopTimer() {
  if (tasks.size === 0 && timer !== null) {
    clearInterval(timer)
    timer = null
  }
}

export interface PollingOptions {
  /** 注册时立即执行一次（替代调用方手写的首次 refresh()） */
  immediate?: boolean
  /** 页面从隐藏切回时立即补一次 */
  runOnShow?: boolean
}

/** 注册一个轮询任务，返回注销函数。intervalMs 会被取整到 1s 粒度。 */
export function registerPolling(fn: () => void, intervalMs: number, options: PollingOptions = {}): () => void {
  const every = Math.max(1, Math.round(intervalMs / TICK_MS))
  const id = nextId++
  tasks.set(id, { fn, every, nextAt: tickCount + every, runOnShow: options.runOnShow ?? false })
  ensureTimer()
  if (options.immediate) fn()
  let stopped = false
  return () => {
    if (stopped) return
    stopped = true
    tasks.delete(id)
    maybeStopTimer()
  }
}

/** 组件内使用：卸载自动注销。 */
export function usePolling(fn: () => void, intervalMs: number, options: PollingOptions = {}) {
  const stop = registerPolling(fn, intervalMs, options)
  onUnmounted(stop)
  return stop
}

/** 调试用：当前活跃任务数 / 定时器是否运行。 */
export function pollingStats() { return { tasks: tasks.size, running: timer !== null } }