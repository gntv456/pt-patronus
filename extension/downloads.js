// 下载任务面板数据层（对标 PT-depiler「下载器任务管理」）：
// 多下载器切换、任务列表（hash/name/progress/state/速度）、暂停/继续/删除、多来源添加。
// 后端已有完整代理端点：GET /downloaders、GET /downloaders/:id/torrents、
// POST /downloaders/:id/{pause,resume,remove}、POST /downloaders/:id/torrents。

export async function listDownloaders(cfg) {
  const base = cfg.server.replace(/\/$/, '')
  const resp = await fetch(`${base}/api/v1/downloaders`, { headers: { 'X-API-Key': cfg.apiKey } })
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
  const body = await resp.json()
  return body.downloaders || []
}

export async function listTorrents(cfg, dlId) {
  const base = cfg.server.replace(/\/$/, '')
  const resp = await fetch(`${base}/api/v1/downloaders/${dlId}/torrents?limit=500`, { headers: { 'X-API-Key': cfg.apiKey } })
  const body = await resp.json().catch(() => ({}))
  if (!resp.ok) throw new Error(body.error || `HTTP ${resp.status}`)
  return { torrents: body.torrents || [], total: body.total || 0, error: body.error || '' }
}

// globalStats 下载器全局统计（含 free_space 剩余空间；协议不暴露时为 0）。
export async function globalStats(cfg, dlId) {
  const base = cfg.server.replace(/\/$/, '')
  const resp = await fetch(`${base}/api/v1/downloaders/${dlId}/stats`, { headers: { 'X-API-Key': cfg.apiKey } })
  const body = await resp.json().catch(() => ({}))
  if (!resp.ok || body.error) throw new Error(body.error || `HTTP ${resp.status}`)
  return body
}

async function op(cfg, dlId, action, payload) {
  const base = cfg.server.replace(/\/$/, '')
  const resp = await fetch(`${base}/api/v1/downloaders/${dlId}/${action}`, {
    method: 'POST',
    headers: { 'X-API-Key': cfg.apiKey, 'Content-Type': 'application/json' },
    body: payload ? JSON.stringify(payload) : undefined,
  })
  const body = await resp.json().catch(() => ({}))
  if (!resp.ok || body.ok === false) throw new Error(body.error || `HTTP ${resp.status}`)
  return body
}

// pause/resume/remove 按后端契约：hash 数组（单任务包装成 [hash]）。
export const pauseTorrent = (cfg, dlId, hash) => op(cfg, dlId, 'pause', { hashes: [hash] })
export const resumeTorrent = (cfg, dlId, hash) => op(cfg, dlId, 'resume', { hashes: [hash] })
export const removeTorrent = (cfg, dlId, hash) => op(cfg, dlId, 'remove', { hashes: [hash] })

// addByUrl 添加下载（种子链接/磁力；服务端按 URL 注册域反查站点凭据取 .torrent）。
export function addByUrl(cfg, dlId, url) {
  return op(cfg, dlId, 'torrents', { url, tags: ['ptpatronus-extension'] })
}

// findReseed 跨站找同种（辅种最小版）：POST /keepseed/find {name,size}。
// 服务端跨站搜索 + 评分（名称/大小/TMDB/IMDb 指纹），score≥60 的候选即「同种」。
export async function findReseed(cfg, name, size = 0) {
  const base = cfg.server.replace(/\/$/, '')
  const resp = await fetch(`${base}/api/v1/keepseed/find`, {
    method: 'POST',
    headers: { 'X-API-Key': cfg.apiKey, 'Content-Type': 'application/json' },
    body: JSON.stringify({ name: String(name).slice(0, 512), size }),
  })
  const body = await resp.json().catch(() => ({}))
  if (!resp.ok || body.error) throw new Error(body.error || `HTTP ${resp.status}`)
  return body.candidates || []
}

export function fmtBytes(n) {
  if (!n || n <= 0) return '0'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
  return `${v >= 100 ? Math.round(v) : v.toFixed(1)} ${units[i]}`
}

export function fmtSpeed(n) {
  return n > 0 ? `${fmtBytes(n)}/s` : '—'
}
