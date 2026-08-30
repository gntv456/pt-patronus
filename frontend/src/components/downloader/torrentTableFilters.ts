// 种子表格的筛选/排序/分页状态与纯函数（stateGroup/stateLabel/magnetOf）。
// 从 TorrentTable.vue 迁出；op/emit 回调经参数注入。
import { ref, computed, watch, type Ref, type ComputedRef } from 'vue'
import type { Torrent } from '@/api/downloader'
import { useI18n } from 'vue-i18n'

export function stateGroup(s: string): string {
  if (s.includes('error')) return 'error'
  if (s.includes('upload') || s.includes('seed')) return 'seeding'
  if (s.includes('pause') || s.includes('stop')) return 'paused'
  if (s.includes('download')) return 'downloading'
  return 'other'
}

export function useStateLabel() {
  const { t } = useI18n()
  return (s: string): string => {
    const map: Record<string, string> = {
      error: t('tt.st.error'),
      missingFiles: t('tt.st.missingFiles'),
      uploading: t('tt.st.seeding'),
      forcedUP: t('tt.st.forcedUP'),
      stalledUP: t('tt.st.stalledUP'),
      queuedUP: t('tt.st.queuedUP'),
      pausedUP: t('tt.st.pausedUP'),
      checkingUP: t('tt.st.checking'),
      downloading: t('tt.st.downloading'),
      forcedDL: t('tt.st.forcedDL'),
      stalledDL: t('tt.st.stalledDL'),
      queuedDL: t('tt.st.queuedDL'),
      pausedDL: t('tt.st.pausedDL'),
      checkingDL: t('tt.st.checking'),
      metaDL: t('tt.st.metaDL'),
      allocating: t('tt.st.allocating'),
      checkingResumeData: t('tt.st.checking'),
      moving: t('tt.st.moving'),
      stopped: t('tt.st.stopped'),
      paused: t('tt.st.paused'),
      queued: t('tt.st.queued'),
      seeding: t('tt.st.seeding'),
      unknown: t('tt.st.unknown'),
    }
    return map[s] || s
  }
}

export function magnetOf(t: Torrent) {
  return `magnet:?xt=urn:btih:${t.info_hash || t.hash}&dn=${encodeURIComponent(t.name)}`
}

export interface TorrentTableFilters {
  search: Ref<string>
  stateFilter: Ref<string>
  categoryFilter: Ref<string>
  sortKey: Ref<keyof Torrent | ''>
  sortOrder: Ref<'' | 'asc' | 'desc'>
  page: Ref<number>
  pageSize: Ref<number>
  filtered: ComputedRef<Torrent[]>
  paged: ComputedRef<Torrent[]>
  hasFilters: ComputedRef<boolean>
  onSortChange: ({ prop, order }: { prop: string | null; order: 'ascending' | 'descending' | null }) => void
  clearFilters: () => void
}

export function useTorrentTableFilters(torrents: readonly Torrent[]): TorrentTableFilters {
  const search = ref('')
  const stateFilter = ref('all')
  const categoryFilter = ref('')
  const sortKey = ref<keyof Torrent | ''>('')
  const sortOrder = ref<'' | 'asc' | 'desc'>('')
  const page = ref(1)
  const pageSize = ref(50)

  function onSortChange({ prop, order }: { prop: string | null; order: 'ascending' | 'descending' | null }) {
    sortKey.value = (prop as keyof Torrent) || ''
    sortOrder.value = order === 'ascending' ? 'asc' : order === 'descending' ? 'desc' : ''
  }

  const filtered = computed(() => {
    let arr = torrents.filter((t) => {
      const needle = search.value.trim().toLowerCase()
      if (needle && !`${t.name} ${t.hash} ${t.category}`.toLowerCase().includes(needle)) return false
      if (stateFilter.value !== 'all' && stateGroup(t.state) !== stateFilter.value) return false
      if (categoryFilter.value && t.category !== categoryFilter.value) return false
      return true
    })
    if (sortKey.value && sortOrder.value) {
      const key = sortKey.value
      arr = [...arr].sort((a, b) => {
        const av = a[key] as number | string
        const bv = b[key] as number | string
        const cmp = typeof av === 'number' && typeof bv === 'number' ? av - bv : String(av).localeCompare(String(bv))
        return sortOrder.value === 'asc' ? cmp : -cmp
      })
    }
    return arr
  })

  const paged = computed(() => {
    const start = (page.value - 1) * pageSize.value
    return filtered.value.slice(start, start + pageSize.value)
  })
  const hasFilters = computed(() => Boolean(search.value || categoryFilter.value || stateFilter.value !== 'all'))

  watch([search, stateFilter, categoryFilter, sortKey, sortOrder], () => {
    page.value = 1
  })

  function clearFilters() {
    search.value = ''
    stateFilter.value = 'all'
    categoryFilter.value = ''
  }

  return { search, stateFilter, categoryFilter, sortKey, sortOrder, page, pageSize, filtered, paged, hasFilters, onSortChange, clearFilters }
}
