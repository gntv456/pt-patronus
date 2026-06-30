<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { entertainmentApi, type GameStats, type PrizeDef, type GameRecord, type PlayResult } from '@/api/entertainment'

// 通用抽奖组件（批1）：覆盖多站幸运转盘/老虎机/抽奖。按 game 调 play/playMulti/stats/prizes/history。
const props = defineProps<{ siteId: number; game: string; name: string }>()

const stats = ref<GameStats | null>(null)
const prizes = ref<PrizeDef[]>([])
const records = ref<GameRecord[]>([])
const playing = ref(false)
const last = ref<PlayResult | null>(null)

async function refreshStats() {
  stats.value = await entertainmentApi.stats(props.siteId, props.game).catch(() => stats.value)
}
async function refreshHistory() {
  records.value = await entertainmentApi.history(props.siteId, props.game).catch(() => [])
}

async function draw(count: number) {
  if (playing.value) return
  playing.value = true
  try {
    const res = count === 1
      ? await entertainmentApi.play(props.siteId, props.game)
      : await entertainmentApi.playMulti(props.siteId, props.game, count)
    last.value = res
    if (res.success) {
      const wins = (res.prizes || []).filter((p) => p.is_win)
      ElMessage.success(wins.length ? `🎉 中奖 ${wins.length} 次` : (res.prize || '完成'))
    } else {
      ElMessage.warning(res.message || '抽奖失败')
    }
    await Promise.all([refreshStats(), refreshHistory()])
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '请求失败')
  } finally {
    playing.value = false
  }
}

onMounted(async () => {
  prizes.value = await entertainmentApi.prizes(props.siteId, props.game).catch(() => [] as PrizeDef[])
  await Promise.all([refreshStats(), refreshHistory()])
})
</script>

<template>
  <div class="spinner">
    <div class="hero">
      <div class="hero-title">🎰 {{ name }}</div>
      <div class="balance">余额 <b>{{ stats?.balance?.toLocaleString() ?? '—' }}</b></div>
    </div>

    <section class="glass section ops">
      <button class="op primary" :disabled="playing" @click="draw(1)">{{ playing ? '抽奖中…' : '立即抽奖' }}</button>
      <button class="op" :disabled="playing" @click="draw(10)">连抽 10 次</button>
      <button class="op" :disabled="playing" @click="draw(20)">连抽 20 次</button>
      <button class="op" :disabled="playing" @click="draw(50)">连抽 50 次</button>
    </section>

    <div v-if="last" class="result" :class="last.success ? 'win' : 'lose'">
      <span class="badge">{{ last.success ? '🎉' : '✗' }}</span>
      <div class="r-body">
        <span v-if="last.prize" class="r-prize">{{ last.prize }}</span>
        <span v-if="last.balance" class="r-bal">余额 {{ last.balance.toLocaleString() }}</span>
        <div v-if="last.message" class="r-msg text-muted">{{ last.message }}</div>
      </div>
    </div>

    <section v-if="prizes.length" class="glass section">
      <div class="sec-title">奖品</div>
      <div class="prize-strip">
        <div v-for="p in prizes" :key="p.id" class="prize-card">{{ p.name }}</div>
      </div>
    </section>

    <section v-if="records.length" class="glass section">
      <div class="sec-title">抽奖记录</div>
      <div v-for="(r, i) in records.slice(0, 10)" :key="i" class="rec-row">
        <span :class="{ muted: r.prize_name === '未中奖' }">{{ r.prize_name }}</span>
        <span v-if="r.amount" class="rec-amt">×{{ r.amount }}</span>
        <span class="text-muted rec-time">{{ r.created_at }}</span>
      </div>
    </section>
  </div>
</template>

<style scoped>
.spinner { display: flex; flex-direction: column; gap: 16px; max-width: 640px; margin: 0 auto; }
.hero { padding: 18px; border-radius: 14px; background: linear-gradient(135deg, #6366f1, #8b5cf6); text-align: center; }
.hero-title { font-size: 22px; font-weight: 800; color: #fff; }
.balance { margin-top: 4px; font-size: 13px; color: rgba(255,255,255,0.92); }
.balance b { color: #fde68a; font-size: 16px; }

.section { padding: 16px; border-radius: 14px; }
.sec-title { font-weight: 700; margin-bottom: 10px; font-size: 15px; }
.ops { display: flex; gap: 8px; flex-wrap: wrap; justify-content: center; }
.op { padding: 9px 16px; border: 1px solid #6366f1; border-radius: 10px; cursor: pointer; font-size: 13px; font-weight: 600; color: #6366f1; background: transparent; }
.op.primary { color: #fff; background: linear-gradient(135deg, #6366f1, #8b5cf6); border: 0; box-shadow: 0 4px 12px rgba(99,102,241,0.4); }
.op:disabled { opacity: 0.6; cursor: wait; }

.result { padding: 12px; border-radius: 12px; display: flex; gap: 10px; align-items: flex-start; }
.result.win { background: rgba(34, 197, 94, 0.12); }
.result.lose { background: rgba(231, 76, 60, 0.1); }
.badge { font-size: 18px; }
.r-body { display: flex; flex-direction: column; gap: 2px; }
.r-prize { font-weight: 700; color: #16a34a; }
.r-bal { font-size: 12px; color: var(--text-muted, #999); }
.r-msg { font-size: 12px; }

.prize-strip { display: flex; gap: 8px; overflow-x: auto; padding-bottom: 4px; }
.prize-card { flex: 0 0 auto; padding: 8px 14px; border-radius: 10px; background: var(--glass-bg, rgba(255,255,255,0.06)); border: 1px solid var(--border-soft, rgba(255,255,255,0.08)); font-size: 13px; font-weight: 600; }

.rec-row { display: flex; gap: 10px; align-items: center; padding: 6px 0; font-size: 13px; border-bottom: 1px solid var(--border-soft, rgba(255,255,255,0.05)); }
.rec-row .muted { color: var(--text-muted, #999); }
.rec-amt { color: #22c55e; font-weight: 600; }
.rec-time { margin-left: auto; font-size: 11px; }
</style>
