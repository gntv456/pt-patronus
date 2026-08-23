<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { entertainmentApi, type BetEvent, type BetEventDetail, type PlayResult } from '@/api/entertainment'

// OurBits 赛事押注（菠菜）：列表 → 盘口（选项+赔率）→ 下单 → 结果。
const props = defineProps<{ siteId: number; game?: string }>()
const game = props.game ?? 'ourbits_bet'

const balance = ref<number | null>(null)
const events = ref<BetEvent[]>([])
const loading = ref(false)
const detail = ref<BetEventDetail | null>(null) // 非 null = 详情视图
const detailLoading = ref(false)
const selectedOpt = ref<string>('')
const amount = ref(100)
const playing = ref(false)
const last = ref<PlayResult | null>(null)

// 选项按赔率升序（热门在前，更直观）。
const sortedOptions = computed(() =>
  detail.value ? [...(detail.value.options || [])].sort((a, b) => a.odds - b.odds) : [],
)

async function refresh() {
  loading.value = true
  try {
    const [evs, st] = await Promise.all([
      entertainmentApi.events(props.siteId, game).catch(() => [] as BetEvent[]),
      entertainmentApi.stats(props.siteId, game).catch(() => null),
    ])
    events.value = evs
    if (st) balance.value = st.balance
  } finally {
    loading.value = false
  }
}

async function openEvent(ev: BetEvent) {
  detail.value = null
  detailLoading.value = true
  selectedOpt.value = ''
  last.value = null
  try {
    detail.value = await entertainmentApi.eventDetail(props.siteId, game, ev.id)
    if (detail.value.my_option) selectedOpt.value = ''
  } catch {
    ElMessage.error('加载盘口失败')
  } finally {
    detailLoading.value = false
  }
}

function backToList() {
  detail.value = null
  selectedOpt.value = ''
  last.value = null
}

async function doBet() {
  if (!selectedOpt.value || amount.value <= 0) {
    ElMessage.warning('请选择选项并输入押注金额')
    return
  }
  if (playing.value) return
  playing.value = true
  try {
    const res = await entertainmentApi.bet(props.siteId, game, { selection: selectedOpt.value, amount: amount.value })
    last.value = res
    if (res.success) {
      ElMessage.success(res.message || '押注成功')
      if (res.balance) balance.value = res.balance
    } else {
      ElMessage.warning(res.message || '押注失败')
    }
    await refresh()
    if (detail.value) {
      const d = await entertainmentApi.eventDetail(props.siteId, game, detail.value.event.id).catch(() => null)
      if (d) detail.value = d
    }
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '请求失败')
  } finally {
    playing.value = false
  }
}

const fmt = (n: number) => n.toLocaleString()

onMounted(refresh)
</script>

<template>
  <div class="eventbet" v-loading="loading">
    <div class="hero">
      <div class="hero-title">⚽ 赛事押注</div>
      <div class="balance">余额 <b>{{ balance != null ? fmt(Math.floor(balance)) : '—' }}</b> 魔力</div>
    </div>

    <!-- 盘口详情视图 -->
    <template v-if="detail">
      <div class="glass section">
        <div class="ev-head">
          <span class="ev-title">{{ detail.event.title }}</span>
          <button class="link-btn" @click="backToList">‹ 返回列表</button>
        </div>
        <div class="ev-meta">
          <span>截止 {{ detail.event.end_time }}</span>
          <span v-if="detail.event.remain" class="remain">剩余 {{ detail.event.remain }}</span>
        </div>
        <p v-if="detail.my_option" class="my-bet">已押：{{ detail.my_option }}</p>
      </div>

      <div class="glass section" v-loading="detailLoading">
        <div class="sec-title">选项（按赔率，热门在前）</div>
        <div class="opts">
          <label
            v-for="o in sortedOptions"
            :key="o.id"
            class="opt"
            :class="{ sel: selectedOpt === o.id, mine: detail.my_option === o.name }"
          >
            <input type="radio" :value="o.id" v-model="selectedOpt" :disabled="!!detail.my_option" />
            <span class="opt-name">{{ o.name }}</span>
            <span class="opt-odds">×{{ o.odds }}</span>
          </label>
        </div>

        <template v-if="!detail.my_option">
          <div class="bet-row">
            <span class="lbl">押注魔力</span>
            <input v-model.number="amount" type="number" class="amt" min="1" />
            <button class="op primary" :disabled="playing || !selectedOpt" @click="doBet">
              {{ playing ? '提交中…' : '押注' }}
            </button>
          </div>
          <p class="hint">可获 = 押注 × 赔率（赛事结束后结算派彩）</p>
        </template>
      </div>
    </template>

    <!-- 列表视图 -->
    <template v-else>
      <div v-if="!events.length && !loading" class="glass empty">暂无进行中的竞猜</div>
      <div v-for="ev in events" :key="ev.id" class="glass ev-card" @click="openEvent(ev)">
        <div class="ev-card-title">
          <span>{{ ev.title }}</span>
          <span class="status" :class="ev.status">{{ ev.status === 'open' ? '进行中' : ev.status }}</span>
        </div>
        <div class="ev-card-meta">
          <span>截止 {{ ev.end_time }}</span>
          <span v-if="ev.remain" class="remain">剩余 {{ ev.remain }}</span>
        </div>
        <div class="ev-card-stats">
          <span>庄家 {{ ev.bookmaker || '—' }}</span>
          <span>{{ ev.bet_count }} 人下注</span>
          <span>投注 {{ fmt(Math.floor(ev.total_stake)) }} 魔力</span>
          <span :class="{ mine: ev.my_stake !== '-' && ev.my_stake !== '' }">我 {{ ev.my_stake }}</span>
        </div>
        <div class="ev-card-go">查看盘口 ›</div>
      </div>
    </template>

    <div v-if="last" class="result" :class="last.success ? 'win' : 'lose'">
      <span class="badge">{{ last.success ? '✓' : '✗' }}</span>
      <div class="r-body">
        <span v-if="last.prize" class="r-prize">{{ last.prize }}</span>
        <span v-if="last.balance" class="r-bal">余额 {{ fmt(Math.floor(last.balance)) }}</span>
        <div v-if="last.message" class="r-msg text-muted">{{ last.message }}</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.eventbet { display: flex; flex-direction: column; gap: var(--sp-4); max-width: 640px; margin: 0 auto; }
.hero { padding: 18px; border-radius: var(--radius-md); background: linear-gradient(135deg, #059669, #0ea5e9); text-align: center; }
.hero-title { font-size: var(--fs-title); font-weight: 800; color: #fff; }
.balance { margin-top: 4px; font-size: var(--fs-body); color: rgba(255,255,255,0.92); }
.balance b { color: #fde68a; font-size: var(--fs-lg); }

.section { padding: 18px; border-radius: var(--radius-md); display: flex; flex-direction: column; gap: var(--sp-3); }
.sec-title { font-weight: 700; font-size: var(--fs-lg); align-self: flex-start; }
.ev-head { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-2); }
.ev-title { font-weight: 800; font-size: var(--fs-xl); }
.ev-meta { display: flex; gap: var(--sp-3); font-size: var(--fs-body); color: var(--text-muted, #999); }
.remain { color: var(--pt-warn, #f59e0b); font-weight: 600; }
.my-bet { font-size: var(--fs-body); color: var(--pt-primary, #6c5ce7); font-weight: 600; }
.link-btn { background: none; border: 0; color: var(--pt-primary, #6c5ce7); cursor: pointer; font-size: var(--fs-body); }

.opts { display: grid; grid-template-columns: 1fr 1fr; gap: var(--sp-2); }
.opt { display: flex; align-items: center; gap: var(--sp-2); padding: 9px 12px; border-radius: var(--radius-sm); border: 1px solid var(--border-soft, rgba(255,255,255,0.15)); background: var(--glass-bg, rgba(255,255,255,0.04)); cursor: pointer; font-size: var(--fs-md); }
.opt.sel { border-color: var(--pt-primary, #6c5ce7); background: var(--pt-primary-bg); }
.opt.mine { border-color: var(--pt-warn, #f59e0b); }
.opt-name { flex: 1; font-weight: 600; }
.opt-odds { color: var(--pt-warn, #f59e0b); font-weight: 700; }

.bet-row { display: flex; align-items: center; gap: var(--sp-2); flex-wrap: wrap; }
.lbl { font-size: var(--fs-body); color: var(--text-muted, #999); }
.amt { padding: 8px 12px; border-radius: var(--radius-sm); border: 1px solid var(--border-soft, rgba(255,255,255,0.15)); background: var(--glass-bg, rgba(255,255,255,0.06)); color: inherit; width: 130px; font-size: var(--fs-lg); }
.op { padding: 11px 24px; border: 0; border-radius: var(--radius-sm); cursor: pointer; font-weight: 700; font-size: var(--fs-lg); color: #fff; margin-left: auto; }
.op.primary { background: linear-gradient(135deg, #059669, #0ea5e9); }
.op:disabled { opacity: 0.6; cursor: wait; }
.hint { font-size: var(--fs-xs); color: var(--text-muted, #999); }

.empty { padding: 36px; text-align: center; color: var(--text-muted, #999); border-radius: var(--radius-md); }
.ev-card { padding: 14px 16px; border-radius: var(--radius-md); cursor: pointer; display: flex; flex-direction: column; gap: var(--sp-1); transition: transform var(--dur-base) var(--ease-out-smooth), box-shadow var(--dur-base) var(--ease-out-smooth), border-color var(--dur-base) var(--ease-out-smooth); }
.ev-card:hover { transform: translateY(-1px); }
.ev-card-title { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-2); font-weight: 700; font-size: var(--fs-lg); }
.status { font-size: var(--fs-xs); padding: 2px 8px; border-radius: var(--radius-xs); background: rgba(5,150,105,0.16); color: #059669; font-weight: 600; }
.status.ended { background: rgba(100,116,139,0.18); color: #94a3b8; }
.ev-card-meta { display: flex; gap: var(--sp-3); font-size: var(--fs-body); color: var(--text-muted, #999); }
.ev-card-stats { display: flex; flex-wrap: wrap; gap: var(--sp-3); font-size: var(--fs-xs); color: var(--text-muted, #999); }
.ev-card-stats .mine { color: var(--pt-warn, #f59e0b); font-weight: 600; }
.ev-card-go { font-size: var(--fs-xs); color: var(--pt-primary, #6c5ce7); font-weight: 600; }

.result { padding: 12px; border-radius: var(--radius-sm); display: flex; gap: var(--sp-2); align-items: flex-start; }
.result.win { background: rgba(5, 150, 105, 0.12); }
.result.lose { background: var(--pt-danger-bg); }
.badge { font-weight: 700; }
.r-body { display: flex; flex-direction: column; gap: var(--sp-1); }
.r-prize { font-weight: 700; color: var(--pt-success); }
.r-bal { font-size: var(--fs-xs); color: var(--text-muted, #999); }
.r-msg { font-size: var(--fs-xs); }
</style>
