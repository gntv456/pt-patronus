// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 站点数据聚合（对标竞品「站点个人数据仪表」）：拉 /api/v1/dashboard 总览，
// 汇总上传/下载/分享率/做种/魔力 + 每站明细（等级/上传/下载/做种体积/魔力时产）。
// POST /dashboard/sync 可即时抓取全部站点（站均 1-3s，全量较慢——面板手动触发）。
// 请求统一走 api.js 的 api()；字节格式化复用 util.js 的 fmtBytes。
import { api } from './api.js'
import { fmtBytes } from './util.js'

export { fmtBytes }

export async function overview(cfg) {
  return api(cfg, '/api/v1/dashboard')
}

export async function syncAll(cfg) {
  return api(cfg, '/api/v1/dashboard/sync', { method: 'POST' })
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
