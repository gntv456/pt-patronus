<script setup lang="ts">
import { ref, computed, onMounted, defineAsyncComponent } from 'vue'
import { siteApi } from '@/api/site'
import { useAuthStore } from '@/stores/auth'
import type { DashboardShareData } from '@/utils/dashboardImage'
import { useDashboardData } from '@/composables/useDashboardData'
import { useDashboardSync } from '@/composables/useDashboardSync'
import { useDashboardActions } from '@/composables/useDashboardActions'
// 导出对话框含 661 行 canvas 绘制逻辑，仅在用户点「导出图片」时需要 → 异步加载，不进首屏 chunk。
const DashboardExportDialog = defineAsyncComponent(() => import('@/components/dashboard/DashboardExportDialog.vue'))
import DashboardRealtimeCard from '@/components/dashboard/DashboardRealtimeCard.vue'
import DashboardResourceCard from '@/components/dashboard/DashboardResourceCard.vue'
import DashboardAutomationCard from '@/components/dashboard/DashboardAutomationCard.vue'
import DashboardAICard from '@/components/dashboard/DashboardAICard.vue'
import DashboardTaskCards from '@/components/dashboard/DashboardTaskCards.vue'
import DashboardActivityTimeline from '@/components/dashboard/DashboardActivityTimeline.vue'
import DashboardRingChart, { type RingSegment } from '@/components/dashboard/DashboardRingChart.vue'
import DashboardSyncCard from '@/components/dashboard/DashboardSyncCard.vue'
import DashboardTrendSection from '@/components/dashboard/DashboardTrendSection.vue'
import DashboardHealthStrip from '@/components/dashboard/DashboardHealthStrip.vue'
import EmptyState from '@/components/EmptyState.vue'

const { overview, trendDays, trendLoading, changeTrendDays, latestUpdatedText, loading, loadError, load, allSites } = useDashboardData()
const { syncing, syncTotal, syncDone, syncStats } = useDashboardSync()
const {
  refreshing, loadCached, refreshConnectivity,
  connectedCount, disabledCount, healthSegments, unreadCount,
} = useDashboardActions()

// 区③④⑥ 组件 ref：syncStats 完成后触发刷新自动化/AI/时间线数据。
const autoCard = ref<{ refresh: () => Promise<void> } | null>(null)
const aiCard = ref<{ refresh: () => Promise<void> } | null>(null)
const taskCards = ref<{ refresh: () => Promise<void> } | null>(null)
const timelineCard = ref<{ refresh: () => Promise<void> } | null>(null)

const auth = useAuthStore()
const exportVisible = ref(false)

function onSyncDone() {
  void loadCached().then(() => refreshConnectivity(true))
  autoCard.value?.refresh()
  aiCard.value?.refresh()
  taskCards.value?.refresh()
  timelineCard.value?.refresh()
}

async function handleSync() {
  await syncStats(onSyncDone)
}

// 打开导出配置弹窗：首次拉站点列表(含 base_url)缓存。
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

onMounted(() => {
  load()
  void loadCached().then(() => refreshConnectivity())
})
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
            <el-button type="primary" :loading="syncing" @click="() => syncStats()">
              <GlassIcon name="refresh" from="#ffffff" to="#dbeafe" :size="16" class="btn-ico" />
              <span>{{ syncing ? (syncTotal ? `刷新中 ${syncDone}/${syncTotal}` : '准备刷新') : '刷新数据' }}</span>
            </el-button>
          </div>
        </div>

    <DashboardHealthStrip />
    <DashboardSyncCard />

    <!-- 加载失败：overview 为空且出错时显示带重试的错误条 -->
    <div v-if="loadError && !overview && !loading" class="ops-surface load-error">
      <span class="le-msg">仪表盘加载失败：{{ loadError }}</span>
      <el-button size="small" @click="load">重试</el-button>
    </div>

    <DashboardTrendSection />

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

    <!-- 区②.5 任务卡片 -->
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

    <!-- 导出图片配置弹窗 -->
    <DashboardExportDialog v-model="exportVisible" :data="exportData" />
  </div>
</template>


<style scoped>
.dashboard { display: flex; flex-direction: column; gap: 18px; }
.dashboard { gap: 14px; letter-spacing: 0; }
.page-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.page-head { flex-wrap: wrap; }
.head-actions { display: flex; gap: 8px; }
.page-title { margin: 4px 0 6px; font-size: 22px; font-weight: 700; }
.page-title { margin: 0; }
.page-heading { display: flex; flex-direction: column; gap: 2px; }
.page-subtitle, .updated-at { font-size: 12px; color: var(--text-muted); }
.head-actions { flex-wrap: wrap; }
.period-control { display: inline-flex; background: var(--surface-muted); border-radius: 8px; padding: 2px; }
.period-control button { border: 0; background: transparent; padding: 4px 10px; border-radius: 6px; font-size: 12px; color: var(--text-muted); cursor: pointer; }
.period-control button.active { background: var(--card); color: var(--text-strong); box-shadow: 0 1px 2px rgba(0,0,0,0.08); }
.period-control button:disabled { opacity: 0.5; cursor: not-allowed; }
.btn-ico { margin-right: 4px; vertical-align: middle; }
.setup-card { padding: 6px; }
.setup-card a { text-decoration: none; }
.ops-surface, .sync-card {
  background: var(--card);
  border: 1px solid var(--divider, rgba(0,0,0,0.08));
  border-radius: 12px;
}
.load-error { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-3); padding: var(--sp-3) var(--sp-4); margin-bottom: var(--sp-3); }
.load-error .le-msg { color: var(--text-strong); }

/* —— 仪表盘新增区布局 —— */
.zone { display: grid; gap: 14px; }
.zone-3col { grid-template-columns: repeat(3, minmax(0, 1fr)); }
.zone-2col { grid-template-columns: repeat(2, minmax(0, 1fr)); }
.zone-card { padding: 16px 18px; }
.zc-head { margin-bottom: 14px; }
.zc-head h3 { margin: 0 0 2px; font-size: 14px; font-weight: 700; }
.zc-head p { margin: 0; font-size: 11px; color: var(--text-muted); }
.health-foot { margin-top: 12px; padding-top: 10px; border-top: 1px solid var(--divider); font-size: 12px; color: var(--text-muted); display: flex; align-items: center; justify-content: space-between; }
.health-foot b { color: var(--text-strong); font-size: 15px; }

@media (max-width: 1180px) {
  .zone-3col { grid-template-columns: 1fr 1fr; }
  .zone-3col > section:first-child { grid-column: 1 / -1; }
}
@media (max-width: 900px) {
  .zone-3col, .zone-2col { grid-template-columns: 1fr; }
}
@media (max-width: 640px) {
  .page-heading { align-items: flex-start; flex-direction: column; gap: 2px; }
  .head-actions { gap: 6px; }
  .updated-at { width: 100%; }
  .period-control { order: 2; }
}
</style>

