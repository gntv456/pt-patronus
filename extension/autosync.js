// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Cookie 自动同步引擎（CookieCloud 协议自动推送，闭环本站生态）：
//   浏览器启动 + 每 N 小时（chrome.alarms）跑一轮 syncNow：
//   1. → 上行：浏览器侧指纹 ≠ 服务器侧指纹的站点，把浏览器 cookie 推给服务器
//      （你刚在浏览器登录过的站自动被服务器接管保活；服务器每小时 KeepAlive 会
//      刷新 session 并回写轮换 cookie，从此不依赖浏览器开着）
//   2. ← 下行：浏览器侧已无 cookie、服务器侧仍有效的站点，把服务器 cookie 写回
//      浏览器（需要管理员登录；非管理员静默跳过）——浏览器侧失效自动复活
//   3. 兜底通知：两侧都无 cookie 的站点提醒重新登录（当天去重，防轰炸）
// 同步范围只限服务器已录入的站点名单（ListActive 口径），不做全浏览器 cookie 收集。
import { getConfig, api } from './api.js'
import { serverCookies, browserCookieHeader, pushBrowserToServer, pullServerToBrowser, registeredDomain } from './cookies.js'

export const ALARM_NAME = 'ptp-cookie-autosync'
const STATE_KEY = 'autosync' // chrome.storage.local 里的开关与最近结果
const DEFAULT_STATE = { enabled: false, intervalHours: 4, lastRun: 0, lastResult: null }

export async function getAutosyncState() {
  return { ...DEFAULT_STATE, ...(await chrome.storage.local.get(STATE_KEY))[STATE_KEY] }
}

export async function setAutosyncState(patch) {
  const cur = await getAutosyncState()
  const next = { ...cur, ...patch }
  await chrome.storage.local.set({ [STATE_KEY]: next })
  await syncAlarm(next)
  return next
}

// syncAlarm 按开关注册/清除定时器（periodInMinutes 最小 1，业务用小时级）。
// 闹钟操作全程容错：alarms 异常（权限/平台限制）只记日志，绝不影响同步主流程——
// 状态落库（setAutosyncState）是同步链路的一环，它抛错会顶掉整个同步结果。
export async function syncAlarm(state) {
  try {
    const st = state || (await getAutosyncState())
    if (st.enabled) {
      const minutes = Math.max(1, Math.round((st.intervalHours || 4) * 60))
      await chrome.alarms.create(ALARM_NAME, { periodInMinutes: minutes, delayInMinutes: minutes })
    } else {
      await chrome.alarms.clear(ALARM_NAME)
    }
  } catch (e) {
    console.warn('[ptp] autosync alarm op failed:', e && e.message)
  }
}

// browserFingerprint 与后端同口径：cookie 头 sha256 前 12 位 hex。
export async function browserFingerprint(baseURL) {
  const header = await browserCookieHeader(baseURL)
  if (!header) return { header: '', fp: '' }
  const buf = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(header))
  const fp = [...new Uint8Array(buf)].slice(0, 6).map((b) => b.toString(16).padStart(2, '0')).join('')
  return { header, fp }
}

const todayKey = () => new Date().toISOString().slice(0, 10)

// defaultDeps 是同步动作的真实出口（指纹/上行/下行/明细）。
// 抽成可注入的 deps：逐站循环必须能被测试打到（否则「一站抛错会不会带走整批」只能靠读代码相信）。
const defaultDeps = {
  fingerprint: browserFingerprint,
  push: pushBrowserToServer,
  pull: pullServerToBrowser,
  detailed: (cfg) => serverCookies(cfg, true),
}

// syncOneSite 处理单个站点，返回这一条的终态。
// 关键：错误在**这一条**的范围内收住并回 action='failed'，不往外抛——
// 往外抛会让 for 循环整批中断（曾经的形状：第 3 站被拒 ⇒ 第 4 站往后这一轮全不处理，
// 而摘要只留一句笼统 error，用户既不知道是谁、也不知道少了哪些站）。
export async function syncOneSite(s, cfg, isAdmin, deps) {
  const name = s.name || s.base_url || String(s.id)
  try {
    const { header, fp } = await deps.fingerprint(s.base_url)
    if (fp && fp !== s.fingerprint) {
      await deps.push(cfg, s, header)
      return { action: 'push', name }
    }
    if (!fp && (s.has_cookie || s.stale)) {
      if (isAdmin && s.has_cookie) {
        const list = await deps.detailed(cfg)
        const detailed = list.find((x) => x.id === s.id)
        if (detailed && detailed.cookie) {
          const n = await deps.pull(s, detailed.cookie)
          if (n > 0) return { action: 'pull', name }
        }
      }
      // 非管理员静默跳过下行（reveal 需 admin；台账页有可视化指引）。
      return { action: 'skip', name }
    }
    if (!fp && !s.has_cookie && !s.stale) return { action: 'dead', name }
    return { action: 'skip', name }
  } catch (e) {
    return { action: 'failed', name, error: (e && e.message) || String(e) }
  }
}

// syncPass 跑一遍站点名单，返回各项账面。单站失败只进 failed，不影响后续站点。
export async function syncPass(sites, cfg, isAdmin, deps) {
  const d = deps || defaultDeps
  const out = { pushed: 0, pulled: 0, pushedNames: [], pulledNames: [], dead: [], failed: [] }
  for (const s of sites) {
    if (!s.is_active || !s.base_url) continue
    const r = await syncOneSite(s, cfg, isAdmin, d)
    if (r.action === 'push') {
      out.pushed++
      out.pushedNames.push(r.name)
    } else if (r.action === 'pull') {
      out.pulled++
      out.pulledNames.push(r.name)
    } else if (r.action === 'dead') {
      out.dead.push(r.name)
    } else if (r.action === 'failed') {
      out.failed.push({ name: r.name, error: r.error })
    }
  }
  return out
}

// notifyDead 双侧皆空的提醒（storage.local 记当天已提醒站点，去重）。
async function notifyDead(names) {
  if (!names.length) return
  const k = 'autosync_notified'
  const { [k]: notified = {} } = await chrome.storage.local.get(k)
  const day = todayKey()
  const fresh = names.filter((n) => !notified[day] || !notified[day].includes(n))
  if (!fresh.length) return
  // 只保留今天的记录（顺带清理历史日期）。
  await chrome.storage.local.set({ [k]: { [day]: [...(notified[day] || []), ...fresh] } })
  chrome.notifications.create({
    type: 'basic',
    iconUrl: chrome.runtime.getURL('icons/icon128.png'),
    title: 'PTPatronus Cookie 同步',
    message: `以下站点浏览器与服务器均无有效 cookie，请重新登录：${fresh.slice(0, 8).join('、')}${fresh.length > 8 ? ` 等 ${fresh.length} 个站` : ''}`,
  })
}

// syncNow 跑一轮双向同步；返回摘要（options 页展示「最近同步」用）。
// 不抛错——自动任务失败只记状态；手动触发（options 页「立即同步」）由调用方决定提示。
export async function syncNow() {
  const result = { at: Date.now(), pushed: 0, pulled: 0, dead: [], failed: [], error: '' }
  try {
    const cfg = await getConfig()
    if (!cfg.server || !cfg.token) {
      result.error = '未登录'
      return result
    }
    const isAdmin = !!(cfg.user && cfg.user.is_admin)
    const sites = await serverCookies(cfg)
    const pass = await syncPass(sites, cfg, isAdmin, defaultDeps)
    result.pushed = pass.pushed
    result.pulled = pass.pulled
    result.dead = pass.dead
    result.failed = pass.failed
    result.detail = { pushed: pass.pushedNames, pulled: pass.pulledNames }
    await notifyDead(deadNames)
  } catch (e) {
    result.error = e.message
  }
  await setAutosyncState({ lastRun: result.at, lastResult: result })
  return result
}

// initAutosync 装配入口（background SW 启动时调用）：恢复闹钟 + 绑定触发器。
export function initAutosync() {
  syncAlarm().catch(() => {})
  chrome.alarms.onAlarm.addListener((alarm) => {
    if (alarm.name === ALARM_NAME) syncNow().catch(() => {})
  })
  chrome.runtime.onStartup.addListener(() => {
    // 浏览器启动即同步一轮（登录过的站第一时间被服务器接管保活）。
    syncNow().catch(() => {})
  })
  // SW 每次被唤醒（消息/事件）时若距上次同步超过间隔也补跑一轮：
  // alarms 在浏览器长期睡眠后可能被合并，这里做兜底。
  chrome.storage.onChanged.addListener((changes, area) => {
    if (area === 'local' && changes.autosync) {
      const next = changes.autosync.newValue || DEFAULT_STATE
      if (next.enabled) syncAlarm(next).catch(() => {})
    }
  })
}

// shouldCatchUpOnWake 供 background 消息处理末尾调用（SW 唤醒兜底路径）。
export async function maybeSyncOnWake() {
  const st = await getAutosyncState()
  if (!st.enabled || !st.lastRun) return
  const elapsed = Date.now() - st.lastRun
  if (elapsed > (st.intervalHours || 4) * 3600 * 1000) {
    syncNow().catch(() => {})
  }
}
