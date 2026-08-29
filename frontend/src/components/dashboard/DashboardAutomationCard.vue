<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, computed, onMounted } from 'vue'
import { workflowApi, type WorkflowStats } from '@/api/workflow'
import { brushflowApi, type BrushFlowTask } from '@/api/brushflow'
import { contractApi, type ContractRule } from '@/api/contract'
import { transferApi, type TransferHistory } from '@/api/transfer'
import { fmtBytes } from '@/utils/format'
import DashboardRingChart, { type RingSegment } from './DashboardRingChart.vue'

// DashboardAutomationCard — 区③自动化运行：并发聚合工作流/刷流/契约/整理四源 + 契约状态分布环形图。
// 各源 allSettled，单项失败不阻断（.catch→空），对齐 Dashboard 现有并发范式。
const { t } = useI18n()
const wf = ref<WorkflowStats | null>(null)
const brush = ref<BrushFlowTask[]>([])
const contract = ref<ContractRule[]>([])
const transfer = ref<TransferHistory[]>([])

async function load() {
  const [w, b, c, t] = await Promise.all([
    workflowApi.stats().catch(() => null),
    brushflowApi.list().catch(() => [] as BrushFlowTask[]),
    contractApi.list().catch(() => [] as ContractRule[]),
    transferApi.history('', 200).catch(() => [] as TransferHistory[]),
  ])
  wf.value = w
  brush.value = b
  contract.value = c
  transfer.value = t
}
onMounted(load)
defineExpose({ refresh: load })

const brushEnabled = computed(() => brush.value.filter((t) => t.enabled).length)
const brushUpload = computed(() => brush.value.reduce((s, t) => s + (t.total_upload || 0), 0))
const brushAdded = computed(() => brush.value.reduce((s, t) => s + (t.total_added || 0), 0))

const contractSegments = computed<RingSegment[]>(() => {
  const en = contract.value.filter((r) => r.enabled)
  const cnt = (s: string) => en.filter((r) => r.last_status === s).length
  return [
    { label: t('dac.ok'), value: cnt('ok'), color: '#16a34a' },
    { label: t('dac.breach'), value: cnt('breach'), color: '#dc2626' },
    { label: t('dac.error'), value: cnt('error'), color: '#d97706' },
    { label: t('dac.unchecked'), value: en.filter((r) => !r.last_status).length, color: '#94a3b8' },
  ]
})
const breachCount = computed(() => contractSegments.value[1]?.value ?? 0)

const tSuccess = computed(() => transfer.value.filter((h) => h.status === 'success').length)
const tFailed = computed(() => transfer.value.filter((h) => h.status === 'failed').length)
const tRate = computed(() => {
  const tot = tSuccess.value + tFailed.value
  return tot > 0 ? Math.round((tSuccess.value / tot) * 100) : null
})
const todayStr = new Date().toISOString().slice(0, 10)
const tToday = computed(() => transfer.value.filter((h) => (h.created_at || '').slice(0, 10) === todayStr).length)
</script>

<template>
  <div class="dash-card">
    <div class="dc-head">
      <div>
        <h3>{{ t('dac.title') }}</h3>
        <p>{{ t('dac.sub') }}</p>
      </div>
      <span v-if="breachCount" class="dc-tag danger">{{ t('dac.breachTag', { n: breachCount }) }}</span>
    </div>

    <div class="auto-body">
      <div class="auto-grid">
        <div class="auto-cell">
          <span class="ac-label">{{ t('dac.wf') }}</span>
          <strong
            >{{ wf?.enabled ?? '-' }}<small>/{{ wf?.total ?? 0 }}</small></strong
          >
          <small class="ac-sub">{{ t('dac.enabledTotal') }}</small>
          <small class="ac-sub2">{{
            t('dac.wfRuns', { ok: wf?.runs_success ?? 0, fail: wf?.runs_failed ?? 0 })
          }}</small>
        </div>
        <div class="auto-cell">
          <span class="ac-label">{{ t('dac.brush') }}</span>
          <strong>{{ brushEnabled }}</strong>
          <small class="ac-sub">{{ t('dac.enabledTasks') }}</small>
          <small class="ac-sub2">{{ t('dac.brushStats', { up: fmtBytes(brushUpload), n: brushAdded }) }}</small>
        </div>
        <div class="auto-cell">
          <span class="ac-label">{{ t('dac.transfer') }}</span>
          <strong>{{ tRate === null ? '-' : tRate + '%' }}</strong>
          <small class="ac-sub">{{ t('dac.successRate') }}</small>
          <small class="ac-sub2">{{ t('dac.transferStats', { today: tToday, fail: tFailed }) }}</small>
        </div>
        <div class="auto-cell">
          <span class="ac-label">{{ t('dac.breachLabel') }}</span>
          <strong :class="{ danger: breachCount }">{{ breachCount }}</strong>
          <small class="ac-sub">{{ t('dac.needsAttn') }}</small>
          <small class="ac-sub2">{{ t('dac.contractsN', { n: contract.filter((r) => r.enabled).length }) }}</small>
        </div>
      </div>

      <div class="auto-ring">
        <div class="ar-title">{{ t('dac.distTitle') }}</div>
        <DashboardRingChart :items="contractSegments" :size="116" :thickness="13" legend />
      </div>
    </div>
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
.dc-tag.danger {
  color: var(--pt-danger);
  background: var(--pt-danger-bg);
}
.auto-body {
  display: flex;
  gap: var(--sp-4);
  align-items: stretch;
  flex: 1;
}
.auto-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--sp-2);
  flex: 1;
}
.auto-cell {
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
  padding: 11px;
  border-radius: var(--radius-xs);
  background: var(--surface-muted);
}
.ac-label {
  font-size: var(--fs-2xs);
  color: var(--text-muted);
}
.auto-cell strong {
  font-size: var(--fs-title);
  font-weight: 700;
  letter-spacing: -0.3px;
  line-height: 1.2;
}
.auto-cell strong small {
  font-size: var(--fs-body);
  font-weight: 600;
  color: var(--text-muted);
}
.auto-cell strong.danger {
  color: var(--pt-danger);
}
.ac-sub {
  font-size: var(--fs-2xs);
  color: var(--text-muted);
}
.ac-sub2 {
  font-size: var(--fs-2xs);
  color: var(--text-muted);
  opacity: 0.85;
}
.auto-ring {
  flex: none;
  width: 220px;
  padding: 12px;
  border-radius: var(--radius-xs);
  background: var(--surface-muted);
}
.ar-title {
  font-size: var(--fs-xs);
  color: var(--text-muted);
  margin-bottom: 10px;
}
.auto-ring :deep(.ring-chart) {
  flex-direction: column;
  gap: var(--sp-2);
}
.auto-ring :deep(.ring-legend) {
  font-size: var(--fs-2xs);
}
@media (max-width: 900px) {
  .auto-body {
    flex-direction: column;
  }
  .auto-ring {
    width: 100%;
  }
}
@media (max-width: 480px) {
  .auto-grid {
    grid-template-columns: 1fr;
  }
}
</style>
