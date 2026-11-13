<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { fmtSpeed } from '@/utils/format'

const { t } = useI18n()

defineProps<{
  stats?: { dl_rate: number; up_rate: number } | null
  active: number
  seeding: number
  paused: number
  errors: number
  total: number
}>()
</script>

<template>
  <section class="statsbar" :aria-label="t('sb.aria')">
    <div class="metric metric-download">
      <span class="metric-label">{{ t('sb.dlRate') }}</span>
      <strong>{{ fmtSpeed(stats?.dl_rate ?? 0) }}</strong>
    </div>
    <div class="metric metric-upload">
      <span class="metric-label">{{ t('sb.upRate') }}</span>
      <strong>{{ fmtSpeed(stats?.up_rate ?? 0) }}</strong>
    </div>
    <div class="metric">
      <span class="metric-label">{{ t('drc.downloading') }}</span>
      <strong>{{ active }}</strong>
    </div>
    <div class="metric">
      <span class="metric-label">{{ t('sb.seedingNow') }}</span>
      <strong>{{ seeding }}</strong>
    </div>
    <div class="metric metric-muted">
      <span class="metric-label">{{ t('sb.paused') }}</span>
      <strong>{{ paused }}</strong>
    </div>
    <div class="metric metric-error">
      <span class="metric-label">{{ t('sb.errors') }}</span>
      <strong>{{ errors }}</strong>
    </div>
    <div class="metric metric-total">
      <span class="metric-label">{{ t('sb.total') }}</span>
      <strong>{{ total }}</strong>
    </div>
  </section>
</template>

<style scoped>
.statsbar {
  display: grid;
  grid-template-columns: 1.35fr 1.35fr repeat(5, 1fr);
  gap: var(--sp-1);
  overflow: hidden;
  margin-bottom: 10px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-xs);
  background: var(--color-border);
}
.metric {
  display: flex;
  min-width: 0;
  flex-direction: column;
  justify-content: center;
  gap: var(--sp-1);
  min-height: 48px;
  padding: 8px 12px;
  background: var(--color-surface);
}
.metric-label {
  color: var(--text-muted);
  font-size: var(--fs-2xs);
}
.metric strong {
  font-variant-numeric: tabular-nums;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--fs-lg);
  line-height: 1.1;
}
.metric-download strong {
  color: var(--pt-primary-2);
}
.metric-upload strong {
  color: var(--pt-success);
}
.metric-muted strong {
  color: var(--text-muted);
}
.metric-error strong {
  color: var(--pt-danger);
}
.metric-total {
  background: var(--color-primary-soft);
}

@media (max-width: 760px) {
  .statsbar {
    grid-template-columns: repeat(2, 1fr);
  }
}
</style>
