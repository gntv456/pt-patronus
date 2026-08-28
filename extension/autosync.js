// Cookie 自动同步引擎（对标 CookieCloud 扩展的自动推送，闭环本站生态）：
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
// 注意：cookies_page.js 有一份相同实现（fpOf），这里独立导出供自动同步复用。
export async function browserFingerprint(baseURL) {
  const header = await browserCookieHeader(baseURL)
  if (!header) return { header: '', fp: '' }
  const buf = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(header))
  const fp = [...new Uint8Array(buf)].slice(0, 6).map((b) => b.toString(16).padStart(2, '0')).join('')
  return { header, fp }
}

const todayKey = () => new Date().toISOString().slice(0, 10)

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
  const result = { at: Date.now(), pushed: 0, pulled: 0, dead: [], error: '' }
  try {
    const cfg = await getConfig()
    if (!cfg.server || !cfg.token) {
      result.error = '未登录'
      return result
    }
    const isAdmin = !!(cfg.user && cfg.user.is_admin)
    const sites = await serverCookies(cfg)
    const pushedNames = []
    const pulledNames = []
    const deadNames = []
    for (const s of sites) {
      if (!s.is_active || !s.base_url) continue
      const { header, fp } = await browserFingerprint(s.base_url)
      if (fp && fp !== s.fingerprint) {
        // → 上行：浏览器有且与服务器不同（新登录/轮换后首次发现）。
        await pushBrowserToServer(cfg, s, header)
        result.pushed++
        pushedNames.push(s.name)
      } else if (!fp && (s.has_cookie || s.stale)) {
        // ← 下行：浏览器缺失、服务器有（含 stale 密文——交给 reveal 侧判定）。
        if (isAdmin && s.has_cookie) {
          const detailed = (await serverCookies(cfg, true)).find((x) => x.id === s.id)
          if (detailed && detailed.cookie) {
            const n = await pullServerToBrowser(s, detailed.cookie)
            if (n > 0) { result.pulled++; pulledNames.push(s.name) }
          }
        }
        // 非管理员静默跳过下行（reveal 需 admin；台账页有可视化指引）。
      } else if (!fp && !s.has_cookie && !s.stale) {
        deadNames.push(s.name)
      }
    }
    result.dead = deadNames
    result.detail = { pushed: pushedNames, pulled: pulledNames }
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
