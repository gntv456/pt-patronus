<script setup lang="ts">
// 站点健康状态条 + 无采集数据告警卡。
// 数据来自 useDashboardData（overview/站点数）与 useDashboardActions（联通计数/待办）。
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useDashboardData } from '@/composables/useDashboardData'
import { useDashboardActions } from '@/composables/useDashboardActions'

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
          <small>站点健康</small>
          <strong v-if="refreshing">刷新中…</strong>
          <strong v-else>{{ connectedCount }} / {{ configuredSiteCount }} 联通</strong>
        </span>
      </button>
      <button type="button" class="health-item is-danger" @click="openSiteMatrix('issues')">
        <span class="status-dot" />
        <span>未联通</span>
        <b>{{ disconnectedCount }}</b>
      </button>
      <button
        v-if="(overview?.no_data_sites?.length ?? 0) > 0"
        type="button"
        class="health-item is-warning"
        @click="openSiteMatrix('issues')"
      >
        <span class="status-dot" />
        <span>无数据</span>
        <b>{{ overview?.no_data_sites?.length }}</b>
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

    <div v-if="(overview?.no_data_sites?.length ?? 0) > 0" class="nodata-alert glass">
      <span class="nodata-badge">⚠</span>
      <div class="nodata-body">
        <div class="nodata-title">
          <strong>{{ overview?.no_data_sites?.length }} 个站点无采集数据</strong>
          <span class="nodata-hint">疑似 Cookie 失效或未采集，签到 / 统计 / H&amp;R 自动化可能已停摆</span>
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
      <el-button text type="warning" @click="openSiteMatrix('issues')">查看站点</el-button>
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

/* 无采集数据站点告警卡（保号可观测性：疑似 cookie 失效/未采集的 PT 站首屏红字汇总） */
.nodata-alert {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 14px 18px;
  margin-bottom: 16px;
  border-left: 3px solid #f59e0b;
}
.nodata-badge {
  flex: 0 0 auto;
  width: 34px;
  height: 34px;
  display: grid;
  place-items: center;
  border-radius: 50%;
  background: rgba(245, 158, 11, 0.15);
  color: #f59e0b;
  font-size: 18px;
}
.nodata-body { flex: 1 1 auto; min-width: 0; }
.nodata-title { display: flex; flex-wrap: wrap; align-items: baseline; gap: 8px; }
.nodata-hint { font-size: 12px; color: var(--text-muted); }
.nodata-names { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 8px; }
.nodata-chip {
  border: 1px solid rgba(245, 158, 11, 0.4);
  background: rgba(245, 158, 11, 0.08);
  color: var(--text);
  border-radius: 12px;
  padding: 2px 10px;
  font-size: 12px;
  cursor: pointer;
}
.nodata-chip:hover { background: rgba(245, 158, 11, 0.18); }
.health-item.is-warning .status-dot { background: #d97706; }
.health-item.is-success b { color: #16a34a; }
.health-item.is-success .status-dot { background: #16a34a; }
.health-item.is-muted b { color: var(--text-muted); }
</style>
