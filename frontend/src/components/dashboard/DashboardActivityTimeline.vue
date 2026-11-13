<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, onMounted } from 'vue'
import { noticeApi, type Notice } from '@/api/notice'
import { fmtTime } from '@/utils/format'
import SptIcon from '@/components/SptIcon.vue'

// DashboardActivityTimeline — 区⑥最近活动：聚合通知中心（notice 表已覆盖整理/下载/订阅/契约/工作流等
// 23 类事件源）。source→emoji 映射常见来源，未匹配用通用铃铛。
const { t } = useI18n()
const items = ref<Notice[]>([])
const failed = ref(false)
const loading = ref(true)
async function load() {
  failed.value = false
  loading.value = true
  try {
    const all = await noticeApi.list().catch(() => {
      // 读不到通知中心 ≠ 近期没有活动：不标记就会把故障画成空时间线。
      failed.value = true
      return [] as Notice[]
    })
    items.value = all.slice(0, 30)
  } finally {
    loading.value = false
  }
}
onMounted(load)
defineExpose({ refresh: load })

// 结构性图标统一用单色 SptIcon（去 emoji），来源 -> 图标名。
const SOURCE_ICON: Record<string, string> = {
  dl_done: 'download',
  transfer_done: 'film',
  transfer_fail: 'alert-triangle',
  subscribe_hit: 'target',
  subscribe_list_fail: 'alert-octagon',
  rss_list_fail: 'alert-octagon',
  strm_list_fail: 'alert-octagon',
  brushflow_list_fail: 'alert-octagon',
  contract_list_fail: 'alert-octagon',
  dirsync_list_fail: 'alert-octagon',
  brushflow: 'refresh',
  cloud: 'cloud',
  contract: 'file',
  mediaserver: 'film',
  rss_hit: 'rss',
  workflow: 'git-branch',
  reseed_done: 'seed',
  freeleech: 'gift',
  announce: 'megaphone',
  new_mail: 'mail',
  assess_pass: 'check-circle',
  assess_fail: 'x',
  site_offline: 'wifi',
  host_offline: 'server',
  nas_offline: 'hard-drive',
  camera_offline: 'eye-off',
  backup_done: 'save',
  backup_import: 'download',
  site_anniv: 'calendar',
  medal_new: 'medal',
  medal_available: 'medal',
  medal_price_drop: 'trending-down',
  medal_price_rise: 'trending-up',
  medal_out_of_stock: 'package',
  webhook: 'link',
}
function icon(src: string): string {
  return SOURCE_ICON[src] || 'bell'
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
        <h3>{{ t('dat.title') }}</h3>
        <p>{{ t('dat.sub') }}</p>
      </div>
      <span v-if="failed" class="dc-tag">{{ t('common.loadFailed') }}</span>
    </div>

    <div v-if="loading" class="dc-empty tl-loading" aria-busy="true">
      <el-skeleton :rows="4" animated style="width: 100%" />
    </div>
    <div v-else-if="items.length" class="timeline">
      <div v-for="it in items" :key="it.id" class="tl-item">
        <span class="tl-dot" :class="`lv-${it.level || 'info'}`" />
        <SptIcon :name="icon(it.source)" :size="16" class="tl-icon" aria-hidden="true" />
        <div class="tl-body">
          <span class="tl-title">{{ it.title }}</span>
          <span class="tl-src">{{ it.source }}</span>
        </div>
        <span class="tl-time">{{ shortTime(it.created_at) }}</span>
      </div>
    </div>
    <div v-else class="dc-empty">{{ t('dat.empty') }}</div>
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
  font-size: var(--fs-lg);
}
.dc-head p {
  margin: 3px 0 0;
  color: var(--text-muted);
  font-size: var(--fs-2xs);
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
  gap: var(--sp-2);
  min-height: 36px;
  padding: 0 8px;
  border-bottom: 1px solid var(--color-border);
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
  background: var(--color-success);
}
.tl-dot.lv-warning {
  background: var(--color-warning);
}
.tl-dot.lv-danger {
  background: var(--color-danger);
}
.tl-dot.lv-info {
  background: var(--chart-1);
}
.tl-icon {
  justify-self: center;
  color: var(--color-text-muted);
}
.tl-loading {
  min-height: 120px;
}
.tl-body {
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
  min-width: 0;
}
.tl-title {
  font-size: var(--fs-xs);
  color: var(--text-strong);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tl-src {
  font-size: var(--fs-2xs);
  color: var(--text-muted);
}
.tl-time {
  font-size: var(--fs-2xs);
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
  font-size: var(--fs-xs);
}
</style>
