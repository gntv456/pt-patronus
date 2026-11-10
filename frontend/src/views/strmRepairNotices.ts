// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// #291：STRM 修复结果的「该说什么」抽成纯函数。
// 后端一次修复给三个计数（repaired/skipped/failed），还可能带上「规则表这次读不出来」——
// 那时被保守跳过的那批 .strm **仍然是坏的**，和「登记型没有文件」共用同一个 skipped。
// 混在一条成功提示里就等于报喜不报忧，所以这类记录要单独出一条警告。
// 抽出来的另一个原因：Cloud.vue 已在行数基线上（只许缩不许涨）。

export interface StrmRepairCounts {
  repaired: number
  skipped: number
  failed?: number
  rules_unavailable?: boolean
  rules_skipped?: number
}

export interface StrmRepairNotice {
  kind: 'success' | 'warning'
  text: string
}

type Translate = (key: string, params?: Record<string, unknown>) => string

export function strmRepairNotices(r: StrmRepairCounts, t: Translate): StrmRepairNotice[] {
  const failed = r.failed ?? 0
  const notices: StrmRepairNotice[] = [
    {
      kind: failed ? 'warning' : 'success',
      text: t(failed ? 'cloud.strmRepairPartial' : 'cloud.strmRepaired', {
        repaired: r.repaired,
        skipped: r.skipped,
        failed,
      }),
    },
  ]
  if (r.rules_unavailable) {
    notices.push({ kind: 'warning', text: t('cloud.strmRepairRulesUnreadable', { n: r.rules_skipped ?? 0 }) })
  }
  return notices
}
