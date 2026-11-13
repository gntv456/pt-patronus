<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { entertainmentApi, type GameStats, type PlayResult } from '@/api/entertainment'
import { entFail } from '@/utils/entFail'

// HDVIDEO HiLo 专用组件（批17）：翻牌 High/Low——Play 只读带 hilo_state（牌面/彩池/倍率/连胜），
// 开局/猜大小/兑现全部可操作。
const { t } = useI18n()
const props = defineProps<{ siteId: number; game: string; name: string }>()

interface HiloState {
  playing: boolean
  card?: string
  pot?: number
  mult?: number
  streak?: number
  hi_avail: boolean
  hi_mult?: number
  lo_avail: boolean
  lo_mult?: number
}

const stats = ref<GameStats | null>(null)
const state = ref<HiloState | null>(null)
const busy = ref(false)
const bet = ref(100)
const last = ref<PlayResult | null>(null)

async function refresh() {
  try {
    const res = await entertainmentApi.play(props.siteId, props.game)
    stats.value = await entertainmentApi.stats(props.siteId, props.game).catch(entFail('stats', stats.value))
    const raw = res.raw as { hilo_state?: HiloState } | undefined
    state.value = raw?.hilo_state ?? null
    if (!res.success) ElMessage.warning(res.message)
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '状态获取失败')
  }
}

async function act(fn: () => Promise<PlayResult>) {
  if (busy.value) return
  busy.value = true
  try {
    const res = await fn()
    last.value = res
    if (res.success) ElMessage.success(res.prize || res.message || 'OK')
    else ElMessage.warning(res.message || '操作失败')
    await refresh()
  } finally {
    busy.value = false
  }
}

const start = () => act(() => entertainmentApi.bet(props.siteId, props.game, { selection: '', amount: bet.value }))
const guess = (dir: 'hi' | 'lo') =>
  act(() => entertainmentApi.bet(props.siteId, props.game, { selection: dir, amount: 0 }))
const cashout = () =>
  act(() => entertainmentApi.bet(props.siteId, props.game, { selection: 'cashout', amount: 0 }))

onMounted(refresh)
</script>

<template>
  <div class="hilo">
    <div class="hero">
      <div class="hero-title"><SptIcon name="gamepad" :size="16" aria-hidden="true" /> {{ name }}</div>
      <div class="hero-sub">
        {{ t('bj.balance') }} <b>{{ stats?.balance?.toLocaleString() ?? '—' }}</b> 电影票
      </div>
    </div>

    <section class="glass section table-zone">
      <div class="card-area">
        <div class="card-face" :class="{ active: state?.playing }">
          <span v-if="state?.card" aria-hidden="true">{{ state.card }}</span>
          <span v-else class="text-muted">未开局</span>
        </div>
      </div>

      <div v-if="state?.playing" class="status-line">
        彩池 <b class="pot">{{ (state.pot ?? 0).toLocaleString() }}</b> 票 ·
        ×{{ (state.mult ?? 1).toFixed(2) }} · 连胜 {{ state.streak ?? 0 }}
      </div>

      <div v-if="state?.playing" class="ops">
        <button class="dir hi" :disabled="busy || !state.hi_avail" @click="guess('hi')">
          <SptIcon name="arrow-up" :size="16" aria-hidden="true" /> 高 <span v-if="state.hi_mult">×{{ state.hi_mult.toFixed(2) }}</span>
        </button>
        <button class="dir lo" :disabled="busy || !state.lo_avail" @click="guess('lo')">
          <SptIcon name="arrow-down" :size="16" aria-hidden="true" /> 低 <span v-if="state.lo_mult">×{{ state.lo_mult.toFixed(2) }}</span>
        </button>
        <button class="cash" :disabled="busy" @click="cashout()"><SptIcon name="download" :size="16" aria-hidden="true" /> 兑现</button>
      </div>

      <div v-else class="ops start">
        <el-input-number v-model="bet" :min="1" />
        <el-button type="primary" size="large" :loading="busy" @click="start()">投入 {{ bet }} 票开局</el-button>
      </div>
    </section>

    <div v-if="last" class="result" :class="last.success ? 'win' : 'lose'">
      <span class="badge"><SptIcon :name="last.success ? 'check' : 'x'" :size="16" aria-hidden="true" />{{ last.success ? '成功' : '失败' }}</span>
      <div class="r-body">
        <span v-if="last.prize" class="r-prize">{{ last.prize }}</span>
        <span v-if="last.balance" class="r-bal">余 {{ last.balance.toLocaleString() }}</span>
        <div v-if="last.message" class="r-msg text-muted">{{ last.message }}</div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="css">
.hilo { display: flex; flex-direction: column; gap: var(--sp-4); max-width: 520px; margin: 0 auto; }
.hero { padding: 18px; border-radius: var(--radius-md); background: var(--color-surface-2); border: 1px solid var(--color-border); color: var(--color-text); text-align: center; }
.hero-title { font-size: var(--fs-title); font-weight: 800; }
.hero-sub { margin-top: 6px; color: var(--color-text-secondary); }
.section { padding: 16px; }
.table-zone { display: flex; flex-direction: column; align-items: center; gap: 14px; }
.card-area { perspective: 600px; }
.card-face { width: 120px; height: 168px; border-radius: 12px; background: var(--color-surface); border: 3px solid var(--color-border-strong); display: flex; align-items: center; justify-content: center; font-size: 44px; font-weight: 900; color: var(--color-text); transition: border-color var(--dur-base) var(--ease), box-shadow var(--dur-base) var(--ease); }
.card-face.active { border-color: var(--color-warning); box-shadow: 0 0 0 3px var(--color-warning-soft); }
.status-line { font-size: 14px; }
.pot { color: var(--pt-warn-fg); font-size: 17px; font-variant-numeric: tabular-nums; }
.ops { display: flex; gap: 10px; width: 100%; justify-content: center; }
.ops.start { align-items: center; }
.dir, .cash { flex: 1; max-width: 150px; min-height: 24px; padding: 14px 8px; border-radius: 10px; border: none; font-size: 16px; font-weight: 800; cursor: pointer; color: var(--color-on-accent); transition: box-shadow var(--dur-fast) var(--ease); }
.dir.hi { background: var(--color-danger); }
.dir.lo { background: var(--color-primary); }
.cash { background: var(--color-warning); }
.dir:hover:not(:disabled), .cash:hover:not(:disabled) { box-shadow: var(--shadow-card); }
.dir:disabled, .cash:disabled { opacity: .5; cursor: not-allowed; }
.dir span { font-size: 12px; opacity: .85; display: block; margin-top: 2px; }
.result { display: flex; gap: 10px; padding: 12px 16px; border-radius: var(--radius-md); align-items: flex-start; }
.result.win { background: var(--color-success-soft); color: var(--pt-success-fg); }
.result.lose { background: var(--color-danger-soft); color: var(--pt-danger-fg); }
.badge { font-weight: 800; }
.r-prize { font-weight: 800; margin-right: 8px; }
</style>
