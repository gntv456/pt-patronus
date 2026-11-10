// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import type { DashboardExportConfig, RankMetric } from './dashboardExportConfig'
import type { DashboardShareData, DashboardShareSite, NumField, RankDef, Champion } from './dashboardImageTypes'
import { fmtBytes, sum, earliestSite } from './dashboardImageHelpers'

// ---------- 数据派生 ----------
export function buildNumericFields(d: DashboardShareData, config: DashboardExportConfig): NumField[] {
  const totalRatio = d.totals.downloaded > 0 ? d.totals.uploaded / d.totals.downloaded : 0
  const all: NumField[] = [
    { key: 'uploaded', label: '总上传', color: '#2ecc71', value: fmtBytes(d.totals.uploaded) },
    { key: 'downloaded', label: '总下载', color: '#4285f4', value: fmtBytes(d.totals.downloaded) },
    { key: 'seeding', label: '总做种', color: '#38ef7d', value: `${d.totals.seeding}` },
    { key: 'seedingSize', label: '做种体积', color: '#43cea2', value: fmtBytes(d.totals.seeding_size) },
    { key: 'bonus', label: '总魔力', color: '#f7971e', value: `${Math.round(d.totals.bonus)}` },
    { key: 'ratio', label: '总分享率', color: '#e74c3c', value: totalRatio.toFixed(2) },
    { key: 'uploadCount', label: '总发布数', color: '#9b59b6', value: `${sum(d.sites, (s) => s.uploads)}` },
    {
      key: 'bonusPerHour',
      label: '总时魔/h',
      color: '#e67e22',
      value: sum(d.sites, (s) => s.bonus_per_hour).toFixed(1),
    },
  ]
  const f = config.fields as Record<string, boolean>
  return all.filter((m) => f[m.key])
}

export const RANK_DEFS: RankDef[] = [
  { key: 'uploaded', label: '上传', color: '#2ecc71', pick: (s) => s.uploaded, fmt: (v) => fmtBytes(v) },
  { key: 'downloaded', label: '下载', color: '#4285f4', pick: (s) => s.downloaded, fmt: (v) => fmtBytes(v) },
  { key: 'uploads', label: '发布', color: '#9b59b6', pick: (s) => s.uploads, fmt: (v) => `${v}` },
  { key: 'seeding', label: '做种', color: '#38ef7d', pick: (s) => s.seeding, fmt: (v) => `${v}` },
  { key: 'ratio', label: '分享率', color: '#e74c3c', pick: (s) => s.ratio, fmt: (v) => v.toFixed(2) },
  { key: 'bonus', label: '魔力', color: '#f7971e', pick: (s) => s.bonus, fmt: (v) => `${Math.round(v)}` },
  { key: 'bonus_per_hour', label: '时魔', color: '#e67e22', pick: (s) => s.bonus_per_hour, fmt: (v) => v.toFixed(1) },
]

export function rankChampions(sites: DashboardShareSite[], metrics: RankMetric[]): Champion[] {
  const out: Champion[] = []
  for (const key of metrics) {
    const def = RANK_DEFS.find((d) => d.key === key)
    if (!def) continue
    let best: DashboardShareSite | null = null
    for (const s of sites) {
      if (best === null || def.pick(s) > def.pick(best)) best = s
    }
    if (best && def.pick(best) > 0) out.push({ def, site: best, value: def.pick(best) })
  }
  return out
}

// 按 config 过滤/排序/截断出要画明细的站点。
export function computeViewSites(d: DashboardShareData, config: DashboardExportConfig): DashboardShareSite[] {
  let v = d.sites
  if (config.selectedSiteIds.length) v = v.filter((s) => config.selectedSiteIds.includes(s.site_id))
  v = [...v].sort((a, b) => b.uploaded - a.uploaded)
  if (config.siteLimit > 0) v = v.slice(0, config.siteLimit)
  return v
}

// ---------- 布局（绝对坐标，绘制共用，杜绝漂移） ----------
export function layout(d: DashboardShareData, config: DashboardExportConfig) {
  const HEADER_H = 84
  const gridRowH = 34
  const rankRowH = 36
  const siteRowH = 28
  const earliestH = 22
  const SITE_ROW_H = 116 + (config.showIdentity ? 18 : 0) // +18 容纳身份行（用户名/ID/邮箱）
  const FOOTER_H = 56

  const numeric = buildNumericFields(d, config)
  const gridRows = Math.ceil(numeric.length / 2)
  const champions = config.showRank ? rankChampions(d.sites, config.rankMetrics) : []
  const rankRows = Math.ceil(champions.length / 2)
  const earliest = config.fields.earliestJoin ? earliestSite(d.sites) : null
  const viewSites = config.showSites ? computeViewSites(d, config) : []

  let y = HEADER_H
  const headDividerY = y
  y += 20
  const siteRowY = y + 20 // 总站点/PT龄 baseline
  y += siteRowH
  let earliestY = 0
  if (earliest) {
    y += 4
    earliestY = y + 16
    y += earliestH
  }
  let gridY = 0
  if (numeric.length) {
    y += 8
    gridY = y
    y += gridRowH * gridRows
    y += 12
  }
  let rankDividerY = 0
  let rankTitleY = 0
  let rankGridY = 0
  if (champions.length) {
    y += 8
    rankDividerY = y
    y += 16
    rankTitleY = y + 14
    y += 30 // 标题行 + 与网格的间距，避免「各项冠军」与第一行内容重叠
    rankGridY = y
    y += rankRowH * rankRows
  }
  let siteDividerY = 0
  let siteTitleY = 0
  let siteGridY = 0
  if (viewSites.length) {
    y += 8
    siteDividerY = y
    y += 24
    siteTitleY = y + 14
    y += 30
    siteGridY = y
    y += Math.ceil(viewSites.length / 2) * SITE_ROW_H
    y += 8
  }
  const totalH = y + FOOTER_H
  const footerY = totalH - 30

  return {
    numeric,
    gridRowH,
    gridY,
    champions,
    rankRowH,
    rankDividerY,
    rankTitleY,
    rankGridY,
    earliest,
    earliestY,
    siteRowY,
    viewSites,
    SITE_ROW_H,
    siteDividerY,
    siteTitleY,
    siteGridY,
    headDividerY,
    totalH,
    footerY,
  }
}
