// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

/**
 * SiteTorrentFindDialog 的卡片聚合 + 前端过滤逻辑（从组件抽出，纯逻辑无 UI）。
 * 行为不变：结果扁平化 → enrich → 过滤 → 按卡片聚合 → promo+seeders 排序。
 */
import { computed, ref, type ComputedRef } from 'vue'
import type { SiteSearchResult } from '@/api/search'
import type { Site } from '@/api/site'
import {
  cardGroupKey,
  enrichTorrent,
  relativeTime,
  volumeFactorLabel,
  type EnrichedMeta,
  type TorrentTag,
} from '@/utils/torrentMeta'
import { useSearchFilter } from '@/composables/useSearchFilter'
import type { FindRow } from '@/components/SiteTorrentFindDialog.vue'

type EnrichedFindRow = FindRow & EnrichedMeta
export type CardGroup = {
  key: string
  primary: EnrichedFindRow
  more: EnrichedFindRow[]
  tags: TorrentTag[]
  promo: { text: string; tone: string } | null
  timeText: string
}

export function useTorrentFindCards(results: ComputedRef<SiteSearchResult[]>, siteMap: ComputedRef<Map<number, Site>>) {
  const flatRows = computed<FindRow[]>(() =>
    results.value.flatMap((r) =>
      (r.torrents ?? []).map((t) => ({
        ...t,
        site_id: r.site_id,
        site_name: r.site_name,
        base_url: siteMap.value.get(r.site_id)?.base_url || '',
      })),
    ),
  )

  // 派生筛选 meta + 前端过滤 composable（筛后聚合分组，保留 promo+seeders 卡片排序）。
  const enrichedRows = computed<EnrichedFindRow[]>(() => flatRows.value.map((r) => enrichTorrent(r)))
  const { filter, sort, options, filtered, hasActive, activeTags, removeFilter, clearAll } =
    useSearchFilter(enrichedRows)

  const hitSites = computed(() => results.value.filter((r) => !r.error && (r.torrents?.length ?? 0) > 0).length)
  const hitTotal = computed(() => flatRows.value.length)
  const failedSites = computed(() => results.value.filter((r) => r.error))
  const showFailed = ref(false)

  const cards = computed<CardGroup[]>(() => {
    const map = new Map<string, EnrichedFindRow[]>()
    for (const row of filtered.value) {
      const key = cardGroupKey(row)
      const list = map.get(key)
      if (list) list.push(row)
      else map.set(key, [row])
    }
    const groups: CardGroup[] = []
    for (const [, list] of map) {
      list.sort((a, b) => (b.seeders || 0) - (a.seeders || 0))
      const primary = list[0]
      // tags/promo 已由 enrichTorrent 一次提取；副标题只参与识别不重复展示。
      groups.push({
        key: cardGroupKey(primary),
        primary,
        more: list.slice(1),
        tags: primary.tags,
        promo: primary.promo,
        timeText: relativeTime(primary.time || primary.pub_date),
      })
    }
    groups.sort((a, b) => {
      const promoRank = (g: CardGroup) => {
        if (!g.promo) return 2
        if (g.promo.tone === 'free') return 0
        return 1
      }
      const d = promoRank(a) - promoRank(b)
      if (d !== 0) return d
      return (b.primary.seeders || 0) - (a.primary.seeders || 0)
    })
    return groups
  })

  // 更多来源
  const moreVisible = ref(false)
  const moreItems = ref<FindRow[]>([])

  function openMore(group: CardGroup) {
    moreItems.value = group.more
    moreVisible.value = true
  }

  function promoOf(row: FindRow) {
    return volumeFactorLabel(row.uploadvolumefactor, row.downloadvolumefactor, row.free, [
      ...(row.tags || []),
      ...(row.labels || []),
    ])
  }

  function openDetail(row: FindRow) {
    if (row.detail_url) window.open(row.detail_url, '_blank', 'noopener')
  }

  return {
    flatRows,
    enrichedRows,
    filter,
    sort,
    options,
    filtered,
    hasActive,
    activeTags,
    removeFilter,
    clearAll,
    hitSites,
    hitTotal,
    failedSites,
    showFailed,
    cards,
    moreVisible,
    moreItems,
    openMore,
    promoOf,
    openDetail,
  }
}
