// PTPatronus 助手 —— 配置与 API 封装（popup/options/background 共用）。
// 鉴权：好学账号登录（POST /auth/hxpt/cookie-login）换 JWT，之后所有请求带
// Authorization: Bearer；令牌过期（401）时优先 POST /auth/refresh 滑动续期
// （不再打扰好学站），refresh 也失败才退回好学 cookie 兑换。
// cookie 与 token 只存 chrome.storage.local（本机），不进 sync。
// hxptCreds（账号密码）经 secrets.js 密封存储（PBKDF2+AES-GCM）——
// 本机任何能读 profile 的进程不再能直读明文密码（安全审计 P2）。
import { baseUrl } from './util.js'
import { readSecret, writeSecret, migratePlaintextSecrets } from './secrets.js'

const DEFAULTS = { server: '', downloaderId: 0 }
// 会话字段（local）：hxptCookie=好学登录 cookie；token=JWT；user=登录用户（含 is_admin）；
// hxptCreds=账号密码登录的凭据（密封存储，供续期失效时静默重登）。
const SESSION_DEFAULTS = { hxptCookie: '', token: '', user: null, hxptCreds: null }

// 启动迁移：把旧版明文 hxptCreds 密封（失败静默——不阻断正常流程）。
migratePlaintextSecrets().catch(() => {})

export async function getConfig() {
  const cfg = await chrome.storage.sync.get(DEFAULTS)
  const sess = await chrome.storage.local.get(SESSION_DEFAULTS)
  // hxptCreds 可能是密封形态 {salt,iv,data}：解封后供 isConfigured/renew 使用。
  let creds = sess.hxptCreds
  if (creds && creds.salt && creds.iv && creds.data) {
    const r = await readSecret('hxptCreds')
    creds = r.ok ? r.value : null
  }
  return { ...cfg, ...sess, hxptCreds: creds }
}

export async function saveConfig(patch) {
  const { server, downloaderId } = patch
  await chrome.storage.sync.set({ ...(server !== undefined ? { server } : {}), ...(downloaderId !== undefined ? { downloaderId } : {}) })
}

// 已登录判定：有 token 或有可用重登凭据（cookie/账号密码，首请求时会自动换 token）。
export function isConfigured(cfg) {
  return !!(cfg && cfg.server && (cfg.token || cfg.hxptCookie || (cfg.hxptCreds && cfg.hxptCreds.username)))
}

// saveSession / clearSession 登录态写入/登出清除（好学 cookie 与 JWT 同生命周期）。
// hxptCreds 走密封存储（secrets.js）；其余字段明文本地存储（token 短时效、cookie 同会话）。
export async function saveSession({ hxptCookie, token, user, hxptCreds }) {
  const patch = {}
  if (hxptCookie !== undefined) patch.hxptCookie = hxptCookie
  if (token !== undefined) patch.token = token
  if (user !== undefined) patch.user = user
  if (Object.keys(patch).length) await chrome.storage.local.set(patch)
  if (hxptCreds !== undefined) {
    if (hxptCreds === null) await chrome.storage.local.remove('hxptCreds')
    else await writeSecret('hxptCreds', hxptCreds)
  }
}

export async function clearSession() {
  await chrome.storage.local.remove(['hxptCookie', 'token', 'user', 'hxptCreds'])
}

export { baseUrl }

// fetchJson 带网络错误语义化：Failed to fetch → 服务器不可达排查提示。
async function fetchJson(url, options) {
  let resp
  try {
    resp = await fetch(url, options)
  } catch {
    throw new Error('服务器不可达：检查地址/端口是否正确、后端是否启动、防火墙是否放行')
  }
  const body = await resp.json().catch(() => ({}))
  return { resp, body }
}

// localLogin 本地密码登录（好学站点关闭后的主路径，#issue）：POST /auth/local/login
// → {token, user}，并落库会话。凭据存本机供续期失效时静默重登。
export async function localLogin(server, { username, password }) {
  const { resp, body } = await fetchJson(`${baseUrl({ server })}/api/v1/auth/local/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
  if (!resp.ok) {
    const err = new Error(body.message || body.error || `HTTP ${resp.status}`)
    err.status = resp.status
    throw err
  }
  const user = body.user || {}
  await saveSession({ hxptCookie: '', token: body.token, user, hxptCreds: { username, password } })
  return { token: body.token, user }
}

// login 好学 Cookie 登录：POST /auth/hxpt/cookie-login → {token, user}，并落库会话。
export async function login(server, hxptCookie) {
  const { resp, body } = await fetchJson(`${baseUrl({ server })}/api/v1/auth/hxpt/cookie-login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ cookie: hxptCookie }),
  })
  if (!resp.ok) {
    throw new Error(body.message || body.error || `HTTP ${resp.status}`)
  }
  const user = body.user || {}
  await saveSession({ hxptCookie, token: body.token, user, hxptCreds: null })
  return { token: body.token, user }
}

// fetchCaptcha 账号登录 Step A：POST /auth/hxpt/captcha（取验证码图/探测挑战模式）。
export async function fetchCaptcha(server) {
  const { resp, body } = await fetchJson(`${baseUrl({ server })}/api/v1/auth/hxpt/captcha`, { method: 'POST' })
  if (!resp.ok) {
    throw new Error(body.error || body.message || `HTTP ${resp.status}`)
  }
  return body
}

// loginByPassword 好学账号密码登录 Step B：POST /auth/hxpt/login（服务端代理好学验证）。
// challenge 由 Step A 的返回回传（extra_form/challenge_mode 驱动后端挑战分支）。
export async function loginByPassword(server, { username, password, imagehash, imagestring, twoStepCode, extraForm, challengeMode }) {
  const { resp, body } = await fetchJson(`${baseUrl({ server })}/api/v1/auth/hxpt/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      username,
      password,
      imagehash: imagehash || '',
      imagestring: imagestring || '',
      two_step_code: twoStepCode || '',
      extra_form: extraForm || {},
      challenge_mode: !!challengeMode,
    }),
  })
  if (resp.status === 409) {
    throw new Error(body.message || '好学已开启挑战响应认证，请改用 Cookie 登录')
  }
  if (!resp.ok) {
    const err = new Error(body.message || body.error || `HTTP ${resp.status}`)
    err.status = resp.status
    err.needCaptchaRefresh = resp.status === 401
    throw err
  }
  const user = body.user || {}
  // 凭据存本机供续期失效时静默重登（与 Web 端「记住登录」同语义）。
  await saveSession({ hxptCookie: '', token: body.token, user, hxptCreds: { username, password } })
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
  // 2) 兜底重登：优先好学 cookie 兑换；没有 cookie 用账号密码静默重登
  //    （密码登录含验证码状态机，服务端无验证码时可行；失败即要求重新登录）。
  if (sess.hxptCookie) {
    try {
      return await login(server, sess.hxptCookie)
    } catch { /* cookie 也失效，继续看有无账号凭据 */ }
  }
  // hxptCreds 密封存储：解封后使用（readSecret 失败 = 无凭据）。
  let creds = sess.hxptCreds
  if (creds && creds.salt && creds.iv && creds.data) {
    const r = await readSecret('hxptCreds')
    creds = r.ok ? r.value : null
  }
  if (creds && creds.username) {
    try {
      return await loginByPassword(server, creds)
    } catch { /* 密码路径失败（验证码/挑战模式）→ 要求重新登录 */ }
  }
  await clearSession()
  throw new Error('登录已失效，请重新登录')
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
  if (resp.status === 401 && (cfg.token || cfg.hxptCookie || (cfg.hxptCreds && cfg.hxptCreds.username))) {
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
      if (resp.status === 401 && (cfg.token || cfg.hxptCookie || (cfg.hxptCreds && cfg.hxptCreds.username)) && !retried) {
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
