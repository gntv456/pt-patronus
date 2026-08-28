// PTPatronus 助手 —— 配置与 API 封装（popup/options/background 共用）。
// 鉴权：好学账号登录（POST /auth/hxpt/cookie-login）换 JWT，之后所有请求带
// Authorization: Bearer；令牌过期（401）时优先 POST /auth/refresh 滑动续期
// （不再打扰好学站），refresh 也失败才退回好学 cookie 兑换。
// cookie 与 token 只存 chrome.storage.local（本机），不进 sync。
import { baseUrl } from './util.js'

const DEFAULTS = { server: '', downloaderId: 0 }
// 会话字段（local）：hxptCookie=好学登录 cookie；token=JWT；user=登录用户（含 is_admin）。
const SESSION_DEFAULTS = { hxptCookie: '', token: '', user: null }

export async function getConfig() {
  const cfg = await chrome.storage.sync.get(DEFAULTS)
  const sess = await chrome.storage.local.get(SESSION_DEFAULTS)
  return { ...cfg, ...sess }
}

export async function saveConfig(patch) {
  const { server, downloaderId } = patch
  await chrome.storage.sync.set({ ...(server !== undefined ? { server } : {}), ...(downloaderId !== undefined ? { downloaderId } : {}) })
}

// 已登录判定：有 token 或有 cookie（后者首请求时会自动换 token）。
export function isConfigured(cfg) {
  return !!(cfg && cfg.server && (cfg.token || cfg.hxptCookie))
}

// saveSession / clearSession 登录态写入/登出清除（好学 cookie 与 JWT 同生命周期）。
export async function saveSession({ hxptCookie, token, user }) {
  const patch = {}
  if (hxptCookie !== undefined) patch.hxptCookie = hxptCookie
  if (token !== undefined) patch.token = token
  if (user !== undefined) patch.user = user
  await chrome.storage.local.set(patch)
}

export async function clearSession() {
  await chrome.storage.local.remove(['hxptCookie', 'token', 'user'])
}

export { baseUrl }

// login 好学 Cookie 登录：POST /auth/hxpt/cookie-login → {token, user}，并落库会话。
export async function login(server, hxptCookie) {
  const resp = await fetch(`${baseUrl({ server })}/api/v1/auth/hxpt/cookie-login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ cookie: hxptCookie }),
  })
  const body = await resp.json().catch(() => ({}))
  if (!resp.ok) {
    throw new Error(body.message || body.error || `HTTP ${resp.status}`)
  }
  const user = body.user || {}
  await saveSession({ hxptCookie, token: body.token, user })
  return { token: body.token, user }
}

// renew 续期（401 时）：先 refresh（服务端滑动续期，不碰好学）；refresh 失败
// （token 被吊销/登出/过期太久）再退回好学 cookie 重新兑换。两条路都失败时
// 清空会话并抛「请重新登录」语义错误，上层引导用户回选项页。
async function renew(server) {
  const sess = await chrome.storage.local.get(SESSION_DEFAULTS)
  // 1) refresh：仍有有效 token 时最稳，不向好学发任何请求。
  if (sess.token) {
    try {
      const resp = await fetch(`${baseUrl({ server })}/api/v1/auth/refresh`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${sess.token}` },
      })
      if (resp.ok) {
        const body = await resp.json()
        await saveSession({ token: body.token, user: body.user })
        return { token: body.token, user: body.user }
      }
    } catch { /* 网络错误走 cookie 兜底 */ }
  }
  // 2) 好学 cookie 兑换（refresh 失败：token 已过期/吊销）。
  if (!sess.hxptCookie) {
    await clearSession()
    throw new Error('登录已失效，请重新登录')
  }
  try {
    return await login(server, sess.hxptCookie)
  } catch (e) {
    await clearSession()
    throw new Error('登录已失效，请重新登录')
  }
}

// rawFetch 单次请求（Bearer 头）；返回原始 Response。
function rawFetch(cfg, path, options = {}) {
  const headers = {
    ...(cfg.token ? { Authorization: `Bearer ${cfg.token}` } : {}),
    'Content-Type': 'application/json',
    ...(options.headers || {}),
  }
  return fetch(baseUrl(cfg) + path, { ...options, headers })
}

// api 统一请求封装：拼 base URL + Bearer 头；非 2xx 抛错（message 取后端 error 字段）。
// 401 时自动续期一次（refresh → cookie 兑换）再重放；全失败则抛「请重新登录」。
export async function api(cfg, path, options = {}) {
  let resp = await rawFetch(cfg, path, options)
  if (resp.status === 401 && (cfg.token || cfg.hxptCookie)) {
    const renewed = await renew(cfg.server)
    resp = await rawFetch({ ...cfg, token: renewed.token }, path, options)
  }
  if (!resp.ok) {
    let msg = `HTTP ${resp.status}`
    try {
      const body = await resp.json()
      if (body && (body.error || body.message)) msg = body.message || body.error
    } catch { /* 非 JSON 错误体 */ }
    throw new Error(msg)
  }
  return resp.json()
}

// 跨站搜索种子：POST /api/v1/search?keyword=&category=（后端为 POST，避免关键词长时 URL 超限）。
// 返回按站聚合的 [{site_name, site_id, torrents:[…], error}]（失败站也占位，error 载明原因）。
export async function searchTorrents(cfg, keyword, category = '') {
  const q = new URLSearchParams({ keyword })
  if (category) q.set('category', category)
  const data = await api(cfg, `/api/v1/search?${q}`, { method: 'POST' })
  return data.results || []
}

// searchTorrentsStream 流式跨站搜索：GET /api/v1/search/stream（SSE）。
// 每站完成推一帧 event:batch {results:[{site_name, torrents, error}]}，结束推 event:done。
// onBatch 逐站回调（搜出一个显示一个），返回 promise 在 done/错误时 resolve。
// 401 时先续期（refresh → cookie 兑换）再重放一次（SSE 长连接无法中途续，整条重来）。
export function searchTorrentsStream(cfgIn, keyword, onBatch, category = '') {
  return new Promise((resolve) => {
    const q = new URLSearchParams({ keyword })
    if (category) q.set('category', category)
    const run = async (cfg, retried) => {
      let resp
      try {
        resp = await fetch(`${baseUrl(cfg)}/api/v1/search/stream?${q}`, {
          headers: cfg.token ? { Authorization: `Bearer ${cfg.token}` } : {},
        })
      } catch (e) {
        resolve({ error: e.message })
        return
      }
      if (resp.status === 401 && (cfg.token || cfg.hxptCookie) && !retried) {
        try {
          const renewed = await renew(cfg.server)
          await run({ ...cfg, token: renewed.token }, true)
        } catch (e) {
          resolve({ error: e.message })
        }
        return
      }
      if (!resp.ok || !resp.body) {
        resolve({ error: `HTTP ${resp.status}` })
        return
      }
      const reader = resp.body.getReader()
      const dec = new TextDecoder()
      let buf = ''
      let done = false
      while (!done) {
        const { value, done: rDone } = await reader.read()
        if (rDone) break
        buf += dec.decode(value, { stream: true })
        // SSE 帧以空行分隔；每帧形如 "event: xxx\ndata: {json}"。
        let idx
        while ((idx = buf.indexOf('\n\n')) !== -1) {
          const frame = buf.slice(0, idx)
          buf = buf.slice(idx + 2)
          let event = 'message'
          let data = ''
          for (const line of frame.split('\n')) {
            if (line.startsWith('event:')) event = line.slice(6).trim()
            else if (line.startsWith('data:')) data += line.slice(5).trim()
          }
          if (!data) continue
          let parsed
          try { parsed = JSON.parse(data) } catch { continue }
          if (event === 'batch' && parsed.results) {
            for (const g of parsed.results) onBatch(g)
          } else if (event === 'done') {
            done = true
          }
        }
      }
      resolve({})
    }
    run(cfgIn, false).catch((e) => resolve({ error: e.message }))
  })
}

// 影视搜索（找 TMDB/豆瓣条目 → 一键订阅用）：GET /api/v1/media/search?keyword=
export async function searchMedia(cfg, keyword) {
  const data = await api(cfg, `/api/v1/media/search?keyword=${encodeURIComponent(keyword)}&type=video`)
  return data.results || []
}

// 一键添加订阅：POST /api/v1/subscriptions（title/type/source/source_id 来自媒体搜索结果）。
export async function createSubscription(cfg, item) {
  return api(cfg, '/api/v1/subscriptions', {
    method: 'POST',
    body: JSON.stringify({
      title: item.title,
      year: item.year || 0,
      type: item.type,
      source: item.source,
      source_id: item.source_id,
    }),
  })
}

// 打开 Web UI 搜索页（站点快捷入口/查看更多结果）。
export function openWebSearch(cfg, keyword) {
  const base = baseUrl(cfg)
  const url = keyword ? `${base}/search?keyword=${encodeURIComponent(keyword)}` : `${base}/search`
  chrome.tabs.create({ url })
}
