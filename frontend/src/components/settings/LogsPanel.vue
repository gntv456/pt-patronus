<script setup lang="ts">
// 运行日志面板：级别筛选、刷新、复制、清空。
// 状态与逻辑来自 useLogs 组合式函数。
import { useLogs } from '@/composables/useLogs'

const { logs, logLevel, logLoading, loadLogs, clearLogs, copyLogs, fmtTime, attrText } = useLogs()
</script>

<template>
        <div class="logs-bar">
          <el-select v-model="logLevel" @change="loadLogs" style="width: 140px">
            <el-option label="全部 (Info+)" value="" />
            <el-option label="Debug+" value="debug" />
            <el-option label="Warn+" value="warn" />
            <el-option label="Error" value="error" />
          </el-select>
          <el-button :loading="logLoading" @click="loadLogs">刷新</el-button>
          <el-button :disabled="!logs.length" @click="copyLogs">复制</el-button>
          <el-button type="danger" plain :disabled="!logs.length" @click="clearLogs">清空</el-button>
          <span class="text-muted log-count">{{ logs.length }} 条（最新在前）</span>
        </div>
        <div class="logs-list glass" v-loading="logLoading">
          <div v-for="(e, i) in logs" :key="i" class="log-line" :class="e.level.toLowerCase()">
            <span class="log-time">{{ fmtTime(e.time) }}</span>
            <span class="log-level">{{ e.level }}</span>
            <span class="log-msg">{{ e.message }}</span>
            <span v-if="attrText(e.attrs)" class="log-attrs">{{ attrText(e.attrs) }}</span>
          </div>
          <div v-if="!logs.length && !logLoading" class="log-empty text-muted">暂无日志</div>
        </div>
</template>

<style scoped>
.logs-bar { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; }
.log-count { font-size: 12px; }
.logs-list {
  padding: 8px;
  max-height: 62vh;
  overflow-y: auto;
  font-family: ui-monospace, Menlo, Consolas, monospace;
  font-size: 12px;
}
.log-line { display: flex; gap: 8px; padding: 3px 6px; border-radius: 4px; align-items: baseline; }
.log-line:hover { background: rgba(108, 92, 231, 0.06); }
.log-line.error { color: var(--pt-danger); }
.log-line.warn { color: var(--pt-warn, #f59e0b); }
.log-line.debug { color: var(--text-muted); }
.log-time { color: var(--text-muted); flex-shrink: 0; }
.log-level { font-weight: 700; flex-shrink: 0; width: 52px; }
.log-msg { flex-shrink: 0; }
.log-attrs { color: var(--text-muted); opacity: 0.85; word-break: break-all; }
.log-empty { padding: 30px; text-align: center; }
</style>
