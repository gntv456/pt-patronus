<script setup lang="ts">
import { fmtSpeed } from '@/utils/format'

defineProps<{
  stats?: { dl_rate: number; up_rate: number } | null
  active: number
  seeding: number
  paused: number
  total: number
}>()
</script>

<template>
  <div class="statsbar glass">
    <div class="stat">
      <span class="lbl">↓ 总下载</span>
      <span class="val dl">{{ fmtSpeed(stats?.dl_rate ?? 0) }}</span>
    </div>
    <div class="stat">
      <span class="lbl">↑ 总上传</span>
      <span class="val up">{{ fmtSpeed(stats?.up_rate ?? 0) }}</span>
    </div>
    <div class="divider" />
    <div class="stat"><span class="lbl">下载中</span><span class="val">{{ active }}</span></div>
    <div class="stat"><span class="lbl">做种</span><span class="val">{{ seeding }}</span></div>
    <div class="stat"><span class="lbl">暂停</span><span class="val">{{ paused }}</span></div>
    <div class="stat"><span class="lbl">总计</span><span class="val">{{ total }}</span></div>
  </div>
</template>

<style scoped>
.statsbar {
  display: flex;
  gap: 22px;
  padding: 12px 16px;
  margin-bottom: 14px;
  flex-wrap: wrap;
  align-items: center;
}
.stat {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.lbl {
  font-size: 11px;
  color: var(--text-muted);
}
.val {
  font-size: 15px;
  font-weight: 600;
}
.val.dl {
  color: var(--pt-primary-2);
}
.val.up {
  color: var(--pt-success);
}
.divider {
  width: 1px;
  height: 28px;
  background: rgba(127, 127, 127, 0.25);
}
</style>
