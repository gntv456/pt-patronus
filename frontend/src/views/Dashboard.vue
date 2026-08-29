<script setup lang="ts">
import { useI18n } from 'vue-i18n'
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
import DashboardWorkbench from '@/components/dashboard/DashboardWorkbench.vue'
import DashboardWidgets from '@/components/dashboard/DashboardWidgets.vue'
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
      ElMessage.warning(t('dashboard.loadSitesFailed'))
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

const { t } = useI18n()
onMounted(() => {
  load()
  void loadCached().then(() => refreshConnectivity())
})
</script>


<template>
  <div class="dashboard">
        <div class="page-head">
          <div class="page-heading">
            <h2 class="page-title">{{ t('dashboard.title') }}</h2>
            <span class="page-subtitle">{{ t('dashboard.subtitle') }}</span>
          </div>
          <div class="head-actions">
            <span class="updated-at">{{ latestUpdatedText }}</span>
            <div class="period-control" :aria-label="t('dashboard.trendPeriod')">
              <button
                v-for="days in ([7, 30, 90] as const)"
                :key="days"
                type="button"
                :class="{ active: trendDays === days }"
                :disabled="trendLoading"
                @click="changeTrendDays(days)"
              >
                {{ days }}{{ t('dashboard.daysUnit') }}
              </button>
            </div>
            <el-button :disabled="!overview" @click="openExport">
              <GlassIcon name="download" from="#2563eb" to="#0f766e" :size="16" class="btn-ico" />
              <span>{{ t('dashboard.exportImage') }}</span>
            </el-button>
            <el-button type="primary" :loading="syncing" @click="() => syncStats()">
              <GlassIcon name="refresh" from="#ffffff" to="#dbeafe" :size="16" class="btn-ico" />
              <span>{{ syncing ? (syncTotal ? t('dashboard.refreshing', { done: syncDone, total: syncTotal }) : t('dashboard.preparing')) : t('dashboard.refresh') }}</span>
            </el-button>
          </div>
        </div>

    <!-- 工作台（对标 MSaber 天序工作台）：问候/体检分/今日流转/待办/动态/摘要/媒体库态势 -->
    <DashboardWorkbench />

    <!-- 自定义部件区（对标 MSaber 首页定制）：快捷工具等，可增删/拖拽排序/尺寸预设 -->
    <DashboardWidgets />

    <!-- 新用户引导向导：无站点且无下载器时首屏展示 -->
    <div v-if="!loading && overview && overview.site_count === 0 && overview.downloader_count === 0" class="onboarding-wizard ops-surface">
      <div class="ob-header">
        <h2>{{ t('dashboard.welcome.title') }}</h2>
        <p>{{ t('dashboard.welcome.subtitle') }}</p>
      </div>
      <div class="ob-steps">
        <div class="ob-step">
          <div class="ob-num">1</div>
          <div class="ob-body">
            <h4>{{ t('dashboard.welcome.step1Title') }}</h4>
            <p>{{ t('dashboard.welcome.step1Desc') }}</p>
            <router-link to="/sites"><el-button type="primary" size="small">{{ t('dashboard.addSite') }}</el-button></router-link>
          </div>
        </div>
        <div class="ob-step">
          <div class="ob-num">2</div>
          <div class="ob-body">
            <h4>{{ t('dashboard.welcome.step2Title') }}</h4>
            <p>{{ t('dashboard.welcome.step2Desc') }}</p>
            <router-link to="/downloaders"><el-button type="primary" size="small">{{ t('dashboard.addDownloader') }}</el-button></router-link>
          </div>
        </div>
        <div class="ob-step">
          <div class="ob-num">3</div>
          <div class="ob-body">
            <h4>{{ t('dashboard.welcome.step3Title') }}</h4>
            <p>{{ t('dashboard.welcome.step3Desc') }}</p>
            <el-button type="primary" size="small" :loading="syncing" @click="() => syncStats()">{{ t('dashboard.welcome.refreshNow') }}</el-button>
          </div>
        </div>
      </div>
    </div>

    <template v-if="!loading && overview && (overview.site_count > 0 || overview.downloader_count > 0)">
    <DashboardHealthStrip />
    <DashboardSyncCard />

    <!-- 加载失败：overview 为空且出错时显示带重试的错误条 -->
    <div v-if="loadError && !overview && !loading" class="ops-surface load-error">
      <span class="le-msg">{{ t('dashboard.loadFailed', { err: loadError }) }}</span>
      <el-button size="small" @click="load">{{ t('dashboard.retry') }}</el-button>
    </div>

    <DashboardTrendSection />

    <!-- 区② 实时运维：站点健康分布 · 下载器实时速率 · 系统资源 -->
    <div class="zone zone-3col">
      <section class="ops-surface zone-card">
        <div class="zc-head"><h3>{{ t('dashboard.siteHealth') }}</h3><p>{{ t('dashboard.connectivityDist') }}</p></div>
        <DashboardRingChart
          :items="healthSegments"
          :size="116"
          :thickness="13"
          :center-value="String(connectedCount)"
          :center-label="t('dashboard.online')"
        />
        <div class="health-foot">{{ t('dashboard.unreadNotices') }}<b>{{ unreadCount }}</b></div>
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
        :title="t('dashboard.emptyTitle')"
        :description="t('dashboard.emptyDesc')"
      >
        <router-link to="/sites"><el-button type="primary">{{ t('dashboard.addSite') }}</el-button></router-link>
        <router-link to="/downloaders"><el-button>{{ t('dashboard.addDownloader') }}</el-button></router-link>
      </EmptyState>
    </div>

    <!-- 导出图片配置弹窗 -->
    </template>

    <!-- 导出图片配置弹窗 -->
    <DashboardExportDialog v-model="exportVisible" :data="exportData" />
  </div>
</template>


<style scoped>
.dashboard { display: flex; flex-direction: column; gap: var(--sp-4); }
.dashboard { gap: var(--sp-3); letter-spacing: 0; }
.page-head { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-3); }
.page-head { flex-wrap: wrap; }
.head-actions { display: flex; gap: var(--sp-2); }
.page-title { margin: 4px 0 6px; font-size: var(--fs-title); font-weight: 700; }
.page-title { margin: 0; }
.page-heading { display: flex; flex-direction: column; gap: var(--sp-1); }
.page-subtitle, .updated-at { font-size: var(--fs-xs); color: var(--text-muted); }
.head-actions { flex-wrap: wrap; }
.period-control { display: inline-flex; background: var(--surface-muted); border-radius: var(--radius-xs); padding: 2px; }
.period-control button { border: 0; background: transparent; padding: 4px 10px; border-radius: var(--radius-xs); font-size: var(--fs-xs); color: var(--text-muted); cursor: pointer; transition: background var(--dur-base) var(--ease-out-smooth), color var(--dur-base) var(--ease-out-smooth); }
.period-control button.active { background: var(--card); color: var(--text-strong); box-shadow: var(--shadow-xs); }
.period-control button:disabled { opacity: 0.5; cursor: not-allowed; }
.btn-ico { margin-right: 4px; vertical-align: middle; }
.setup-card { padding: 6px; }
.setup-card a { text-decoration: none; }
.ops-surface, .sync-card {
  background: var(--card);
  border: 1px solid var(--divider, rgba(0,0,0,0.08));
  border-radius: var(--radius-md);
  transition: box-shadow var(--dur-base) var(--ease-out-smooth), border-color var(--dur-base) var(--ease-out-smooth);
}
.load-error { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-3); padding: var(--sp-3) var(--sp-4); margin-bottom: var(--sp-3); }
.load-error .le-msg { color: var(--text-strong); }

/* —— 仪表盘新增区布局 —— */
.zone { display: grid; gap: var(--sp-3); }
.zone-3col { grid-template-columns: repeat(3, minmax(0, 1fr)); }
.zone-2col { grid-template-columns: repeat(2, minmax(0, 1fr)); }
.zone-card { padding: 16px 18px; }
.zc-head { margin-bottom: 14px; }
.zc-head h3 { margin: 0 0 2px; font-size: var(--fs-md); font-weight: 700; }
.zc-head p { margin: 0; font-size: var(--fs-2xs); color: var(--text-muted); }
.health-foot { margin-top: 12px; padding-top: 10px; border-top: 1px solid var(--divider); font-size: var(--fs-xs); color: var(--text-muted); display: flex; align-items: center; justify-content: space-between; }
.health-foot b { color: var(--text-strong); font-size: var(--fs-lg); }

@media (max-width: 1180px) {
  .zone-3col { grid-template-columns: 1fr 1fr; }
  .zone-3col > section:first-child { grid-column: 1 / -1; }
}
@media (max-width: 900px) {
  .zone-3col, .zone-2col { grid-template-columns: 1fr; }
}
@media (max-width: 640px) {
  .page-heading { align-items: flex-start; flex-direction: column; gap: var(--sp-1); }
  .head-actions { gap: var(--sp-1); }
  .updated-at { width: 100%; }
  .period-control { order: 2; }
}
.onboarding-wizard { padding: 32px 28px; margin-bottom: 16px; text-align: center; }
.ob-header h2 { margin: 0 0 6px; font-size: var(--fs-title); font-weight: 700; }
.ob-header p { margin: 0 0 24px; color: var(--text-muted); font-size: var(--fs-md); }
.ob-steps { display: flex; gap: var(--sp-5); flex-wrap: wrap; justify-content: center; max-width: 800px; margin: 0 auto; }
.ob-step { flex: 1; min-width: 200px; max-width: 260px; display: flex; flex-direction: column; align-items: center; gap: var(--sp-2); padding: 20px 16px; border-radius: var(--radius-sm); border: 1px solid var(--glass-border); background: rgba(255,255,255,0.04); }
.ob-num { width: 36px; height: 36px; border-radius: 50%; display: flex; align-items: center; justify-content: center; font-size: var(--fs-xl); font-weight: 700; color: #fff; background: linear-gradient(135deg, #6c5ce7, #4285f4); }
.ob-body { text-align: center; }
.ob-body h4 { margin: 0 0 4px; font-size: var(--fs-lg); font-weight: 600; }
.ob-body p { margin: 0 0 12px; font-size: var(--fs-xs); color: var(--text-muted); line-height: 1.5; }
@media (max-width: 768px) { .ob-steps { flex-direction: column; align-items: center; } .ob-step { max-width: 100%; } }
/* 卡片水润 staggered 入场 */
.zone > * { animation: pt-fade-in-up var(--dur-slow) var(--ease-out-smooth) both; }
.zone > *:nth-child(1) { animation-delay: 0ms; }
.zone > *:nth-child(2) { animation-delay: 50ms; }
.zone > *:nth-child(3) { animation-delay: 100ms; }
.zone > *:nth-child(4) { animation-delay: 150ms; }
.zone > *:nth-child(5) { animation-delay: 200ms; }
.zone > *:nth-child(6) { animation-delay: 250ms; }
.page-head { animation: pt-fade-in var(--dur-base) var(--ease-out-smooth) both; }
.updated-at { transition: color var(--dur-base) var(--ease-out-smooth); }

</style>

