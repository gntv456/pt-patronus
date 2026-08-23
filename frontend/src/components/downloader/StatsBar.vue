<script setup lang="ts">
import { fmtSpeed } from '@/utils/format'

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
  <section class="statsbar" aria-label="下载器实时统计">
    <div class="metric metric-download">
      <span class="metric-label">下载速度</span>
      <strong>{{ fmtSpeed(stats?.dl_rate ?? 0) }}</strong>
    </div>
    <div class="metric metric-upload">
      <span class="metric-label">上传速度</span>
      <strong>{{ fmtSpeed(stats?.up_rate ?? 0) }}</strong>
    </div>
    <div class="metric">
      <span class="metric-label">下载中</span>
      <strong>{{ active }}</strong>
    </div>
    <div class="metric">
      <span class="metric-label">做种中</span>
      <strong>{{ seeding }}</strong>
    </div>
    <div class="metric metric-muted">
      <span class="metric-label">暂停</span>
      <strong>{{ paused }}</strong>
    </div>
    <div class="metric metric-error">
      <span class="metric-label">异常</span>
      <strong>{{ errors }}</strong>
    </div>
    <div class="metric metric-total">
      <span class="metric-label">任务总数</span>
      <strong>{{ total }}</strong>
    </div>
  </section>
</template>

<style scoped>
.statsbar { display: grid; grid-template-columns: 1.35fr 1.35fr repeat(5, 1fr); gap: var(--sp-1); overflow: hidden; margin-bottom: 10px; border: 1px solid var(--glass-border, rgba(127,127,127,.18)); border-radius: var(--radius-xs); background: var(--glass-border, rgba(127,127,127,.18)); }
.metric { display: flex; min-width: 0; flex-direction: column; justify-content: center; gap: var(--sp-1); min-height: 48px; padding: 8px 12px; background: var(--surface, rgba(255,255,255,.04)); }
.metric-label { color: var(--text-muted); font-size: var(--fs-2xs); }
.metric strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: var(--fs-lg); line-height: 1.1; }
.metric-download strong { color: var(--pt-primary-2); }
.metric-upload strong { color: var(--pt-success); }
.metric-muted strong { color: var(--text-muted); }
.metric-error strong { color: var(--pt-danger); }
.metric-total { background: color-mix(in srgb, var(--pt-primary-2) 7%, var(--surface, transparent)); }

@media (max-width: 760px) {
  .statsbar { grid-template-columns: repeat(2, 1fr); }
}
</style>
