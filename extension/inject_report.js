// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// inject_report.js 补注入结果的汇总文案（设置页「授予全部站点权限」用）。
// 抽成纯函数的原因：后台 injectAllTabs 只有真浏览器才有 chrome.tabs，测不到；
// 而「N 个页面没生效时该说什么」正是会被写歪的那一层，必须能断言。
// 口径：受限页（chrome://、扩展页）与黑名单是**合法跳过**，不许混进失败里，
// 否则用户学到「授权了也没用」这个假结论；失败数必须带原因，否则等于没说。

// injectLive 真的能用上增强的页面数（本轮注入成功 + 之前就已注入）。
export function injectLive(r) {
  if (!r) return 0
  return (r.injected || 0) + (r.ready || 0)
}

export function injectSummaryKind(r) {
  if (!r) return 'err'
  return r.failed || r.error ? 'err' : 'ok'
}

export function injectSummaryText(r) {
  if (!r || typeof r.tried !== 'number') return '补注入结果读不出来（后台没回数据）'
  if (!r.tried) return r.error ? `⚠ ${r.error}` : '当前没有已打开的页面，刷新页面即生效'
  const live = injectLive(r)
  const skip = r.skipped ? `（${r.skipped} 个受限页/黑名单跳过，不算失败）` : ''
  if (r.failed) {
    const why = r.error ? `：${String(r.error).slice(0, 40)}` : ''
    return `⚠ ${live} 个页面已生效，${r.failed} 个注入失败${why}${skip}`
  }
  return `${live} 个页面已生效${skip}`
}
