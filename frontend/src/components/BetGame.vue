<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, onMounted, computed } from 'vue'
import { entertainmentApi, type GameStats, type PlayResult, type BetRequest } from '@/api/entertainment'

// 通用下注组件（批2）：按 game.id 后缀渲染不同玩法（押大小/双色球选号/翻面/红包/赛事押注）。
const { t } = useI18n()
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
  if (g.endsWith('_unionlotto') || g === 'hdkylin_lotto' || g === 'zmpt_lotto') return 'lotto'
  if (g === 'crab_fastthree') return 'fastthree'
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
    if (res.success) ElMessage.success(res.prize || res.message || t('bg.done'))
    else ElMessage.warning(res.message || t('common.actionFailed'))
    await refreshStats()
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || t('common.requestFailed'))
  } finally {
    playing.value = false
  }
}

const betBS = (t: string) => doBet({ selection: t, amount: amount.value })
const betFlip = () => doBet({ selection: '0', amount: 0 })
const betClaim = () => doBet({ selection: 'claim', amount: 0 })
const betLotto = () => doBet({ selection: '', amount: 166, extra: { reds: reds.value, blue: blue.value } })
// 麒麟双色球：后端 Selection 直接吃 "红,红,红,红,红,红|蓝"（蟹黄堡双色球仍走 extra）
const betHdkylinLotto = () => doBet({ selection: `${reds.value}|${blue.value}`, amount: 0 })
const fastThreeChar = ref('皮卡丘')
const betFastThree = () =>
  doBet({ selection: fastThreeChar.value, amount: amount.value > 1 ? amount.value : 0 })

onMounted(refreshStats)
</script>

<template>
  <div class="betgame">
    <div class="hero">
      <div class="hero-title">🎲 {{ name }}</div>
      <div class="balance">
        {{ t('bj.balance') }} <b>{{ stats?.balance?.toLocaleString() ?? '—' }}</b>
      </div>
    </div>

    <section class="glass section">
      <template v-if="mode === 'bigsmall'">
        <div class="sec-title">{{ t('bg.stake') }}</div>
        <input v-model.number="amount" type="number" class="amt-input" min="1" />
        <div class="ops">
          <button class="op big" :disabled="playing" @click="betBS('big')">{{ t('bg.big') }}</button>
          <button class="op small" :disabled="playing" @click="betBS('small')">{{ t('bg.small') }}</button>
        </div>
      </template>

      <template v-else-if="mode === 'lotto'">
        <div class="sec-title">{{ t('bg.lottoTitle') }}</div>
        <div class="row">
          <span>{{ t('bg.red') }}</span
          ><input v-model="reds" class="txt" placeholder="1,2,3,4,5,6" />
        </div>
        <div class="row">
          <span>{{ t('bg.blue') }}</span
          ><input v-model="blue" class="txt" placeholder="7" />
        </div>
        <!-- 蟹黄堡走 extra（betLotto），麒麟走 selection（betHdkylinLotto） -->
        <button
          class="op primary"
          :disabled="playing"
          @click="['hdkylin_lotto', 'zmpt_lotto'].includes(game) ? betHdkylinLotto() : betLotto()"
        >
          {{ playing ? t('bg.betting') : t('bg.bet') }}
        </button>
      </template>

      <template v-else-if="mode === 'fastthree'">
        <div class="sec-title">🦀 快三 · 角色投注</div>
        <div class="ops">
          <button
            v-for="c in ['皮卡丘', '杰尼龟', '妙蛙种子']"
            :key="c"
            class="op"
            :class="{ primary: fastThreeChar === c }"
            :disabled="playing"
            @click="fastThreeChar = c"
          >
            {{ c }}
          </button>
        </div>
        <div class="row">
          <span>注数</span>
          <input v-model.number="amount" type="number" class="txt" min="1" max="10000" />
          <span class="text-muted">1=单注，>1=批量</span>
        </div>
        <button class="op primary" :disabled="playing" @click="betFastThree">
          {{ playing ? t('bg.betting') : t('bg.bet') }}
        </button>
      </template>

      <template v-else-if="mode === 'flip'">
        <div class="sec-title">{{ t('bg.flipTitle') }}</div>
        <button class="op primary" :disabled="playing" @click="betFlip">
          {{ playing ? t('bg.flipping') : t('bg.flip') }}
        </button>
      </template>

      <template v-else-if="mode === 'redpacket'">
        <div class="sec-title">{{ t('bg.redpack') }}</div>
        <button class="op primary" :disabled="playing" @click="betClaim">
          {{ playing ? t('bg.working') : t('bg.claim') }}
        </button>
      </template>

      <template v-else-if="mode === 'bet'">
        <div class="sec-title">{{ t('bg.matchTitle') }}</div>
        <p class="text-muted placeholder">{{ t('bg.matchPlaceholder') }}</p>
      </template>
    </section>

    <div v-if="last" class="result" :class="last.success ? 'win' : 'lose'">
      <span class="badge">{{ last.success ? '✓' : '✗' }}</span>
      <div class="r-body">
        <span v-if="last.prize" class="r-prize">{{ last.prize }}</span>
        <span v-if="last.balance" class="r-bal">{{ t('bj.balance') }} {{ last.balance.toLocaleString() }}</span>
        <div v-if="last.message" class="r-msg text-muted">{{ last.message }}</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.betgame {
  display: flex;
  flex-direction: column;
  gap: var(--sp-4);
  max-width: 640px;
  margin: 0 auto;
}
.hero {
  padding: 18px;
  border-radius: var(--radius-md);
  background: linear-gradient(135deg, #db2777, #f59e0b);
  text-align: center;
}
.hero-title {
  font-size: var(--fs-title);
  font-weight: 800;
  color: #fff;
}
.balance {
  margin-top: 4px;
  font-size: var(--fs-body);
  color: rgba(255, 255, 255, 0.92);
}
.balance b {
  color: #fde68a;
  font-size: var(--fs-lg);
}

.section {
  padding: 18px;
  border-radius: var(--radius-md);
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
  align-items: center;
}
.sec-title {
  font-weight: 700;
  font-size: var(--fs-lg);
  align-self: flex-start;
}
.amt-input {
  padding: 8px 12px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--border-soft, rgba(255, 255, 255, 0.15));
  background: var(--glass-bg, rgba(255, 255, 255, 0.06));
  color: inherit;
  width: 160px;
  font-size: var(--fs-lg);
}
.ops {
  display: flex;
  gap: var(--sp-3);
}
.op {
  padding: 11px 24px;
  border: 0;
  border-radius: var(--radius-sm);
  cursor: pointer;
  font-weight: 700;
  font-size: var(--fs-lg);
  color: #fff;
}
.op.big {
  background: linear-gradient(135deg, #dc2626, #ef4444);
}
.op.small {
  background: linear-gradient(135deg, #2563eb, #3b82f6);
}
.op.primary {
  background: linear-gradient(135deg, #db2777, #f59e0b);
}
.op:disabled {
  opacity: 0.6;
  cursor: wait;
}

.row {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  width: 100%;
}
.row span {
  width: 40px;
  font-size: var(--fs-body);
  color: var(--text-muted, #999);
}
.txt {
  flex: 1;
  padding: 8px 12px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--border-soft, rgba(255, 255, 255, 0.15));
  background: var(--glass-bg, rgba(255, 255, 255, 0.06));
  color: inherit;
  font-size: var(--fs-md);
}
.placeholder {
  font-size: var(--fs-body);
}

.result {
  padding: 12px;
  border-radius: var(--radius-sm);
  display: flex;
  gap: var(--sp-2);
  align-items: flex-start;
}
.result.win {
  background: var(--pt-success-bg);
}
.result.lose {
  background: var(--pt-danger-bg);
}
.badge {
  font-weight: 700;
}
.r-body {
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
}
.r-prize {
  font-weight: 700;
  color: var(--pt-success);
}
.r-bal {
  font-size: var(--fs-xs);
  color: var(--text-muted, #999);
}
.r-msg {
  font-size: var(--fs-xs);
}
</style>
