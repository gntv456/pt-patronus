<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// TaskIndicator — 顶栏长任务指示器（W4 任务中心）。
// 消费 useWs 的 task-progress（后端逐站/逐任务推送）：有进行中任务时显示迷你进度条
// （kind 图标 + x/y + tooltip 最近条目），finished 走满后由 useWs 2.5s 清除自动消失。
// 切页不丢反馈——这是 W3「syncAll 只有按钮转圈」的根治方案。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useWs } from '@/composables/useWs'
import GlassIcon from '@/components/GlassIcon.vue'

const { t } = useI18n()
const { taskProgress } = useWs()

// 进行中的任务（未 finished）。通常 0-1 个；多个时全部并排显示。
const activeTasks = computed(() => Object.values(taskProgress.value).filter((p) => !p.finished))

// kind → 展示元信息（图标 + 名称）。新任务族在此登记。
const KIND_META: Record<string, { icon: string; label: string }> = {
  'site-sync': { icon: 'sites', label: '' }, // label 走 i18n（下方 computed）
}
function kindLabel(kind: string): string {
  if (kind === 'site-sync') return t('task.siteSync')
  return kind
}
function kindIcon(kind: string): string {
  return KIND_META[kind]?.icon ?? 'dashboard'
}
</script>

<template>
  <div v-if="activeTasks.length" class="task-indicator" role="status">
    <div
      v-for="p in activeTasks"
      :key="p.kind"
      class="ti-item"
      :title="p.site ? kindLabel(p.kind) + ' · ' + p.site : kindLabel(p.kind)"
      :aria-label="kindLabel(p.kind) + (p.site ? ' · ' + p.site : '') + ' ' + p.done + '/' + (p.total || '?')"
    >
      <GlassIcon :name="kindIcon(p.kind)" :size="16" class="ti-icon" />
      <span class="ti-count">{{ p.done }}/{{ p.total || '?' }}</span>
      <span class="ti-bar">
        <span class="ti-bar-fill" :style="{ width: p.total ? (p.done / p.total) * 100 + '%' : '100%' }"></span>
      </span>
    </div>
  </div>
</template>

<style scoped>
.task-indicator {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
}
.ti-item {
  display: flex;
  align-items: center;
  gap: var(--sp-1);
  padding: 3px 8px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-control);
  background: var(--color-surface-2);
}
.ti-icon {
  flex: none;
}
.ti-count {
  font-size: var(--fs-xs);
  color: var(--color-text-secondary);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.ti-bar {
  width: 48px;
  height: 4px;
  border-radius: var(--radius-pill);
  background: var(--color-border);
  overflow: hidden;
}
.ti-bar-fill {
  display: block;
  height: 100%;
  border-radius: var(--radius-pill);
  background: var(--color-primary);
  transition: width var(--dur-base) var(--ease);
}
</style>
