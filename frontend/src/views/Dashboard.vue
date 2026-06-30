<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { dashboardApi, type DashboardOverview, type SiteHistoryPoint, type SiteStatItem, type DashboardTrendPoint } from '@/api/dashboard'
import { siteApi, type Site } from '@/api/site'
import { useWs } from '@/composables/useWs'
import { useAuthStore } from '@/stores/auth'
import type { DashboardShareData } from '@/utils/dashboardImage'
import Sparkline from '@/components/dashboard/Sparkline.vue'
import DashboardExportDialog from '@/components/dashboard/DashboardExportDialog.vue'

const overview = ref<DashboardOverview | null>(null)
const trend = ref<DashboardTrendPoint[]>([])
// sparkline 数据源：近 14 天跨站聚合趋势（指标卡内嵌迷你折线 + 日环比 + 导出图片共用）。
const loading = ref(false)
const syncing = ref(false)
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

// 立即抓取所有启用站点统计：解决「加入站点后数据全为 0」——不必等后端 30 分钟定时同步。
async function syncStats() {
  syncing.value = true
  try {
    overview.value = await dashboardApi.sync()
    ElMessage.success('统计已同步')
  } catch {
    ElMessage.warning('同步失败，请稍后重试')
  } finally {
    syncing.value = false
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
  } catch {
    ElMessage.warning(`${row.site_name} 刷新失败`)
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
  <div class="dashboard" v-loading="loading">
    <div class="page-head">
      <h2 class="page-title">仪表盘</h2>
      <div class="head-actions">
        <el-button :disabled="!overview" @click="openExport">
          <GlassIcon name="download" from="#6c5ce7" to="#4285f4" :size="16" class="btn-ico" />
          <span>导出图片</span>
        </el-button>
        <el-button :loading="syncing" @click="syncStats">
          <GlassIcon name="seeding" from="#11998e" to="#38ef7d" :size="16" class="btn-ico" />
          <span>刷新统计</span>
        </el-button>
      </div>
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

    <!-- 指标卡片墙 -->
    <div class="metric-grid">
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

    <!-- 站点明细 -->
    <div class="sites-card glass">
      <div class="card-head">
        <span>站点明细</span>
        <span class="text-muted">站点 {{ overview?.site_count ?? 0 }} · 下载器 {{ overview?.downloader_count ?? 0 }}</span>
      </div>
      <el-table :data="overview?.sites ?? []" style="width: 100%" empty-text="暂无数据，等待统计同步">
        <el-table-column prop="site_name" label="站点" min-width="90" fixed="left" />
        <el-table-column label="等级" min-width="90">
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
        <el-table-column label="发布数" width="70">
          <template #default="{ row }">{{ row.uploads > 0 ? row.uploads : '-' }}</template>
        </el-table-column>
        <el-table-column prop="seeding" label="做种" width="60" />
        <el-table-column label="做种体积" min-width="100">
          <template #default="{ row }">{{ row.seeding_size > 0 ? fmtBytes(row.seeding_size) : '-' }}</template>
        </el-table-column>
        <el-table-column prop="leeching" label="下载中" width="70" />
        <el-table-column label="魔力" min-width="90">
          <template #default="{ row }">{{ row.bonus > 0 ? row.bonus.toFixed(0) : '-' }}</template>
        </el-table-column>
        <el-table-column label="时魔/时" width="90">
          <template #default="{ row }">{{ row.bonus_per_hour > 0 ? row.bonus_per_hour.toFixed(1) : '-' }}</template>
        </el-table-column>
        <el-table-column label="加入时间" min-width="100">
          <template #default="{ row }">{{ row.join_date || '-' }}</template>
        </el-table-column>
        <el-table-column label="更新时间" min-width="135">
          <template #default="{ row }">{{ row.updated_at ? fmtTime(row.updated_at) : '-' }}</template>
        </el-table-column>
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

/* 告警卡：Cookie 失效 / H&R 危险（最高优先级，红色左边框） */
.alert-card { padding: 16px 18px; border-left: 3px solid #e74c3c; }
.alert-head { display: flex; align-items: center; gap: 8px; }
.alert-title { font-weight: 700; font-size: 15px; color: #e74c3c; }
.alert-desc { margin: 8px 0 10px; font-size: 12.5px; color: #8b95a3; }
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
.hr-meta { font-size: 12px; color: #8b95a3; white-space: nowrap; }
.hr-more { margin: 6px 0 0; font-size: 12px; color: #8b95a3; }

/* 日环比徽标（总上传/总下载卡） */
.trend-badge {
  font-size: 12.5px;
  font-weight: 600;
  padding: 1px 7px;
  border-radius: 999px;
}
.trend-badge.trend-up { color: #2ecc71; background: rgba(46, 204, 113, 0.14); }
.trend-badge.trend-down { color: #ff6b6b; background: rgba(255, 107, 107, 0.14); }
</style>
