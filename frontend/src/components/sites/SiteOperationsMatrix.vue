<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { dashboardApi, type SiteHistoryPoint, type SiteStatItem } from '@/api/dashboard'
import type { Site, SiteConnectivityStatus } from '@/api/site'
import GlassIcon from '@/components/GlassIcon.vue'
import SiteConnectivityStatusBadge from '@/components/SiteConnectivityStatus.vue'
import SiteFavicon from '@/components/SiteFavicon.vue'
import { safeUrl } from '@/utils/safeUrl'

type MatrixFilter = 'all' | 'issues' | 'connected' | 'disabled'
type MatrixLayout = 'wide' | 'balanced' | 'compact'

const props = withDefaults(
  defineProps<{
    sites: Site[]
    connectivity: Record<number, SiteConnectivityStatus>
    connectivityLoading?: boolean
    cookieAlerts?: readonly string[]
    loadingSites?: boolean
    testingSiteId?: number | null
    checkiningSiteId?: number | null
    canDelete?: boolean
  }>(),
  {
    connectivityLoading: false,
    cookieAlerts: () => [],
    loadingSites: false,
    testingSiteId: null,
    checkiningSiteId: null,
    canDelete: false,
  },
)

const emit = defineEmits<{
  create: []
  edit: [site: Site]
  remove: [site: Site]
  batchRemove: [sites: Site[]]
  test: [site: Site]
  checkin: [site: Site]
  detail: [site: Site]
}>()

const route = useRoute()
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
const panelRef = ref<HTMLElement | null>(null)
const panelWidth = ref(0)
const matrixLayout = computed<MatrixLayout>(() => {
  if (panelWidth.value >= 1450) return 'wide'
  if (panelWidth.value >= 1050) return 'balanced'
  return 'compact'
})
let resizeObserver: ResizeObserver | null = null
let visitPopup: Window | null = null
let visitCancelled = false

const historyVisible = ref(false)
const historyLoading = ref(false)
const historySiteName = ref('')
const history = ref<SiteHistoryPoint[]>([])

// 批量删除选中（跨页保留：reserve-selection + row-key=site_id）
const selectedSites = ref<Site[]>([])
function handleSelectionChange(rows: SiteStatItem[]) {
  selectedSites.value = rows.map((r) => siteConfig(r.site_id)).filter((s): s is Site => !!s)
}

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

function wait(duration: number) {
  return new Promise<void>((resolve) => window.setTimeout(resolve, duration))
}

async function visitInactiveSites(
  onProgress?: (current: number, total: number, siteName: string) => void,
): Promise<number> {
  const targets = [...inactiveSites.value]
  if (!targets.length) {
    ElMessage.info('没有最后访问超过 30 天的启用站点')
    return 0
  }

  const popup = window.open('about:blank', 'ptpatronus_inactive_sites')
  if (!popup) {
    ElMessage.error('浏览器阻止了访问窗口，请允许本站打开弹窗后重试')
    return 0
  }

  visitPopup = popup
  visitCancelled = false
  let visited = 0
  for (let index = 0; index < targets.length; index++) {
    if (visitCancelled || popup.closed) {
      ElMessage.warning(`访问任务已停止，已完成 ${visited} / ${targets.length} 个站点`)
      return visited
    }
    const { site } = targets[index]
    if (!site) continue
    const url = safeUrl(site.base_url)
    onProgress?.(index + 1, targets.length, site.name)
    try {
      popup.location.href = url
      popup.focus()
      await wait(8_000)
      visited++
      if (!popup.closed) popup.location.href = 'about:blank'
      await wait(250)
    } catch {
      // 单个站点导航失败时继续处理后续站点。
    }
  }

  if (!popup.closed) popup.close()
  visitPopup = null
  ElMessage.success(`已依次访问 ${visited} 个超过 30 天未访问的站点`)
  return visited
}

function showConnectivityReason(row: SiteStatItem) {
  const status = props.connectivity[row.site_id]
  if (!status || status.status === 'connected') return
  const title = status.status === 'disabled' ? `${row.site_name} 已停用` : `${row.site_name} 未联通`
  ElMessageBox.alert(status.error || '站点未返回具体失败原因', title, {
    type: status.status === 'disabled' ? 'warning' : 'error',
    confirmButtonText: '知道了',
  })
}

function openDetail(row: SiteStatItem) {
  const site = siteConfig(row.site_id)
  if (!site) {
    ElMessage.warning('该统计记录没有对应的站点配置')
    return
  }
  emit('detail', site)
}

function handleRowCommand(command: string, row: SiteStatItem) {
  if (command === 'refresh') {
    void syncOne(row)
    return
  }
  if (command === 'history') {
    void viewHistory(row)
    return
  }
  const site = siteConfig(row.site_id)
  if (!site) {
    ElMessage.warning('该统计记录没有对应的站点配置')
    return
  }
  if (command === 'detail') emit('detail', site)
  else if (command === 'checkin') emit('checkin', site)
  else if (command === 'test') emit('test', site)
  else if (command === 'edit') emit('edit', site)
  else if (command === 'remove') emit('remove', site)
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

function fmtBytes(value: number) {
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

function fmtTime(value: string) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  const pad = (part: number) => String(part).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

function fmtDate(value: string) {
  if (!value) return '-'
  return value.match(/^\d{4}-\d{2}-\d{2}/)?.[0] || value
}

function fmtNumber(value: number, digits = 0): string {
  if (!value) return '-'
  return value.toLocaleString('zh-CN', { maximumFractionDigits: digits })
}

function fmtRatio(value: number): string {
  return value > 0 ? value.toFixed(2) : '-'
}

function matrixRowClassName({ row }: { row: SiteStatItem }): string {
  if (siteHasIssue(row)) return 'matrix-row-issue'
  if (props.connectivity[row.site_id]?.status === 'disabled') return 'matrix-row-disabled'
  return ''
}

function routeFilter(value: unknown): MatrixFilter {
  return ['issues', 'connected', 'disabled'].includes(String(value)) ? (String(value) as MatrixFilter) : 'all'
}

watch(
  () => route.query.matrix,
  (value) => {
    matrixFilter.value = routeFilter(value)
  },
  { immediate: true },
)

watch([siteQuery, matrixFilter, sortKey, sortDir, pageSize], () => {
  currentPage.value = 1
})
watch(
  () => matrixRows.value.length,
  (length) => {
    currentPage.value = Math.min(currentPage.value, Math.max(1, Math.ceil(length / pageSize.value)))
  },
)

onMounted(() => {
  void reload()
  const panel = panelRef.value
  if (!panel) return
  panelWidth.value = panel.clientWidth
  resizeObserver = new ResizeObserver(([entry]) => {
    panelWidth.value = Math.round(entry.contentRect.width)
  })
  resizeObserver.observe(panel)
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  visitCancelled = true
  if (visitPopup && !visitPopup.closed) visitPopup.close()
  visitPopup = null
})

defineExpose({ reload, inactiveSiteCount, visitInactiveSites })
</script>

<template>
  <section ref="panelRef" class="operations-matrix glass">
    <div class="matrix-head">
      <div>
        <h3>站点运营矩阵</h3>
        <p>当前 {{ pageRange }} / {{ matrixRows.length }} 个匹配站点 · 共 {{ allRows.length }} 个站点</p>
      </div>
      <div class="matrix-tools">
        <el-input v-model="siteQuery" clearable placeholder="搜索站点、用户、等级" class="matrix-search" />
        <div class="matrix-filter" aria-label="站点状态筛选">
          <button type="button" :class="{ active: matrixFilter === 'all' }" @click="matrixFilter = 'all'">全部</button>
          <button type="button" :class="{ active: matrixFilter === 'issues' }" @click="matrixFilter = 'issues'">异常</button>
          <button type="button" :class="{ active: matrixFilter === 'connected' }" @click="matrixFilter = 'connected'">已联通</button>
          <button type="button" :class="{ active: matrixFilter === 'disabled' }" @click="matrixFilter = 'disabled'">已停用</button>
        </div>
        <el-button size="small" :loading="syncing" @click="syncAll">刷新统计</el-button>
        <el-button
          v-if="canDelete"
          size="small"
          type="danger"
          plain
          :disabled="!selectedSites.length"
          @click="emit('batchRemove', selectedSites)"
        >
          批量删除{{ selectedSites.length ? `(${selectedSites.length})` : '' }}
        </el-button>
      </div>
    </div>

    <div v-if="loadError" class="matrix-error">
      <span>{{ loadError }}</span>
      <button type="button" @click="reload">重试</button>
    </div>

    <el-table
      :key="matrixLayout"
      :data="pagedRows"
      class="matrix-table"
      :class="`is-${matrixLayout}`"
      style="width: 100%"
      empty-text="暂无匹配站点统计"
      :row-class-name="matrixRowClassName"
      :default-sort="{ prop: 'updated_at', order: 'descending' }"
      @sort-change="onSortChange"
      row-key="site_id"
      @selection-change="handleSelectionChange"
    >
      <el-table-column v-if="canDelete" type="selection" width="36" :reserve-selection="true" fixed="left" />
      <el-table-column type="expand" width="32" fixed="left">
        <template #default="{ row }">
          <div class="site-expand">
            <div class="exp-cell"><span>站点定义</span><b>{{ siteConfig(row.site_id)?.definition || '-' }}</b></div>
            <div class="exp-cell"><span>平台</span><b>{{ siteConfig(row.site_id)?.platform || '-' }}</b></div>
            <div class="exp-cell exp-address"><span>地址</span><b>{{ siteConfig(row.site_id)?.base_url || '-' }}</b></div>
            <div class="exp-cell">
              <span>配置状态</span>
              <b class="config-flags">
                <i :class="['config-state', siteConfig(row.site_id)?.is_active ? 'is-active' : 'is-disabled']">
                  {{ siteConfig(row.site_id)?.is_active ? '启用' : '停用' }}
                </i>
                <i v-if="cookieAlerts.includes(row.site_name)" class="config-state is-warning">Cookie 失效</i>
              </b>
            </div>
            <div class="exp-cell"><span>用户 ID</span><b>{{ row.user_id || '-' }}</b></div>
            <div class="exp-cell"><span>邮箱</span><b>{{ row.email || '-' }}</b></div>
            <div class="exp-cell"><span>真实上传</span><b>{{ row.true_uploaded > 0 ? fmtBytes(row.true_uploaded) : '-' }}</b></div>
            <div class="exp-cell"><span>真实下载</span><b>{{ row.true_downloaded > 0 ? fmtBytes(row.true_downloaded) : '-' }}</b></div>
            <div class="exp-cell"><span>唯一作品组</span><b>{{ fmtNumber(row.unique_groups) }}</b></div>
            <div class="exp-cell"><span>Perfect FLAC</span><b>{{ fmtNumber(row.perfect_flac) }}</b></div>
            <div class="exp-cell"><span>已邀请</span><b>{{ fmtNumber(row.invited) }}</b></div>
            <div class="exp-cell"><span>做种积分</span><b>{{ fmtNumber(row.seeding_bonus, 2) }}</b></div>
            <div class="exp-cell"><span>做种积分/h</span><b>{{ fmtNumber(row.seeding_bonus_per_hour, 2) }}</b></div>
            <div class="exp-cell"><span>抢救积分/h</span><b>{{ fmtNumber(row.rescue_seeding_bonus_per_hour, 2) }}</b></div>
            <div class="exp-cell"><span>认领数 / 上限</span><b>{{ row.claim_count || 0 }} / {{ row.max_claim || 0 }}</b></div>
          </div>
        </template>
      </el-table-column>

      <el-table-column v-if="matrixLayout === 'compact'" label="站点 / 状态" width="145" fixed="left">
        <template #default="{ row }">
          <div class="site-identity is-compact site-name-btn" role="button" @click="openDetail(row as SiteStatItem)">
            <SiteFavicon :name="row.site_name" :base-url="siteBaseUrl(row.site_id)" :size="26" />
            <span>
              <b>{{ row.site_name }}</b>
              <SiteConnectivityStatusBadge
                :status="connectivity[row.site_id]"
                :loading="connectivityLoading"
                @reason.stop="showConnectivityReason(row as SiteStatItem)"
              />
            </span>
          </div>
        </template>
      </el-table-column>
            <el-table-column
        v-if="matrixLayout !== 'compact'"
        prop="site_name"
        label="站点"
        min-width="160"
        fixed="left"
        sortable="custom"
      >
        <template #default="{ row }">
          <button type="button" class="site-identity site-name-btn" @click="openDetail(row as SiteStatItem)">
            <SiteFavicon :name="row.site_name" :base-url="siteBaseUrl(row.site_id)" :size="26" />
            <span>
              <b>{{ row.site_name }}</b>
              <small>{{ siteBaseUrl(row.site_id).replace(/^https?:\/\//, '').replace(/\/$/, '') || '未配置地址' }}</small>
            </span>
          </button>
        </template>
      </el-table-column>
<el-table-column
        v-if="matrixLayout !== 'compact'"
        prop="status"
        label="状态"
        :width="matrixLayout === 'wide' ? 84 : 80"
        fixed="left"
        sortable="custom"
      >
        <template #default="{ row }">
          <SiteConnectivityStatusBadge
            :status="connectivity[row.site_id]"
            :loading="connectivityLoading"
            @reason="showConnectivityReason(row as SiteStatItem)"
          />
        </template>
      </el-table-column>
      <el-table-column v-if="matrixLayout === 'compact'" label="用户 / 等级" min-width="100">
        <template #default="{ row }">
          <div class="stacked-stat">
            <div class="stacked-line"><span>用户</span><b :title="row.username">{{ row.username || '-' }}</b></div>
            <div class="stacked-line"><span>等级</span><b :title="row.level">{{ row.level || '-' }}</b></div>
          </div>
        </template>
      </el-table-column>
      <el-table-column
        v-else
        prop="username"
        label="用户名"
        :min-width="matrixLayout === 'wide' ? 90 : 70"
        show-overflow-tooltip
        sortable="custom"
      />
      <el-table-column
        v-if="matrixLayout !== 'compact'"
        prop="level"
        label="等级"
        :min-width="matrixLayout === 'wide' ? 72 : 64"
        show-overflow-tooltip
        sortable="custom"
      >
        <template #default="{ row }"><span class="level-tag">{{ row.level || '-' }}</span></template>
      </el-table-column>
      <el-table-column v-if="matrixLayout === 'wide'" prop="uploaded" label="上传" min-width="96" align="right" sortable="custom">
        <template #default="{ row }">{{ fmtBytes(row.uploaded) }}</template>
      </el-table-column>
      <el-table-column v-if="matrixLayout === 'wide'" prop="downloaded" label="下载" min-width="96" align="right" sortable="custom">
        <template #default="{ row }">{{ fmtBytes(row.downloaded) }}</template>
      </el-table-column>
      <el-table-column v-else label="上传 / 下载" min-width="100">
        <template #default="{ row }">
          <div class="stacked-stat">
            <div class="stacked-line"><span>上传</span><b>{{ fmtBytes(row.uploaded) }}</b></div>
            <div class="stacked-line"><span>下载</span><b>{{ fmtBytes(row.downloaded) }}</b></div>
          </div>
        </template>
      </el-table-column>
      <el-table-column v-if="matrixLayout === 'wide'" prop="true_ratio" label="真实分享率" min-width="82" align="right" sortable="custom">
        <template #default="{ row }"><b :class="row.true_ratio >= 1 ? 'ratio-good' : row.true_ratio > 0 ? 'ratio-risk' : ''">{{ fmtRatio(row.true_ratio) }}</b></template>
      </el-table-column>
      <el-table-column v-if="matrixLayout === 'wide'" prop="ratio" label="站点分享率" min-width="82" align="right" sortable="custom">
        <template #default="{ row }"><b :class="row.ratio >= 1 ? 'ratio-good' : row.ratio > 0 ? 'ratio-risk' : ''">{{ fmtRatio(row.ratio) }}</b></template>
      </el-table-column>
      <el-table-column v-else label="分享率" min-width="90">
        <template #default="{ row }">
          <div class="stacked-stat">
            <div class="stacked-line"><span>真实</span><b :class="row.true_ratio >= 1 ? 'ratio-good' : row.true_ratio > 0 ? 'ratio-risk' : ''">{{ fmtRatio(row.true_ratio) }}</b></div>
            <div class="stacked-line"><span>站点</span><b :class="row.ratio >= 1 ? 'ratio-good' : row.ratio > 0 ? 'ratio-risk' : ''">{{ fmtRatio(row.ratio) }}</b></div>
          </div>
        </template>
      </el-table-column>
      <el-table-column v-if="matrixLayout === 'compact'" label="做种 / 体积" min-width="100">
        <template #default="{ row }">
          <div class="stacked-stat">
            <div class="stacked-line"><span>做种</span><b>{{ fmtNumber(row.seeding) }}</b></div>
            <div class="stacked-line"><span>体积</span><b>{{ row.seeding_size > 0 ? fmtBytes(row.seeding_size) : '-' }}</b></div>
          </div>
        </template>
      </el-table-column>
      <el-table-column v-else prop="seeding" label="做种" :min-width="matrixLayout === 'wide' ? 50 : 44" align="right" sortable="custom" />
      <el-table-column
        v-if="matrixLayout !== 'compact'"
        prop="seeding_size"
        label="做种体积"
        :min-width="matrixLayout === 'wide' ? 90 : 80"
        align="right"
        sortable="custom"
      >
        <template #default="{ row }">{{ row.seeding_size > 0 ? fmtBytes(row.seeding_size) : '-' }}</template>
      </el-table-column>
      <el-table-column v-if="matrixLayout === 'compact'" label="发布 / 加入" min-width="100">
        <template #default="{ row }">
          <div class="stacked-stat">
            <div class="stacked-line"><span>发布</span><b>{{ fmtNumber(row.uploads) }}</b></div>
            <div class="stacked-line"><span>加入</span><b>{{ fmtDate(row.join_date) }}</b></div>
          </div>
        </template>
      </el-table-column>
      <el-table-column v-else prop="uploads" label="发布" :min-width="matrixLayout === 'wide' ? 50 : 44" align="right" sortable="custom">
        <template #default="{ row }">{{ fmtNumber(row.uploads) }}</template>
      </el-table-column>
      <el-table-column v-if="matrixLayout === 'wide'" prop="bonus" label="魔力" min-width="100" align="right" sortable="custom">
        <template #default="{ row }">{{ fmtNumber(row.bonus, 2) }}</template>
      </el-table-column>
      <el-table-column v-if="matrixLayout === 'wide'" prop="bonus_per_hour" label="时魔/h" min-width="72" align="right" sortable="custom">
        <template #default="{ row }">{{ fmtNumber(row.bonus_per_hour, 2) }}</template>
      </el-table-column>
      <el-table-column v-else label="魔力 / 时魔" min-width="100">
        <template #default="{ row }">
          <div class="stacked-stat">
            <div class="stacked-line"><span>魔力</span><b>{{ fmtNumber(row.bonus, 2) }}</b></div>
            <div class="stacked-line"><span>时魔/h</span><b>{{ fmtNumber(row.bonus_per_hour, 2) }}</b></div>
          </div>
        </template>
      </el-table-column>
      <el-table-column
        v-if="matrixLayout !== 'compact'"
        prop="join_date"
        label="加入"
        :min-width="matrixLayout === 'wide' ? 88 : 80"
        show-overflow-tooltip
        sortable="custom"
      >
        <template #default="{ row }">{{ fmtDate(row.join_date) }}</template>
      </el-table-column>
      <el-table-column v-if="matrixLayout === 'wide'" prop="last_access_at" label="最后访问" min-width="135" sortable="custom">
        <template #default="{ row }">{{ row.last_access_at ? fmtTime(row.last_access_at) : '-' }}</template>
      </el-table-column>
      <el-table-column v-if="matrixLayout === 'wide'" prop="updated_at" label="更新时间" min-width="135" sortable="custom">
        <template #default="{ row }">{{ row.updated_at ? fmtTime(row.updated_at) : '-' }}</template>
      </el-table-column>
      <el-table-column v-else label="访问 / 更新" min-width="140">
        <template #default="{ row }">
          <div class="stacked-stat">
            <div class="stacked-line"><span>访问</span><b>{{ row.last_access_at ? fmtTime(row.last_access_at) : '-' }}</b></div>
            <div class="stacked-line"><span>更新</span><b>{{ row.updated_at ? fmtTime(row.updated_at) : '-' }}</b></div>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="操作" :min-width="matrixLayout === 'wide' ? 68 : 60">
        <template #default="{ row }">
          <div class="row-actions">
            <button class="row-act" title="用户历史数据" @click="viewHistory(row as SiteStatItem)">
              <GlassIcon name="history" from="#2563eb" to="#0891b2" :size="16" />
            </button>
            <el-dropdown trigger="click" @command="handleRowCommand($event, row as SiteStatItem)">
              <button class="row-act" title="站点操作">
                <GlassIcon name="settings" from="#475569" to="#2563eb" :size="16" />
              </button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="detail">站点详情</el-dropdown-item>
                <el-dropdown-item command="refresh" :disabled="refreshing.has(row.site_id)">
                    {{ refreshing.has(row.site_id) ? '刷新中…' : '刷新统计' }}
                  </el-dropdown-item>
                  <el-dropdown-item
                    v-if="siteConfig(row.site_id)?.platform === 'nexusphp'"
                    command="checkin"
                    :disabled="checkiningSiteId === row.site_id"
                  >
                    {{ checkiningSiteId === row.site_id ? '签到中…' : '签到' }}
                  </el-dropdown-item>
                  <el-dropdown-item
                    v-if="siteConfig(row.site_id)"
                    command="test"
                    :disabled="testingSiteId === row.site_id"
                  >
                    {{ testingSiteId === row.site_id ? '测试中…' : '连接测试' }}
                  </el-dropdown-item>
                  <el-dropdown-item v-if="siteConfig(row.site_id)" command="edit">编辑配置</el-dropdown-item>
                  <el-dropdown-item v-if="canDelete && siteConfig(row.site_id)" command="remove" divided>删除站点</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </template>
      </el-table-column>
      <template #empty>
        <div class="matrix-empty">
          <strong>暂无站点</strong>
          <span>添加 PT 站点并配置 Cookie 或 API Key 后即可查看运营统计。</span>
          <el-button type="primary" size="small" @click="emit('create')">添加站点</el-button>
        </div>
      </template>
    </el-table>

    <div v-if="matrixRows.length > 8" class="matrix-pagination">
      <el-pagination
        v-model:current-page="currentPage"
        v-model:page-size="pageSize"
        :page-sizes="[8, 15, 30]"
        :total="matrixRows.length"
        layout="total, sizes, prev, pager, next"
        background
      />
    </div>

    <el-dialog v-model="historyVisible" :title="`${historySiteName} · 历史数据`" width="780px" append-to-body>
      <el-table :data="history" v-loading="historyLoading" max-height="480" empty-text="暂无历史快照" size="small">
        <el-table-column label="时间" min-width="135">
          <template #default="{ row }">{{ fmtTime(row.snap_at) }}</template>
        </el-table-column>
        <el-table-column label="等级" min-width="80" prop="level" />
        <el-table-column label="上传" min-width="95">
          <template #default="{ row }">{{ fmtBytes(row.uploaded) }}</template>
        </el-table-column>
        <el-table-column label="下载" min-width="95">
          <template #default="{ row }">{{ fmtBytes(row.downloaded) }}</template>
        </el-table-column>
        <el-table-column label="分享率" width="75">
          <template #default="{ row }">{{ fmtRatio(row.ratio) }}</template>
        </el-table-column>
        <el-table-column label="做种" width="60" prop="seeding" />
        <el-table-column label="发布" width="60" prop="uploads" />
        <el-table-column label="魔力" min-width="85">
          <template #default="{ row }">{{ fmtNumber(row.bonus) }}</template>
        </el-table-column>
      </el-table>
    </el-dialog>
  </section>
</template>

<style scoped>
.operations-matrix {
  --matrix-fixed-bg: rgba(249, 250, 255, 0.98);
  --matrix-fixed-head-bg: rgb(243, 245, 250);
  min-width: 0;
  overflow: hidden;
}
:global(.dark) .operations-matrix {
  --matrix-fixed-bg: rgba(30, 32, 44, 0.98);
  --matrix-fixed-head-bg: rgb(35, 37, 49);
}
.matrix-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 15px 16px 12px;
}
.matrix-head h3 { margin: 0; font-size: 15px; }
.matrix-head p { margin: 3px 0 0; color: var(--text-muted); font-size: 11px; }
.matrix-tools {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}
.matrix-search { width: 200px; }
.matrix-sort { width: 125px; }
.matrix-filter {
  display: inline-flex;
  padding: 2px;
  border: 1px solid var(--divider);
  border-radius: 6px;
  background: var(--surface-muted);
}
.matrix-filter button {
  min-height: 26px;
  padding: 0 9px;
  border: 0;
  border-radius: 4px;
  color: var(--text-muted);
  background: transparent;
  font: inherit;
  font-size: 11px;
  cursor: pointer;
}
.matrix-filter button.active {
  color: #2563eb;
  background: var(--glass-bg-strong);
  font-weight: 650;
}
.matrix-error {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 9px 16px;
  border-top: 1px solid rgba(220, 38, 38, 0.15);
  color: #dc2626;
  background: rgba(220, 38, 38, 0.06);
  font-size: 12px;
}
.matrix-error button { border: 0; color: #2563eb; background: transparent; cursor: pointer; }
.matrix-table {
  border-top: 1px solid var(--divider);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}
:deep(.matrix-table .el-table__header th) {
  height: 42px;
  color: var(--text-muted);
  background: rgba(148, 163, 184, 0.08) !important;
  font-size: 11px;
  font-weight: 650;
}
:deep(.matrix-table .el-table__cell) { padding: 8px 0; }
:deep(.matrix-table .cell) { padding-right: 6px; padding-left: 6px; }
:deep(.matrix-table.is-wide .cell) { padding-right: 8px; padding-left: 8px; }
:deep(.matrix-table.is-compact .cell) { padding-right: 5px; padding-left: 5px; }
:deep(.matrix-table .el-table__fixed) { background: transparent; }
:deep(.matrix-table td.el-table-fixed-column--left) { background: var(--matrix-fixed-bg) !important; }
:deep(.matrix-table th.el-table-fixed-column--left) { background: var(--matrix-fixed-head-bg) !important; }
:deep(.matrix-row-issue td:first-child) { box-shadow: inset 3px 0 #dc2626; }
:deep(.matrix-row-issue .site-favicon) { filter: saturate(0.45); }
:deep(.matrix-row-disabled) { opacity: 0.68; }
.site-identity { display: flex; align-items: center; gap: 6px; min-width: 0; }
.site-identity > span {
  display: flex;
  align-items: flex-start;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
  overflow: hidden;
}
.site-identity b,
.site-identity small {
  display: block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.site-identity b { font-size: 12px; }
.site-identity small { color: var(--text-muted); font-size: 9px; }
.site-identity.is-compact :deep(.connectivity-status) {
  min-height: 20px;
  padding: 1px 5px;
  gap: 4px;
  font-size: 11px;
  line-height: 16px;
}
.level-tag {
  display: block;
  max-width: 100%;
  padding: 1px 4px;
  overflow: hidden;
  border-radius: 4px;
  color: #2563eb;
  background: rgba(37, 99, 235, 0.1);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ratio-good { color: #15803d; font-weight: 650; }
.ratio-risk { color: #dc2626; font-weight: 650; }
.stacked-stat { display: flex; flex-direction: column; gap: 4px; width: 100%; }
.stacked-line {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
  min-width: 0;
  white-space: nowrap;
}
.stacked-line + .stacked-line { padding-top: 4px; border-top: 1px solid var(--divider); }
.stacked-line > span { flex: 0 0 auto; color: var(--text-muted); font-size: 10px; }
.stacked-line > b {
  min-width: 0;
  overflow: hidden;
  font-size: 12px;
  font-weight: 600;
  text-overflow: ellipsis;
}
.site-expand {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(145px, 1fr));
  gap: 12px 20px;
  padding: 14px 20px 16px 52px;
  background: rgba(148, 163, 184, 0.06);
}
.exp-cell { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.exp-cell span { color: var(--text-muted); font-size: 11px; }
.exp-cell b { overflow-wrap: anywhere; font-size: 12px; }
.exp-address { grid-column: span 2; }
.config-flags { display: flex; align-items: center; flex-wrap: wrap; gap: 5px; }
.config-state {
  display: inline-flex;
  align-items: center;
  min-height: 20px;
  padding: 0 6px;
  border-radius: 4px;
  font-size: 10px;
  font-style: normal;
  font-weight: 600;
}
.config-state.is-active { color: #15803d; background: rgba(22, 163, 74, 0.1); }
.config-state.is-disabled { color: var(--text-muted); background: var(--surface-muted); }
.config-state.is-warning { color: #b45309; background: rgba(217, 119, 6, 0.12); }
.row-actions { display: flex; align-items: center; justify-content: center; gap: 4px; }
.row-act {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  padding: 0;
  border: 0;
  border-radius: 6px;
  background: transparent;
  cursor: pointer;
}
.row-act:hover { background: rgba(37, 99, 235, 0.1); }
.row-act:disabled { opacity: 0.5; cursor: progress; }
:deep(.matrix-table td:last-child .cell) { padding: 0 4px; }
.matrix-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 28px 16px;
  color: var(--text-muted);
}
.matrix-empty strong { color: var(--text-strong); font-size: 14px; }
.matrix-empty span { font-size: 12px; }
.matrix-pagination {
  display: flex;
  justify-content: flex-end;
  padding: 12px 16px 14px;
  border-top: 1px solid var(--divider);
}

@media (max-width: 900px) {
  .matrix-head { align-items: flex-start; flex-direction: column; }
  .matrix-tools { width: 100%; justify-content: flex-start; }
}
@media (max-width: 640px) {
  .matrix-search { width: 100%; }
  .matrix-filter { max-width: 100%; overflow-x: auto; }
  .matrix-filter button { flex: 0 0 auto; }
  .matrix-pagination { justify-content: flex-start; overflow-x: auto; }
  :deep(.matrix-table .el-table-fixed-column--left) {
    position: static !important;
    right: auto !important;
    left: auto !important;
    box-shadow: none !important;
  }
  :deep(.matrix-table td.el-table-fixed-column--left) { background: transparent !important; }
  :deep(.matrix-table th.el-table-fixed-column--left) { background: rgba(148, 163, 184, 0.08) !important; }
}
.site-name-btn {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  border: none;
  background: transparent;
  color: var(--text-strong);
  cursor: pointer;
  padding: 0;
  font: inherit;
  font-weight: 600;
  width: 100%;
  text-align: left;
}
.site-name-btn:hover { color: #6c5ce7; }
</style>
