<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { MonitorAlert } from '@/api/monitor'

// AlertList — 告警列表（按级别着色，未读可标记）。支持单条删除。
const { t } = useI18n()

defineProps<{ alerts: MonitorAlert[] }>()
const emit = defineEmits<{ read: [MonitorAlert]; ack: [MonitorAlert]; silence: [MonitorAlert]; del: [MonitorAlert] }>()

const levelColor: Record<string, string> = {
  danger: '#e74c3c',
  warning: '#f7971e',
  info: 'var(--pt-primary-2)',
}
</script>

<template>
  <div class="alert-list">
    <div v-if="!alerts.length" class="empty">{{ t('al.empty') }}</div>
    <div v-for="a in alerts" :key="a.id" class="alert-item" :class="{ unread: !a.read, resolved: !!a.resolved_at }">
      <span class="dot" :style="{ background: levelColor[a.level] || '#9ca3af' }"></span>
      <div class="al-body">
        <div class="al-msg">{{ a.message }}</div>
        <div class="al-meta">
          {{ a.metric }} · {{ a.value }} · {{ new Date(a.created_at).toLocaleString() }}
          <span v-if="a.resolved_at" class="resolved-tag">{{ t('al.resolved') }}</span>
          <span v-if="a.acknowledged_at" class="ack-tag">{{ t('al.ackedBy', { name: a.acknowledged_by || '' }) }}</span>
          <span v-if="a.silenced_until && new Date(a.silenced_until).getTime() > Date.now()" class="silence-tag">
            {{ t('al.silencedUntil') }} {{ new Date(a.silenced_until).toLocaleTimeString('zh-CN', { hour12: false }) }}
          </span>
        </div>
      </div>
      <div class="alert-actions">
        <el-button v-if="!a.read" size="small" text @click="emit('read', a)">{{ t('al.markRead') }}</el-button>
        <el-button v-if="!a.acknowledged_at" size="small" text type="primary" @click="emit('ack', a)">{{
          t('common.confirm')
        }}</el-button>
        <el-button v-if="!a.resolved_at" size="small" text type="warning" @click="emit('silence', a)">{{
          t('al.silence')
        }}</el-button>
        <el-button size="small" text type="danger" @click="emit('del', a)">{{ t('common.delete') }}</el-button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.alert-list {
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
  height: 100%;
  overflow: auto;
  padding-right: 2px;
}
.empty {
  color: var(--text-muted);
  font-size: var(--fs-body);
  padding: 16px;
  text-align: center;
}
.alert-item {
  display: flex;
  align-items: flex-start;
  gap: var(--sp-2);
  padding: 8px 10px;
  border-radius: var(--radius-sm);
  background: var(--glass-bg);
}
.alert-item.unread {
  background: var(--pt-primary-bg);
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
  font-size: var(--fs-body);
  color: var(--text-strong);
}
.al-meta {
  font-size: var(--fs-2xs);
  color: var(--text-muted);
  margin-top: 2px;
}
.resolved-tag {
  color: var(--pt-success);
  margin-left: 4px;
}
.ack-tag {
  color: var(--pt-primary-2);
  margin-left: 4px;
}
.silence-tag {
  color: #f7971e;
  margin-left: 4px;
}
.alert-actions {
  display: flex;
  flex: none;
  gap: var(--sp-1);
  align-items: center;
}
@media (max-width: 520px) {
  .alert-item {
    display: grid;
    grid-template-columns: 8px minmax(0, 1fr);
  }
  .alert-actions {
    grid-column: 2;
    justify-content: flex-start;
  }
}
</style>
