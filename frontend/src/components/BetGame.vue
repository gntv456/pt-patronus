<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { entertainmentApi, type GameStats, type PlayResult, type BetRequest } from '@/api/entertainment'

// 通用下注组件（批2）：按 game.id 后缀渲染不同玩法（押大小/双色球选号/翻面/红包/赛事押注）。
const props = defineProps<{ siteId: number; game: string; name: string }>()

const stats = ref<GameStats | null>(null)
const playing = ref(false)
const last = ref<PlayResult | null>(null)
const amount = ref(100)
const reds = ref('1,2,3,4,5,6')
const blue = ref('7')

const mode = computed(() => {
  const g = props.game
  if (g.endsWith('_bigsmall') || g.endsWith('_sicbo')) return 'bigsmall'
  if (g.endsWith('_unionlotto')) return 'lotto'
  if (g.endsWith('_flip')) return 'flip'
  if (g.endsWith('_redpacket')) return 'redpacket'
  if (g.endsWith('_bet')) return 'bet'
  return 'generic'
})

async function refreshStats() {
  stats.value = await entertainmentApi.stats(props.siteId, props.game).catch(() => stats.value)
}
async function doBet(req: BetRequest) {
  if (playing.value) return
  playing.value = true
  try {
    const res = await entertainmentApi.bet(props.siteId, props.game, req)
    last.value = res
    if (res.success) ElMessage.success(res.prize || res.message || '完成')
    else ElMessage.warning(res.message || '操作失败')
    await refreshStats()
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '请求失败')
  } finally {
    playing.value = false
  }
}

const betBS = (t: string) => doBet({ selection: t, amount: amount.value })
const betFlip = () => doBet({ selection: '0', amount: 0 })
const betClaim = () => doBet({ selection: 'claim', amount: 0 })
const betLotto = () => doBet({ selection: '', amount: 166, extra: { reds: reds.value, blue: blue.value } })

onMounted(refreshStats)
</script>

<template>
  <div class="betgame">
    <div class="hero">
      <div class="hero-title">🎲 {{ name }}</div>
      <div class="balance">余额 <b>{{ stats?.balance?.toLocaleString() ?? '—' }}</b></div>
    </div>

    <section class="glass section">
      <template v-if="mode === 'bigsmall'">
        <div class="sec-title">押注金额</div>
        <input v-model.number="amount" type="number" class="amt-input" min="1" />
        <div class="ops">
          <button class="op big" :disabled="playing" @click="betBS('big')">押 大</button>
          <button class="op small" :disabled="playing" @click="betBS('small')">押 小</button>
        </div>
      </template>

      <template v-else-if="mode === 'lotto'">
        <div class="sec-title">选号（6 红 1 蓝，每注 166）</div>
        <div class="row"><span>红球</span><input v-model="reds" class="txt" placeholder="1,2,3,4,5,6" /></div>
        <div class="row"><span>蓝球</span><input v-model="blue" class="txt" placeholder="7" /></div>
        <button class="op primary" :disabled="playing" @click="betLotto">{{ playing ? '投注中…' : '投注' }}</button>
      </template>

      <template v-else-if="mode === 'flip'">
        <div class="sec-title">翻面夺宝</div>
        <button class="op primary" :disabled="playing" @click="betFlip">{{ playing ? '翻面中…' : '🃏 翻一张' }}</button>
      </template>

      <template v-else-if="mode === 'redpacket'">
        <div class="sec-title">红包</div>
        <button class="op primary" :disabled="playing" @click="betClaim">{{ playing ? '处理中…' : '🧧 领取红包' }}</button>
      </template>

      <template v-else-if="mode === 'bet'">
        <div class="sec-title">赛事押注</div>
        <p class="text-muted placeholder">押注表单待补抓 bet_odds.php，暂未支持自动押注。</p>
      </template>
    </section>

    <div v-if="last" class="result" :class="last.success ? 'win' : 'lose'">
      <span class="badge">{{ last.success ? '✓' : '✗' }}</span>
      <div class="r-body">
        <span v-if="last.prize" class="r-prize">{{ last.prize }}</span>
        <span v-if="last.balance" class="r-bal">余额 {{ last.balance.toLocaleString() }}</span>
        <div v-if="last.message" class="r-msg text-muted">{{ last.message }}</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.betgame { display: flex; flex-direction: column; gap: 16px; max-width: 640px; margin: 0 auto; }
.hero { padding: 18px; border-radius: 14px; background: linear-gradient(135deg, #db2777, #f59e0b); text-align: center; }
.hero-title { font-size: 22px; font-weight: 800; color: #fff; }
.balance { margin-top: 4px; font-size: 13px; color: rgba(255,255,255,0.92); }
.balance b { color: #fde68a; font-size: 16px; }

.section { padding: 18px; border-radius: 14px; display: flex; flex-direction: column; gap: 12px; align-items: center; }
.sec-title { font-weight: 700; font-size: 15px; align-self: flex-start; }
.amt-input { padding: 8px 12px; border-radius: 10px; border: 1px solid var(--border-soft, rgba(255,255,255,0.15)); background: var(--glass-bg, rgba(255,255,255,0.06)); color: inherit; width: 160px; font-size: 15px; }
.ops { display: flex; gap: 12px; }
.op { padding: 11px 24px; border: 0; border-radius: 11px; cursor: pointer; font-weight: 700; font-size: 15px; color: #fff; }
.op.big { background: linear-gradient(135deg, #dc2626, #ef4444); }
.op.small { background: linear-gradient(135deg, #2563eb, #3b82f6); }
.op.primary { background: linear-gradient(135deg, #db2777, #f59e0b); }
.op:disabled { opacity: 0.6; cursor: wait; }

.row { display: flex; align-items: center; gap: 10px; width: 100%; }
.row span { width: 40px; font-size: 13px; color: var(--text-muted, #999); }
.txt { flex: 1; padding: 8px 12px; border-radius: 10px; border: 1px solid var(--border-soft, rgba(255,255,255,0.15)); background: var(--glass-bg, rgba(255,255,255,0.06)); color: inherit; font-size: 14px; }
.placeholder { font-size: 13px; }

.result { padding: 12px; border-radius: 12px; display: flex; gap: 10px; align-items: flex-start; }
.result.win { background: rgba(34, 197, 94, 0.12); }
.result.lose { background: rgba(231, 76, 60, 0.1); }
.badge { font-weight: 700; }
.r-body { display: flex; flex-direction: column; gap: 2px; }
.r-prize { font-weight: 700; color: #16a34a; }
.r-bal { font-size: 12px; color: var(--text-muted, #999); }
.r-msg { font-size: 12px; }
</style>
