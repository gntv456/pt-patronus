<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { entertainmentApi, type GameStats, type PrizeDef, type GameRecord, type PlayResult } from '@/api/entertainment'

const props = defineProps<{ siteId: number }>()
const GAME = 'novahd_scratch'

const stats = ref<GameStats | null>(null)
const prizes = ref<PrizeDef[]>([])
const records = ref<GameRecord[]>([])
const scratching = ref(false)
const last = ref<PlayResult | null>(null)

const remaining = computed(() => {
  if (!stats.value) return '—'
  return Math.max(0, stats.value.max_daily - stats.value.today_spins)
})

async function refreshStats() {
  stats.value = await entertainmentApi.stats(props.siteId, GAME).catch(() => stats.value)
}
async function refreshHistory() {
  records.value = await entertainmentApi.history(props.siteId, GAME).catch(() => [])
}

async function scratch() {
  if (scratching.value) return
  scratching.value = true
  try {
    const res = await entertainmentApi.play(props.siteId, GAME)
    last.value = res
    if (res.success) ElMessage.success(res.prize ? `${res.prize}：+${res.amount} 魔力值` : '完成')
    else ElMessage.warning(res.message || '刮奖失败')
    await Promise.all([refreshStats(), refreshHistory()])
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '请求失败')
  } finally {
    scratching.value = false
  }
}

onMounted(async () => {
  prizes.value = await entertainmentApi.prizes(props.siteId, GAME).catch(() => [] as PrizeDef[])
  await Promise.all([refreshStats(), refreshHistory()])
})
</script>

<template>
  <div class="scratch">
    <div class="hero">
      <div class="hero-title">🎫 魔力值刮刮乐</div>
      <div class="balance">余额 <b>{{ stats?.balance?.toLocaleString() ?? '—' }}</b> 魔力值</div>
    </div>

    <section class="glass section buy">
      <div class="sec-title">刮奖中心</div>
      <div class="buy-info">购买资格消耗 5000 魔力值 · 今日剩余 {{ remaining }}/{{ stats?.max_daily ?? 5 }}</div>
      <button class="buy-btn" :disabled="scratching" @click="scratch">
        <span v-if="scratching" class="spinner" />
        {{ scratching ? '刮奖中…' : '购买刮奖资格（消耗 5000 魔力值）' }}
      </button>
    </section>

    <div v-if="last" class="result" :class="last.success ? 'win' : 'lose'">
      <span class="badge">{{ last.success ? '🎉' : '✗' }}</span>
      <span class="r-msg">{{ last.success ? `${last.prize} · 获得 ${last.amount} 魔力值` : last.message }}</span>
    </div>

    <section class="glass section">
      <div class="sec-title">中奖规则</div>
      <div class="rule-grid">
        <div v-for="p in prizes" :key="p.id" class="rule-card" :class="{ miss: p.type === '99' }">
          {{ p.name }}
        </div>
      </div>
    </section>

    <section v-if="records.length" class="glass section">
      <div class="sec-title">我的刮奖记录</div>
      <div v-for="(r, i) in records.slice(0, 10)" :key="i" class="rec-row">
        <span :class="{ muted: r.prize_name === '未中奖' }">{{ r.prize_name }}</span>
        <span v-if="r.amount" class="rec-amt">+{{ r.amount }}</span>
        <span class="text-muted rec-time">{{ r.created_at }}</span>
      </div>
    </section>
  </div>
</template>

<style scoped>
.scratch { display: flex; flex-direction: column; gap: var(--sp-4); max-width: 640px; margin: 0 auto; }
.hero { padding: 18px; border-radius: var(--radius-md); background: linear-gradient(135deg, #e67e22, #f59e0b); text-align: center; }
.hero-title { font-size: var(--fs-title); font-weight: 800; color: #fff; }
.balance { margin-top: 6px; font-size: var(--fs-body); color: rgba(255,255,255,0.92); }
.balance b { color: #fff; font-size: var(--fs-xl); }

.section { padding: 16px; border-radius: var(--radius-md); }
.sec-title { font-weight: 700; margin-bottom: 12px; font-size: var(--fs-lg); }
.buy { text-align: center; }
.buy-info { font-size: var(--fs-body); color: var(--text-muted, #999); margin-bottom: 14px; }
.buy-btn { padding: 11px 24px; border: 0; border-radius: var(--radius-sm); cursor: pointer; font-weight: 700; color: #fff; background: linear-gradient(135deg, #22c55e, #16a34a); box-shadow: 0 4px 14px var(--pt-success-bg); display: inline-flex; align-items: center; gap: var(--sp-2); }
.buy-btn:disabled { opacity: 0.75; cursor: wait; }
.spinner { width: 16px; height: 16px; border: 2px solid rgba(255,255,255,0.4); border-top-color: #fff; border-radius: 50%; animation: sp 0.8s linear infinite; }
@keyframes sp { to { transform: rotate(360deg); } }

.rule-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: var(--sp-2); }
@media (max-width: 520px) { .rule-grid { grid-template-columns: repeat(2, 1fr); } }
.rule-card { padding: 12px 8px; border-radius: var(--radius-sm); text-align: center; font-size: var(--fs-body); font-weight: 600; background: var(--pt-warn-bg); color: #b45309; }
.rule-card.miss { background: rgba(148,163,184,0.12); color: var(--text-muted, #999); }

.result { padding: 12px; border-radius: var(--radius-sm); display: flex; gap: var(--sp-2); align-items: center; }
.result.win { background: var(--pt-success-bg); }
.result.lose { background: var(--pt-danger-bg); }
.badge { font-size: var(--fs-xl); }
.r-msg { font-size: var(--fs-body); font-weight: 600; }

.rec-row { display: flex; gap: var(--sp-2); align-items: center; padding: 6px 0; font-size: var(--fs-body); border-bottom: 1px solid var(--border-soft, rgba(255,255,255,0.05)); }
.rec-row .muted { color: var(--text-muted, #999); }
.rec-amt { color: var(--pt-success); font-weight: 600; }
.rec-time { margin-left: auto; font-size: var(--fs-2xs); }
</style>
