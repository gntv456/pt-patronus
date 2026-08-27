// 站点数据聚合（对标竞品「站点个人数据仪表」）：拉 /api/v1/dashboard 总览，
// 汇总上传/下载/分享率/做种/魔力 + 每站明细（等级/上传/下载/做种体积/魔力时产）。
// POST /dashboard/sync 可即时抓取全部站点（站均 1-3s，全量较慢——面板手动触发）。

export async function overview(cfg) {
  const base = cfg.server.replace(/\/$/, '')
  const resp = await fetch(`${base}/api/v1/dashboard`, { headers: { 'X-API-Key': cfg.apiKey } })
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
  return resp.json()
}

export async function syncAll(cfg) {
  const base = cfg.server.replace(/\/$/, '')
  const resp = await fetch(`${base}/api/v1/dashboard/sync`, { method: 'POST', headers: { 'X-API-Key': cfg.apiKey } })
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
  return resp.json()
}

// fmtBytes 人类可读容量（GB/TB/PB，1 位小数）。
export function fmtBytes(n) {
  if (!n || n <= 0) return '0'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
  return `${v >= 100 ? Math.round(v) : v.toFixed(1)} ${units[i]}`
}

// fmtBonus 魔力值统一 1 位小数（竞品口径）。
export function fmtBonus(n) {
  if (n == null || Number.isNaN(n)) return '—'
  return n >= 10000 ? `${(n / 10000).toFixed(1)}万` : n.toFixed(1)
}

// fmtRatio 分享率：0 保护为 '—'（除零）。
export function fmtRatio(up, down) {
  if (!down) return '∞'
  return (up / down).toFixed(2)
}
