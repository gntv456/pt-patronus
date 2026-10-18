// 种子表格的筛选/排序/分页状态与纯函数（stateGroup/stateLabel/magnetOf）。
// 从 TorrentTable.vue 迁出；op/emit 回调经参数注入。
import { ref, computed, watch, type Ref, type ComputedRef } from 'vue'
import type { Torrent } from '@/api/downloader'
import { useI18n } from 'vue-i18n'

// qB 状态归并（全站统一口径，卡片/抽屉/筛选共用）：
//   做种族 = uploading/stalledUP/queuedUP/forcedUP（UP 后缀=做种方向，stalled=无连接仍在做种）
//   下载族 = downloading/stalledDL/metaDL/queuedDL/forcedDL（DL 后缀）
// checking（校验中）/moving（移动中）不属于任何方向，先排除再匹配后缀。
// 仅按字面 pause|stop 判暂停；error/missingFiles 单列。uTorrent 的 Seeding/Downloading 也能命中。
export function stateGroup(s: string): string {
  const q = s.toLowerCase()
  if (q.includes('error') || q.includes('missing')) return 'error'
  if (q.includes('pause') || q.includes('stop')) return 'paused'
  if (q.includes('check') || q.includes('moving')) return 'other'
  if (q.endsWith('up') || q.includes('upload') || q.includes('seed')) return 'seeding'
  if (q.endsWith('dl') || q.includes('download') || q.includes('leech') || q.includes('meta')) return 'downloading'
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

// #382 torrents 以前收 `readonly Torrent[]`（**按值**）。`filtered` 里的 `torrents.filter(...)`
// 只在**首次求值**时访问数组元素，而那时抽屉刚挂载、REST/WS 还没回来 ⇒ 数组是空的
// ⇒ computed 没建立任何依赖 ⇒ 后续数据到达不会触发重算，filtered 永远停在空数组。
// 症状极像「数据到了但被筛掉」：计数显示 `0 / 1 个任务`（total 取 props.torrents.length 是活的，
// shown 取 filtered.length 是死的），搜索/筛选全默认、无「清除筛选」按钮、接口全 200。
// 本仓 Playwright core-flow E2E 实测：抽屉在搜索用例跑过之后打开必现（REST 晚于组件初始化），
// 单跑下载器用例则不复现 —— 正是这个时序。
// 改成收 getter：每次求值都重新读当前的响应式数组，依赖关系在 computed 内直接建立。
export function useTorrentTableFilters(torrents: () => readonly Torrent[]): TorrentTableFilters {
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
    let arr = torrents().filter((t) => {
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
