<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import {
  entertainmentApi,
  type BigSmallState,
  type BigSmallChat,
  type BigSmallTask,
  type BigSmallProp,
  type BigSmallRedpacket,
} from '@/api/entertainment'

// 猜大小富组件（好学 hxpt_bigsmall + 熊猫 panda_bigsmall 共用）。
// 时间：后端 BigSmallState 给权威 countdown/seal_countdown/status 快照；前端按 fetchedAt 本地每秒递减
// （自动补偿往返耗时/时钟偏差），仅在新局(≤0)/封盘切换/下注后/25s 兜底回拉。封盘=status≠betting 或 seal_remaining≤0。
// 聊天另走 5s 轻量轮询（bigsmallChat）。
const props = defineProps<{ siteId: number; game: string; name: string }>()

const state = ref<BigSmallState | null>(null)
const loading = ref(false)
const fetchedAt = ref(0) // 上次 state 拉取的 Date.now()
const now = ref(Date.now()) // 每秒 tick 更新，驱动 remaining 重算

const isHxpt = computed(() => props.game.startsWith('hxpt'))

// ── 倒计时（快照 + 客户端递减） ──
const elapsed = computed(() => Math.floor((now.value - fetchedAt.value) / 1000))
const remaining = computed(() => Math.max(0, (state.value?.round.countdown ?? 0) - elapsed.value))
const sealRemaining = computed(() => Math.max(0, (state.value?.round.seal_countdown ?? 0) - elapsed.value))
const roundStatus = computed(() => state.value?.round.status ?? '')
const betting = ref(false)
const canBet = computed(() => roundStatus.value === 'betting' && sealRemaining.value > 0 && !betting.value && !!state.value)
const statusText = computed(() => {
  if (roundStatus.value === 'sealed') return '已封盘，即将开奖…'
  if (roundStatus.value === 'finished') return '本局已结束，下一局即将开始'
  if (roundStatus.value === 'betting' && sealRemaining.value <= 3) return `即将封盘（${sealRemaining.value}s）`
  return `下注中 · 剩余 ${remaining.value}s`
})

// ── 下注 ──
const amount = ref(1000)
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
        const { value } = await ElMessageBox.prompt('请输入图中字符', '安全验证', {
          inputPattern: /\S+/,
          inputErrorMessage: '不能为空',
        })
        extra.captcha_hash = (res.raw as { imagehash?: string })?.imagehash ?? ''
        extra.captcha_code = value
        res = await entertainmentApi.bet(props.siteId, props.game, { selection: type, amount: amount.value, extra })
      } catch {
        /* 用户取消 */
      }
    }
    if (res.success) ElMessage.success(res.prize || res.message || '下注成功')
    else ElMessage.warning(res.message || '下注失败')
    await load(true)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '请求失败')
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
    if (chat.value.length === 0 && s.chat?.length) {
      chat.value = s.chat
      lastChatId.value = s.chat.reduce((m, c) => Math.max(m, c.id), 0)
    }
    if (!silent) ElMessage.success('已刷新')
  } catch (e: unknown) {
    if (!silent) ElMessage.error((e as Error).message || '加载失败')
  } finally {
    loading.value = false
  }
}

// ── 聊天（5s 轻量轮询） ──
const chat = ref<BigSmallChat[]>([])
const chatInput = ref('')
const lastChatId = ref(0)
const sending = ref(false)
function mergeChat(inc: BigSmallChat[]) {
  const map = new Map(chat.value.map((m) => [m.id, m]))
  for (const m of inc) map.set(m.id, m)
  chat.value = [...map.values()].sort((a, b) => a.id - b.id).slice(-60)
  if (inc.length) lastChatId.value = chat.value.reduce((m, c) => Math.max(m, c.id), lastChatId.value)
}
async function refreshChat(silent = true) {
  try {
    const list = await entertainmentApi.bigsmallChat(props.siteId, props.game, lastChatId.value)
    if (list.length) mergeChat(list)
  } catch {
    /* 静默 */
  }
}
async function sendChat() {
  const msg = chatInput.value.trim()
  if (!msg || sending.value) return
  sending.value = true
  try {
    const res = await entertainmentApi.bigsmallSendChat(props.siteId, props.game, msg)
    if (res.success) {
      chatInput.value = ''
      await refreshChat()
    } else ElMessage.warning(res.message || '发送失败')
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '发送失败')
  } finally {
    sending.value = false
  }
}

// ── 任务 / 道具 / 红包 ──
async function doTask(t: BigSmallTask, action: 'claim' | 'reward') {
  try {
    const res = await entertainmentApi.bigsmallTask(props.siteId, props.game, action, t.id)
    ElMessage[res.success ? 'success' : 'warning'](res.message || (action === 'reward' ? '已领取' : '已接取'))
    if (res.success) await load(true)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '操作失败')
  }
}
async function useRefund() {
  try {
    const res = await entertainmentApi.bigsmallTask(props.siteId, props.game, 'refund', 0)
    ElMessage[res.success ? 'success' : 'warning'](res.message || '操作结果')
    if (res.success) await load(true)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '操作失败')
  }
}
async function useProp(p: BigSmallProp) {
  try {
    const res = await entertainmentApi.bigsmallItem(props.siteId, props.game, p.tag)
    ElMessage[res.success ? 'success' : 'warning'](res.message || '已使用')
    if (res.success) await load(true)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '使用失败')
  }
}
async function claimRed(r: BigSmallRedpacket) {
  try {
    const res = await entertainmentApi.bigsmallRedpacket(props.siteId, props.game, 'claim', { id: r.id })
    ElMessage[res.success ? 'success' : 'warning'](res.message || '操作结果')
    if (res.success) await load(true)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '操作失败')
  }
}

// 红包发送表单
const rpAmount = ref(1000)
const rpCount = ref(5)
const rpGreeting = ref('恭喜发财')
const rpSending = ref(false)
async function sendRed() {
  if (rpSending.value) return
  rpSending.value = true
  try {
    const res = await entertainmentApi.bigsmallRedpacket(props.siteId, props.game, 'send', {
      amount: String(rpAmount.value),
      count: String(rpCount.value),
      greeting: rpGreeting.value,
    })
    ElMessage[res.success ? 'success' : 'warning'](res.message || '操作结果')
    if (res.success) await load(true)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '发送失败')
  } finally {
    rpSending.value = false
  }
}

// ── 排行 / 记录 tab ──
const rankTab = ref(0)
const recTab = ref<'my' | 'all'>('my')

const dotColor = (r: string) => (r === 'big' ? '#ef4444' : r === 'house' ? '#22c55e' : '#3b82f6')
const dotLabel = (r: string) => (r === 'big' ? '大' : r === 'house' ? '庄' : '小')
const fmt = (n: number | undefined) => (n ? n.toLocaleString() : '0')

// ── 定时器 ──
let tickTimer: ReturnType<typeof setInterval> | null = null
let pollTimer: ReturnType<typeof setInterval> | null = null
let chatTimer: ReturnType<typeof setInterval> | null = null
function startTimers() {
  stopTimers()
  tickTimer = setInterval(() => {
    now.value = Date.now()
    // 边界刷新：倒计时到 0（新局）/ 下注中即将封盘（切 sealed）
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

// 排名配色类名（1/2/3 名）
const topCls = ['gold', 'silver', 'bronze']
function top(i: number): string {
  return topCls[i] ?? ''
}
</script>

<template>
  <div class="bs">
    <!-- hero -->
    <div class="hero">
      <div class="hero-title">🎲 {{ name }}</div>
      <div class="balance">
        余额 <b>{{ fmt(state?.balance) }}</b>
        <span v-if="state?.currency" class="cur">{{ state.currency }}</span>
        <span v-if="state?.payout" class="payout">· 赔率 {{ state.payout }}×</span>
      </div>
    </div>

    <!-- 状态 + 倒计时 + 局信息 -->
    <div class="stat-bar">
      <div class="stat-tile">
        <div class="stat-val" :class="{ warn: roundStatus !== 'betting' }">{{ remaining }}s</div>
        <div class="stat-lbl">倒计时</div>
      </div>
      <div class="stat-tile">
        <div class="stat-val">{{ roundStatus === 'betting' ? '可下注' : roundStatus === 'sealed' ? '封盘' : '已结束' }}</div>
        <div class="stat-lbl">{{ statusText }}</div>
      </div>
      <div class="stat-tile">
        <div class="stat-val">第 {{ state?.round.game_no || '?' }} 局</div>
        <div class="stat-lbl">{{ state?.round.player_count ?? 0 }} 人 · 总注 {{ fmt(state?.round.total_bet) }}</div>
      </div>
      <button class="refresh-btn" :disabled="loading" @click="load(false)">⟳</button>
    </div>

    <!-- 上局结果 -->
    <div v-if="state?.last_result" class="last-res" :class="(state.last_result.net_result ?? -1) >= 0 ? 'win' : 'lose'">
      上局 #{{ state.last_result.game_no }} 开 <b>{{ dotLabel(state.last_result.result || '') }}</b>，
      你押 {{ dotLabel(state.last_result.bet_type) }} {{ fmt(state.last_result.amount) }} →
      <b>{{ (state.last_result.net_result ?? 0) >= 0 ? '+' : '' }}{{ fmt(state.last_result.net_result ?? 0) }}</b>
    </div>

    <div class="main">
      <!-- 左：走势 + 下注 -->
      <div class="left">
        <section class="glass panel">
          <div class="sec-title">📊 走势分析</div>
          <div class="trend-row">
            <div class="trend-block">
              <div class="trend-cap">40场</div>
              <div class="bar-row"><span class="bar big" :style="{ width: state?.trend40.big_pct + '%' }">大 {{ state?.trend40.big_count }}</span></div>
              <div class="bar-row"><span class="bar small" :style="{ width: state?.trend40.small_pct + '%' }">小 {{ state?.trend40.small_count }}</span></div>
              <div v-if="state?.has_house" class="trend-house">通杀 {{ state?.trend40.house_count }}</div>
            </div>
            <div class="trend-block">
              <div class="trend-cap">最近10场</div>
              <div class="bar-row"><span class="bar big" :style="{ width: state?.trend10.big_pct + '%' }">大 {{ state?.trend10.big_count }}</span></div>
              <div class="bar-row"><span class="bar small" :style="{ width: state?.trend10.small_pct + '%' }">小 {{ state?.trend10.small_count }}</span></div>
            </div>
          </div>
          <div v-if="state?.recent_results?.length" class="dots">
            <span v-for="(r, i) in state.recent_results" :key="i" class="dot" :style="{ background: dotColor(r) }" :title="dotLabel(r)">{{ dotLabel(r) }}</span>
          </div>
          <div v-if="state?.trend_text" class="trend-text">{{ state.trend_text }}</div>
        </section>

        <section class="glass panel">
          <div class="sec-title">💰 下注</div>
          <div class="my-bet" v-if="state?.my_bet">本局已押 <b>{{ dotLabel(state.my_bet.bet_type) }}</b> {{ fmt(state.my_bet.amount) }}</div>
          <div class="bet-amount">
            <input v-model.number="amount" type="number" class="amt" :min="state?.min_bet ?? 1" />
            <span class="hint">最低 {{ state?.min_bet ?? 0 }}</span>
            <button class="quick allin" @click="allIn">梭哈</button>
          </div>
          <div class="quicks">
            <button v-for="q in quickAmounts" :key="q" class="quick" @click="amount = q">{{ q.toLocaleString() }}</button>
          </div>
          <select v-if="isHxpt && state?.props?.length" v-model="propId" class="prop-sel">
            <option value="0">不使用道具</option>
            <option v-for="p in state.props.filter((p) => p.usable)" :key="p.id" :value="p.id">{{ p.name }} ×{{ p.quantity }}</option>
          </select>
          <div class="ops">
            <button class="op big" :disabled="!canBet" @click="doBet('big')">押 大</button>
            <button class="op small" :disabled="!canBet" @click="doBet('small')">押 小</button>
          </div>
          <div class="betters">
            <div class="bet-col big"><span class="bet-col-cap">押大</span>
              <span v-if="!state?.big_bets?.length" class="empty">暂无</span>
              <span v-else v-for="(b, i) in state.big_bets.slice(0, 8)" :key="i" class="bet-name">{{ b.username }} {{ fmt(b.amount) }}</span>
            </div>
            <div class="bet-col small"><span class="bet-col-cap">押小</span>
              <span v-if="!state?.small_bets?.length" class="empty">暂无</span>
              <span v-else v-for="(b, i) in state.small_bets.slice(0, 8)" :key="i" class="bet-name">{{ b.username }} {{ fmt(b.amount) }}</span>
            </div>
          </div>
        </section>
      </div>

      <!-- 右：排行 -->
      <aside class="side">
        <section class="glass panel">
          <div class="sec-title">🏆 用户排行</div>
          <div v-if="!state?.rankings?.length" class="empty">暂无排行</div>
          <template v-else>
            <div class="tabs">
              <button v-for="(r, i) in state.rankings" :key="r.key" class="tab" :class="{ on: rankTab === i }" @click="rankTab = i">{{ r.title }}</button>
            </div>
            <div class="rank-list">
              <div v-for="(e, i) in state.rankings[rankTab]?.entries ?? []" :key="i" class="rank-row">
                <span class="rank-no" :class="top(i)">{{ i + 1 }}</span>
                <span class="rank-name">{{ e.username }}</span>
                <span class="rank-amt">{{ fmt(Math.abs(e.amount)) }}</span>
                <span v-if="e.count" class="rank-cnt">{{ e.count }}次</span>
              </div>
              <div v-if="!(state.rankings[rankTab]?.entries?.length)" class="empty">暂无数据</div>
            </div>
          </template>
        </section>

        <!-- 任务 / 道具 -->
        <section v-if="state?.tasks?.length || state?.props?.length" class="glass panel">
          <div class="sec-title">🎯 任务 / 道具</div>
          <div v-for="t in state?.tasks ?? []" :key="t.id" class="task-row">
            <div class="task-info">
              <span class="task-title">{{ t.title }}</span>
              <span v-if="t.progress_text" class="task-prog">{{ t.progress_text }}</span>
            </div>
            <button v-if="t.status === 'rewardable'" class="mini ok" @click="doTask(t, 'reward')">领奖</button>
            <button v-else-if="t.status === 'idle'" class="mini" @click="doTask(t, 'claim')">接取</button>
            <span v-else class="task-st">{{ t.status === 'doing' ? '进行中' : '已完成' }}</span>
          </div>
          <!-- 好学亏返卡 -->
          <button v-if="isHxpt && state?.props?.some((p) => p.usable === false)" class="mini warn full" @click="useRefund">💳 使用亏返卡</button>
          <!-- 熊猫道具独立使用 -->
          <div v-for="p in isHxpt ? [] : state?.props ?? []" :key="p.id" class="prop-row">
            <span class="prop-name">{{ p.name }} ×{{ p.quantity }}</span>
            <button class="mini" @click="useProp(p)">使用</button>
          </div>
        </section>

        <!-- 红包 -->
        <section class="glass panel">
          <div class="sec-title">🧧 红包</div>
          <div class="rp-form">
            <input v-model.number="rpAmount" type="number" class="rp-amt" placeholder="总额" />
            <input v-model.number="rpCount" type="number" class="rp-amt" placeholder="个数" />
          </div>
          <input v-model="rpGreeting" class="rp-greet" placeholder="祝福语" />
          <button class="mini ok full" :disabled="rpSending" @click="sendRed">{{ rpSending ? '发送中…' : '发红包' }}</button>
          <div v-if="state?.redpackets?.length" class="rp-list">
            <button v-for="r in state.redpackets" :key="r.id" class="rp-item" @click="claimRed(r)">
              🧧 {{ r.count }}个 / {{ fmt(r.total) }}
            </button>
          </div>
        </section>
      </aside>
    </div>

    <!-- 游戏记录 -->
    <section class="glass panel">
      <div class="sec-title-row">
        <div class="sec-title">📋 游戏记录</div>
        <div class="tabs sm">
          <button class="tab" :class="{ on: recTab === 'my' }" @click="recTab = 'my'">我的(近10场)</button>
          <button class="tab" :class="{ on: recTab === 'all' }" @click="recTab = 'all'">所有人(近20场)</button>
        </div>
      </div>
      <div class="rec-table">
        <div class="rec-head">
          <span>编号</span><span>开始</span><span>结束</span><span>人数</span><span>总注</span><span>结果</span>
          <template v-if="recTab === 'my'"><span>我的下注</span><span>参与结果</span></template>
          <span v-else>下注用户</span>
        </div>
        <div v-for="(r, i) in (recTab === 'my' ? state?.my_records : state?.all_records) ?? []" :key="i" class="rec-row">
          <span>{{ r.game_no }}</span>
          <span class="muted">{{ r.start_time }}</span>
          <span class="muted">{{ r.end_time }}</span>
          <span>{{ r.player_count }}</span>
          <span>{{ fmt(r.total_bet) }}</span>
          <span :class="r.result">{{ dotLabel(r.result) }}</span>
          <template v-if="recTab === 'my'">
            <span>{{ r.my_bet_type ? dotLabel(r.my_bet_type) + ' ' + fmt(r.my_bet) : '—' }}</span>
            <span :class="(r.my_result ?? 0) >= 0 ? 'big' : 'small'">{{ (r.my_result ?? 0) >= 0 ? '+' : '' }}{{ fmt(r.my_result) }}</span>
          </template>
          <span v-else class="muted bets-cell">{{ r.bets || '—' }}</span>
        </div>
        <div v-if="!((recTab === 'my' ? state?.my_records : state?.all_records)?.length)" class="empty">暂无记录</div>
      </div>
    </section>

    <!-- 聊天 -->
    <section class="glass panel chat-panel">
      <div class="sec-title">💬 聊天</div>
      <div class="chat-list">
        <div v-for="m in chat" :key="m.id" class="chat-msg">
          <span class="chat-user">{{ m.username }}</span>
          <span class="chat-text">{{ m.message }}</span>
        </div>
        <div v-if="!chat.length" class="empty">暂无消息</div>
      </div>
      <div class="chat-input">
        <input v-model="chatInput" placeholder="发条消息…" @keyup.enter="sendChat" />
        <button :disabled="sending" @click="sendChat">发送</button>
      </div>
    </section>
  </div>
</template>

<style scoped>
.bs { display: flex; flex-direction: column; gap: 14px; max-width: 860px; margin: 0 auto; }
.hero { padding: 18px; border-radius: 14px; background: linear-gradient(135deg, #db2777, #f59e0b); text-align: center; }
.hero-title { font-size: 22px; font-weight: 800; color: #fff; }
.balance { margin-top: 4px; font-size: 13px; color: rgba(255,255,255,0.92); }
.balance b { color: #fde68a; font-size: 16px; }
.cur { margin-left: 4px; }
.payout { margin-left: 8px; opacity: 0.85; }

.stat-bar { display: flex; gap: 8px; align-items: stretch; }
.stat-tile { flex: 1; text-align: center; padding: 10px 4px; border-radius: 12px; background: var(--glass-bg, rgba(255,255,255,0.06)); border: 1px solid var(--border-soft, rgba(255,255,255,0.08)); }
.stat-val { font-size: 18px; font-weight: 800; color: var(--pt-primary, #6c5ce7); }
.stat-val.warn { color: var(--pt-warn, #f59e0b); }
.stat-lbl { font-size: 11px; color: var(--text-muted, #999); margin-top: 2px; }
.refresh-btn { width: 40px; border: 0; border-radius: 12px; background: var(--glass-bg, rgba(255,255,255,0.06)); border: 1px solid var(--border-soft, rgba(255,255,255,0.08)); color: inherit; cursor: pointer; font-size: 18px; }
.refresh-btn:disabled { opacity: 0.5; cursor: wait; }

.last-res { padding: 10px 14px; border-radius: 10px; font-size: 13px; }
.last-res.win { background: rgba(34,197,94,0.12); }
.last-res.lose { background: rgba(231,76,60,0.1); }

.main { display: grid; grid-template-columns: 1fr; gap: 14px; }
@media (min-width: 720px) { .main { grid-template-columns: 1fr 280px; } }
.left, .side { display: flex; flex-direction: column; gap: 14px; }
.panel { padding: 14px; border-radius: 14px; }
.sec-title { font-weight: 700; font-size: 15px; margin-bottom: 10px; }
.sec-title-row { display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px; }
.sec-title-row .sec-title { margin-bottom: 0; }
.empty { font-size: 13px; color: var(--text-muted, #999); padding: 8px 0; }

/* 走势 */
.trend-row { display: flex; gap: 14px; }
.trend-block { flex: 1; }
.trend-cap { font-size: 11px; color: var(--text-muted, #999); margin-bottom: 4px; }
.bar-row { height: 18px; margin: 3px 0; }
.bar { display: inline-block; height: 18px; line-height: 18px; color: #fff; font-size: 11px; border-radius: 4px; padding: 0 6px; white-space: nowrap; }
.bar.big { background: #ef4444; }
.bar.small { background: #3b82f6; }
.trend-house { font-size: 11px; color: #22c55e; margin-top: 2px; }
.dots { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 10px; }
.dot { width: 20px; height: 20px; border-radius: 50%; color: #fff; font-size: 10px; line-height: 20px; text-align: center; font-weight: 700; }
.trend-text { margin-top: 8px; text-align: center; font-size: 12px; color: var(--pt-primary, #6c5ce7); }

/* 下注 */
.my-bet { font-size: 12px; color: var(--text-muted, #999); margin-bottom: 8px; }
.bet-amount { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
.amt { flex: 1; padding: 8px; border-radius: 8px; border: 1px solid var(--border-soft, rgba(255,255,255,0.15)); background: var(--glass-bg, rgba(255,255,255,0.06)); color: inherit; font-size: 15px; }
.hint { font-size: 11px; color: var(--text-muted, #999); }
.quicks { display: flex; flex-wrap: wrap; gap: 6px; margin-bottom: 8px; }
.quick { padding: 5px 10px; border-radius: 8px; border: 1px solid var(--border-soft, rgba(255,255,255,0.15)); background: var(--glass-bg, rgba(255,255,255,0.06)); color: inherit; cursor: pointer; font-size: 12px; }
.quick.allin { background: linear-gradient(135deg, #f59e0b, #db2777); color: #fff; border: 0; font-weight: 700; }
.prop-sel { width: 100%; padding: 7px; border-radius: 8px; border: 1px solid var(--border-soft, rgba(255,255,255,0.15)); background: var(--glass-bg, rgba(255,255,255,0.06)); color: inherit; font-size: 13px; margin-bottom: 8px; }
.ops { display: flex; gap: 10px; margin-bottom: 10px; }
.op { flex: 1; padding: 12px; border: 0; border-radius: 11px; cursor: pointer; font-weight: 800; font-size: 16px; color: #fff; }
.op.big { background: linear-gradient(135deg, #dc2626, #ef4444); }
.op.small { background: linear-gradient(135deg, #2563eb, #3b82f6); }
.op:disabled { opacity: 0.45; cursor: not-allowed; }
.betters { display: flex; gap: 8px; font-size: 11px; }
.bet-col { flex: 1; padding: 8px; border-radius: 8px; min-height: 40px; }
.bet-col.big { background: rgba(239,68,68,0.1); }
.bet-col.small { background: rgba(59,130,246,0.1); }
.bet-col-cap { font-weight: 700; display: block; margin-bottom: 2px; }
.bet-name { display: block; color: var(--text-muted, #aaa); }

/* 排行 */
.tabs { display: flex; flex-wrap: wrap; gap: 4px; margin-bottom: 8px; }
.tabs.sm .tab { font-size: 11px; padding: 3px 8px; }
.tab { padding: 4px 10px; border-radius: 8px; border: 1px solid var(--border-soft, rgba(255,255,255,0.15)); background: transparent; color: var(--text-muted, #999); cursor: pointer; font-size: 12px; }
.tab.on { background: var(--pt-primary, #6c5ce7); color: #fff; border-color: transparent; }
.rank-list { display: flex; flex-direction: column; gap: 4px; }
.rank-row { display: flex; align-items: center; gap: 6px; padding: 4px 0; font-size: 12px; border-bottom: 1px solid var(--border-soft, rgba(255,255,255,0.05)); }
.rank-no { width: 18px; height: 18px; line-height: 18px; text-align: center; border-radius: 50%; background: rgba(255,255,255,0.1); font-size: 10px; }
.rank-no.gold { background: #f59e0b; color: #fff; }
.rank-no.silver { background: #9ca3af; color: #fff; }
.rank-no.bronze { background: #b45309; color: #fff; }
.rank-name { flex: 1; font-weight: 600; }
.rank-amt { color: #16a34a; font-weight: 600; }
.rank-cnt { color: var(--text-muted, #999); font-size: 10px; }

/* 任务/道具/红包 */
.task-row, .prop-row { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 6px 0; font-size: 12px; border-bottom: 1px solid var(--border-soft, rgba(255,255,255,0.05)); }
.task-info { display: flex; flex-direction: column; }
.task-title { font-weight: 600; }
.task-prog { font-size: 10px; color: var(--text-muted, #999); }
.task-st { font-size: 11px; color: var(--text-muted, #999); }
.mini { padding: 4px 10px; border-radius: 7px; border: 1px solid var(--border-soft, rgba(255,255,255,0.2)); background: var(--glass-bg, rgba(255,255,255,0.06)); color: inherit; cursor: pointer; font-size: 11px; }
.mini.ok { background: #16a34a; color: #fff; border-color: transparent; }
.mini.warn { background: #f59e0b; color: #fff; border-color: transparent; }
.mini.full { width: 100%; margin-top: 6px; padding: 7px; }
.rp-form { display: flex; gap: 6px; margin-bottom: 6px; }
.rp-amt { flex: 1; padding: 6px; border-radius: 7px; border: 1px solid var(--border-soft, rgba(255,255,255,0.15)); background: var(--glass-bg, rgba(255,255,255,0.06)); color: inherit; font-size: 12px; }
.rp-greet { width: 100%; padding: 6px; border-radius: 7px; border: 1px solid var(--border-soft, rgba(255,255,255,0.15)); background: var(--glass-bg, rgba(255,255,255,0.06)); color: inherit; font-size: 12px; margin-bottom: 6px; box-sizing: border-box; }
.rp-list { display: flex; flex-direction: column; gap: 4px; margin-top: 8px; }
.rp-item { padding: 6px; border-radius: 7px; border: 1px solid #dc2626; background: rgba(220,38,38,0.1); color: inherit; cursor: pointer; font-size: 12px; }

/* 记录 */
.rec-table { font-size: 12px; }
.rec-head, .rec-row { display: grid; grid-template-columns: 1.2fr 1.4fr 1.4fr 0.5fr 1fr 0.6fr 1.4fr; gap: 4px; padding: 5px 0; border-bottom: 1px solid var(--border-soft, rgba(255,255,255,0.05)); }
.rec-head { color: var(--text-muted, #999); font-weight: 600; }
.rec-row .big { color: #ef4444; font-weight: 700; }
.rec-row .small { color: #3b82f6; font-weight: 700; }
.rec-row .house { color: #22c55e; font-weight: 700; }
.muted { color: var(--text-muted, #999); }
.bets-cell { font-size: 11px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

/* 聊天 */
.chat-panel { display: flex; flex-direction: column; }
.chat-list { max-height: 200px; overflow-y: auto; display: flex; flex-direction: column; gap: 4px; margin-bottom: 8px; }
.chat-msg { font-size: 12px; }
.chat-user { font-weight: 700; color: var(--pt-primary, #6c5ce7); margin-right: 6px; }
.chat-input { display: flex; gap: 6px; }
.chat-input input { flex: 1; padding: 7px; border-radius: 8px; border: 1px solid var(--border-soft, rgba(255,255,255,0.15)); background: var(--glass-bg, rgba(255,255,255,0.06)); color: inherit; font-size: 13px; }
.chat-input button { padding: 0 16px; border: 0; border-radius: 8px; background: var(--pt-primary, #6c5ce7); color: #fff; cursor: pointer; font-size: 13px; }
.chat-input button:disabled { opacity: 0.5; }
</style>
