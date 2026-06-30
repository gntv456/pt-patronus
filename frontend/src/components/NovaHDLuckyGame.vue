<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { entertainmentApi, type GameStats, type PrizeDef, type GameRecord, type PlayResult } from '@/api/entertainment'

const props = defineProps<{ siteId: number }>()
const GAME = 'novahd_lucky'

const stats = ref<GameStats | null>(null)
const prizes = ref<PrizeDef[]>([])
const records = ref<GameRecord[]>([])
const spinning = ref(false)
const last = ref<PlayResult | null>(null)

async function refreshStats() {
  stats.value = await entertainmentApi.stats(props.siteId, GAME).catch(() => stats.value)
}
async function refreshHistory() {
  records.value = await entertainmentApi.history(props.siteId, GAME).catch(() => [])
}

async function draw(count: number) {
  if (spinning.value) return
  spinning.value = true
  try {
    const res = count === 1
      ? await entertainmentApi.play(props.siteId, GAME)
      : await entertainmentApi.playMulti(props.siteId, GAME, count)
    last.value = res
    if (res.success) {
      const wins = (res.prizes || []).filter((p) => p.is_win)
      ElMessage.success(wins.length ? `🎉 中奖 ${wins.length} 次` : (res.message || '完成'))
    } else {
      ElMessage.warning(res.message || '抽奖失败')
    }
    await Promise.all([refreshStats(), refreshHistory()])
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '请求失败')
  } finally {
    spinning.value = false
  }
}

function emoji(name: string) {
  if (/VIP|会员/.test(name)) return '👑'
  if (/彩虹/.test(name)) return '🌈'
  if (/邀请/.test(name)) return '✉️'
  if (/魔力/.test(name)) return '🪙'
  return '🎁'
}

onMounted(async () => {
  prizes.value = await entertainmentApi.prizes(props.siteId, GAME).catch(() => [] as PrizeDef[])
  await Promise.all([refreshStats(), refreshHistory()])
})
</script>

<template>
  <div class="lucky">
    <div class="hero">
      <div class="hero-title">🎡 幸运抽奖</div>
    </div>

    <!-- 抽奖信息 -->
    <section class="glass section">
      <div class="info-grid">
        <div class="info-card">
          <b>{{ stats?.balance?.toLocaleString() ?? '—' }}</b><small>当前魔力</small>
        </div>
        <div class="info-card">
          <b>{{ stats?.cost_per_play?.toLocaleString() ?? '—' }}</b><small>每次消耗</small>
        </div>
        <div class="info-card">
          <b>{{ stats?.today_spins ?? 0 }}/{{ stats?.max_daily || '?' }}</b><small>今日已抽</small>
        </div>
      </div>
    </section>

    <!-- 奖品列表 -->
    <section v-if="prizes.length" class="glass section">
      <div class="sec-title">奖品列表</div>
      <div class="prize-strip">
        <div v-for="p in prizes" :key="p.id" class="prize-card">
          <span class="p-emoji">{{ emoji(p.name) }}</span>
          <span class="p-name">{{ p.name }}</span>
        </div>
      </div>
    </section>

    <!-- 抽奖操作 -->
    <section class="glass section ops">
      <button class="op-btn primary" :disabled="spinning" @click="draw(1)">
        {{ spinning ? '抽奖中…' : '立即抽奖' }}
      </button>
      <button class="op-btn" :disabled="spinning" @click="draw(10)">连抽 10 次</button>
      <button class="op-btn" :disabled="spinning" @click="draw(20)">连抽 20 次</button>
      <button class="op-btn" :disabled="spinning" @click="draw(50)">连抽 50 次</button>
    </section>

    <div v-if="last" class="result" :class="last.success ? 'win' : 'lose'">
      <span class="badge">{{ last.success ? '🎉' : '✗' }}</span>
      <div class="r-detail">
        <div v-if="last.prizes?.length" class="r-list">
          <span v-for="(p, i) in last.prizes.filter((x) => x.is_win).slice(0, 8)" :key="i" class="r-pill">
            {{ p.prize_name }}<template v-if="p.amount"> ×{{ p.amount }}</template>
          </span>
          <span v-if="!last.prizes.some((x) => x.is_win)" class="text-muted">很遗憾，未中奖</span>
        </div>
        <span v-else-if="last.message" class="r-msg">{{ last.message }}</span>
      </div>
    </div>

    <!-- 记录 -->
    <section v-if="records.length" class="glass section">
      <div class="sec-title">抽奖记录</div>
      <div v-for="(r, i) in records.slice(0, 10)" :key="i" class="rec-row">
        <span :class="{ muted: r.prize_name === '未中奖' }">{{ r.prize_name }}</span>
        <span v-if="r.amount" class="rec-amt">+{{ r.amount }}</span>
        <span class="text-muted rec-time">{{ r.created_at }}</span>
      </div>
    </section>
  </div>
</template>

<style scoped>
.lucky { display: flex; flex-direction: column; gap: 16px; max-width: 640px; margin: 0 auto; }
.hero { padding: 18px; border-radius: 14px; background: linear-gradient(135deg, #7c3aed, #2563eb); text-align: center; }
.hero-title { font-size: 22px; font-weight: 800; color: #fff; letter-spacing: 1px; }

.section { padding: 16px; border-radius: 14px; }
.sec-title { font-weight: 700; margin-bottom: 12px; font-size: 15px; }

.info-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 10px; }
.info-card { display: flex; flex-direction: column; align-items: center; gap: 3px; padding: 14px 8px; border-radius: 12px; background: var(--glass-bg, rgba(255,255,255,0.06)); border: 1px solid var(--border-soft, rgba(255,255,255,0.08)); }
.info-card b { font-size: 18px; color: #2563eb; }
.info-card small { font-size: 11px; color: var(--text-muted, #999); }

.prize-strip { display: flex; gap: 8px; overflow-x: auto; padding-bottom: 4px; }
.prize-card { flex: 0 0 auto; display: flex; flex-direction: column; align-items: center; gap: 3px; padding: 10px 12px; border-radius: 10px; background: var(--glass-bg, rgba(255,255,255,0.06)); border: 1px solid var(--border-soft, rgba(255,255,255,0.08)); min-width: 70px; }
.p-emoji { font-size: 22px; }
.p-name { font-size: 11px; font-weight: 600; white-space: nowrap; }

.ops { display: flex; gap: 8px; flex-wrap: wrap; justify-content: center; }
.op-btn { padding: 9px 16px; border: 1px solid #2563eb; border-radius: 10px; cursor: pointer; font-size: 13px; font-weight: 600; color: #2563eb; background: transparent; }
.op-btn.primary { color: #fff; background: linear-gradient(135deg, #2563eb, #7c3aed); border: 0; box-shadow: 0 4px 12px rgba(37,99,235,0.4); }
.op-btn:disabled { opacity: 0.6; cursor: wait; }

.result { padding: 12px; border-radius: 12px; display: flex; gap: 10px; align-items: flex-start; }
.result.win { background: rgba(34, 197, 94, 0.12); }
.result.lose { background: rgba(231, 76, 60, 0.1); }
.badge { font-size: 18px; }
.r-list { display: flex; flex-wrap: wrap; gap: 6px; }
.r-pill { padding: 3px 10px; border-radius: 8px; font-size: 12px; font-weight: 600; background: rgba(34,197,94,0.18); color: #16a34a; }
.r-msg { font-size: 13px; }

.rec-row { display: flex; gap: 10px; align-items: center; padding: 6px 0; font-size: 13px; border-bottom: 1px solid var(--border-soft, rgba(255,255,255,0.05)); }
.rec-row .muted { color: var(--text-muted, #999); }
.rec-amt { color: #22c55e; font-weight: 600; }
.rec-time { margin-left: auto; font-size: 11px; }
</style>
