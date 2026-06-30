<script setup lang="ts">
import type { MonitorAlert } from '@/api/monitor'

// AlertList — 告警列表（按级别着色，未读可标记）。
defineProps<{ alerts: MonitorAlert[] }>()
const emit = defineEmits<{ read: [MonitorAlert] }>()

const levelColor: Record<string, string> = {
  danger: '#e74c3c',
  warning: '#f7971e',
  info: '#4285f4',
}
</script>

<template>
  <div class="alert-list">
    <div v-if="!alerts.length" class="empty">暂无告警</div>
    <div v-for="a in alerts" :key="a.id" class="alert-item" :class="{ unread: !a.read, resolved: !!a.resolved_at }">
      <span class="dot" :style="{ background: levelColor[a.level] || '#9ca3af' }"></span>
      <div class="al-body">
        <div class="al-msg">{{ a.message }}</div>
        <div class="al-meta">
          {{ a.metric }} · {{ a.value }} · {{ new Date(a.created_at).toLocaleString() }}
          <span v-if="a.resolved_at" class="resolved-tag">已恢复</span>
        </div>
      </div>
      <el-button v-if="!a.read" size="small" text @click="emit('read', a)">已读</el-button>
    </div>
  </div>
</template>

<style scoped>
.alert-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 320px;
  overflow: auto;
}
.empty {
  color: var(--text-muted);
  font-size: 13px;
  padding: 16px;
  text-align: center;
}
.alert-item {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 8px 10px;
  border-radius: var(--radius-sm);
  background: var(--glass-bg);
}
.alert-item.unread {
  background: rgba(108, 92, 231, 0.1);
}
.alert-item.resolved {
  opacity: 0.6;
}
.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-top: 5px;
  flex: none;
}
.al-body {
  flex: 1;
  min-width: 0;
}
.al-msg {
  font-size: 13px;
  color: var(--text-strong);
}
.al-meta {
  font-size: 11px;
  color: var(--text-muted);
  margin-top: 2px;
}
.resolved-tag {
  color: var(--pt-success);
  margin-left: 4px;
}
</style>
