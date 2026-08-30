<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, toRef } from 'vue'
import { useI18n } from 'vue-i18n'
import { entertainmentApi, type BigSmallState } from '@/api/entertainment'
import { useBigSmallChat, useBigSmallItems, fmt } from '@/composables/useBigSmallGame'
import BigSmallSidebar from '@/components/BigSmallSidebar.vue'

// 猜大小富组件（好学 hxpt_bigsmall + 熊猫 panda_bigsmall 共用）。
const props = defineProps<{ siteId: number; game: string; name: string }>()

const state = ref<BigSmallState | null>(null)
const loading = ref(false)
const fetchedAt = ref(0) // 上次 state 拉取的 Date.now()
const now = ref(Date.now()) // 每秒 tick 更新，驱动 remaining 重算

const isHxpt = computed(() => props.game.startsWith('hxpt'))
const siteIdRef = toRef(props, 'siteId')
const gameRef = toRef(props, 'game')

// ── 倒计时（快照 + 客户端递减） ──
const elapsed = computed(() => Math.floor((now.value - fetchedAt.value) / 1000))
const remaining = computed(() => Math.max(0, (state.value?.round.countdown ?? 0) - elapsed.value))
const sealRemaining = computed(() => Math.max(0, (state.value?.round.seal_countdown ?? 0) - elapsed.value))
const roundStatus = computed(() => state.value?.round.status ?? '')
const betting = ref(false)
const canBet = computed(
  () => roundStatus.value === 'betting' && sealRemaining.value > 0 && !betting.value && !!state.value,
)
const statusText = computed(() => {
  if (roundStatus.value === 'sealed') return t('bs.waitSealed')
  if (roundStatus.value === 'finished') return t('bs.waitFinished')
  if (roundStatus.value === 'betting' && sealRemaining.value <= 3)
    return t('bs.waitSealing', { s: sealRemaining.value })
  return t('bs.waitBetting', { s: remaining.value })
})

// ── 下注 ──
const amount = ref(1000)
const { t } = useI18n()
const propId = ref('0') // 好学道具随注
const quickAmounts = [1000, 5000, 10000, 50000, 100000]
function allIn() {
  const bal = state.value?.balance ?? 0
  const max = state.value?.max_bet ?? bal
  amount.value = Math.floor(Math.min(bal, max > 0 ? max : bal))
}
async function doBet(type: 'big' | 'small') {
  if (!canBet.value) return
  betting.value = true
  try {
    const extra: Record<string, string> = {}
    if (isHxpt.value && propId.value !== '0') extra.prop_id = propId.value
    let res = await entertainmentApi.bet(props.siteId, props.game, { selection: type, amount: amount.value, extra })
    // 好学验证码：code=3 → 收 code 带 hash 重下
    if (!res.success && (res.raw as { captcha_required?: boolean })?.captcha_required) {
      try {
        const { value } = await ElMessageBox.prompt(t('bs.captchaPrompt'), t('bs.captchaTitle'), {
          inputPattern: /\S+/,
          inputErrorMessage: t('bs.notEmpty'),
        })
        extra.captcha_hash = (res.raw as { imagehash?: string })?.imagehash ?? ''
        extra.captcha_code = value
        res = await entertainmentApi.bet(props.siteId, props.game, { selection: type, amount: amount.value, extra })
      } catch {
        /* 用户取消 */
      }
    }
    if (res.success) ElMessage.success(res.prize || res.message || t('bs.betOk'))
    else ElMessage.warning(res.message || t('bs.betFailed'))
    await load(true)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('common.requestFailed'))
  } finally {
    betting.value = false
  }
}

// ── 拉取 ──
async function load(silent = false) {
  if (loading.value) return
  loading.value = true
  try {
    const s = await entertainmentApi.bigsmallState(props.siteId, props.game)
    state.value = s
    fetchedAt.value = Date.now()
    now.value = fetchedAt.value
    if (s.min_bet && amount.value < s.min_bet) amount.value = Math.floor(s.min_bet)
    // 聊天用 state 首播 + 后续轮询增量
    initChatFromState(s)
    if (!silent) ElMessage.success(t('bs.refreshed'))
  } catch (e: unknown) {
    if (!silent) ElMessage.error((e as Error).message || t('common.loadFailed'))
  } finally {
    loading.value = false
  }
}

// ── 聊天 / 任务 / 道具 / 红包（composable） ──
const { chat, chatInput, sending, refreshChat, sendChat, initChatFromState } = useBigSmallChat(
  siteIdRef,
  gameRef,
  load,
  state,
)
const { doTask, useRefund, useProp, claimRed, rpAmount, rpCount, rpGreeting, rpSending, sendRed } = useBigSmallItems(
  siteIdRef,
  gameRef,
  load,
)

// ── 排行 / 记录 tab ──
const rankTab = ref(0)
const recTab = ref<'my' | 'all'>('my')

const dotColor = (r: string) => (r === 'big' ? '#ef4444' : r === 'house' ? '#22c55e' : '#3b82f6')
const dotLabel = (r: string) => (r === 'big' ? t('bs.big') : r === 'house' ? t('bs.house') : t('bs.small'))

// 排名配色类名（1/2/3 名）
const topCls = ['gold', 'silver', 'bronze']
function top(i: number): string {
  return topCls[i] ?? ''
}

// ── 定时器 ──
let tickTimer: ReturnType<typeof setInterval> | null = null
let pollTimer: ReturnType<typeof setInterval> | null = null
let chatTimer: ReturnType<typeof setInterval> | null = null
function startTimers() {
  stopTimers()
  tickTimer = setInterval(() => {
    now.value = Date.now()
    if (loading.value) return
    if (remaining.value <= 0) load(true)
    else if (roundStatus.value === 'betting' && sealRemaining.value <= 0) load(true)
  }, 1000)
  pollTimer = setInterval(() => {
    if (document.visibilityState === 'hidden' || loading.value) return
    load(true)
  }, 25000)
  chatTimer = setInterval(() => {
    if (document.visibilityState === 'hidden') return
    refreshChat()
  }, 5000)
}
function stopTimers() {
  for (const t of [tickTimer, pollTimer, chatTimer]) if (t) clearInterval(t)
  tickTimer = pollTimer = chatTimer = null
}

onMounted(async () => {
  await load(true)
  startTimers()
})
onBeforeUnmount(stopTimers)
</script>

<template>
  <div class="bs">
    <!-- hero -->
    <div class="hero">
      <div class="hero-title">🎲 {{ name }}</div>
      <div class="balance">
        {{ t('bs.balance') }} <b>{{ fmt(state?.balance) }}</b>
        <span v-if="state?.currency" class="cur">{{ state.currency }}</span>
        <span v-if="state?.payout" class="payout">· {{ t('bs.payout', { n: state.payout }) }}</span>
      </div>
    </div>

    <!-- 状态 + 倒计时 + 局信息 -->
    <div class="stat-bar">
      <div class="stat-tile">
        <div class="stat-val" :class="{ warn: roundStatus !== 'betting' }">{{ remaining }}s</div>
        <div class="stat-lbl">{{ t('bs.countdown') }}</div>
      </div>
      <div class="stat-tile">
        <div class="stat-val">
          {{ roundStatus === 'betting' ? t('bs.canBet') : roundStatus === 'sealed' ? t('bs.sealed') : t('bs.ended') }}
        </div>
        <div class="stat-lbl">{{ statusText }}</div>
      </div>
      <div class="stat-tile">
        <div class="stat-val">{{ t('bs.roundN', { n: state?.round.game_no || '?' }) }}</div>
        <div class="stat-lbl">
          {{ t('bs.roundMeta', { players: state?.round.player_count ?? 0, total: fmt(state?.round.total_bet) }) }}
        </div>
      </div>
      <button class="refresh-btn" :disabled="loading" :aria-label="t('bs.refreshRound')" @click="load(false)">⟳</button>
    </div>

    <!-- 上局结果 -->
    <div v-if="state?.last_result" class="last-res" :class="(state.last_result.net_result ?? -1) >= 0 ? 'win' : 'lose'">
      {{
        t('bs.lastResult', {
          no: state.last_result.game_no,
          result: dotLabel(state.last_result.result || ''),
          bet: dotLabel(state.last_result.bet_type),
          amount: fmt(state.last_result.amount),
        })
      }}
      →
      <b>{{ (state.last_result.net_result ?? 0) >= 0 ? '+' : '' }}{{ fmt(state.last_result.net_result ?? 0) }}</b>
    </div>

    <div class="main">
      <!-- 左：走势 + 下注 -->
      <div class="left">
        <section class="glass panel">
          <div class="sec-title">📊 {{ t('bs.trend') }}</div>
          <div class="trend-row">
            <div class="trend-block">
              <div class="trend-cap">{{ t('bs.last40') }}</div>
              <div class="bar-row">
                <span class="bar big" :style="{ width: state?.trend40.big_pct + '%' }"
                  >{{ t('bs.big') }} {{ state?.trend40.big_count }}</span
                >
              </div>
              <div class="bar-row">
                <span class="bar small" :style="{ width: state?.trend40.small_pct + '%' }"
                  >{{ t('bs.small') }} {{ state?.trend40.small_count }}</span
                >
              </div>
              <div v-if="state?.has_house" class="trend-house">
                {{ t('bs.houseKill', { n: state?.trend40.house_count }) }}
              </div>
            </div>
            <div class="trend-block">
              <div class="trend-cap">{{ t('bs.last10') }}</div>
              <div class="bar-row">
                <span class="bar big" :style="{ width: state?.trend10.big_pct + '%' }"
                  >{{ t('bs.big') }} {{ state?.trend10.big_count }}</span
                >
              </div>
              <div class="bar-row">
                <span class="bar small" :style="{ width: state?.trend10.small_pct + '%' }"
                  >{{ t('bs.small') }} {{ state?.trend10.small_count }}</span
                >
              </div>
            </div>
          </div>
          <div v-if="state?.recent_results?.length" class="dots">
            <span
              v-for="(r, i) in state.recent_results"
              :key="i"
              class="dot"
              :style="{ background: dotColor(r) }"
              :title="dotLabel(r)"
              >{{ dotLabel(r) }}</span
            >
          </div>
          <div v-if="state?.trend_text" class="trend-text">{{ state.trend_text }}</div>
        </section>

        <section class="glass panel">
          <div class="sec-title">💰 {{ t('bs.placeBet') }}</div>
          <div class="my-bet" v-if="state?.my_bet">
            {{ t('bs.myBet', { side: dotLabel(state.my_bet.bet_type), amount: fmt(state.my_bet.amount) }) }}
          </div>
          <div class="bet-amount">
            <input v-model.number="amount" type="number" class="amt" :min="state?.min_bet ?? 1" />
            <span class="hint">{{ t('bs.minBet', { n: state?.min_bet ?? 0 }) }}</span>
            <button class="quick allin" @click="allIn">{{ t('bs.allIn') }}</button>
          </div>
          <div class="quicks">
            <button v-for="q in quickAmounts" :key="q" class="quick" @click="amount = q">
              {{ q.toLocaleString() }}
            </button>
          </div>
          <select v-if="isHxpt && state?.props?.length" v-model="propId" class="prop-sel">
            <option value="0">{{ t('bs.noProp') }}</option>
            <option v-for="p in state.props.filter((p) => p.usable)" :key="p.id" :value="p.id">
              {{ p.name }} ×{{ p.quantity }}
            </option>
          </select>
          <div class="ops">
            <button class="op big" :disabled="!canBet" @click="doBet('big')">{{ t('bs.betBig') }}</button>
            <button class="op small" :disabled="!canBet" @click="doBet('small')">{{ t('bs.betSmall') }}</button>
          </div>
          <div class="betters">
            <div class="bet-col big">
              <span class="bet-col-cap">{{ t('bs.bigSide') }}</span>
              <span v-if="!state?.big_bets?.length" class="empty">{{ t('bs.none') }}</span>
              <span v-else v-for="(b, i) in state.big_bets.slice(0, 8)" :key="i" class="bet-name"
                >{{ b.username }} {{ fmt(b.amount) }}</span
              >
            </div>
            <div class="bet-col small">
              <span class="bet-col-cap">{{ t('bs.smallSide') }}</span>
              <span v-if="!state?.small_bets?.length" class="empty">{{ t('bs.none') }}</span>
              <span v-else v-for="(b, i) in state.small_bets.slice(0, 8)" :key="i" class="bet-name"
                >{{ b.username }} {{ fmt(b.amount) }}</span
              >
            </div>
          </div>
        </section>
      </div>

      <!-- 右：排行 / 任务 / 红包 -->
      <BigSmallSidebar
        :state="state"
        :is-hxpt="isHxpt"
        v-model:rank-tab="rankTab"
        v-model:rp-amount="rpAmount"
        v-model:rp-count="rpCount"
        v-model:rp-greeting="rpGreeting"
        :rp-sending="rpSending"
        @do-task="doTask"
        @use-refund="useRefund"
        @use-prop="useProp"
        @claim-red="claimRed"
        @send-red="sendRed"
      />
    </div>

    <!-- 游戏记录 -->
    <section class="glass panel">
      <div class="sec-title-row">
        <div class="sec-title">📋 {{ t('bs.records') }}</div>
        <div class="tabs sm">
          <button class="tab" :class="{ on: recTab === 'my' }" @click="recTab = 'my'">{{ t('bs.myRec') }}</button>
          <button class="tab" :class="{ on: recTab === 'all' }" @click="recTab = 'all'">{{ t('bs.allRec') }}</button>
        </div>
      </div>
      <div class="rec-table">
        <div class="rec-head">
          <span>{{ t('bs.no') }}</span
          ><span>{{ t('bs.start') }}</span
          ><span>{{ t('bs.end') }}</span
          ><span>{{ t('bs.players') }}</span
          ><span>{{ t('bs.total') }}</span
          ><span>{{ t('bs.result') }}</span>
          <template v-if="recTab === 'my'"
            ><span>{{ t('bs.myBetCol') }}</span
            ><span>{{ t('bs.myResult') }}</span></template
          >
          <span v-else>{{ t('bs.betUsers') }}</span>
        </div>
        <div
          v-for="(r, i) in (recTab === 'my' ? state?.my_records : state?.all_records) ?? []"
          :key="i"
          class="rec-row"
        >
          <span>{{ r.game_no }}</span>
          <span class="muted">{{ r.start_time }}</span>
          <span class="muted">{{ r.end_time }}</span>
          <span>{{ r.player_count }}</span>
          <span>{{ fmt(r.total_bet) }}</span>
          <span :class="r.result">{{ dotLabel(r.result) }}</span>
          <template v-if="recTab === 'my'">
            <span>{{ r.my_bet_type ? dotLabel(r.my_bet_type) + ' ' + fmt(r.my_bet) : '—' }}</span>
            <span :class="(r.my_result ?? 0) >= 0 ? 'big' : 'small'"
              >{{ (r.my_result ?? 0) >= 0 ? '+' : '' }}{{ fmt(r.my_result) }}</span
            >
          </template>
          <span v-else class="muted bets-cell">{{ r.bets || '—' }}</span>
        </div>
        <div v-if="!(recTab === 'my' ? state?.my_records : state?.all_records)?.length" class="empty">
          {{ t('bs.noRecords') }}
        </div>
      </div>
    </section>

    <!-- 聊天 -->
    <section class="glass panel chat-panel">
      <div class="sec-title">💬 {{ t('bs.chat') }}</div>
      <div class="chat-list">
        <div v-for="m in chat" :key="m.id" class="chat-msg">
          <span class="chat-user">{{ m.username }}</span>
          <span class="chat-text">{{ m.message }}</span>
        </div>
        <div v-if="!chat.length" class="empty">{{ t('bs.noMessages') }}</div>
      </div>
      <div class="chat-input">
        <input v-model="chatInput" :placeholder="t('bs.chatPh')" @keyup.enter="sendChat" />
        <button :disabled="sending" @click="sendChat">{{ t('bs.send') }}</button>
      </div>
    </section>
  </div>
</template>

<style scoped src="./BigSmallGame.css"></style>
