<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { SiteConnectivityStatus as SiteConnectivityStatusData } from '@/api/site'

const { t } = useI18n()

const props = defineProps<{
  status?: SiteConnectivityStatusData
  loading?: boolean
}>()

const emit = defineEmits<{
  reason: []
}>()

function label() {
  if (props.loading) return t('scs.checking')
  if (!props.status) return t('scs.notChecked')
  if (props.status.status === 'connected') return t('scs.connected')
  if (props.status.status === 'disabled') return t('scs.disabled')
  return t('scs.disconnected')
}

function canOpenReason() {
  return !props.loading && !!props.status && props.status.status !== 'connected'
}

// 上次检测相对时间（鼠标悬停 title 用，让用户感知缓存新旧而非误以为是实时）；无 checked_at 返回空串。
function lastCheckedLabel(): string {
  const checkedAt = props.status?.checked_at
  if (!checkedAt) return ''
  const ms = Date.now() - new Date(checkedAt).getTime()
  if (Number.isNaN(ms) || ms < 0) return ''
  const min = Math.floor(ms / 60000)
  if (min < 1) return t('scs.lastJustNow')
  if (min < 60) return t('scs.lastMin', { n: min })
  const hr = Math.floor(min / 60)
  if (hr < 24) return t('scs.lastHr', { n: hr })
  return t('scs.lastDay', { n: Math.floor(hr / 24) })
}

// 站点稳定性统计（成功率 + 平均耗时，对标 MP SiteStatistic）；无累计数据返回空串。
function statsLabel(): string {
  const s = props.status?.success ?? 0
  const f = props.status?.fail ?? 0
  if (s + f === 0) return ''
  const rate = Math.round((s / (s + f)) * 100)
  const avg = props.status?.avg_seconds ?? 0
  const avgStr = avg > 0 ? t('scs.avg', { v: avg < 1 ? Math.round(avg * 1000) + 'ms' : avg.toFixed(1) + 's' }) : ''
  return t('scs.rate', { rate, s, total: s + f }) + avgStr
}

// badge title：优先显示统计 + 上次检测；都无则用 fallback。
function fullTitle(fallback: string): string {
  const parts = [statsLabel(), lastCheckedLabel()].filter(Boolean)
  return parts.length ? parts.join(' · ') : fallback
}
</script>

<template>
  <button
    v-if="canOpenReason()"
    type="button"
    class="connectivity-status is-clickable"
    :class="`is-${status?.status}`"
    :title="fullTitle(t('scs.viewReason'))"
    @click="emit('reason')"
  >
    <span class="connectivity-dot" aria-hidden="true" />
    <span>{{ label() }}</span>
  </button>
  <span
    v-else
    class="connectivity-status"
    :class="loading || !status ? 'is-pending' : `is-${status.status}`"
    :title="fullTitle('')"
  >
    <span class="connectivity-dot" aria-hidden="true" />
    <span>{{ label() }}</span>
  </span>
</template>

<style scoped>
.connectivity-status {
  display: inline-flex;
  align-items: center;
  gap: var(--sp-1);
  min-height: 24px;
  padding: 2px 8px;
  border: 0;
  border-radius: var(--radius-pill);
  font: inherit;
  font-size: var(--fs-xs);
  line-height: 18px;
  white-space: nowrap;
}
.connectivity-status.is-clickable {
  cursor: pointer;
}
.connectivity-status.is-clickable:hover {
  filter: brightness(1.12);
}
.connectivity-dot {
  width: 7px;
  height: 7px;
  flex: 0 0 7px;
  border-radius: 50%;
  background: currentColor;
}
.is-connected {
  color: var(--pt-success, #16a34a);
  background: var(--pt-success-bg);
}
.is-disconnected {
  color: var(--pt-danger, #dc2626);
  background: var(--pt-danger-bg);
}
.is-disabled {
  color: var(--text-muted, #94a3b8);
  background: rgba(148, 163, 184, 0.14);
}
.is-pending {
  color: var(--text-muted, #94a3b8);
  background: rgba(148, 163, 184, 0.1);
}
.is-pending .connectivity-dot {
  animation: connectivity-pulse 1.1s ease-in-out infinite;
}
@keyframes connectivity-pulse {
  50% {
    opacity: 0.3;
  }
}
</style>
