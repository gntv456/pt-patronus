<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { computed } from 'vue'
import { useWs } from '@/composables/useWs'
import { fmtSpeed } from '@/utils/format'

// DashboardRealtimeCard — 区②下载器实时速率：直接消费 WS torrents 单例（DeepReadonly），
// 跨下载器汇总速率与任务状态。WS 自动更新，无需轮询。
const { t } = useI18n()
const { torrents } = useWs()

const count = computed(() => torrents.value.length)
// 防御 null torrents（后端出错下载器 snapshot.Torrents 为 Go nil → JSON null）
const snaps = computed(() => torrents.value.map((t) => ({ ...t, torrents: t.torrents ?? [] })))
const dlRate = computed(() => snaps.value.reduce((s, t) => s + (t.stats?.dl_rate ?? 0), 0))
const upRate = computed(() => snaps.value.reduce((s, t) => s + (t.stats?.up_rate ?? 0), 0))
const downloading = computed(() =>
  snaps.value.reduce((s, t) => s + t.torrents.filter((x) => /download|leech/i.test(x.state)).length, 0),
)
const seeding = computed(() =>
  snaps.value.reduce((s, t) => s + t.torrents.filter((x) => /upload|seed/i.test(x.state)).length, 0),
)
const total = computed(() => snaps.value.reduce((s, t) => s + t.torrents.length, 0))
const errored = computed(() => snaps.value.filter((t) => t.error).length)
</script>

<template>
  <div class="dash-card">
    <div class="dc-head">
      <div>
        <h3>{{ t('drc.title') }}</h3>
        <p>{{ count ? t('drc.subLive', { n: count }) : t('drc.subAgg') }}</p>
      </div>
      <span v-if="errored" class="dc-tag warn">{{ t('drc.offlineN', { n: errored }) }}</span>
    </div>
    <div v-if="count" class="dc-body">
      <div class="speed-row">
        <div class="speed-cell dl">
          <span class="lbl">↓ {{ t('drc.totalDl') }}</span>
          <strong>{{ fmtSpeed(dlRate) }}</strong>
        </div>
        <div class="speed-cell up">
          <span class="lbl">↑ {{ t('drc.totalUp') }}</span>
          <strong>{{ fmtSpeed(upRate) }}</strong>
        </div>
      </div>
      <div class="mini-row">
        <span
          ><b>{{ downloading }}</b> {{ t('drc.downloading') }}</span
        >
        <span
          ><b>{{ seeding }}</b> {{ t('drc.seeding') }}</span
        >
        <span
          ><b>{{ total }}</b> {{ t('drc.tasks') }}</span
        >
      </div>
    </div>
    <div v-else class="dc-empty">{{ t('drc.empty') }}</div>
  </div>
</template>

<style scoped>
.dash-card {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.dc-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--sp-2);
  margin-bottom: 12px;
}
.dc-head h3 {
  margin: 0;
  font-size: var(--fs-lg);
  line-height: 1.35;
}
.dc-head p {
  margin: 3px 0 0;
  color: var(--text-muted);
  font-size: var(--fs-2xs);
}
.dc-tag {
  flex: none;
  padding: 2px 8px;
  border-radius: var(--radius-pill);
  font-size: var(--fs-2xs);
}
.dc-tag.warn {
  color: #d97706;
  background: rgba(217, 119, 6, 0.13);
}
.dc-body {
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
  flex: 1;
}
.speed-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--sp-2);
}
.speed-cell {
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
  padding: 12px;
  border-radius: var(--radius-xs);
  background: var(--surface-muted);
}
.speed-cell .lbl {
  font-size: var(--fs-2xs);
  color: var(--text-muted);
}
.speed-cell strong {
  font-size: var(--fs-title);
  font-weight: 700;
  letter-spacing: -0.3px;
}
.speed-cell.dl strong {
  color: var(--pt-primary-2, #2563eb);
}
.speed-cell.up strong {
  color: var(--pt-success, #2ecc71);
}
.mini-row {
  display: flex;
  gap: var(--sp-4);
  font-size: var(--fs-xs);
  color: var(--text-muted);
}
.mini-row b {
  color: var(--text-strong);
  font-size: var(--fs-md);
  margin-right: 3px;
}
.dc-empty {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 90px;
  color: var(--text-muted);
  font-size: var(--fs-xs);
}
</style>
