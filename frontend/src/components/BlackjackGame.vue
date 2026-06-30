<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { entertainmentApi, type BlackjackResult } from '@/api/entertainment'

// 二十一点状态机（批3）：new/hit/stand/double，渲染牌面与结果。
const props = defineProps<{ siteId: number; game: string; name: string }>()

const state = ref<BlackjackResult | null>(null)
const acting = ref(false)

const finished = computed(() => state.value?.stage === 'finished')

async function act(action: string) {
  if (acting.value) return
  acting.value = true
  try {
    const res = await entertainmentApi.blackjack(props.siteId, props.game, action)
    state.value = res
    if (res.message) {
      if (res.result === 'win' || res.result === 'blackjack') ElMessage.success(res.message)
      else if (res.result === 'lose') ElMessage.warning(res.message)
      else ElMessage.info(res.message)
    }
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '请求失败')
  } finally {
    acting.value = false
  }
}

const resultText = computed(() => {
  switch (state.value?.result) {
    case 'win':
      return '🎉 赢'
    case 'blackjack':
      return '🎉 Blackjack!'
    case 'lose':
      return '✗ 输'
    case 'push':
      return '平局'
    default:
      return ''
  }
})

onMounted(() => act('new'))
</script>

<template>
  <div class="bj">
    <div class="hero">
      <div class="hero-title">🃏 {{ name }}</div>
      <div class="balance">余额 <b>{{ state?.balance?.toLocaleString() ?? '—' }}</b></div>
    </div>

    <section v-if="state" class="glass section">
      <div class="dealer">
        <div class="who">庄家</div>
        <div class="cards">
          <span v-for="(c, i) in state.dealer_cards" :key="'d' + i" class="card">{{ c }}</span>
          <span v-if="!state.dealer_cards.length" class="text-muted">—</span>
        </div>
      </div>
      <div class="player">
        <div class="who">玩家</div>
        <div class="cards">
          <span v-for="(c, i) in state.player_cards" :key="'p' + i" class="card">{{ c }}</span>
          <span v-if="!state.player_cards.length" class="text-muted">—</span>
        </div>
      </div>
      <div v-if="resultText" class="result-tag" :class="state.result">{{ resultText }}</div>
      <div v-if="state.message" class="msg text-muted">{{ state.message }}</div>
    </section>

    <section class="glass section ops">
      <button class="op" :disabled="acting || !finished" @click="act('new')">新一局</button>
      <button class="op hit" :disabled="acting || finished" @click="act('hit')">要牌 (Hit)</button>
      <button class="op stand" :disabled="acting || finished" @click="act('stand')">停牌 (Stand)</button>
      <button class="op double" :disabled="acting || finished" @click="act('double')">加倍 (Double)</button>
    </section>
  </div>
</template>

<style scoped>
.bj { display: flex; flex-direction: column; gap: 16px; max-width: 640px; margin: 0 auto; }
.hero { padding: 18px; border-radius: 14px; background: linear-gradient(135deg, #059669, #14b8a6); text-align: center; }
.hero-title { font-size: 22px; font-weight: 800; color: #fff; }
.balance { margin-top: 4px; font-size: 13px; color: rgba(255,255,255,0.92); }
.balance b { color: #fde68a; font-size: 16px; }

.section { padding: 16px; border-radius: 14px; }
.dealer, .player { display: flex; align-items: center; gap: 12px; padding: 6px 0; }
.player { border-top: 1px solid var(--border-soft, rgba(255,255,255,0.08)); margin-top: 6px; padding-top: 12px; }
.who { width: 44px; font-size: 13px; color: var(--text-muted, #999); font-weight: 600; }
.cards { display: flex; gap: 6px; flex-wrap: wrap; }
.card { display: inline-block; min-width: 30px; padding: 6px 8px; border-radius: 8px; background: #fff; color: #111; font-weight: 700; font-size: 16px; text-align: center; box-shadow: 0 2px 4px rgba(0,0,0,0.2); }

.result-tag { margin-top: 12px; display: inline-block; padding: 6px 16px; border-radius: 10px; font-weight: 800; font-size: 16px; }
.result-tag.win, .result-tag.blackjack { background: rgba(34,197,94,0.2); color: #22c55e; }
.result-tag.lose { background: rgba(239,68,68,0.2); color: #ef4444; }
.result-tag.push { background: rgba(148,163,184,0.2); color: #94a3b8; }
.msg { margin-top: 8px; font-size: 13px; }

.ops { display: flex; gap: 8px; flex-wrap: wrap; justify-content: center; }
.op { padding: 10px 18px; border: 1px solid var(--border-soft, rgba(255,255,255,0.2)); border-radius: 10px; cursor: pointer; font-weight: 600; font-size: 13px; color: inherit; background: var(--glass-bg, rgba(255,255,255,0.06)); }
.op.hit { color: #fff; background: #2563eb; border: 0; }
.op.stand { color: #fff; background: #dc2626; border: 0; }
.op.double { color: #fff; background: #f59e0b; border: 0; }
.op:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
