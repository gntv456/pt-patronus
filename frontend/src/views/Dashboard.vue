<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { dashboardApi, type DashboardOverview, type SiteStatItem, type DashboardTrendPoint } from '@/api/dashboard'
import { siteApi, type Site, type SiteConnectivityStatus } from '@/api/site'
import { useWs } from '@/composables/useWs'
import { useAuthStore } from '@/stores/auth'
import type { DashboardShareData } from '@/utils/dashboardImage'
import DashboardTrendChart from '@/components/dashboard/DashboardTrendChart.vue'
import DashboardExportDialog from '@/components/dashboard/DashboardExportDialog.vue'
import EmptyState from '@/components/EmptyState.vue'
import SiteFavicon from '@/components/SiteFavicon.vue'
import DashboardRealtimeCard from '@/components/dashboard/DashboardRealtimeCard.vue'
import DashboardResourceCard from '@/components/dashboard/DashboardResourceCard.vue'
import DashboardAutomationCard from '@/components/dashboard/DashboardAutomationCard.vue'
import DashboardAICard from '@/components/dashboard/DashboardAICard.vue'
import DashboardTaskCards from '@/components/dashboard/DashboardTaskCards.vue'
import DashboardActivityTimeline from '@/components/dashboard/DashboardActivityTimeline.vue'
import DashboardRingChart, { type RingSegment } from '@/components/dashboard/DashboardRingChart.vue'

type SyncLog = {
  id: number
  time: string
  site: string
  status: 'info' | 'running' | 'success' | 'error'
  message: string
}

type TrendMode = 'traffic' | 'seeding' | 'size' | 'bonus'
type ActionTone = 'danger' | 'warning' | 'info'
type DashboardAction = {
  key: string
  tone: ActionTone
  site: string
  title: string
  detail: string
  siteId?: number
}

const overview = ref<DashboardOverview | null>(null)
const trend = ref<DashboardTrendPoint[]>([])
const trendDays = ref<7 | 30 | 90>(30)
const trendMode = ref<TrendMode>('traffic')
const trendLoading = ref(false)
const loading = ref(false)
// 加载失败持久态：overview 为空且出错时显示带重试的错误条，而非全 0 指标卡（避免静默）。
const loadError = ref('')
// 首屏加载（无 overview）：用骨架屏占位指标卡，替代整页 spinner，避免布局抖动。
const initialLoading = computed(() => loading.value && !overview.value)
const syncing = ref(false)
const syncTotal = ref(0)
const syncDone = ref(0)
const syncSuccess = ref(0)
const syncCurrent = ref('')
const syncErrors = ref<{ site: string; error: string }[]>([])
const syncLogs = ref<SyncLog[]>([])
const exportVisible = ref(false)
const allSites = ref<Site[]>([]) // 站点列表(含 base_url)，导出时按 id join favicon 用
const connectivity = ref<Record<number, SiteConnectivityStatus>>({})
const connectivityLoading = ref(false)

// 区③④⑥ 组件 ref：syncStats 完成后触发刷新自动化/AI/时间线数据。
const autoCard = ref<{ refresh: () => Promise<void> } | null>(null)
const aiCard = ref<{ refresh: () => Promise<void> } | null>(null)
const taskCards = ref<{ refresh: () => Promise<void> } | null>(null)
const timelineCard = ref<{ refresh: () => Promise<void> } | null>(null)

// WS 实时告警：H&R 危险种子 / Cookie 失效站点（后端 hr-alert / cookie-alert 推送）；
// notices（实时通知）供仪表盘未读计数；下载器速率由 RealtimeCard 自取 useWs 单例。
const { hrAlerts, cookieAlerts, notices } = useWs()
const auth = useAuthStore()
const router = useRouter()

function openSiteMatrix(filter: 'all' | 'issues' | 'disabled') {
  void router.push({ name: 'sites', query: { matrix: filter } })
}

async function loadConnectivity() {
  connectivityLoading.value = true
  const [statusResult, sitesResult] = await Promise.allSettled([siteApi.statuses(), siteApi.list()])
  if (statusResult.status === 'fulfilled') {
    connectivity.value = Object.fromEntries(statusResult.value.map((status) => [status.site_id, status]))
  } else {
    connectivity.value = {}
  }
  if (sitesResult.status === 'fulfilled') {
    allSites.value = sitesResult.value
  }
  connectivityLoading.value = false
}

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    // 统计和趋势先加载，联通检查在后台执行，避免慢站点拖住仪表盘首屏。
    const [ov, tr] = await Promise.all([dashboardApi.overview(), dashboardApi.trend(trendDays.value)])
    overview.value = ov
    trend.value = tr
  } catch (e: unknown) {
    // 仅在真出错（网络/5xx）时进 catch（无数据是 200 正常返回，不抛）。提示而非静默，
    // 避免用户面对全 0 指标卡分不清是「暂无数据」还是「加载失败」。
    loadError.value = (e as Error).message || '仪表盘加载失败'
    ElMessage.error(loadError.value)
  } finally {
    loading.value = false
  }
  void loadConnectivity()
}

async function changeTrendDays(days: 7 | 30 | 90) {
  if (trendDays.value === days || trendLoading.value) return
  trendDays.value = days
  trendLoading.value = true
  try {
    trend.value = await dashboardApi.trend(days)
  } catch (e: unknown) {
    ElMessage.warning(`趋势加载失败：${errorText(e)}`)
  } finally {
    trendLoading.value = false
  }
}

// 日环比百分比：最后一天 vs 前一天；数据不足或分母为 0 返回 null（对齐移动端 _pct）。
function pctOf(pick: (p: DashboardTrendPoint) => number): number | null {
  if (trend.value.length < 2) return null
  const last = pick(trend.value[trend.value.length - 1])
  const prev = pick(trend.value[trend.value.length - 2])
  if (!prev) return null
  return ((last - prev) / prev) * 100
}
const upPct = computed(() => pctOf((p) => p.uploaded))
const dlPct = computed(() => pctOf((p) => p.downloaded))
const syncPercent = computed(() => (syncTotal.value ? Math.round((syncDone.value / syncTotal.value) * 100) : 0))
function fmtPct(p: number | null): string {
  if (p === null) return ''
  return `${p >= 0 ? '+' : ''}${p.toFixed(1)}%`
}

const trendSeries = computed(() => {
  if (trendMode.value === 'traffic') {
    return [
      { label: '上传总量', color: '#0f766e', values: trend.value.map((point) => point.uploaded) },
      { label: '下载总量', color: '#2563eb', values: trend.value.map((point) => point.downloaded) },
    ]
  }
  if (trendMode.value === 'seeding') {
    return [{ label: '做种数量', color: '#16a34a', values: trend.value.map((point) => point.seeding) }]
  }
  if (trendMode.value === 'size') {
    return [{ label: '做种体积', color: '#0891b2', values: trend.value.map((point) => point.seeding_size) }]
  }
  return [{ label: '魔力', color: '#d97706', values: trend.value.map((point) => point.bonus) }]
})
const trendUnit = computed<'bytes' | 'count'>(() =>
  trendMode.value === 'traffic' || trendMode.value === 'size' ? 'bytes' : 'count',
)
const trendDaysAxis = computed(() => trend.value.map((point) => point.day))

const metricItems = computed(() => [
  { label: '总上传', value: fmtBytes(overview.value?.totals.uploaded ?? 0), change: fmtPct(upPct.value), tone: 'teal' },
  { label: '总下载', value: fmtBytes(overview.value?.totals.downloaded ?? 0), change: fmtPct(dlPct.value), tone: 'blue' },
  { label: '真实分享率', value: (overview.value?.totals.true_ratio ?? 0) > 0 ? overview.value!.totals.true_ratio.toFixed(2) : '-', change: '', tone: 'indigo' },
  { label: '总做种', value: (overview.value?.totals.seeding ?? 0).toLocaleString('zh-CN'), change: fmtBytes(overview.value?.totals.seeding_size ?? 0), tone: 'green' },
  { label: '总魔力', value: Math.round(overview.value?.totals.bonus ?? 0).toLocaleString('zh-CN'), change: `${(overview.value?.totals.bonus_per_hour ?? 0).toFixed(1)}/h`, tone: 'amber' },
])

const siteBaseUrls = computed(() => new Map(allSites.value.map((site) => [site.id, site.base_url])))
function siteBaseUrl(siteId: number): string {
  return siteBaseUrls.value.get(siteId) || ''
}

const connectivityValues = computed(() => Object.values(connectivity.value))
const connectedCount = computed(() => connectivityValues.value.filter((item) => item.status === 'connected').length)
const disconnectedCount = computed(() => connectivityValues.value.filter((item) => item.status === 'disconnected').length)
const disabledCount = computed(() => connectivityValues.value.filter((item) => item.status === 'disabled').length)
const configuredSiteCount = computed(() => allSites.value.length || overview.value?.site_count || 0)

// 区②站点健康分布环形图分段 + 未读通知数（WS notices 近似；下拉刷新以通知中心为准）。
const healthSegments = computed<RingSegment[]>(() => [
  { label: '在线', value: connectedCount.value, color: '#16a34a' },
  { label: '未联通', value: disconnectedCount.value, color: '#dc2626' },
  { label: '已停用', value: disabledCount.value, color: '#94a3b8' },
])
const unreadCount = computed(() => notices.value.filter((n) => !n.read).length)

const dashboardActions = computed<DashboardAction[]>(() => {
  const actions: DashboardAction[] = []
  for (const site of cookieAlerts.value) {
    actions.push({ key: `cookie-${site}`, tone: 'danger', site, title: 'Cookie 已失效', detail: '登录态失效，统计与自动化已暂停' })
  }
  for (const status of connectivityValues.value) {
    if (status.status !== 'disconnected' || cookieAlerts.value.includes(status.site_name)) continue
    actions.push({ key: `connect-${status.site_id}`, tone: 'danger', site: status.site_name, title: '站点未联通', detail: status.error || '未返回具体原因', siteId: status.site_id })
  }
  hrAlerts.value.forEach((alert, index) => {
    actions.push({
      key: `hr-${index}`,
      tone: 'warning',
      site: alert.site,
      title: alert.title,
      detail: `${alert.status}${alert.deadline ? ` · 剩 ${alert.deadline}` : ''}`,
    })
  })
  for (const item of syncErrors.value) {
    actions.push({ key: `sync-${item.site}`, tone: 'info', site: item.site, title: '统计同步失败', detail: item.error })
  }
  return actions
})
const totalIssueCount = computed(() => dashboardActions.value.length)

function showAction(action: DashboardAction) {
  if (action.siteId) {
    const row = overview.value?.sites.find((site) => site.site_id === action.siteId)
    if (row) {
      showConnectivityReason(row)
      return
    }
  }
  ElMessageBox.alert(action.detail, `${action.site} · ${action.title}`, {
    type: action.tone === 'danger' ? 'error' : action.tone === 'warning' ? 'warning' : 'info',
    confirmButtonText: '知道了',
  })
}

const latestUpdatedAt = computed(() => {
  let latest = 0
  for (const site of overview.value?.sites ?? []) latest = Math.max(latest, new Date(site.updated_at).getTime() || 0)
  return latest
})
const latestUpdatedText = computed(() => {
  if (!latestUpdatedAt.value) return '尚未同步'
  const seconds = Math.max(0, Math.floor((Date.now() - latestUpdatedAt.value) / 1000))
  if (seconds < 60) return '刚刚更新'
  if (seconds < 3600) return `${Math.floor(seconds / 60)} 分钟前更新`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)} 小时前更新`
  return fmtTime(new Date(latestUpdatedAt.value).toISOString())
})

function errorText(e: unknown): string {
  return (e as Error)?.message || '未知错误'
}

let syncLogSeq = 0
function addSyncLog(status: SyncLog['status'], site: string, message: string) {
  syncLogs.value.unshift({
    id: ++syncLogSeq,
    time: new Date().toLocaleTimeString('zh-CN', { hour12: false }),
    site,
    status,
    message,
  })
}

// 逐站抓取启用站点统计：显示进度与失败站点，避免整批长请求超时后只看到笼统失败。
async function syncStats() {
  if (syncing.value) return
  syncing.value = true
  syncTotal.value = 0
  syncDone.value = 0
  syncSuccess.value = 0
  syncCurrent.value = '读取站点列表'
  syncErrors.value = []
  syncLogs.value = []
  addSyncLog('info', '刷新统计', '正在读取启用站点列表')
  try {
    const sites = await siteApi.list()
    allSites.value = sites
    const activeSites = sites.filter((s) => s.is_active)
    syncTotal.value = activeSites.length
    if (!activeSites.length) {
      addSyncLog('error', '刷新统计', '没有启用的站点可同步')
      ElMessage.warning('没有启用的站点可同步')
      return
    }
    addSyncLog('info', '刷新统计', `共 ${activeSites.length} 个启用站点，开始逐站同步`)

    for (const site of activeSites) {
      syncCurrent.value = site.name
      const started = performance.now()
      addSyncLog('running', site.name, '开始同步')
      try {
        const ov = await dashboardApi.syncOne(site.id)
        overview.value = ov
        syncSuccess.value += 1
        const cost = Math.max(0.1, (performance.now() - started) / 1000).toFixed(1)
        const item = ov.sites.find((s) => s.site_id === site.id)
        const stat = item
          ? `上传 ${fmtBytes(item.uploaded)}，下载 ${fmtBytes(item.downloaded)}，做种 ${item.seeding}，分享率 ${item.ratio > 0 ? item.ratio.toFixed(2) : '-'}`
          : '已写入最新统计'
        addSyncLog('success', site.name, `同步成功，用时 ${cost}s，${stat}`)
      } catch (e: unknown) {
        const msg = errorText(e)
        syncErrors.value.push({ site: site.name, error: msg })
        addSyncLog('error', site.name, `同步失败：${msg}`)
      } finally {
        syncDone.value += 1
      }
    }

    trend.value = await dashboardApi.trend(trendDays.value).catch(() => trend.value)
    if (syncErrors.value.length) {
      const msg = `同步完成：成功 ${syncSuccess.value} 个，失败 ${syncErrors.value.length} 个`
      addSyncLog(syncSuccess.value ? 'info' : 'error', '刷新统计', msg)
      if (syncSuccess.value) ElMessage.warning(msg)
      else ElMessage.error(`${msg}，请查看下方失败原因`)
    } else {
      addSyncLog('success', '刷新统计', `全部同步成功：${syncSuccess.value} 个站点`)
      ElMessage.success(`统计已同步：${syncSuccess.value} 个站点`)
    }
  } catch (e: unknown) {
    addSyncLog('error', '刷新统计', `同步失败：${errorText(e)}`)
    ElMessage.error(`同步失败：${errorText(e)}`)
  } finally {
    syncing.value = false
    syncCurrent.value = ''
    void loadConnectivity()
    // 站点统计刷新后，联动刷新自动化/AI/活动时间线卡片。
    autoCard.value?.refresh()
    aiCard.value?.refresh()
    taskCards.value?.refresh()
    timelineCard.value?.refresh()
  }
}

// 打开导出配置弹窗：首次拉站点列表(含 base_url)缓存，供每站 favicon + 站点白名单选项。
async function openExport() {
  if (!allSites.value.length) {
    try {
      allSites.value = await siteApi.list()
    } catch {
      ElMessage.warning('站点列表加载失败')
    }
  }
  exportVisible.value = true
}

// 组装导出数据（全量 sites join base_url，排序/筛选交给导出配置 DashboardExportConfig）。
const exportData = computed<DashboardShareData | null>(() => {
  if (!overview.value) return null
  const bu = new Map(allSites.value.map((s) => [s.id, s.base_url]))
  return {
    username: auth.username,
    siteCount: overview.value.site_count,
    downloaderCount: overview.value.downloader_count,
    totals: {
      uploaded: overview.value.totals.uploaded,
      downloaded: overview.value.totals.downloaded,
      seeding: overview.value.totals.seeding,
      seeding_size: overview.value.totals.seeding_size,
      bonus: overview.value.totals.bonus,
    },
    sites: (overview.value.sites ?? []).map((s) => ({
      site_id: s.site_id,
      site_name: s.site_name,
      username: s.username,
      user_id: s.user_id,
      email: s.email,
      level: s.level,
      uploaded: s.uploaded,
      downloaded: s.downloaded,
      ratio: s.ratio,
      seeding: s.seeding,
      leeching: s.leeching,
      seeding_size: s.seeding_size,
      uploads: s.uploads,
      bonus: s.bonus,
      bonus_per_hour: s.bonus_per_hour,
      join_date: s.join_date,
      updated_at: s.updated_at,
      base_url: bu.get(s.site_id) || '',
    })),
  }
})

function showConnectivityReason(row: SiteStatItem) {
  const status = connectivity.value[row.site_id]
  if (!status || status.status === 'connected') return
  const title = status.status === 'disabled' ? `${row.site_name} 已停用` : `${row.site_name} 未联通`
  ElMessageBox.alert(status.error || '站点未返回具体失败原因', title, {
    type: 'error',
    confirmButtonText: '知道了',
  })
}

function fmtBytes(n: number) {
  if (!n) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let i = 0
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++ }
  return `${n.toFixed(2)} ${u[i]}`
}

// 格式化更新时间为「YYYY-MM-DD HH:mm」。
function fmtTime(s: string) {
  if (!s) return '-'
  const d = new Date(s)
  if (isNaN(d.getTime())) return s
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

function siteConfigByName(name: string): Site | undefined {
  return allSites.value.find((site) => site.name === name)
}

onMounted(load)
</script>

<template>
  <div class="dashboard">
    <div class="page-head">
      <div class="page-heading">
        <h2 class="page-title">仪表盘</h2>
        <span class="page-subtitle">站点运行与账号增长</span>
      </div>
      <div class="head-actions">
        <span class="updated-at">{{ latestUpdatedText }}</span>
        <div class="period-control" aria-label="趋势周期">
          <button
            v-for="days in ([7, 30, 90] as const)"
            :key="days"
            type="button"
            :class="{ active: trendDays === days }"
            :disabled="trendLoading"
            @click="changeTrendDays(days)"
          >
            {{ days }}天
          </button>
        </div>
        <el-button :disabled="!overview" @click="openExport">
          <GlassIcon name="download" from="#2563eb" to="#0f766e" :size="16" class="btn-ico" />
          <span>导出图片</span>
        </el-button>
        <el-button type="primary" :loading="syncing" @click="syncStats">
          <GlassIcon name="refresh" from="#ffffff" to="#dbeafe" :size="16" class="btn-ico" />
          <span>{{ syncing ? (syncTotal ? `刷新中 ${syncDone}/${syncTotal}` : '准备刷新') : '刷新数据' }}</span>
        </el-button>
      </div>
    </div>

    <div class="health-strip ops-surface">
      <button type="button" class="health-main" @click="openSiteMatrix('all')">
        <span class="health-icon health-ok"><i /></span>
        <span>
          <small>站点健康</small>
          <strong v-if="connectivityLoading">检测中</strong>
          <strong v-else>{{ connectedCount }} / {{ configuredSiteCount }} 联通</strong>
        </span>
      </button>
      <button type="button" class="health-item is-danger" @click="openSiteMatrix('issues')">
        <span class="status-dot" />
        <span>未联通</span>
        <b>{{ disconnectedCount }}</b>
      </button>
      <button type="button" class="health-item is-muted" @click="openSiteMatrix('disabled')">
        <span class="status-dot" />
        <span>已停用</span>
        <b>{{ disabledCount }}</b>
      </button>
      <div class="health-item is-success">
        <span class="status-dot" />
        <span>下载器配置</span>
        <b>{{ overview?.downloader_count ?? 0 }}</b>
      </div>
      <div class="health-item" :class="totalIssueCount ? 'is-warning' : 'is-success'">
        <span class="status-dot" />
        <span>待处理</span>
        <b>{{ totalIssueCount }}</b>
      </div>
    </div>

    <div v-if="syncing || syncErrors.length" class="sync-card glass" :class="{ 'has-error': syncErrors.length && !syncing }">
      <div class="sync-head">
        <div class="sync-title">
          <GlassIcon name="refresh" from="#11998e" to="#38ef7d" :size="22" />
          <div>
            <strong v-if="syncing">正在刷新统计 {{ syncDone }}/{{ syncTotal || '—' }}</strong>
            <strong v-else>刷新完成：成功 {{ syncSuccess }} 个，失败 {{ syncErrors.length }} 个</strong>
            <p v-if="syncing">{{ syncCurrent ? `当前：${syncCurrent}` : '正在准备同步任务' }}</p>
            <p v-else>失败原因已列出，可单独刷新对应站点或检查 Cookie / 站点适配。</p>
          </div>
        </div>
        <span class="sync-percent">{{ syncPercent }}%</span>
      </div>
      <el-progress
        :percentage="syncPercent"
        :status="syncErrors.length && !syncing ? 'exception' : undefined"
        :stroke-width="8"
      />
      <div v-if="syncLogs.length" class="sync-log-box">
        <div class="sync-log-title">同步日志</div>
        <ul class="sync-logs">
          <li v-for="log in syncLogs" :key="log.id" :class="`log-${log.status}`">
            <span class="log-time">{{ log.time }}</span>
            <b>{{ log.site }}</b>
            <span>{{ log.message }}</span>
          </li>
        </ul>
      </div>
      <ul v-if="syncErrors.length" class="sync-errors">
        <li v-for="err in syncErrors" :key="err.site">
          <b>{{ err.site }}</b>
          <span>{{ err.error }}</span>
        </li>
      </ul>
    </div>

    <!-- 加载失败：overview 为空且出错时显示带重试的错误条，而非全 0 指标卡 -->
    <div v-if="loadError && !overview && !loading" class="ops-surface load-error">
      <span class="le-msg">仪表盘加载失败：{{ loadError }}</span>
      <el-button size="small" @click="load">重试</el-button>
    </div>

    <div class="operations-grid">
      <section class="trend-panel ops-surface" v-loading="trendLoading">
        <div class="section-head">
          <div>
            <h3>账号增长趋势</h3>
            <p>跨站聚合数据 · 最近 {{ trendDays }} 天</p>
          </div>
          <div class="metric-switch" aria-label="趋势指标">
            <button type="button" :class="{ active: trendMode === 'traffic' }" @click="trendMode = 'traffic'">上传/下载</button>
            <button type="button" :class="{ active: trendMode === 'seeding' }" @click="trendMode = 'seeding'">做种数</button>
            <button type="button" :class="{ active: trendMode === 'size' }" @click="trendMode = 'size'">做种体积</button>
            <button type="button" :class="{ active: trendMode === 'bonus' }" @click="trendMode = 'bonus'">魔力</button>
          </div>
        </div>

        <div v-if="initialLoading" class="metric-strip">
          <div v-for="i in 5" :key="i" class="metric-item">
            <el-skeleton-item variant="text" style="width: 52%" />
            <el-skeleton-item variant="h1" style="width: 74%; margin-top: 8px" />
          </div>
        </div>
        <div v-else class="metric-strip">
          <div v-for="item in metricItems" :key="item.label" class="metric-item" :class="`tone-${item.tone}`">
            <span>{{ item.label }}</span>
            <strong>{{ item.value }}</strong>
            <small v-if="item.change">{{ item.change }}</small>
          </div>
        </div>

        <div class="chart-legend">
          <span v-for="item in trendSeries" :key="item.label">
            <i :style="{ backgroundColor: item.color }" />{{ item.label }}
          </span>
        </div>
        <DashboardTrendChart :days="trendDaysAxis" :series="trendSeries" :unit="trendUnit" />
      </section>

      <aside class="action-panel ops-surface">
        <div class="section-head action-heading">
          <div>
            <h3>待处理事项</h3>
            <p>{{ totalIssueCount ? `${totalIssueCount} 项需要关注` : '当前运行正常' }}</p>
          </div>
          <span class="issue-counter" :class="{ empty: !totalIssueCount }">{{ totalIssueCount }}</span>
        </div>
        <div v-if="dashboardActions.length" class="action-list">
          <button
            v-for="action in dashboardActions.slice(0, 6)"
            :key="action.key"
            type="button"
            class="action-row"
            @click="showAction(action)"
          >
            <SiteFavicon
              :name="action.site"
              :base-url="siteConfigByName(action.site)?.base_url"
              :size="30"
            />
            <span class="action-copy">
              <b>{{ action.site }}</b>
              <span>{{ action.title }}</span>
              <small>{{ action.detail }}</small>
            </span>
            <i class="action-severity" :class="`tone-${action.tone}`" />
          </button>
        </div>
        <div v-else class="action-empty">
          <span class="empty-check">✓</span>
          <strong>暂无待处理事项</strong>
          <p>站点连接、Cookie 和 H&amp;R 状态正常</p>
        </div>
        <router-link to="/sites" class="action-footer">前往站点管理</router-link>
      </aside>
    </div>

    <!-- 区② 实时运维：站点健康分布 · 下载器实时速率 · 系统资源 -->
    <div class="zone zone-3col">
      <section class="ops-surface zone-card">
        <div class="zc-head"><h3>站点健康</h3><p>联通状态分布</p></div>
        <DashboardRingChart
          :items="healthSegments"
          :size="116"
          :thickness="13"
          :center-value="String(connectedCount)"
          center-label="在线"
        />
        <div class="health-foot">未读通知<b>{{ unreadCount }}</b></div>
      </section>
      <section class="ops-surface zone-card"><DashboardRealtimeCard /></section>
      <section class="ops-surface zone-card"><DashboardResourceCard /></section>
    </div>

    <!-- 区②.5 任务卡片（menu-redesign §5.4 D 类全局总览入口） -->
    <div class="zone">
      <section class="ops-surface zone-card">
        <DashboardTaskCards ref="taskCards" />
      </section>
    </div>

    <!-- 区③④ 自动化运行 · AI 用量与成本 -->
    <div class="zone zone-2col">
      <section class="ops-surface zone-card"><DashboardAutomationCard ref="autoCard" /></section>
      <section class="ops-surface zone-card"><DashboardAICard ref="aiCard" /></section>
    </div>

    <!-- 区⑥ 最近活动时间线 -->
    <div class="zone">
      <section class="ops-surface zone-card"><DashboardActivityTimeline ref="timelineCard" /></section>
    </div>

    <div v-if="!loading && overview && !(overview.sites?.length)" class="setup-card ops-surface">
      <EmptyState
        icon="🧭"
        title="还没有可展示的站点数据"
        description="先添加 PT 站点并配置下载器，再刷新统计。之后这里会汇总上传、下载、魔力、做种和 H&R 风险。"
      >
        <router-link to="/sites"><el-button type="primary">添加站点</el-button></router-link>
        <router-link to="/downloaders"><el-button>添加下载器</el-button></router-link>
      </EmptyState>
    </div>

    <!-- 导出图片配置弹窗（字段/主题/脱敏/背景图/冠军/站点筛选，配置持久化） -->
    <DashboardExportDialog v-model="exportVisible" :data="exportData" />
  </div>
</template>

<style scoped>
.dashboard { display: flex; flex-direction: column; gap: 18px; }
.page-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.head-actions { display: flex; gap: 8px; }
.page-title { margin: 4px 0 6px; font-size: 22px; font-weight: 700; }
.btn-ico { margin-right: 4px; vertical-align: middle; }
.metric-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(210px, 1fr)); gap: 16px; }
.metric-card { padding: 20px; }
.setup-card { padding: 6px; }
.setup-card a { text-decoration: none; }
.metric-top { display: flex; align-items: center; justify-content: space-between; }
.metric-icon { font-size: 20px; }
.metric-value { font-size: 28px; font-weight: 700; letter-spacing: -0.5px; }
.metric-label { margin-top: 4px; font-size: 14px; }
.sites-card { padding: 20px; }
.card-head { display: flex; align-items: baseline; gap: 12px; margin-bottom: 8px; font-weight: 600; font-size: 15px; }
:deep(.el-table), :deep(.el-table tr), :deep(.el-table td), :deep(.el-table th) {
  background-color: transparent !important;
}

.sync-card {
  padding: 14px 16px;
  border-left: 3px solid #11998e;
}
.sync-card.has-error {
  border-left-color: #e74c3c;
}
.sync-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  margin-bottom: 10px;
}
.sync-title {
  display: flex;
  align-items: flex-start;
  gap: 9px;
  min-width: 0;
}
.sync-title strong {
  display: block;
  font-size: 14px;
  color: var(--text-strong);
}
.sync-title p {
  margin: 4px 0 0;
  font-size: 12.5px;
  color: var(--text-muted);
  overflow-wrap: anywhere;
}
.sync-percent {
  flex: 0 0 auto;
  font-size: 13px;
  font-weight: 700;
  color: var(--text-muted);
}
.sync-log-box {
  margin-top: 12px;
}
.sync-log-title {
  margin-bottom: 6px;
  font-size: 12px;
  font-weight: 700;
  color: var(--text-muted);
}
.sync-logs {
  list-style: none;
  margin: 0;
  padding: 0;
  max-height: 220px;
  overflow: auto;
  display: grid;
  gap: 5px;
}
.sync-logs li {
  display: grid;
  grid-template-columns: 72px minmax(92px, 150px) minmax(0, 1fr);
  align-items: start;
  gap: 8px;
  padding: 7px 9px;
  border-radius: 8px;
  background: rgba(148, 163, 184, 0.08);
  font-size: 12.5px;
}
.sync-logs .log-time {
  color: var(--text-muted);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
}
.sync-logs b {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sync-logs span:last-child {
  color: var(--text-muted);
  overflow-wrap: anywhere;
}
.sync-logs .log-running {
  background: rgba(66, 133, 244, 0.1);
}
.sync-logs .log-running b {
  color: #4285f4;
}
.sync-logs .log-success {
  background: rgba(17, 153, 142, 0.1);
}
.sync-logs .log-success b {
  color: #11998e;
}
.sync-logs .log-error {
  background: rgba(231, 76, 60, 0.1);
}
.sync-logs .log-error b {
  color: #e74c3c;
}
.sync-errors {
  list-style: none;
  margin: 10px 0 0;
  padding: 0;
  display: grid;
  gap: 6px;
}
.sync-errors li {
  display: grid;
  grid-template-columns: minmax(90px, 160px) minmax(0, 1fr);
  gap: 10px;
  padding: 7px 9px;
  border-radius: 8px;
  background: rgba(231, 76, 60, 0.08);
  font-size: 12.5px;
}
.sync-errors b {
  color: #e74c3c;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sync-errors span {
  color: var(--text-muted);
  overflow-wrap: anywhere;
}

/* 告警卡：Cookie 失效 / H&R 危险（最高优先级，红色左边框） */
.alert-card { padding: 16px 18px; border-left: 3px solid #e74c3c; }
.alert-head { display: flex; align-items: center; gap: 8px; }
.alert-title { font-weight: 700; font-size: 15px; color: #e74c3c; }
.alert-desc { margin: 8px 0 10px; font-size: 12.5px; color: var(--text-muted); }
.chip-wrap { display: flex; flex-wrap: wrap; gap: 6px; }
.danger-chip {
  padding: 2px 10px;
  border-radius: 999px;
  font-size: 12.5px;
  color: #fff;
  background: #e74c3c;
  white-space: nowrap;
}
.hr-list { list-style: none; margin: 8px 0 0; padding: 0; }
.hr-list li { display: flex; align-items: baseline; gap: 8px; padding: 5px 0; }
.hr-dot {
  width: 6px; height: 6px; border-radius: 50%; background: #ff6b6b;
  flex: none; transform: translateY(3px);
}
.hr-title {
  font-weight: 600; font-size: 13.5px;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 46%;
}
.hr-meta { font-size: 12px; color: var(--text-muted); white-space: nowrap; }
.hr-more { margin: 6px 0 0; font-size: 12px; color: var(--text-muted); }

/* 日环比徽标（总上传/总下载卡） */
.trend-badge {
  font-size: 12.5px;
  font-weight: 600;
  padding: 1px 7px;
  border-radius: 999px;
}
.trend-badge.trend-up { color: #2ecc71; background: rgba(46, 204, 113, 0.14); }
.trend-badge.trend-down { color: #ff6b6b; background: rgba(255, 107, 107, 0.14); }

@media (max-width: 640px) {
  .sync-head {
    flex-direction: column;
  }
  .sync-errors li {
    grid-template-columns: 1fr;
  }
  .sync-logs li {
    grid-template-columns: 68px 1fr;
  }
  .sync-logs li span:last-child {
    grid-column: 1 / -1;
  }
}
/* 分享率列：主值（实际分享率优先，否则站点上报）+ 站点上报副值；主值按 ≥1 着色。 */
.ratio-cell { display: flex; flex-direction: column; line-height: 1.2; }
.ratio-cell .trend-up { color: #2ecc71; }
.ratio-cell .trend-down { color: #ff6b6b; }
.ratio-cell .ratio-sub { font-size: 11px; color: var(--text-muted); }
.load-error { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-3); padding: var(--sp-3) var(--sp-4); margin-bottom: var(--sp-3); }
.load-error .le-msg { color: var(--text-strong); }
</style>

<style scoped>
/* 运行控制台视觉层：克制的实色面、细边框与紧凑数据密度。 */
.dashboard {
  gap: 14px;
  letter-spacing: 0;
}
.ops-surface,
.sync-card {
  border: 1px solid rgba(148, 163, 184, 0.24);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.88);
  box-shadow: 0 5px 18px rgba(15, 23, 42, 0.06);
  backdrop-filter: none;
}
:global(:root.dark) .ops-surface,
:global(:root.dark) .sync-card {
  border-color: rgba(148, 163, 184, 0.16);
  background: rgba(30, 32, 44, 0.92);
  box-shadow: 0 6px 20px rgba(0, 0, 0, 0.22);
}
.page-head {
  align-items: center;
  min-height: 46px;
}
.page-heading {
  display: flex;
  align-items: baseline;
  gap: 10px;
  min-width: 0;
}
.page-title {
  margin: 0;
  font-size: 22px;
  line-height: 1.2;
}
.page-subtitle,
.updated-at {
  color: var(--text-muted);
  font-size: 12px;
  white-space: nowrap;
}
.head-actions {
  align-items: center;
  flex-wrap: wrap;
  justify-content: flex-end;
}
.period-control,
.metric-switch {
  display: inline-flex;
  align-items: center;
  padding: 2px;
  border: 1px solid var(--divider);
  border-radius: 6px;
  background: var(--surface-muted);
}
.period-control button,
.metric-switch button {
  min-height: 28px;
  padding: 4px 10px;
  border: 0;
  border-radius: 4px;
  color: var(--text-muted);
  background: transparent;
  font: inherit;
  font-size: 12px;
  line-height: 18px;
  cursor: pointer;
}
.period-control button.active,
.metric-switch button.active {
  color: #2563eb;
  background: var(--glass-bg-strong);
  box-shadow: 0 1px 4px rgba(15, 23, 42, 0.08);
  font-weight: 650;
}
.period-control button:disabled {
  cursor: wait;
  opacity: 0.6;
}

.health-strip {
  display: grid;
  grid-template-columns: minmax(220px, 1.45fr) repeat(4, minmax(126px, 1fr));
  min-height: 68px;
  overflow: hidden;
}
.health-strip button {
  font: inherit;
  cursor: pointer;
}
.health-main,
.health-item {
  display: flex;
  align-items: center;
  gap: 9px;
  min-width: 0;
  padding: 12px 16px;
  border: 0;
  border-right: 1px solid var(--divider);
  color: var(--text-strong);
  background: transparent;
  text-align: left;
}
.health-item:last-child {
  border-right: 0;
}
.health-main:hover,
.health-item:hover {
  background: var(--surface-muted);
}
.health-main > span:last-child {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.health-main small {
  color: var(--text-muted);
  font-size: 11px;
}
.health-main strong {
  overflow: hidden;
  font-size: 16px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.health-icon {
  position: relative;
  width: 30px;
  height: 30px;
  flex: 0 0 30px;
  border-radius: 6px;
  background: rgba(22, 163, 74, 0.12);
}
.health-icon i {
  position: absolute;
  inset: 8px;
  border: 2px solid #16a34a;
  border-radius: 50%;
}
.health-icon i::after {
  position: absolute;
  top: 2px;
  left: 4px;
  width: 3px;
  height: 6px;
  border-right: 2px solid #16a34a;
  border-bottom: 2px solid #16a34a;
  content: '';
  transform: rotate(45deg);
}
.health-item span:not(.status-dot) {
  overflow: hidden;
  color: var(--text-muted);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.health-item b {
  margin-left: auto;
  font-size: 17px;
}
.status-dot {
  width: 8px;
  height: 8px;
  flex: 0 0 8px;
  border-radius: 50%;
  background: #64748b;
}
.health-item.is-danger b { color: #dc2626; }
.health-item.is-danger .status-dot { background: #dc2626; }
.health-item.is-warning b { color: #d97706; }
.health-item.is-warning .status-dot { background: #d97706; }
.health-item.is-success b { color: #16a34a; }
.health-item.is-success .status-dot { background: #16a34a; }
.health-item.is-muted b { color: var(--text-muted); }

.operations-grid {
  display: grid;
  grid-template-columns: minmax(0, 2fr) minmax(310px, 0.82fr);
  gap: 14px;
  align-items: stretch;
}
.trend-panel,
.action-panel {
  min-width: 0;
  padding: 16px;
}
.section-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 14px;
}
.section-head h3 {
  margin: 0;
  font-size: 15px;
  line-height: 1.35;
}
.section-head p {
  margin: 3px 0 0;
  color: var(--text-muted);
  font-size: 11px;
}
.metric-switch {
  flex: 0 0 auto;
}
.metric-strip {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  margin-top: 14px;
  border-top: 1px solid var(--divider);
  border-bottom: 1px solid var(--divider);
}
.metric-item {
  position: relative;
  min-width: 0;
  padding: 12px 12px 11px;
  border-right: 1px solid var(--divider);
}
.metric-item:last-child { border-right: 0; }
.metric-item::before {
  position: absolute;
  top: 13px;
  left: 0;
  width: 3px;
  height: 28px;
  border-radius: 0 2px 2px 0;
  background: #64748b;
  content: '';
}
.metric-item.tone-teal::before { background: #0f766e; }
.metric-item.tone-blue::before { background: #2563eb; }
.metric-item.tone-indigo::before { background: #4f46e5; }
.metric-item.tone-green::before { background: #16a34a; }
.metric-item.tone-amber::before { background: #d97706; }
.metric-item > span {
  display: block;
  overflow: hidden;
  color: var(--text-muted);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.metric-item strong {
  display: block;
  overflow: hidden;
  margin-top: 4px;
  font-size: 18px;
  line-height: 1.2;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.metric-item small {
  display: block;
  overflow: hidden;
  margin-top: 3px;
  color: var(--text-muted);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.chart-legend {
  display: flex;
  justify-content: flex-end;
  gap: 14px;
  min-height: 18px;
  margin: 10px 8px -4px 0;
  color: var(--text-muted);
  font-size: 11px;
}
.chart-legend span { display: inline-flex; align-items: center; gap: 5px; }
.chart-legend i { width: 7px; height: 7px; border-radius: 50%; }

.action-panel {
  display: flex;
  flex-direction: column;
  max-height: 430px;
}
.action-heading { align-items: center; }
.issue-counter {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  color: #b45309;
  background: rgba(217, 119, 6, 0.13);
  font-size: 12px;
  font-weight: 700;
}
.issue-counter.empty {
  color: #15803d;
  background: rgba(22, 163, 74, 0.12);
}
.action-list {
  flex: 1;
  min-height: 0;
  margin-top: 10px;
  overflow-y: auto;
}
.action-row {
  display: grid;
  grid-template-columns: 30px minmax(0, 1fr) 8px;
  align-items: center;
  gap: 9px;
  width: 100%;
  padding: 9px 4px;
  border: 0;
  border-bottom: 1px solid var(--divider);
  color: var(--text-strong);
  background: transparent;
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.action-row:hover { background: var(--surface-muted); }
.action-copy { min-width: 0; }
.action-copy b,
.action-copy span,
.action-copy small {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.action-copy b { font-size: 12px; }
.action-copy span { margin-top: 1px; font-size: 12px; }
.action-copy small { margin-top: 2px; color: var(--text-muted); font-size: 10px; }
.action-severity { width: 7px; height: 7px; border-radius: 50%; }
.action-severity.tone-danger { background: #dc2626; }
.action-severity.tone-warning { background: #d97706; }
.action-severity.tone-info { background: #2563eb; }
.action-empty {
  display: flex;
  flex: 1;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 210px;
  color: var(--text-muted);
  text-align: center;
}
.empty-check {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  margin-bottom: 8px;
  border-radius: 50%;
  color: #16a34a;
  background: rgba(22, 163, 74, 0.12);
  font-weight: 700;
}
.action-empty strong { color: var(--text-strong); font-size: 13px; }
.action-empty p { margin: 4px 0 0; font-size: 11px; }
.action-footer {
  display: block;
  margin-top: 10px;
  color: #2563eb;
  font-size: 12px;
  text-align: center;
  text-decoration: none;
}

.setup-card { padding: 6px; }
.setup-card a { text-decoration: none; }

@media (max-width: 1180px) {
  .health-strip { grid-template-columns: minmax(200px, 1.25fr) repeat(4, minmax(108px, 1fr)); }
  .operations-grid { grid-template-columns: minmax(0, 1fr) 290px; }
  .metric-strip { grid-template-columns: repeat(3, minmax(0, 1fr)); }
  .metric-item:nth-child(3) { border-right: 0; }
  .metric-item:nth-child(n + 4) { border-top: 1px solid var(--divider); }
}
@media (max-width: 900px) {
  .page-head { align-items: flex-start; flex-direction: column; }
  .head-actions { width: 100%; justify-content: flex-start; }
  .updated-at { margin-right: auto; }
  .health-strip { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .health-main { grid-column: 1 / -1; }
  .health-item:nth-child(3) { border-right: 0; }
  .operations-grid { grid-template-columns: 1fr; }
  .action-panel { max-height: none; }
  .action-list { max-height: 330px; }
}
@media (max-width: 640px) {
  .page-heading { align-items: flex-start; flex-direction: column; gap: 2px; }
  .head-actions { gap: 6px; }
  .updated-at { width: 100%; }
  .period-control { order: 2; }
  .health-main,
  .health-item { padding: 10px 12px; }
  .section-head { align-items: flex-start; flex-direction: column; }
  .metric-switch { width: 100%; overflow-x: auto; }
  .metric-switch button { flex: 1 0 auto; }
  .metric-strip { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .metric-item:nth-child(2n) { border-right: 0; }
  .metric-item:nth-child(3) { border-right: 1px solid var(--divider); }
  .metric-item:nth-child(n + 3) { border-top: 1px solid var(--divider); }
  .metric-item:last-child { grid-column: 1 / -1; border-right: 0; }
}

/* —— 仪表盘新增区（实时运维 / 自动化 / 活动时间线）布局 —— */
.zone {
  display: grid;
  gap: 14px;
}
.zone-3col {
  grid-template-columns: minmax(0, 0.85fr) minmax(0, 1fr) minmax(0, 1fr);
}
.zone-2col {
  grid-template-columns: minmax(0, 1.3fr) minmax(0, 1fr);
}
.zone-card {
  min-width: 0;
  padding: 16px;
}
.zc-head {
  margin-bottom: 12px;
}
.zc-head h3 {
  margin: 0;
  font-size: 15px;
}
.zc-head p {
  margin: 3px 0 0;
  color: var(--text-muted);
  font-size: 11px;
}
.health-foot {
  margin-top: 12px;
  padding-top: 10px;
  border-top: 1px solid var(--divider);
  font-size: 12px;
  color: var(--text-muted);
}
.health-foot b {
  margin-left: 4px;
  font-size: 14px;
  color: var(--text-strong);
}
@media (max-width: 1180px) {
  .zone-3col {
    grid-template-columns: 1fr 1fr;
  }
  .zone-3col > section:first-child {
    grid-column: 1 / -1;
  }
}
@media (max-width: 900px) {
  .zone-3col,
  .zone-2col {
    grid-template-columns: 1fr;
  }
}
</style>
