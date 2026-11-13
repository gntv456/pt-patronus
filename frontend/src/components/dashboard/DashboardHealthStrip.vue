<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// 站点健康状态条 + 无采集数据告警卡。
// 数据来自 useDashboardData（overview/站点数）与 useDashboardActions（联通计数/待办）。
import { useI18n } from 'vue-i18n'
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useDashboardData } from '@/composables/useDashboardData'
import { useDashboardActions } from '@/composables/useDashboardActions'
import SptIcon from '@/components/SptIcon.vue'

const { t } = useI18n()
const router = useRouter()
const { overview, allSites } = useDashboardData()
const { refreshing, connectedCount, disconnectedCount, disabledCount, totalIssueCount } = useDashboardActions()

const configuredSiteCount = computed(() => allSites.value.length || overview.value?.site_count || 0)

function openSiteMatrix(filter: 'all' | 'issues' | 'disabled') {
  void router.push({ name: 'sites', query: { matrix: filter } })
}
</script>

<template>
  <div class="health-strip ops-surface">
    <button type="button" class="health-main" @click="openSiteMatrix('all')">
      <span class="health-icon health-ok"><i /></span>
      <span>
        <small>{{ t('dhs.siteHealth') }}</small>
        <strong v-if="refreshing">{{ t('dhs.refreshing') }}</strong>
        <strong v-else>{{ t('dhs.connectedN', { ok: connectedCount, total: configuredSiteCount }) }}</strong>
      </span>
    </button>
    <button type="button" class="health-item is-danger" @click="openSiteMatrix('issues')">
      <span class="status-dot" />
      <span>{{ t('som.issues') }}</span>
      <b>{{ disconnectedCount }}</b>
    </button>
    <button
      v-if="(overview?.no_data_sites?.length ?? 0) > 0"
      type="button"
      class="health-item is-warning"
      @click="openSiteMatrix('issues')"
    >
      <span class="status-dot" />
      <span>{{ t('common.noData') }}</span>
      <b>{{ overview?.no_data_sites?.length }}</b>
    </button>
    <button type="button" class="health-item is-muted" @click="openSiteMatrix('disabled')">
      <span class="status-dot" />
      <span>{{ t('som.disabled') }}</span>
      <b>{{ disabledCount }}</b>
    </button>
    <div class="health-item is-success">
      <span class="status-dot" />
      <span>{{ t('dhs.dlConfig') }}</span>
      <b>{{ overview?.downloader_count ?? 0 }}</b>
    </div>
    <div class="health-item" :class="totalIssueCount ? 'is-warning' : 'is-success'">
      <span class="status-dot" />
      <span>{{ t('dhs.todo') }}</span>
      <b>{{ totalIssueCount }}</b>
    </div>
  </div>

  <div v-if="overview?.stat_read_error" class="statread-alert solid-card">
    <span class="statread-badge"><SptIcon name="alert-circle" :size="18" aria-hidden="true" /></span>
    <div class="statread-body">
      <div class="statread-title">
        <strong>{{ t('dhs.statReadFailed') }}</strong>
        <span class="statread-hint">{{ t('dhs.statReadFailedHint') }}</span>
      </div>
      <div class="statread-reason">{{ overview.stat_read_error }}</div>
    </div>
  </div>

  <div v-if="(overview?.no_data_sites?.length ?? 0) > 0" class="nodata-alert solid-card">
    <span class="nodata-badge"><SptIcon name="alert-triangle" :size="18" aria-hidden="true" /></span>
    <div class="nodata-body">
      <div class="nodata-title">
        <strong>{{ t('dhs.nodataN', { n: overview?.no_data_sites?.length }) }}</strong>
        <span class="nodata-hint">{{ t('dhs.nodataHint') }}</span>
      </div>
      <div class="nodata-names">
        <button
          v-for="s in overview?.no_data_sites"
          :key="s.site_id"
          type="button"
          class="nodata-chip"
          @click="openSiteMatrix('issues')"
        >
          {{ s.site_name }}
        </button>
      </div>
    </div>
    <el-button text type="warning" @click="openSiteMatrix('issues')">{{ t('dhs.viewSites') }}</el-button>
  </div>
</template>

<style scoped>
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
  gap: var(--sp-2);
  min-width: 0;
  padding: 12px 16px;
  border: 0;
  border-right: 1px solid var(--divider);
  color: var(--text-strong);
  background: transparent;
  text-align: left;
  transition:
    background var(--dur-fast) var(--ease-out-smooth),
    color var(--dur-fast) var(--ease-out-smooth),
    border-color var(--dur-fast) var(--ease-out-smooth);
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
  gap: var(--sp-1);
  min-width: 0;
}
.health-main small {
  color: var(--text-muted);
  font-size: var(--fs-2xs);
}
.health-main strong {
  overflow: hidden;
  font-size: var(--fs-lg);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.health-icon {
  position: relative;
  width: 30px;
  height: 30px;
  flex: 0 0 30px;
  border-radius: var(--radius-xs);
  background: var(--pt-success-bg);
}
.health-icon i {
  position: absolute;
  inset: 8px;
  border: 2px solid var(--pt-success);
  border-radius: 50%;
}
.health-icon i::after {
  position: absolute;
  top: 2px;
  left: 4px;
  width: 3px;
  height: 6px;
  border-right: 2px solid var(--color-success);
  border-bottom: 2px solid var(--color-success);
  content: '';
  transform: rotate(45deg);
}
.health-item span:not(.status-dot) {
  overflow: hidden;
  color: var(--text-muted);
  font-size: var(--fs-xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.health-item b {
  margin-left: auto;
  font-size: var(--fs-xl);
  font-variant-numeric: tabular-nums;
}
.status-dot {
  width: 8px;
  height: 8px;
  flex: 0 0 8px;
  border-radius: 50%;
  background: var(--color-text-muted);
}
.health-item.is-danger b {
  color: var(--pt-danger);
}
.health-item.is-danger .status-dot {
  background: var(--color-danger);
}
.health-item.is-warning b {
  color: var(--pt-warn-fg);
}

/* 无采集数据站点告警卡（保号可观测性：疑似 cookie 失效/未采集的 PT 站首屏红字汇总） */
.nodata-alert {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
  padding: 14px 18px;
  margin-bottom: 16px;
  border-left: 3px solid var(--color-warning);
}
/* 统计读取故障：与「站点无数据」分色（红=系统读不到，琥珀=站点侧真没采到），
   两者不会同屏——后端读失败时不再判定无数据。 */
.statread-alert {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
  padding: 14px 18px;
  margin-bottom: 16px;
  border-left: 3px solid var(--color-danger);
}
.statread-badge {
  flex: 0 0 auto;
  width: 34px;
  height: 34px;
  display: grid;
  place-items: center;
  border-radius: var(--radius-control);
  font-weight: 700;
  color: var(--pt-danger-fg);
  background: var(--color-danger-soft);
}
.statread-body {
  flex: 1 1 auto;
  min-width: 0;
}
.statread-title {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: var(--sp-2);
}
.statread-hint {
  font-size: var(--fs-xs);
  color: var(--text-muted);
}
.statread-reason {
  margin-top: 6px;
  font-size: var(--fs-xs);
  color: var(--text-muted);
  word-break: break-all;
}
.nodata-badge {
  flex: 0 0 auto;
  width: 34px;
  height: 34px;
  display: grid;
  place-items: center;
  border-radius: 50%;
  background: var(--color-warning-soft);
  color: var(--pt-warn-fg);
  font-size: var(--fs-xl);
}
.nodata-body {
  flex: 1 1 auto;
  min-width: 0;
}
.nodata-title {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: var(--sp-2);
}
.nodata-hint {
  font-size: var(--fs-xs);
  color: var(--text-muted);
}
.nodata-names {
  display: flex;
  flex-wrap: wrap;
  gap: var(--sp-1);
  margin-top: 8px;
}
.nodata-chip {
  min-height: 24px;
  border: 1px solid var(--color-border-strong);
  background: var(--color-warning-soft);
  color: var(--color-text);
  border-radius: var(--radius-control);
  padding: 2px 10px;
  font-size: var(--fs-xs);
  cursor: pointer;
}
.nodata-chip:hover {
  background: var(--pt-warn-bg);
}
.health-item.is-warning .status-dot {
  background: var(--pt-warn-fg);
}
.health-item.is-success b {
  color: var(--pt-success);
}
.health-item.is-success .status-dot {
  background: var(--color-success);
}
.health-item.is-muted b {
  color: var(--text-muted);
}
</style>
