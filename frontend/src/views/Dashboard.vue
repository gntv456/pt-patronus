<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, computed, onMounted, onUnmounted, defineAsyncComponent, watch } from 'vue'
import { siteApi } from '@/api/site'
import { useAuthStore } from '@/stores/auth'
import type { DashboardShareData } from '@/utils/dashboardImage'
import { useDashboardData, type TrendMode } from '@/composables/useDashboardData'
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
import SptIcon from '@/components/SptIcon.vue'
import { useQueryStates } from '@/composables/useUrlState'

const { overview, trendDays, trendMode, trendLoading, changeTrendDays, latestUpdatedText, loading, loadError, load, allSites } =
  useDashboardData()
const { syncing, syncTotal, syncDone, syncStats, abortSync } = useDashboardSync()
const { refreshing, loadCached, refreshConnectivity, connectedCount, disabledCount, healthSegments, unreadCount } =
  useDashboardActions()

// 空库首屏：无站点且无下载器时只保留新手向导。原顺序把向导排在工作台/部件区
// 之后——新用户第一眼看到的是一排零值指标，引导卡被挤到折叠线以下。
const emptyHome = computed(
  () => !loading.value && !!overview.value && overview.value.site_count === 0 && overview.value.downloader_count === 0,
)

// 区③④⑥ 组件 ref：syncStats 完成后触发刷新自动化/AI/时间线数据。
const autoCard = ref<{ refresh: () => Promise<void> } | null>(null)
const aiCard = ref<{ refresh: () => Promise<void> } | null>(null)
const taskCards = ref<{ refresh: () => Promise<void> } | null>(null)
const timelineCard = ref<{ refresh: () => Promise<void> } | null>(null)

const auth = useAuthStore()
const exportVisible = ref(false)

// 周期(7/30/90)与趋势指标写回 URL query：刷新 / 分享 / 后退可恢复（skill: deep-linking）。
const qs = useQueryStates({ days: '30', mode: 'traffic' })
const VALID_DAYS = [7, 30, 90]
const VALID_MODES: TrendMode[] = ['traffic', 'seeding', 'size', 'bonus']
// 首载前用 query 初始化内部 ref（load() 会按 trendDays 拉趋势数据）。
const initDays = Number(qs.days.value)
if (VALID_DAYS.includes(initDays)) trendDays.value = initDays as 7 | 30 | 90
if (VALID_MODES.includes(qs.mode.value as TrendMode)) trendMode.value = qs.mode.value as TrendMode
watch(trendDays, (v) => {
  if (qs.days.value !== String(v)) qs.days.value = String(v)
})
watch(trendMode, (v) => {
  if (qs.mode.value !== v) qs.mode.value = v
})
watch(
  () => qs.days.value,
  (v) => {
    const n = Number(v)
    if (VALID_DAYS.includes(n) && n !== trendDays.value) void changeTrendDays(n as 7 | 30 | 90)
  },
)
watch(
  () => qs.mode.value,
  (v) => {
    if (VALID_MODES.includes(v as TrendMode) && v !== trendMode.value) trendMode.value = v as TrendMode
  },
)

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
// 离开仪表盘时中止逐站同步循环（下一站前退出，避免后台空转刷统计）。
onUnmounted(() => abortSync())
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
            v-for="days in [7, 30, 90] as const"
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
          <SptIcon name="download" :size="16" class="btn-ico" aria-hidden="true" />
          <span>{{ t('dashboard.exportImage') }}</span>
        </el-button>
        <el-button type="primary" :loading="syncing" @click="() => syncStats()">
          <SptIcon name="refresh" :size="16" class="btn-ico" aria-hidden="true" />
          <span>{{
            syncing
              ? syncTotal
                ? t('dashboard.refreshing', { done: syncDone, total: syncTotal })
                : t('dashboard.preparing')
              : t('dashboard.refresh')
          }}</span>
        </el-button>
      </div>
    </div>

    <!-- 新用户引导向导：空库时置顶展示，工作台/部件区让位（见 emptyHome） -->
    <div v-if="emptyHome" class="onboarding-wizard ops-surface">
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
            <router-link to="/sites"
              ><el-button size="small">{{ t('dashboard.addSite') }}</el-button></router-link
            >
          </div>
        </div>
        <div class="ob-step">
          <div class="ob-num">2</div>
          <div class="ob-body">
            <h4>{{ t('dashboard.welcome.step2Title') }}</h4>
            <p>{{ t('dashboard.welcome.step2Desc') }}</p>
            <router-link to="/downloaders"
              ><el-button size="small">{{ t('dashboard.addDownloader') }}</el-button></router-link
            >
          </div>
        </div>
        <div class="ob-step">
          <div class="ob-num">3</div>
          <div class="ob-body">
            <h4>{{ t('dashboard.welcome.step3Title') }}</h4>
            <p>{{ t('dashboard.welcome.step3Desc') }}</p>
            <el-button size="small" :loading="syncing" @click="() => syncStats()">{{
              t('dashboard.welcome.refreshNow')
            }}</el-button>
          </div>
        </div>
      </div>
    </div>

    <!-- 工作台（天序工作台）：问候/体检分/今日流转/待办/动态/摘要/媒体库态势；
         空库首屏隐藏（零值指标对新用户是噪音，向导优先） -->
    <DashboardWorkbench v-if="!emptyHome" />

    <!-- 自定义部件区：快捷工具等，可增删/拖拽排序/尺寸预设 -->
    <DashboardWidgets v-if="!emptyHome" />

    <!-- 首屏骨架（W3）：overview 未就绪时用指标骨架占位（替代整页空白/转圈）。
         结构对应工作台问候条 + 三列实时卡，避免布局跳动。 -->
    <div v-if="loading" class="dash-skeleton" aria-busy="true">
      <div class="pt-skeleton sk-greet"></div>
      <div class="dash-sk-grid">
        <div v-for="i in 3" :key="i" class="ops-surface zone-card">
          <div class="pt-skeleton sk-line w40"></div>
          <div class="pt-skeleton sk-block"></div>
          <div class="pt-skeleton sk-line w60"></div>
        </div>
      </div>
    </div>

    <template v-if="!loading && overview && (overview.site_count > 0 || overview.downloader_count > 0)">
      <DashboardHealthStrip />
      <DashboardSyncCard />

      <!-- 加载失败：overview 为空且出错时显示带重试的错误条 -->
      <div v-if="loadError && !overview && !loading" class="ops-surface load-error" role="alert">
        <span class="le-msg">{{ t('dashboard.loadFailed', { err: loadError }) }}</span>
        <el-button size="small" @click="load">{{ t('dashboard.retry') }}</el-button>
      </div>

      <DashboardTrendSection />

      <!-- 区② 实时运维：站点健康分布 · 下载器实时速率 · 系统资源 -->
      <div class="zone zone-3col">
        <section class="ops-surface zone-card">
          <div class="zc-head">
            <h3>{{ t('dashboard.siteHealth') }}</h3>
            <p>{{ t('dashboard.connectivityDist') }}</p>
          </div>
          <DashboardRingChart
            :items="healthSegments"
            :size="116"
            :thickness="13"
            :center-value="String(connectedCount)"
            :center-label="t('dashboard.online')"
          />
          <div class="health-foot">
            {{ t('dashboard.unreadNotices') }}<b>{{ unreadCount }}</b>
          </div>
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

      <div v-if="!loading && overview && !overview.sites?.length" class="setup-card ops-surface">
        <EmptyState :title="t('dashboard.emptyTitle')" :description="t('dashboard.emptyDesc')">
          <router-link to="/sites"
            ><el-button>{{ t('dashboard.addSite') }}</el-button></router-link
          >
          <router-link to="/downloaders"
            ><el-button>{{ t('dashboard.addDownloader') }}</el-button></router-link
          >
        </EmptyState>
      </div>

      <!-- 导出图片配置弹窗 -->
    </template>

    <!-- 导出图片配置弹窗 -->
    <DashboardExportDialog v-model="exportVisible" :data="exportData" />
  </div>
</template>

<style scoped src="./Dashboard.css"></style>
