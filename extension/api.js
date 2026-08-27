// PTPatronus 助手 —— 配置与 API 封装（popup/options/background 共用）。
// 鉴权：X-API-Key 头（服务端 API_KEY env；命中即授予管理员身份，与 Servarr 兼容层同源）。

const DEFAULTS = { server: '', apiKey: '' }

export async function getConfig() {
  const cfg = await chrome.storage.sync.get(DEFAULTS)
  return cfg
}

export async function saveConfig(patch) {
  await chrome.storage.sync.set(patch)
}

export function isConfigured(cfg) {
  return !!(cfg && cfg.server && cfg.apiKey)
}

// api 统一请求封装：拼 base URL + API Key 头；非 2xx 抛错（message 取后端 error 字段）。
export async function api(cfg, path, options = {}) {
  const base = cfg.server.replace(/\/$/, '')
  const headers = { 'X-API-Key': cfg.apiKey, 'Content-Type': 'application/json', ...(options.headers || {}) }
  const resp = await fetch(base + path, { ...options, headers })
  if (!resp.ok) {
    let msg = `HTTP ${resp.status}`
    try {
      const body = await resp.json()
      if (body && body.error) msg = body.error
    } catch { /* 非 JSON 错误体 */ }
    throw new Error(msg)
  }
  return resp.json()
}

// 跨站搜索种子：GET /api/v1/search?keyword=&category=
export async function searchTorrents(cfg, keyword, category = '') {
  const q = new URLSearchParams({ keyword })
  if (category) q.set('category', category)
  const data = await api(cfg, `/api/v1/search?${q}`)
  return data.results || []
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
  const base = cfg.server.replace(/\/$/, '')
  const url = keyword ? `${base}/search?keyword=${encodeURIComponent(keyword)}` : `${base}/search`
  chrome.tabs.create({ url })
}
