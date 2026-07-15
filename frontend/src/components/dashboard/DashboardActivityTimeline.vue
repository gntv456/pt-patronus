<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { noticeApi, type Notice } from '@/api/notice'
import { fmtTime } from '@/utils/format'

// DashboardActivityTimeline — 区⑥最近活动：聚合通知中心（notice 表已覆盖整理/下载/订阅/契约/工作流等
// 23 类事件源）。source→emoji 映射常见来源，未匹配用通用铃铛。
const items = ref<Notice[]>([])
async function load() {
  const all = await noticeApi.list().catch(() => [] as Notice[])
  items.value = all.slice(0, 30)
}
onMounted(load)
defineExpose({ refresh: load })

const SOURCE_ICON: Record<string, string> = {
  dl_done: '📥',
  transfer_done: '🎬',
  transfer_fail: '⚠️',
  subscribe_hit: '🎯',
  rss_hit: '📰',
  workflow: '⚙️',
  reseed_done: '🌱',
  freeleech: '🎁',
  announce: '📢',
  new_mail: '✉️',
  assess_pass: '✅',
  assess_fail: '❌',
  site_offline: '🔴',
  host_offline: '🔴',
  nas_offline: '🔴',
  backup_done: '💾',
  backup_import: '💾',
  site_anniv: '🎂',
  medal_new: '🏅',
  medal_available: '🏅',
  medal_price_drop: '🏅',
  medal_price_rise: '🏅',
  medal_out_of_stock: '🏅',
  webhook: '🔗',
}
function icon(src: string): string {
  return SOURCE_ICON[src] || '🔔'
}
// 紧凑时间：MM-DD HH:mm
function shortTime(s: string): string {
  const full = fmtTime(s)
  return full === '-' ? '-' : full.slice(5)
}
</script>

<template>
  <div class="dash-card">
    <div class="dc-head">
      <div>
        <h3>最近活动</h3>
        <p>整理 · 下载 · 订阅 · 契约 · 工作流等事件流</p>
      </div>
    </div>

    <div v-if="items.length" class="timeline">
      <div v-for="it in items" :key="it.id" class="tl-item">
        <span class="tl-dot" :class="`lv-${it.level || 'info'}`" />
        <span class="tl-icon">{{ icon(it.source) }}</span>
        <div class="tl-body">
          <span class="tl-title">{{ it.title }}</span>
          <span class="tl-src">{{ it.source }}</span>
        </div>
        <span class="tl-time">{{ shortTime(it.created_at) }}</span>
      </div>
    </div>
    <div v-else class="dc-empty">暂无最近活动</div>
  </div>
</template>

<style scoped>
.dash-card {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.dc-head {
  margin-bottom: 10px;
}
.dc-head h3 {
  margin: 0;
  font-size: 15px;
}
.dc-head p {
  margin: 3px 0 0;
  color: var(--text-muted);
  font-size: 11px;
}
.timeline {
  flex: 1;
  overflow-y: auto;
  max-height: 320px;
  display: flex;
  flex-direction: column;
}
.tl-item {
  display: grid;
  grid-template-columns: 8px 22px minmax(0, 1fr) auto;
  align-items: center;
  gap: 9px;
  padding: 7px 2px;
  border-bottom: 1px solid var(--divider);
}
.tl-item:last-child {
  border-bottom: 0;
}
.tl-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--text-muted);
}
.tl-dot.lv-success {
  background: #16a34a;
}
.tl-dot.lv-warning {
  background: #d97706;
}
.tl-dot.lv-danger {
  background: #dc2626;
}
.tl-dot.lv-info {
  background: #2563eb;
}
.tl-icon {
  font-size: 15px;
  text-align: center;
}
.tl-body {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}
.tl-title {
  font-size: 12.5px;
  color: var(--text-strong);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tl-src {
  font-size: 10.5px;
  color: var(--text-muted);
}
.tl-time {
  font-size: 11px;
  color: var(--text-muted);
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}
.dc-empty {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 120px;
  color: var(--text-muted);
  font-size: 12.5px;
}
</style>
