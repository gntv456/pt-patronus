<script setup lang="ts">
import type { SiteConnectivityStatus as SiteConnectivityStatusData } from '@/api/site'

const props = defineProps<{
  status?: SiteConnectivityStatusData
  loading?: boolean
}>()

const emit = defineEmits<{
  reason: []
}>()

function label() {
  if (props.loading) return '检测中'
  if (!props.status) return '未检测'
  if (props.status.status === 'connected') return '已联通'
  if (props.status.status === 'disabled') return '已停用'
  return '未联通'
}

function canOpenReason() {
  return !props.loading && !!props.status && props.status.status !== 'connected'
}
</script>

<template>
  <button
    v-if="canOpenReason()"
    type="button"
    class="connectivity-status is-clickable"
    :class="`is-${status?.status}`"
    title="点击查看状态原因"
    @click="emit('reason')"
  >
    <span class="connectivity-dot" aria-hidden="true" />
    <span>{{ label() }}</span>
  </button>
  <span v-else class="connectivity-status" :class="loading || !status ? 'is-pending' : `is-${status.status}`">
    <span class="connectivity-dot" aria-hidden="true" />
    <span>{{ label() }}</span>
  </span>
</template>

<style scoped>
.connectivity-status {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: 24px;
  padding: 2px 8px;
  border: 0;
  border-radius: 999px;
  font: inherit;
  font-size: 12px;
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
  background: rgba(46, 204, 113, 0.16);
}
.is-disconnected {
  color: var(--pt-danger, #dc2626);
  background: rgba(231, 76, 60, 0.14);
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
  50% { opacity: 0.3; }
}
</style>
