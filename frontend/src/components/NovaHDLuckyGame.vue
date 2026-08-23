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
.lucky { display: flex; flex-direction: column; gap: var(--sp-4); max-width: 640px; margin: 0 auto; }
.hero { padding: 18px; border-radius: var(--radius-md); background: linear-gradient(135deg, #7c3aed, #2563eb); text-align: center; }
.hero-title { font-size: var(--fs-title); font-weight: 800; color: #fff; letter-spacing: 1px; }

.section { padding: 16px; border-radius: var(--radius-md); }
.sec-title { font-weight: 700; margin-bottom: 12px; font-size: var(--fs-lg); }

.info-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: var(--sp-2); }
.info-card { display: flex; flex-direction: column; align-items: center; gap: var(--sp-1); padding: 14px 8px; border-radius: var(--radius-sm); background: var(--glass-bg, rgba(255,255,255,0.06)); border: 1px solid var(--border-soft, rgba(255,255,255,0.08)); }
.info-card b { font-size: var(--fs-xl); color: var(--pt-primary-2); }
.info-card small { font-size: var(--fs-2xs); color: var(--text-muted, #999); }

.prize-strip { display: flex; gap: var(--sp-2); overflow-x: auto; padding-bottom: 4px; }
.prize-card { flex: 0 0 auto; display: flex; flex-direction: column; align-items: center; gap: var(--sp-1); padding: 10px 12px; border-radius: var(--radius-sm); background: var(--glass-bg, rgba(255,255,255,0.06)); border: 1px solid var(--border-soft, rgba(255,255,255,0.08)); min-width: 70px; }
.p-emoji { font-size: var(--fs-title); }
.p-name { font-size: var(--fs-2xs); font-weight: 600; white-space: nowrap; }

.ops { display: flex; gap: var(--sp-2); flex-wrap: wrap; justify-content: center; }
.op-btn { padding: 9px 16px; border: 1px solid var(--pt-primary-2); border-radius: var(--radius-sm); cursor: pointer; font-size: var(--fs-body); font-weight: 600; color: var(--pt-primary-2); background: transparent; transition: all var(--dur-base) var(--ease-out-smooth); }
.op-btn:hover:not(:disabled) { transform: translateY(-1px); background: var(--pt-primary-2-bg); }
.op-btn:active:not(:disabled) { transform: scale(0.97); }
.op-btn.primary:hover:not(:disabled) { filter: brightness(1.08); }
.op-btn.primary { color: #fff; background: linear-gradient(135deg, #2563eb, #7c3aed); border: 0; box-shadow: 0 4px 12px var(--pt-primary-2-bg); }
.op-btn:disabled { opacity: 0.6; cursor: wait; }

.result { padding: 12px; border-radius: var(--radius-sm); display: flex; gap: var(--sp-2); align-items: flex-start; }
.result.win { background: var(--pt-success-bg); }
.result.lose { background: var(--pt-danger-bg); }
.badge { font-size: var(--fs-xl); }
.r-list { display: flex; flex-wrap: wrap; gap: var(--sp-1); }
.r-pill { padding: 3px 10px; border-radius: var(--radius-xs); font-size: var(--fs-xs); font-weight: 600; background: var(--pt-success-bg); color: var(--pt-success); }
.r-msg { font-size: var(--fs-body); }

.rec-row { display: flex; gap: var(--sp-2); align-items: center; padding: 6px 0; font-size: var(--fs-body); border-bottom: 1px solid var(--border-soft, rgba(255,255,255,0.05)); }
.rec-row .muted { color: var(--text-muted, #999); }
.rec-amt { color: var(--pt-success); font-weight: 600; }
.rec-time { margin-left: auto; font-size: var(--fs-2xs); }
</style>
