<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import SptIcon from '@/components/SptIcon.vue'

const { t } = useI18n()
// 统一空态：单色线性图标 + 主文案 + 副文案 + 引导操作（插槽）。
// icon prop 保留仅为兼容历史调用点（传入的是 emoji 字符串），模板统一渲染 SptIcon。
withDefaults(
  defineProps<{
    icon?: string
    title?: string
    description?: string
  }>(),
  { icon: '', title: undefined, description: '' },
)
</script>

<template>
  <div class="empty-state">
    <div class="es-icon"><SptIcon name="info" :size="24" aria-hidden="true" /></div>
    <div v-if="title ?? true" class="es-title">{{ title ?? t('common.noData') }}</div>
    <div v-if="description" class="es-desc">{{ description }}</div>
    <div v-if="$slots.default" class="es-actions">
      <slot />
    </div>
  </div>
</template>

<style scoped>
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-8) var(--space-4);
  text-align: center;
}
.es-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  background: var(--color-surface-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-card);
  color: var(--color-text-secondary);
  line-height: 1;
}
.es-title {
  font-size: var(--fs-md);
  font-weight: 600;
  color: var(--color-text);
}
.es-desc {
  font-size: var(--fs-sm);
  color: var(--color-text-secondary);
  max-width: 340px;
  line-height: 1.5;
}
.es-actions {
  margin-top: var(--space-2);
  display: flex;
  gap: var(--space-2);
}
</style>
