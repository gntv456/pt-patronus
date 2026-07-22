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
      <span class="metric-caption">当前总速率</span>
    </div>
    <div class="metric metric-upload">
      <span class="metric-label">上传速度</span>
      <strong>{{ fmtSpeed(stats?.up_rate ?? 0) }}</strong>
      <span class="metric-caption">当前总速率</span>
    </div>
    <div class="metric">
      <span class="metric-label">下载中</span>
      <strong>{{ active }}</strong>
      <span class="metric-caption">个任务</span>
    </div>
    <div class="metric">
      <span class="metric-label">做种中</span>
      <strong>{{ seeding }}</strong>
      <span class="metric-caption">个任务</span>
    </div>
    <div class="metric metric-muted">
      <span class="metric-label">暂停</span>
      <strong>{{ paused }}</strong>
      <span class="metric-caption">个任务</span>
    </div>
    <div class="metric metric-error">
      <span class="metric-label">异常</span>
      <strong>{{ errors }}</strong>
      <span class="metric-caption">需处理</span>
    </div>
    <div class="metric metric-total">
      <span class="metric-label">任务总数</span>
      <strong>{{ total }}</strong>
      <span class="metric-caption">实时快照</span>
    </div>
  </section>
</template>

<style scoped>
.statsbar { display: grid; grid-template-columns: 1.35fr 1.35fr repeat(5, 1fr); gap: 1px; overflow: hidden; margin-bottom: 18px; border: 1px solid var(--glass-border, rgba(127,127,127,.18)); border-radius: 8px; background: var(--glass-border, rgba(127,127,127,.18)); }
.metric { display: flex; min-width: 0; flex-direction: column; justify-content: center; gap: 3px; min-height: 82px; padding: 13px 15px; background: var(--surface, rgba(255,255,255,.04)); }
.metric-label { color: var(--text-muted); font-size: 11px; }
.metric strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 19px; line-height: 1.1; }
.metric-caption { color: var(--text-muted); font-size: 10px; }
.metric-download strong { color: var(--pt-primary-2); }
.metric-upload strong { color: var(--pt-success); }
.metric-muted strong { color: var(--text-muted); }
.metric-error strong { color: var(--pt-danger); }
.metric-total { background: color-mix(in srgb, var(--pt-primary-2) 7%, var(--surface, transparent)); }

@media (max-width: 760px) {
  .statsbar { grid-template-columns: repeat(2, 1fr); }
}
</style>
