// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import type { ArrKey, SortField } from '@/composables/useSearchFilter'

// 搜索筛选条的静态配置选项（维度/时间/排序字段）。
// 抽出为独立文件以收敛 SearchFilterBar.vue 体积。

export const SEARCH_DIMS: { key: ArrKey; labelKey: string; icon: string }[] = [
  { key: 'site', labelKey: 'sfc.site', icon: '🌐' },
  { key: 'promo', labelKey: 'sfc.promo', icon: '🎁' },
  { key: 'res', labelKey: 'sfc.res', icon: '🖥' },
  { key: 'codec', labelKey: 'sfc.codec', icon: '🎬' },
  { key: 'src', labelKey: 'sfc.src', icon: '📦' },
  { key: 'hdr', labelKey: 'sfc.hdr', icon: '✨' },
]

export const TIME_OPTS: { key: 'today' | 'week' | 'month'; labelKey: string }[] = [
  { key: 'today', labelKey: 'sfc.today' },
  { key: 'week', labelKey: 'sfc.week' },
  { key: 'month', labelKey: 'sfc.month' },
]

export const SORT_FIELDS: { key: SortField; labelKey: string }[] = [
  { key: 'seeders', labelKey: 'sfc.seeders' },
  { key: 'size', labelKey: 'sfc.size' },
  { key: 'leechers', labelKey: 'sfc.leechers' },
  { key: 'site', labelKey: 'sfc.site' },
]
