<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// 账号增长趋势面板 + 待处理事项面板。
// 数据来自 useDashboardData（趋势/指标）与 useDashboardActions（待办）。
import { useI18n } from 'vue-i18n'
import DashboardTrendChart from '@/components/dashboard/DashboardTrendChart.vue'
import SiteFavicon from '@/components/SiteFavicon.vue'
import { useDashboardData } from '@/composables/useDashboardData'
import { useDashboardActions } from '@/composables/useDashboardActions'
import SptIcon from '@/components/SptIcon.vue'

const { t } = useI18n()
const {
  trendLoading,
  trendDays,
  trendMode,
  initialLoading,
  metricItems,
  trendSeries,
  trendUnit,
  trendDaysAxis,
  siteConfigByName,
} = useDashboardData()
const { dashboardActions, totalIssueCount, showAction } = useDashboardActions()
</script>

<template>
  <div class="operations-grid">
    <section class="trend-panel ops-surface" :aria-busy="trendLoading">
      <div class="section-head">
        <div>
          <h3>{{ t('dts.title') }}</h3>
          <p>{{ t('dts.sub', { n: trendDays }) }}</p>
        </div>
        <div class="metric-switch" :aria-label="t('dts.metricAria')">
          <button type="button" :class="{ active: trendMode === 'traffic' }" @click="trendMode = 'traffic'">
            {{ t('dts.mTraffic') }}
          </button>
          <button type="button" :class="{ active: trendMode === 'seeding' }" @click="trendMode = 'seeding'">
            {{ t('dts.mSeeding') }}
          </button>
          <button type="button" :class="{ active: trendMode === 'size' }" @click="trendMode = 'size'">
            {{ t('dts.mSize') }}
          </button>
          <button type="button" :class="{ active: trendMode === 'bonus' }" @click="trendMode = 'bonus'">
            {{ t('dts.mBonus') }}
          </button>
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
      <el-skeleton v-if="trendLoading" :rows="6" animated class="trend-sk" />
      <DashboardTrendChart v-else :days="trendDaysAxis" :series="trendSeries" :unit="trendUnit" />
    </section>

    <aside class="action-panel ops-surface">
      <div class="section-head action-heading">
        <div>
          <h3>{{ t('dts.issuesTitle') }}</h3>
          <p>{{ totalIssueCount ? t('dts.issuesN', { n: totalIssueCount }) : t('dts.allOk') }}</p>
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
          <SiteFavicon :name="action.site" :base-url="siteConfigByName(action.site)?.base_url" :size="30" />
          <span class="action-copy">
            <b>{{ action.site }}</b>
            <span>{{ action.title }}</span>
            <small>{{ action.detail }}</small>
          </span>
          <i class="action-severity" :class="`tone-${action.tone}`" :aria-label="action.tone" :title="action.tone" />
        </button>
      </div>
      <div v-else class="action-empty">
        <span class="empty-check"><SptIcon name="check" :size="18" aria-hidden="true" /></span>
        <strong>{{ t('dts.noIssues') }}</strong>
        <p>{{ t('dts.noIssuesHint') }}</p>
      </div>
      <router-link to="/sites" class="action-footer">{{ t('dts.goSites') }}</router-link>
    </aside>
  </div>
</template>

<style scoped src="./DashboardTrendSection.css"></style>
