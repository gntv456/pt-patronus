<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// 运行日志面板：级别筛选、刷新、复制、清空。
// 状态与逻辑来自 useLogs 组合式函数。
import { useI18n } from 'vue-i18n'
import { useLogs } from '@/composables/useLogs'

const { t } = useI18n()

const { logs, logLevel, logLoading, loadLogs, clearLogs, copyLogs, fmtTime, attrText } = useLogs()
</script>

<template>
  <div class="logs-bar">
    <el-select v-model="logLevel" @change="loadLogs" style="width: 140px">
      <el-option :label="t('lp.allLevels')" value="" />
      <el-option label="Debug+" value="debug" />
      <el-option label="Warn+" value="warn" />
      <el-option label="Error" value="error" />
    </el-select>
    <el-button :loading="logLoading" @click="loadLogs">{{ t('common.refresh') }}</el-button>
    <el-button :disabled="!logs.length" @click="copyLogs">{{ t('lp.copy') }}</el-button>
    <el-button type="danger" plain :disabled="!logs.length" @click="clearLogs">{{ t('common.clear') }}</el-button>
    <span class="text-muted log-count">{{ t('lp.count', { n: logs.length }) }}</span>
  </div>
  <div class="logs-list glass" v-loading="logLoading">
    <div v-for="(e, i) in logs" :key="i" class="log-line" :class="e.level.toLowerCase()">
      <span class="log-time">{{ fmtTime(e.time) }}</span>
      <span class="log-level">{{ e.level }}</span>
      <span class="log-msg">{{ e.message }}</span>
      <span v-if="attrText(e.attrs)" class="log-attrs">{{ attrText(e.attrs) }}</span>
    </div>
    <div v-if="!logs.length && !logLoading" class="log-empty text-muted">{{ t('lp.empty') }}</div>
  </div>
</template>

<style scoped>
.logs-bar {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  margin-bottom: 10px;
}
.log-count {
  font-size: var(--fs-xs);
}
.logs-list {
  padding: 8px;
  max-height: 62vh;
  overflow-y: auto;
  font-family: ui-monospace, Menlo, Consolas, monospace;
  font-size: var(--fs-xs);
}
.log-line {
  display: flex;
  gap: var(--sp-2);
  padding: 3px 6px;
  border-radius: var(--radius-2xs);
  align-items: baseline;
  transition:
    background var(--dur-fast) var(--ease-out-smooth),
    color var(--dur-fast) var(--ease-out-smooth),
    border-color var(--dur-fast) var(--ease-out-smooth);
}
.log-line:hover {
  background: var(--pt-primary-bg);
}
.log-line.error {
  color: var(--pt-danger);
}
.log-line.warn {
  color: var(--pt-warn, #f59e0b);
}
.log-line.debug {
  color: var(--text-muted);
}
.log-time {
  color: var(--text-muted);
  flex-shrink: 0;
}
.log-level {
  font-weight: 700;
  flex-shrink: 0;
  width: 52px;
}
.log-msg {
  flex-shrink: 0;
}
.log-attrs {
  color: var(--text-muted);
  opacity: 0.85;
  word-break: break-all;
}
.log-empty {
  padding: 30px;
  text-align: center;
}
</style>
