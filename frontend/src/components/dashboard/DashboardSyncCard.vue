<script setup lang="ts">
// 仪表盘逐站同步进度卡片：进度条、当前站点、同步日志与失败列表。
// 状态来自 useDashboardSync 组合式函数。
import { useI18n } from 'vue-i18n'
import { useDashboardSync } from '@/composables/useDashboardSync'

const { t } = useI18n()

const { syncing, syncTotal, syncDone, syncSuccess, syncCurrent, syncErrors, syncLogs, syncPercent } = useDashboardSync()
</script>

<template>
    <div v-if="syncing || syncErrors.length" class="sync-card glass" :class="{ 'has-error': syncErrors.length && !syncing }">
      <div class="sync-head">
        <div class="sync-title">
          <GlassIcon name="refresh" from="#11998e" to="#38ef7d" :size="22" />
          <div>
            <strong v-if="syncing">{{ t('dsync.running', { done: syncDone, total: syncTotal || '—' }) }}</strong>
            <strong v-else>{{ t('dsync.done', { ok: syncSuccess, fail: syncErrors.length }) }}</strong>
            <p v-if="syncing">{{ syncCurrent ? t('dsync.current', { name: syncCurrent }) : t('dsync.preparing') }}</p>
            <p v-else>{{ t('dsync.failedHint') }}</p>
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
        <div class="sync-log-title">{{ t('dsync.logTitle') }}</div>
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
</template>

<style scoped>
.sync-card {
  padding: 14px 16px;
  border-left: 3px solid #11998e;
}
.sync-card.has-error {
  border-left-color: var(--pt-danger);
}
.sync-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: var(--sp-4);
  margin-bottom: 10px;
}
.sync-title {
  display: flex;
  align-items: flex-start;
  gap: var(--sp-2);
  min-width: 0;
}
.sync-title strong {
  display: block;
  font-size: var(--fs-md);
  color: var(--text-strong);
}
.sync-title p {
  margin: 4px 0 0;
  font-size: var(--fs-xs);
  color: var(--text-muted);
  overflow-wrap: anywhere;
}
.sync-percent {
  flex: 0 0 auto;
  font-size: var(--fs-body);
  font-weight: 700;
  color: var(--text-muted);
}
.sync-log-box {
  margin-top: 12px;
}
.sync-log-title {
  margin-bottom: 6px;
  font-size: var(--fs-xs);
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
  gap: var(--sp-1);
}
.sync-logs li {
  display: grid;
  grid-template-columns: 72px minmax(92px, 150px) minmax(0, 1fr);
  align-items: start;
  gap: var(--sp-2);
  padding: 7px 9px;
  border-radius: var(--radius-xs);
  background: rgba(148, 163, 184, 0.08);
  font-size: var(--fs-xs);
}
.sync-logs .log-time {
  color: var(--text-muted);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: var(--fs-xs);
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
  background: var(--pt-primary-2-bg);
}
.sync-logs .log-running b {
  color: var(--pt-primary-2);
}
.sync-logs .log-success {
  background: rgba(17, 153, 142, 0.1);
}
.sync-logs .log-success b {
  color: #11998e;
}
.sync-logs .log-error {
  background: var(--pt-danger-bg);
}
.sync-logs .log-error b {
  color: var(--pt-danger);
}
.sync-errors {
  list-style: none;
  margin: 10px 0 0;
  padding: 0;
  display: grid;
  gap: var(--sp-1);
}
.sync-errors li {
  display: grid;
  grid-template-columns: minmax(90px, 160px) minmax(0, 1fr);
  gap: var(--sp-2);
  padding: 7px 9px;
  border-radius: var(--radius-xs);
  background: var(--pt-danger-bg);
  font-size: var(--fs-xs);
}
.sync-errors b {
  color: var(--pt-danger);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sync-errors span {
  color: var(--text-muted);
  overflow-wrap: anywhere;
}

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
