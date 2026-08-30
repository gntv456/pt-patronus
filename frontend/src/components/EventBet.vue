<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, computed, onMounted } from 'vue'
import { entertainmentApi, type BetEvent, type BetEventDetail, type PlayResult } from '@/api/entertainment'

// OurBits 赛事押注（菠菜）：列表 → 盘口（选项+赔率）→ 下单 → 结果。
const props = defineProps<{ siteId: number; game?: string }>()
const game = props.game ?? 'ourbits_bet'

const balance = ref<number | null>(null)
const events = ref<BetEvent[]>([])
const loading = ref(false)
const { t } = useI18n()
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
    ElMessage.error(t('eb.loadFailed'))
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
    ElMessage.warning(t('eb.needInput'))
    return
  }
  if (playing.value) return
  playing.value = true
  try {
    const res = await entertainmentApi.bet(props.siteId, game, { selection: selectedOpt.value, amount: amount.value })
    last.value = res
    if (res.success) {
      ElMessage.success(res.message || t('eb.betOk'))
      if (res.balance) balance.value = res.balance
    } else {
      ElMessage.warning(res.message || t('eb.betFailed'))
    }
    await refresh()
    if (detail.value) {
      const d = await entertainmentApi.eventDetail(props.siteId, game, detail.value.event.id).catch(() => null)
      if (d) detail.value = d
    }
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || t('common.requestFailed'))
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
      <div class="hero-title">⚽ {{ t('eb.title') }}</div>
      <div class="balance">
        {{ t('bs.balance') }} <b>{{ balance != null ? fmt(Math.floor(balance)) : '—' }}</b> {{ t('eb.magic') }}
      </div>
    </div>

    <!-- 盘口详情视图 -->
    <template v-if="detail">
      <div class="glass section">
        <div class="ev-head">
          <span class="ev-title">{{ detail.event.title }}</span>
          <button class="link-btn" @click="backToList">‹ {{ t('eb.backList') }}</button>
        </div>
        <div class="ev-meta">
          <span>{{ t('eb.deadline', { v: detail.event.end_time }) }}</span>
          <span v-if="detail.event.remain" class="remain">{{ t('eb.remain', { v: detail.event.remain }) }}</span>
        </div>
        <p v-if="detail.my_option" class="my-bet">{{ t('eb.myBet', { v: detail.my_option }) }}</p>
      </div>

      <div class="glass section" v-loading="detailLoading">
        <div class="sec-title">{{ t('eb.options') }}</div>
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
            <span class="lbl">{{ t('eb.stakeMagic') }}</span>
            <input v-model.number="amount" type="number" class="amt" min="1" />
            <button class="op primary" :disabled="playing || !selectedOpt" @click="doBet">
              {{ playing ? t('eb.submitting') : t('eb.betGo') }}
            </button>
          </div>
          <p class="hint">{{ t('eb.payoutHint') }}</p>
        </template>
      </div>
    </template>

    <!-- 列表视图 -->
    <template v-else>
      <div v-if="!events.length && !loading" class="glass empty">{{ t('eb.noEvents') }}</div>
      <div v-a11y-button v-for="ev in events" :key="ev.id" class="glass ev-card" @click="openEvent(ev)">
        <div class="ev-card-title">
          <span>{{ ev.title }}</span>
          <span class="status" :class="ev.status">{{ ev.status === 'open' ? t('eb.open') : ev.status }}</span>
        </div>
        <div class="ev-card-meta">
          <span>{{ t('eb.deadline', { v: ev.end_time }) }}</span>
          <span v-if="ev.remain" class="remain">{{ t('eb.remain', { v: ev.remain }) }}</span>
        </div>
        <div class="ev-card-stats">
          <span>{{ t('eb.bookmaker', { v: ev.bookmaker || '—' }) }}</span>
          <span>{{ t('eb.bettors', { n: ev.bet_count }) }}</span>
          <span>{{ t('eb.totalStake', { v: fmt(Math.floor(ev.total_stake)) }) }}</span>
          <span :class="{ mine: ev.my_stake !== '-' && ev.my_stake !== '' }">{{ t('eb.me', { v: ev.my_stake }) }}</span>
        </div>
        <div class="ev-card-go">{{ t('eb.viewOdds') }} ›</div>
      </div>
    </template>

    <div v-if="last" class="result" :class="last.success ? 'win' : 'lose'">
      <span class="badge">{{ last.success ? '✓' : '✗' }}</span>
      <div class="r-body">
        <span v-if="last.prize" class="r-prize">{{ last.prize }}</span>
        <span v-if="last.balance" class="r-bal">{{ t('bs.balance') }} {{ fmt(Math.floor(last.balance)) }}</span>
        <div v-if="last.message" class="r-msg text-muted">{{ last.message }}</div>
      </div>
    </div>
  </div>
</template>

<style scoped src="./EventBet.css"></style>
