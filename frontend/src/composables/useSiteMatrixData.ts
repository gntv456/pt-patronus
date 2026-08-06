/**
 * 站点运营矩阵的数据加载 / 排序 / 过滤 / 格式化（从 SiteOperationsMatrix.vue 抽出）。
 * 行为不变：overview 加载 → 搜索/状态过滤 → 列头排序 → 分页。
 */
import { ref, computed, type ComputedRef, type Ref } from 'vue'
import { dashboardApi, type SiteHistoryPoint, type SiteStatItem } from '@/api/dashboard'
import type { Site, SiteConnectivityStatus } from '@/api/site'
import { safeUrl } from '@/utils/safeUrl'

export type MatrixFilter = 'all' | 'issues' | 'connected' | 'disabled'
export type MatrixLayout = 'wide' | 'balanced' | 'compact'

export function fmtBytes(value: number) {
  if (!value) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let next = value
  let unit = 0
  while (next >= 1024 && unit < units.length - 1) {
    next /= 1024
    unit++
  }
  return `${next.toFixed(2)} ${units[unit]}`
}

export function fmtTime(value: string) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  const pad = (part: number) => String(part).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

export function fmtDate(value: string) {
  if (!value) return '-'
  return value.match(/^\d{4}-\d{2}-\d{2}/)?.[0] || value
}

export function fmtNumber(value: number, digits = 0): string {
  if (!value) return '-'
  return value.toLocaleString('zh-CN', { maximumFractionDigits: digits })
}

export function fmtRatio(value: number): string {
  return value > 0 ? value.toFixed(2) : '-'
}

export function useSiteMatrixData(
  props: {
    sites: Site[]
    connectivity: Record<number, SiteConnectivityStatus>
    cookieAlerts: readonly string[]
  },
) {
  const rows = ref<SiteStatItem[]>([])
  const loading = ref(false)
  const syncing = ref(false)
  const loadError = ref('')
  const refreshing = ref(new Set<number>())
  const siteQuery = ref('')
  const matrixFilter = ref<MatrixFilter>('all')
  // 列头点击排序：sortKey=字段名(对齐 el-table-column prop)，sortDir=方向。默认按更新时间降序。
  const sortKey = ref<string>('updated_at')
  const sortDir = ref<'asc' | 'desc'>('desc')
  const currentPage = ref(1)
  const pageSize = ref(8)

  const historyVisible = ref(false)
  const historyLoading = ref(false)
  const historySiteName = ref('')
  const history = ref<SiteHistoryPoint[]>([])

  // 批量删除选中（跨页保留：reserve-selection + row-key=site_id）
  const selectedSites = ref<Site[]>([])

  const siteConfigs = computed(() => new Map(props.sites.map((site) => [site.id, site])))
  const siteBaseUrls = computed(() => new Map(props.sites.map((site) => [site.id, site.base_url])))
  function siteBaseUrl(siteId: number): string {
    return siteBaseUrls.value.get(siteId) || ''
  }
  function siteConfig(siteId: number): Site | undefined {
    return siteConfigs.value.get(siteId)
  }

  function emptyStat(site: Site): SiteStatItem {
    return {
      site_id: site.id,
      site_name: site.name,
      username: site.username || '',
      user_id: '',
      email: '',
      level: '',
      uploaded: 0,
      downloaded: 0,
      ratio: 0,
      true_uploaded: 0,
      true_downloaded: 0,
      true_ratio: 0,
      seeding: 0,
      leeching: 0,
      seeding_size: 0,
      uploads: 0,
      unique_groups: 0,
      perfect_flac: 0,
      invited: 0,
      bonus: 0,
      bonus_per_hour: 0,
      seeding_bonus: 0,
      seeding_bonus_per_hour: 0,
      rescue_seeding_bonus_per_hour: 0,
      claim_count: 0,
      max_claim: 0,
      join_date: site.created_at || '',
      last_access_at: '',
      updated_at: '',
    }
  }

  const allRows = computed(() => {
    const stats = new Map(rows.value.map((row) => [row.site_id, row]))
    const configured = props.sites.map((site) => {
      const stat = stats.get(site.id)
      return stat ? { ...stat, site_name: site.name, username: stat.username || site.username || '' } : emptyStat(site)
    })
    const configuredIds = new Set(props.sites.map((site) => site.id))
    return [...configured, ...rows.value.filter((row) => !configuredIds.has(row.site_id))]
  })
  const inactiveSites = computed(() => {
    const cutoff = Date.now() - 30 * 24 * 60 * 60 * 1000
    return allRows.value
      .map((row) => ({ row, site: siteConfig(row.site_id) }))
      .filter(({ row, site }) => {
        if (!site?.is_active || !/^https?:\/\//i.test(safeUrl(site.base_url))) return false
        const lastAccess = new Date(row.last_access_at).getTime()
        return Number.isFinite(lastAccess) && lastAccess > 0 && lastAccess < cutoff
      })
      .sort((a, b) => new Date(a.row.last_access_at).getTime() - new Date(b.row.last_access_at).getTime())
  })
  const inactiveSiteCount = computed(() => inactiveSites.value.length)

  function siteHasIssue(row: SiteStatItem): boolean {
    return props.connectivity[row.site_id]?.status === 'disconnected' || props.cookieAlerts.includes(row.site_name)
  }

  // 排序取值：status 走 connectivity（row 上无此字段），其余直接取 row 字段。
  // 数值字段(number)数值比较；字符串字段(含 ISO 日期/等级/用户名)localeCompare——ISO 日期字典序等价时间序。
  function sortValue(row: SiteStatItem, key: string): string | number {
    if (key === 'status') return props.connectivity[row.site_id]?.status || ''
    const v = (row as unknown as Record<string, unknown>)[key]
    return (v ?? '') as string | number
  }
  function compareRow(a: SiteStatItem, b: SiteStatItem): number {
    const va = sortValue(a, sortKey.value)
    const vb = sortValue(b, sortKey.value)
    const r =
      typeof va === 'number' && typeof vb === 'number' ? va - vb : String(va).localeCompare(String(vb))
    return sortDir.value === 'asc' ? r : -r
  }
  // el-table 列头排序（sortable="custom"）：点击列头触发，更新 sortKey/sortDir 并回首页；取消排序回默认。
  function onSortChange({ prop, order }: { prop: string | null; order: 'ascending' | 'descending' | null }) {
    if (prop && order) {
      sortKey.value = prop
      sortDir.value = order === 'ascending' ? 'asc' : 'desc'
    } else {
      sortKey.value = 'updated_at'
      sortDir.value = 'desc'
    }
    currentPage.value = 1
  }

  const matrixRows = computed(() => {
    const query = siteQuery.value.trim().toLowerCase()
    let nextRows = [...allRows.value]
    if (query) {
      nextRows = nextRows.filter((row) => {
        const config = siteConfig(row.site_id)
        return [
          row.site_name,
          row.username,
          row.user_id,
          row.email,
          row.level,
          config?.definition,
          config?.platform,
          config?.base_url,
        ].some((value) => value?.toLowerCase().includes(query))
      })
    }
    if (matrixFilter.value === 'issues') nextRows = nextRows.filter(siteHasIssue)
    if (matrixFilter.value === 'connected') {
      nextRows = nextRows.filter((row) => props.connectivity[row.site_id]?.status === 'connected')
    }
    if (matrixFilter.value === 'disabled') {
      nextRows = nextRows.filter((row) => props.connectivity[row.site_id]?.status === 'disabled')
    }
    nextRows.sort(compareRow)
    return nextRows
  })
  const pagedRows = computed(() => {
    const start = (currentPage.value - 1) * pageSize.value
    return matrixRows.value.slice(start, start + pageSize.value)
  })
  const pageRange = computed(() => {
    if (!matrixRows.value.length) return '0'
    const start = (currentPage.value - 1) * pageSize.value + 1
    const end = Math.min(start + pageSize.value - 1, matrixRows.value.length)
    return `${start}-${end}`
  })

  async function reload() {
    loading.value = true
    loadError.value = ''
    try {
      rows.value = (await dashboardApi.overview()).sites ?? []
    } catch (error: unknown) {
      loadError.value = (error as Error).message || '站点统计加载失败'
    } finally {
      loading.value = false
    }
  }

  async function syncAll() {
    if (syncing.value) return
    syncing.value = true
    try {
      rows.value = (await dashboardApi.sync()).sites ?? []
      ElMessage.success('站点统计已刷新')
    } catch (error: unknown) {
      ElMessage.error((error as Error).message || '站点统计刷新失败')
    } finally {
      syncing.value = false
    }
  }

  async function syncOne(row: SiteStatItem) {
    if (refreshing.value.has(row.site_id)) return
    refreshing.value.add(row.site_id)
    try {
      rows.value = (await dashboardApi.syncOne(row.site_id)).sites ?? []
      ElMessage.success(`${row.site_name} 数据已刷新`)
    } catch (error: unknown) {
      ElMessage.error((error as Error).message || `${row.site_name} 刷新失败`)
    } finally {
      refreshing.value.delete(row.site_id)
    }
  }

  async function viewHistory(row: SiteStatItem) {
    historyVisible.value = true
    historyLoading.value = true
    historySiteName.value = row.site_name
    history.value = []
    try {
      history.value = await dashboardApi.history(row.site_id, 30)
    } catch (error: unknown) {
      ElMessage.error((error as Error).message || '历史数据加载失败')
    } finally {
      historyLoading.value = false
    }
  }

  function matrixRowClassName({ row }: { row: SiteStatItem }): string {
    if (siteHasIssue(row)) return 'matrix-row-issue'
    if (props.connectivity[row.site_id]?.status === 'disabled') return 'matrix-row-disabled'
    return ''
  }

  function handleSelectionChange(selRows: SiteStatItem[]) {
    selectedSites.value = selRows.map((r) => siteConfig(r.site_id)).filter((s): s is Site => !!s)
  }

  return {
    rows, loading, syncing, loadError, refreshing,
    siteQuery, matrixFilter, sortKey, sortDir, currentPage, pageSize,
    historyVisible, historyLoading, historySiteName, history,
    selectedSites,
    siteConfigs, siteBaseUrls, siteBaseUrl, siteConfig,
    allRows, inactiveSites, inactiveSiteCount,
    siteHasIssue, onSortChange, matrixRows, pagedRows, pageRange,
    reload, syncAll, syncOne, viewHistory, matrixRowClassName, handleSelectionChange,
  }
}
