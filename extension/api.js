// PTPatronus 助手 —— 配置与 API 封装（popup/options/background 共用）。
// 鉴权：X-API-Key 头（服务端 API_KEY env；命中即授予管理员身份，与 Servarr 兼容层同源）。
import { baseUrl } from './util.js'

const DEFAULTS = { server: '', apiKey: '', downloaderId: 0 }

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

export { baseUrl }

// api 统一请求封装：拼 base URL + API Key 头；非 2xx 抛错（message 取后端 error 字段）。
export async function api(cfg, path, options = {}) {
  const base = baseUrl(cfg)
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
export function searchTorrentsStream(cfg, keyword, onBatch, category = '') {
  return new Promise((resolve) => {
    const q = new URLSearchParams({ keyword })
    if (category) q.set('category', category)
    const base = baseUrl(cfg)
    // fetch + ReadableStream 解析 SSE（EventSource 不支持自定义 X-API-Key 头）。
    fetch(`${base}/api/v1/search/stream?${q}`, { headers: { 'X-API-Key': cfg.apiKey } }).then(async (resp) => {
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
    }).catch((e) => resolve({ error: e.message }))
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
