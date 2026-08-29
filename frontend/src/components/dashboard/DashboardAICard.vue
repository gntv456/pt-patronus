<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, computed, onMounted } from 'vue'
import { agentApi, type AgentStats } from '@/api/agent'
import Sparkline from './Sparkline.vue'

// DashboardAICard — 区④AI 助手用量与成本：消费 GET /agent/stats（13 维度，进程内内存，重启清零）。
const { t } = useI18n()
const stats = ref<AgentStats | null>(null)
async function load() {
  stats.value = await agentApi.stats().catch(() => null)
}
onMounted(load)
defineExpose({ refresh: load })

const totalTokens = computed(
  () => (stats.value?.total_input_tokens ?? 0) + (stats.value?.total_output_tokens ?? 0),
)
const errRate = computed(() => {
  const t = stats.value?.total_requests ?? 0
  const e = stats.value?.error_count ?? 0
  return t > 0 ? Math.round((e / t) * 100) : null
})
const sparkValues = computed(() =>
  (stats.value?.recent ?? [])
    .slice(-20)
    .map((r) => (r.input_tokens || 0) + (r.output_tokens || 0)),
)
function fmtToken(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`
  return String(n)
}
</script>

<template>
  <div class="dash-card">
    <div class="dc-head">
      <div>
        <h3>{{ t('daic.title') }}</h3>
        <p>{{ t('daic.sub') }}</p>
      </div>
    </div>

    <div v-if="!stats" class="dc-empty">{{ t('daic.empty') }}</div>
    <template v-else>
      <div class="ai-grid">
        <div class="ai-cell">
          <span class="ac-label">{{ t('daic.totalReq') }}</span>
          <strong>{{ stats.total_requests }}</strong>
          <small class="ac-sub">{{ t('daic.okTools', { ok: stats.ok_count, tools: stats.tool_calls }) }}</small>
        </div>
        <div class="ai-cell">
          <span class="ac-label">Token</span>
          <strong>{{ fmtToken(totalTokens) }}</strong>
          <small class="ac-sub">↑{{ fmtToken(stats.total_input_tokens) }} ↓{{
            fmtToken(stats.total_output_tokens)
          }}</small>
        </div>
        <div class="ai-cell">
          <span class="ac-label">{{ t('daic.cost') }}</span>
          <strong>${{ stats.total_cost_usd.toFixed(4) }}</strong>
          <small class="ac-sub">{{ t('daic.avgLatency', { s: Math.round((stats.avg_latency_ms ?? 0) / 1000) }) }}</small>
        </div>
        <div class="ai-cell">
          <span class="ac-label">{{ t('daic.errRate') }}</span>
          <strong :class="{ danger: (errRate ?? 0) > 0 }">{{ errRate === null ? '-' : errRate + '%' }}</strong>
          <small class="ac-sub">{{ t('daic.errRateDetail', { err: stats.error_count, rl: stats.rate_limit_count }) }}</small>
        </div>
      </div>
      <div v-if="sparkValues.length >= 2" class="ai-spark">
        <span class="spark-label">{{ t('daic.recentN', { n: sparkValues.length }) }}</span>
        <Sparkline :values="sparkValues" color="#8b5cf6" unit="count" />
      </div>
    </template>
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
.ai-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--sp-2);
}
.ai-cell {
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
.ai-cell strong {
  font-size: var(--fs-title);
  font-weight: 700;
  letter-spacing: -0.3px;
  line-height: 1.2;
}
.ai-cell strong.danger {
  color: var(--pt-danger);
}
.ac-sub {
  font-size: var(--fs-2xs);
  color: var(--text-muted);
  opacity: 0.85;
}
.ai-spark {
  margin-top: 12px;
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
}
.spark-label {
  font-size: var(--fs-2xs);
  color: var(--text-muted);
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
@media (max-width: 560px) {
  .ai-grid {
    grid-template-columns: 1fr 1fr;
  }
}
</style>
