<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { dashboardApi, type DashboardOverview, type SiteHistoryPoint, type SiteStatItem, type DashboardTrendPoint } from '@/api/dashboard'
import { siteApi, type Site } from '@/api/site'
import { useWs } from '@/composables/useWs'
import { useAuthStore } from '@/stores/auth'
import type { DashboardShareData } from '@/utils/dashboardImage'
import Sparkline from '@/components/dashboard/Sparkline.vue'
import DashboardExportDialog from '@/components/dashboard/DashboardExportDialog.vue'
import EmptyState from '@/components/EmptyState.vue'

type SyncLog = {
  id: number
  time: string
  site: string
  status: 'info' | 'running' | 'success' | 'error'
  message: string
}

const overview = ref<DashboardOverview | null>(null)
const trend = ref<DashboardTrendPoint[]>([])
// sparkline 数据源：近 14 天跨站聚合趋势（指标卡内嵌迷你折线 + 日环比 + 导出图片共用）。
const loading = ref(false)
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
// 单站刷新中的 site_id 集合（操作列「刷新数据」按钮 loading）
const refreshing = reactive(new Set<number>())

// WS 实时告警：H&R 危险种子 / Cookie 失效站点（后端 hr-alert / cookie-alert 推送）。
const { hrAlerts, cookieAlerts } = useWs()
const auth = useAuthStore()

async function load() {
  loading.value = true
  try {
    // 并行拉 overview 与趋势，减少等待（对齐移动端 Future.wait）。
    const [ov, tr] = await Promise.all([dashboardApi.overview(), dashboardApi.trend(14)])
    overview.value = ov
    trend.value = tr
  } catch (e: unknown) {
    // 仅在真出错（网络/5xx）时进 catch（无数据是 200 正常返回，不抛）。提示而非静默，
    // 避免用户面对全 0 指标卡分不清是「暂无数据」还是「加载失败」。
    ElMessage.error((e as Error).message || '仪表盘加载失败')
  } finally {
    loading.value = false
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

// 指标卡 sparkline 数据：各字段近 14 天序列 + 共享日期轴（hover tooltip 用）。
const spark = computed(() => ({
  uploaded: trend.value.map((p) => p.uploaded),
  downloaded: trend.value.map((p) => p.downloaded),
  seeding: trend.value.map((p) => p.seeding),
  seeding_size: trend.value.map((p) => p.seeding_size),
  bonus: trend.value.map((p) => p.bonus),
  days: trend.value.map((p) => p.day),
}))
function fmtPct(p: number | null): string {
  if (p === null) return ''
  return `${p >= 0 ? '+' : ''}${p.toFixed(1)}%`
}

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

    trend.value = await dashboardApi.trend(14).catch(() => trend.value)
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
    sites: overview.value.sites.map((s) => ({
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

// 操作列「刷新数据」：仅重新抓取该站点，刷新成功后用返回的总览替换。
async function syncOne(row: SiteStatItem) {
  refreshing.add(row.site_id)
  try {
    overview.value = await dashboardApi.syncOne(row.site_id)
    ElMessage.success(`${row.site_name} 已刷新`)
  } catch (e: unknown) {
    ElMessage.warning(`${row.site_name} 刷新失败：${errorText(e)}`)
  } finally {
    refreshing.delete(row.site_id)
  }
}

// 操作列「用户历史数据」：打开弹窗加载该站点历史快照。
const historyVisible = ref(false)
const historyLoading = ref(false)
const historySiteName = ref('')
const history = ref<SiteHistoryPoint[]>([])

async function viewHistory(row: SiteStatItem) {
  historySiteName.value = row.site_name
  historyVisible.value = true
  history.value = []
  historyLoading.value = true
  try {
    history.value = await dashboardApi.history(row.site_id, 90)
  } catch {
    ElMessage.warning('历史数据加载失败')
  } finally {
    historyLoading.value = false
  }
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

onMounted(load)
</script>

<template>
  <div class="dashboard">
    <div class="page-head">
      <h2 class="page-title">仪表盘</h2>
      <div class="head-actions">
        <el-button :disabled="!overview" @click="openExport">
          <GlassIcon name="download" from="#6c5ce7" to="#4285f4" :size="16" class="btn-ico" />
          <span>导出图片</span>
        </el-button>
        <el-button :loading="syncing" @click="syncStats">
          <GlassIcon name="seeding" from="#11998e" to="#38ef7d" :size="16" class="btn-ico" />
          <span>{{ syncing ? (syncTotal ? `刷新中 ${syncDone}/${syncTotal}` : '准备刷新') : '刷新统计' }}</span>
        </el-button>
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

    <!-- Cookie 失效告警卡（最高优先级：签到/统计/H&R 自动化已停摆） -->
    <div v-if="cookieAlerts.length" class="alert-card glass alert-danger">
      <div class="alert-head">
        <GlassIcon name="alert" from="#e74c3c" to="#c0392b" :size="22" />
        <span class="alert-title">Cookie 失效：{{ cookieAlerts.length }} 个站点</span>
      </div>
      <p class="alert-desc">以下站点 cookie 已失效，签到 / 统计 / H&R 自动化已停摆，请尽快重新登录：</p>
      <div class="chip-wrap">
        <span v-for="s in cookieAlerts" :key="s" class="danger-chip">{{ s }}</span>
      </div>
    </div>

    <!-- H&R 告警卡：危险种子列表（站点 / 标题 / 状态 / 剩余考核时间） -->
    <div v-if="hrAlerts.length" class="alert-card glass alert-danger">
      <div class="alert-head">
        <GlassIcon name="alert" from="#ff6b6b" to="#e74c3c" :size="22" />
        <span class="alert-title">H&amp;R 告警：{{ hrAlerts.length }} 条需关注</span>
      </div>
      <ul class="hr-list">
        <li v-for="(a, i) in hrAlerts.slice(0, 5)" :key="i">
          <span class="hr-dot"></span>
          <span class="hr-title">{{ a.title }}</span>
          <span class="hr-meta">{{ a.site }} · {{ a.status }}<template v-if="a.deadline"> · 剩 {{ a.deadline }}</template></span>
        </li>
      </ul>
      <p v-if="hrAlerts.length > 5" class="hr-more">…及其它 {{ hrAlerts.length - 5 }} 条</p>
    </div>

    <!-- 指标卡片墙：首屏加载用骨架占位，避免 spinner 抖动 -->
    <div v-if="initialLoading" class="metric-grid">
      <div v-for="i in 5" :key="i" class="metric-card glass">
        <div class="metric-top"><el-skeleton-item variant="circle" style="width: 40px; height: 40px" /></div>
        <el-skeleton-item variant="h1" style="width: 60%; margin-top: 10px" />
        <el-skeleton-item variant="text" style="width: 40%; margin-top: 8px" />
        <el-skeleton-item variant="rect" style="width: 100%; height: 36px; margin-top: 10px" />
      </div>
    </div>
    <div v-else class="metric-grid">
      <div class="metric-card glass glass-hover">
        <div class="metric-top">
          <GlassIcon name="upload" from="#2ecc71" to="#1abc9c" :size="40" />
          <span v-if="upPct !== null" class="trend-badge" :class="upPct >= 0 ? 'trend-up' : 'trend-down'">{{ fmtPct(upPct) }}</span>
        </div>
        <div class="metric-value">{{ fmtBytes(overview?.totals.uploaded ?? 0) }}</div>
        <div class="metric-label text-muted">总上传</div>
        <Sparkline :values="spark.uploaded" color="#6c5ce7" unit="bytes" :days="spark.days" />
      </div>
      <div class="metric-card glass glass-hover">
        <div class="metric-top">
          <GlassIcon name="download" from="#4285f4" to="#22d3ee" :size="40" />
          <span v-if="dlPct !== null" class="trend-badge" :class="dlPct >= 0 ? 'trend-up' : 'trend-down'">{{ fmtPct(dlPct) }}</span>
        </div>
        <div class="metric-value">{{ fmtBytes(overview?.totals.downloaded ?? 0) }}</div>
        <div class="metric-label text-muted">总下载</div>
        <Sparkline :values="spark.downloaded" color="#4285f4" unit="bytes" :days="spark.days" />
      </div>
      <div class="metric-card glass glass-hover">
        <div class="metric-top"><GlassIcon name="seeding" from="#38ef7d" to="#11998e" :size="40" /></div>
        <div class="metric-value">{{ overview?.totals.seeding ?? 0 }}</div>
        <div class="metric-label text-muted">总做种</div>
        <Sparkline :values="spark.seeding" color="#38ef7d" unit="count" :days="spark.days" />
      </div>
      <div class="metric-card glass glass-hover">
        <div class="metric-top"><GlassIcon name="backup" from="#43cea2" to="#185a9d" :size="40" /></div>
        <div class="metric-value">{{ fmtBytes(overview?.totals.seeding_size ?? 0) }}</div>
        <div class="metric-label text-muted">总做种体积</div>
        <Sparkline :values="spark.seeding_size" color="#43cea2" unit="bytes" :days="spark.days" />
      </div>
      <div class="metric-card glass glass-hover">
        <div class="metric-top"><GlassIcon name="bonus" from="#f7971e" to="#ffd200" :size="40" /></div>
        <div class="metric-value">{{ (overview?.totals.bonus ?? 0).toFixed(0) }}</div>
        <div class="metric-label text-muted">总魔力</div>
        <Sparkline :values="spark.bonus" color="#f7971e" unit="count" :days="spark.days" />
      </div>
    </div>

    <div v-if="!loading && overview && !overview.sites.length" class="setup-card glass">
      <EmptyState
        icon="🧭"
        title="还没有可展示的站点数据"
        description="先添加 PT 站点并配置下载器，再刷新统计。之后这里会汇总上传、下载、魔力、做种和 H&R 风险。"
      >
        <router-link to="/sites"><el-button type="primary">添加站点</el-button></router-link>
        <router-link to="/downloaders"><el-button>添加下载器</el-button></router-link>
      </EmptyState>
    </div>

    <!-- 站点明细 -->
    <div class="sites-card glass">
      <div class="card-head">
        <span>站点明细</span>
        <span class="text-muted">站点 {{ overview?.site_count ?? 0 }} · 下载器 {{ overview?.downloader_count ?? 0 }}</span>
      </div>
      <el-table :data="overview?.sites ?? []" style="width: 100%" empty-text="暂无数据，等待统计同步">
        <el-table-column type="expand">
          <template #default="{ row }">
            <div class="site-expand">
              <div class="exp-cell"><span class="exp-lbl">发布数</span><span class="exp-val">{{ row.uploads > 0 ? row.uploads : '-' }}</span></div>
              <div class="exp-cell"><span class="exp-lbl">做种体积</span><span class="exp-val">{{ row.seeding_size > 0 ? fmtBytes(row.seeding_size) : '-' }}</span></div>
              <div class="exp-cell"><span class="exp-lbl">下载中</span><span class="exp-val">{{ row.leeching }}</span></div>
              <div class="exp-cell"><span class="exp-lbl">魔力</span><span class="exp-val">{{ row.bonus > 0 ? row.bonus.toFixed(0) : '-' }}</span></div>
              <div class="exp-cell"><span class="exp-lbl">时魔/时</span><span class="exp-val">{{ row.bonus_per_hour > 0 ? row.bonus_per_hour.toFixed(1) : '-' }}</span></div>
              <div class="exp-cell"><span class="exp-lbl">加入时间</span><span class="exp-val">{{ row.join_date || '-' }}</span></div>
              <div class="exp-cell"><span class="exp-lbl">更新时间</span><span class="exp-val">{{ row.updated_at ? fmtTime(row.updated_at) : '-' }}</span></div>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="site_name" label="站点" min-width="100" fixed="left" />
        <el-table-column label="等级" min-width="80">
          <template #default="{ row }">
            <span class="level-tag">{{ row.level || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="上传" min-width="100">
          <template #default="{ row }">{{ fmtBytes(row.uploaded) }}</template>
        </el-table-column>
        <el-table-column label="下载" min-width="100">
          <template #default="{ row }">{{ fmtBytes(row.downloaded) }}</template>
        </el-table-column>
        <el-table-column label="分享率" width="80">
          <template #default="{ row }">
            <span :class="row.ratio >= 1 ? 'trend-up' : 'trend-down'">{{ row.ratio > 0 ? row.ratio.toFixed(2) : '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="seeding" label="做种" width="70" />
        <el-table-column label="操作" width="110" fixed="right">
          <template #default="{ row }">
            <div class="row-actions">
              <button
                class="row-act"
                title="用户历史数据"
                @click="viewHistory(row as SiteStatItem)"
              >
                <GlassIcon name="history" from="#4285f4" to="#22d3ee" :size="16" />
              </button>
              <button
                class="row-act"
                :disabled="refreshing.has(row.site_id)"
                :title="refreshing.has(row.site_id) ? '刷新中…' : '刷新数据'"
                @click="syncOne(row as SiteStatItem)"
              >
                <GlassIcon name="refresh" from="#11998e" to="#38ef7d" :size="16" />
              </button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <!-- 用户历史数据弹窗 -->
    <el-dialog v-model="historyVisible" :title="`${historySiteName} · 历史数据`" width="780px" append-to-body>
      <el-table :data="history" v-loading="historyLoading" max-height="480" empty-text="暂无历史快照" size="small">
        <el-table-column label="时间" min-width="135">
          <template #default="{ row }">{{ fmtTime(row.snap_at) }}</template>
        </el-table-column>
        <el-table-column label="等级" min-width="80">
          <template #default="{ row }">{{ row.level || '-' }}</template>
        </el-table-column>
        <el-table-column label="上传" min-width="95">
          <template #default="{ row }">{{ fmtBytes(row.uploaded) }}</template>
        </el-table-column>
        <el-table-column label="下载" min-width="95">
          <template #default="{ row }">{{ fmtBytes(row.downloaded) }}</template>
        </el-table-column>
        <el-table-column label="分享率" width="75">
          <template #default="{ row }">{{ row.ratio > 0 ? row.ratio.toFixed(2) : '-' }}</template>
        </el-table-column>
        <el-table-column label="做种" width="60">
          <template #default="{ row }">{{ row.seeding || '-' }}</template>
        </el-table-column>
        <el-table-column label="发布" width="60">
          <template #default="{ row }">{{ row.uploads > 0 ? row.uploads : '-' }}</template>
        </el-table-column>
        <el-table-column label="魔力" min-width="85">
          <template #default="{ row }">{{ row.bonus > 0 ? row.bonus.toFixed(0) : '-' }}</template>
        </el-table-column>
      </el-table>
    </el-dialog>

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
.level-tag {
  display: inline-block;
  padding: 1px 8px;
  border-radius: 10px;
  font-size: 12px;
  background: rgba(120, 160, 255, 0.16);
  color: #6ea8fe;
  white-space: nowrap;
}
/* 展开行：次要字段（发布数/做种体积/下载中/魔力/时魔/时间）紧凑网格 */
.site-expand {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
  gap: 10px 24px;
  padding: 4px 16px 10px 50px;
}
.exp-cell { display: flex; flex-direction: column; gap: 2px; }
.exp-lbl { font-size: var(--fs-xs); color: var(--text-muted); }
.exp-val { font-size: var(--fs-md); font-weight: 600; }
:deep(.el-table), :deep(.el-table tr), :deep(.el-table td), :deep(.el-table th) {
  background-color: transparent !important;
}
.row-actions { display: flex; align-items: center; gap: 6px; }
.row-act {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  padding: 0;
  border: none;
  border-radius: 8px;
  background: transparent;
  cursor: pointer;
  transition: background 0.15s ease;
}
.row-act:hover { background: rgba(108, 92, 231, 0.14); }
.row-act:disabled { opacity: 0.5; cursor: progress; }

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
</style>
