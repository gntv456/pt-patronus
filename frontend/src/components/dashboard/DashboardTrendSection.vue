<script setup lang="ts">
// 账号增长趋势面板 + 待处理事项面板。
// 数据来自 useDashboardData（趋势/指标）与 useDashboardActions（待办）。
import { useI18n } from 'vue-i18n'
import DashboardTrendChart from '@/components/dashboard/DashboardTrendChart.vue'
import SiteFavicon from '@/components/SiteFavicon.vue'
import { useDashboardData } from '@/composables/useDashboardData'
import { useDashboardActions } from '@/composables/useDashboardActions'

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
    <section class="trend-panel ops-surface" v-loading="trendLoading">
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
      <DashboardTrendChart :days="trendDaysAxis" :series="trendSeries" :unit="trendUnit" />
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
          <i class="action-severity" :class="`tone-${action.tone}`" />
        </button>
      </div>
      <div v-else class="action-empty">
        <span class="empty-check">✓</span>
        <strong>{{ t('dts.noIssues') }}</strong>
        <p>{{ t('dts.noIssuesHint') }}</p>
      </div>
      <router-link to="/sites" class="action-footer">{{ t('dts.goSites') }}</router-link>
    </aside>
  </div>
</template>

<style scoped>
.operations-grid {
  display: grid;
  grid-template-columns: minmax(0, 2fr) minmax(310px, 0.82fr);
  gap: var(--sp-3);
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
  gap: var(--sp-3);
}
.section-head h3 {
  margin: 0;
  font-size: var(--fs-lg);
  line-height: 1.35;
}
.section-head p {
  margin: 3px 0 0;
  color: var(--text-muted);
  font-size: var(--fs-2xs);
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
.metric-item:last-child {
  border-right: 0;
}
.metric-item::before {
  position: absolute;
  top: 13px;
  left: 0;
  width: 3px;
  height: 28px;
  border-radius: 0 var(--radius-2xs) var(--radius-2xs) 0;
  background: #64748b;
  content: '';
}
.metric-item.tone-teal::before {
  background: #0f766e;
}
.metric-item.tone-blue::before {
  background: #2563eb;
}
.metric-item.tone-indigo::before {
  background: #4f46e5;
}
.metric-item.tone-green::before {
  background: #16a34a;
}
.metric-item.tone-amber::before {
  background: #d97706;
}
.metric-item > span {
  display: block;
  overflow: hidden;
  color: var(--text-muted);
  font-size: var(--fs-2xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.metric-item strong {
  display: block;
  overflow: hidden;
  margin-top: 4px;
  font-size: var(--fs-xl);
  line-height: 1.2;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.metric-item small {
  display: block;
  overflow: hidden;
  margin-top: 3px;
  color: var(--text-muted);
  font-size: var(--fs-2xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.chart-legend {
  display: flex;
  justify-content: flex-end;
  gap: var(--sp-3);
  min-height: 18px;
  margin: 10px 8px -4px 0;
  color: var(--text-muted);
  font-size: var(--fs-2xs);
}
.chart-legend span {
  display: inline-flex;
  align-items: center;
  gap: var(--sp-1);
}
.chart-legend i {
  width: 7px;
  height: 7px;
  border-radius: 50%;
}

.action-panel {
  display: flex;
  flex-direction: column;
  max-height: 430px;
}
.action-heading {
  align-items: center;
}
.issue-counter {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  color: #b45309;
  background: rgba(217, 119, 6, 0.13);
  font-size: var(--fs-xs);
  font-weight: 700;
}
.issue-counter.empty {
  color: var(--pt-success);
  background: var(--pt-success-bg);
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
  gap: var(--sp-2);
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
.action-row:hover {
  background: var(--surface-muted);
}
.action-copy {
  min-width: 0;
}
.action-copy b,
.action-copy span,
.action-copy small {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.action-copy b {
  font-size: var(--fs-xs);
}
.action-copy span {
  margin-top: 1px;
  font-size: var(--fs-xs);
}
.action-copy small {
  margin-top: 2px;
  color: var(--text-muted);
  font-size: var(--fs-2xs);
}
.action-severity {
  width: 7px;
  height: 7px;
  border-radius: 50%;
}
.action-severity.tone-danger {
  background: #dc2626;
}
.action-severity.tone-warning {
  background: #d97706;
}
.action-severity.tone-info {
  background: #2563eb;
}
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
  color: var(--pt-success);
  background: var(--pt-success-bg);
  font-weight: 700;
}
.action-empty strong {
  color: var(--text-strong);
  font-size: var(--fs-body);
}
.action-empty p {
  margin: 4px 0 0;
  font-size: var(--fs-2xs);
}
.action-footer {
  display: block;
  margin-top: 10px;
  color: var(--pt-primary-2);
  font-size: var(--fs-xs);
  text-align: center;
  text-decoration: none;
}

.setup-card {
  padding: 6px;
}
@media (max-width: 1180px) {
  .health-strip {
    grid-template-columns: minmax(200px, 1.25fr) repeat(4, minmax(108px, 1fr));
  }
  .operations-grid {
    grid-template-columns: minmax(0, 1fr) 290px;
  }
  .metric-strip {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
  .metric-item:nth-child(3) {
    border-right: 0;
  }
  .metric-item:nth-child(n + 4) {
    border-top: 1px solid var(--divider);
  }
}
@media (max-width: 900px) {
  .page-head {
    align-items: flex-start;
    flex-direction: column;
  }
  .head-actions {
    width: 100%;
    justify-content: flex-start;
  }
  .updated-at {
    margin-right: auto;
  }
  .health-strip {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .health-main {
    grid-column: 1 / -1;
  }
  .health-item:nth-child(3) {
    border-right: 0;
  }
  .operations-grid {
    grid-template-columns: 1fr;
  }
  .action-panel {
    max-height: none;
  }
  .action-list {
    max-height: 330px;
  }
}
@media (max-width: 640px) {
  .page-heading {
    align-items: flex-start;
    flex-direction: column;
    gap: var(--sp-1);
  }
  .head-actions {
    gap: var(--sp-1);
  }
  .updated-at {
    width: 100%;
  }
  .period-control {
    order: 2;
  }
  .health-main,
  .health-item {
    padding: 10px 12px;
  }
  .section-head {
    align-items: flex-start;
    flex-direction: column;
  }
  .metric-switch {
    width: 100%;
    overflow-x: auto;
  }
  .metric-switch button {
    flex: 1 0 auto;
  }
  .metric-strip {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .metric-item:nth-child(2n) {
    border-right: 0;
  }
  .metric-item:nth-child(3) {
    border-right: 1px solid var(--divider);
  }
  .metric-item:nth-child(n + 3) {
    border-top: 1px solid var(--divider);
  }
  .metric-item:last-child {
    grid-column: 1 / -1;
    border-right: 0;
  }
}
</style>
