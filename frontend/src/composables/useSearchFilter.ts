// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

/**
 * 站点搜索结果筛选 composable。
 * 纯前端过滤：选项从结果集动态提取，过滤/排序在内存 computed 完成，适配 SSE 逐站追加。
 * Search.vue（表格，筛+排）与 SiteTorrentFindDialog.vue（卡片弹窗，仅筛）共用。
 */
import { computed, reactive, type Ref } from 'vue'
import type { EnrichedMeta } from '@/utils/torrentMeta'

export type SortField = 'seeders' | 'size' | 'leechers' | 'site'
export type SortOrder = 'asc' | 'desc'
export type TimePreset = '' | 'today' | 'week' | 'month'
export type ArrKey = 'site' | 'promo' | 'res' | 'codec' | 'src' | 'hdr'

export interface FilterState {
  site: string[]
  promo: string[]
  res: string[]
  codec: string[]
  src: string[]
  hdr: string[]
  onlyCnSub: boolean
  onlyFree: boolean
  sizeMinGB: number
  sizeMaxGB: number
  seedersMin: number
  timePreset: TimePreset
}

export interface FilterOptions {
  site: string[]
  promo: string[]
  res: string[]
  codec: string[]
  src: string[]
  hdr: string[]
  maxSizeGB: number
}

export interface ActiveTag {
  key: keyof FilterState
  value: string
  label: string
}

const GB = 1024 ** 3
const DAY = 86_400_000

/** 行需同时具备来源站点名与做种/下载数（SearchRow / FindRow 都满足）。 */
export function useSearchFilter<T extends EnrichedMeta & { site_name?: string; seeders?: number; leechers?: number }>(
  rows: Ref<T[]>,
) {
  const filter = reactive<FilterState>({
    site: [],
    promo: [],
    res: [],
    codec: [],
    src: [],
    hdr: [],
    onlyCnSub: false,
    onlyFree: false,
    sizeMinGB: 0,
    sizeMaxGB: 0,
    seedersMin: 0,
    timePreset: '',
  })
  const sort = reactive<{ field: SortField; order: SortOrder }>({ field: 'seeders', order: 'desc' })

  function distinct(get: (r: T) => string | undefined | null): string[] {
    const set = new Set<string>()
    for (const r of rows.value) {
      const v = get(r)
      if (v) set.add(v)
    }
    return [...set].sort((a, b) => a.localeCompare(b, 'zh'))
  }

  const options = computed<FilterOptions>(() => ({
    site: distinct((r) => r.site_name),
    promo: distinct((r) => r.promo?.text),
    res: distinct((r) => r.res),
    codec: distinct((r) => r.codec),
    src: distinct((r) => r.src),
    hdr: distinct((r) => r.hdr),
    maxSizeGB: Math.max(0, ...rows.value.map((r) => Math.ceil(r.sizeBytes / GB))),
  }))

  function inSet(arr: string[], v: string): boolean {
    return arr.length === 0 || arr.includes(v)
  }

  function timeMatch(pubMs: number): boolean {
    if (!filter.timePreset || !pubMs) return true // 未选时间 或 行无发布时间 → 放行（避免误删无时间站点）
    const span = filter.timePreset === 'today' ? DAY : filter.timePreset === 'week' ? 7 * DAY : 30 * DAY
    return Date.now() - pubMs <= span
  }

  function matchRow(r: T): boolean {
    if (!inSet(filter.site, r.site_name || '')) return false
    if (!inSet(filter.promo, r.promo?.text || '')) return false
    if (!inSet(filter.res, r.res)) return false
    if (!inSet(filter.codec, r.codec)) return false
    if (!inSet(filter.src, r.src)) return false
    if (!inSet(filter.hdr, r.hdr)) return false
    if (filter.onlyCnSub && !r.cnSub) return false
    // 「仅免费」快捷开关：覆盖 免费/2X免费/4X免费/折扣 等一切促销形态，
    // 比逐个勾选 promo 维度选项快一步（PT 场景下最常用的单维筛选）。
    if (filter.onlyFree && !r.promo) return false
    if (filter.sizeMinGB > 0 && r.sizeBytes < filter.sizeMinGB * GB) return false
    if (filter.sizeMaxGB > 0 && r.sizeBytes > filter.sizeMaxGB * GB) return false
    if (filter.seedersMin > 0 && (r.seeders ?? 0) < filter.seedersMin) return false
    if (!timeMatch(r.pubMs)) return false
    return true
  }

  const filtered = computed(() => rows.value.filter(matchRow))

  const sorted = computed(() => {
    const dir = sort.order === 'asc' ? 1 : -1
    return [...filtered.value].sort((a, b) => {
      switch (sort.field) {
        case 'size':
          return (a.sizeBytes - b.sizeBytes) * dir
        case 'leechers':
          return ((a.leechers ?? 0) - (b.leechers ?? 0)) * dir
        case 'site':
          return (a.site_name || '').localeCompare(b.site_name || '', 'zh') * dir
        default:
          return ((a.seeders ?? 0) - (b.seeders ?? 0)) * dir
      }
    })
  })

  const hasActive = computed(
    () =>
      filter.site.length +
        filter.promo.length +
        filter.res.length +
        filter.codec.length +
        filter.src.length +
        filter.hdr.length >
        0 ||
      filter.onlyCnSub ||
      filter.onlyFree ||
      filter.sizeMinGB > 0 ||
      filter.sizeMaxGB > 0 ||
      filter.seedersMin > 0 ||
      !!filter.timePreset,
  )

  const activeTags = computed<ActiveTag[]>(() => {
    const out: ActiveTag[] = []
    const push = (key: ArrKey) => filter[key].forEach((v) => out.push({ key, value: v, label: v }))
    push('site')
    push('promo')
    push('res')
    push('codec')
    push('src')
    push('hdr')
    if (filter.onlyCnSub) out.push({ key: 'onlyCnSub', value: '1', label: '中字' })
    if (filter.onlyFree) out.push({ key: 'onlyFree', value: '1', label: '免费' })
    if (filter.sizeMinGB > 0 || filter.sizeMaxGB > 0) {
      const max = filter.sizeMaxGB || options.value.maxSizeGB
      out.push({
        key: 'sizeMinGB',
        value: `${filter.sizeMinGB}-${filter.sizeMaxGB}`,
        label: `大小 ${filter.sizeMinGB}-${max}G`,
      })
    }
    if (filter.seedersMin > 0)
      out.push({ key: 'seedersMin', value: String(filter.seedersMin), label: `做种≥${filter.seedersMin}` })
    if (filter.timePreset) {
      const map: Record<Exclude<TimePreset, ''>, string> = { today: '今天', week: '本周', month: '本月' }
      out.push({ key: 'timePreset', value: filter.timePreset, label: map[filter.timePreset] })
    }
    return out
  })

  function removeFilter(tag: ActiveTag) {
    const k = tag.key
    if (k === 'onlyCnSub') filter.onlyCnSub = false
    else if (k === 'onlyFree') filter.onlyFree = false
    else if (k === 'sizeMinGB') {
      filter.sizeMinGB = 0
      filter.sizeMaxGB = 0
    } else if (k === 'seedersMin') filter.seedersMin = 0
    else if (k === 'timePreset') filter.timePreset = ''
    else {
      const arr = filter[k as ArrKey]
      const i = arr.indexOf(tag.value)
      if (i >= 0) arr.splice(i, 1)
    }
  }

  function clearAll() {
    filter.site = []
    filter.promo = []
    filter.res = []
    filter.codec = []
    filter.src = []
    filter.hdr = []
    filter.onlyCnSub = false
    filter.onlyFree = false
    filter.sizeMinGB = 0
    filter.sizeMaxGB = 0
    filter.seedersMin = 0
    filter.timePreset = ''
  }

  return { filter, sort, options, filtered, sorted, hasActive, activeTags, removeFilter, clearAll }
}

export type SearchFilterHandle = ReturnType<typeof useSearchFilter>
