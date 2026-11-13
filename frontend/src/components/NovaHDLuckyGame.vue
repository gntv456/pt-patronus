<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, onMounted } from 'vue'
import { entertainmentApi, type GameStats, type PrizeDef, type GameRecord, type PlayResult } from '@/api/entertainment'
import { entFail } from '@/utils/entFail'

const { t } = useI18n()
// 服务端奖品名占位值（比较用，非文案）
const NOWIN_SENTINEL = '\u672a\u4e2d\u5956'
const props = defineProps<{ siteId: number }>()
const GAME = 'novahd_lucky'

const stats = ref<GameStats | null>(null)
const prizes = ref<PrizeDef[]>([])
const records = ref<GameRecord[]>([])
const spinning = ref(false)
const last = ref<PlayResult | null>(null)

async function refreshStats() {
  stats.value = await entertainmentApi.stats(props.siteId, GAME).catch(entFail('stats', stats.value))
}
async function refreshHistory() {
  records.value = await entertainmentApi.history(props.siteId, GAME).catch(entFail('history', []))
}

async function draw(count: number) {
  if (spinning.value) return
  spinning.value = true
  try {
    const res =
      count === 1
        ? await entertainmentApi.play(props.siteId, GAME)
        : await entertainmentApi.playMulti(props.siteId, GAME, count)
    last.value = res
    if (res.success) {
      const wins = (res.prizes || []).filter((p) => p.is_win)
      ElMessage.success(wins.length ? t('nlg.wonN', { n: wins.length }) : res.message || t('bg.done'))
    } else {
      ElMessage.warning(res.message || t('nlg.drawFailed'))
    }
    await Promise.all([refreshStats(), refreshHistory()])
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || t('common.requestFailed'))
  } finally {
    spinning.value = false
  }
}

function emoji(name: string) {
  if (/VIP|\u4f1a\u5458/.test(name)) return '👑'
  if (/\u5f69\u8679/.test(name)) return '🌈'
  if (/\u9080\u8bf7/.test(name)) return '✉️'
  if (/\u9b54\u529b/.test(name)) return '🪙'
  return '🎁'
}

onMounted(async () => {
  prizes.value = await entertainmentApi.prizes(props.siteId, GAME).catch(entFail('prizes', [] as PrizeDef[]))
  await Promise.all([refreshStats(), refreshHistory()])
})
</script>

<template>
  <div class="lucky">
    <div class="hero">
      <div class="hero-title"><SptIcon name="gift" :size="18" aria-hidden="true" /> {{ t('nlg.title') }}</div>
    </div>

    <!-- 抽奖信息 -->
    <section class="glass section">
      <div class="info-grid">
        <div class="info-card">
          <b>{{ stats?.balance?.toLocaleString() ?? '—' }}</b
          ><small>{{ t('nlg.magicNow') }}</small>
        </div>
        <div class="info-card">
          <b>{{ stats?.cost_per_play?.toLocaleString() ?? '—' }}</b
          ><small>{{ t('nlg.costEach') }}</small>
        </div>
        <div class="info-card">
          <b>{{ stats?.today_spins ?? 0 }}/{{ stats?.max_daily || '?' }}</b
          ><small>{{ t('nlg.todayDrawn') }}</small>
        </div>
      </div>
    </section>

    <!-- 奖品列表 -->
    <section v-if="prizes.length" class="glass section">
      <div class="sec-title">{{ t('nlg.prizes') }}</div>
      <div class="prize-strip">
        <div v-for="p in prizes" :key="p.id" class="prize-card">
          <span class="p-emoji" aria-hidden="true">{{ emoji(p.name) }}</span>
          <span class="p-name">{{ p.name }}</span>
        </div>
      </div>
    </section>

    <!-- 抽奖操作 -->
    <section class="glass section ops">
      <button class="op-btn primary" :disabled="spinning" @click="draw(1)">
        {{ spinning ? t('nlg.drawing') : t('nlg.drawNow') }}
      </button>
      <button class="op-btn" :disabled="spinning" @click="draw(10)">{{ t('sp.multi', { n: 10 }) }}</button>
      <button class="op-btn" :disabled="spinning" @click="draw(20)">{{ t('sp.multi', { n: 20 }) }}</button>
      <button class="op-btn" :disabled="spinning" @click="draw(50)">{{ t('sp.multi', { n: 50 }) }}</button>
    </section>

    <div v-if="last" class="result" :class="last.success ? 'win' : 'lose'">
      <span class="badge"><SptIcon :name="last.success ? 'party-popper' : 'x'" :size="18" aria-hidden="true" /></span>
      <div class="r-detail">
        <div v-if="last.prizes?.length" class="r-list">
          <span v-for="(p, i) in last.prizes.filter((x) => x.is_win).slice(0, 8)" :key="i" class="r-pill">
            {{ p.prize_name }}<template v-if="p.amount"> ×{{ p.amount }}</template>
          </span>
          <span v-if="!last.prizes.some((x) => x.is_win)" class="text-muted">{{ t('nlg.sorryNoWin') }}</span>
        </div>
        <span v-else-if="last.message" class="r-msg">{{ last.message }}</span>
      </div>
    </div>

    <!-- 记录 -->
    <section v-if="records.length" class="glass section">
      <div class="sec-title">{{ t('sc.history') }}</div>
      <div v-for="(r, i) in records.slice(0, 10)" :key="i" class="rec-row">
        <span :class="{ muted: r.prize_name === NOWIN_SENTINEL }">{{ r.prize_name }}</span>
        <span v-if="r.amount" class="rec-amt">+{{ r.amount }}</span>
        <span class="text-muted rec-time">{{ r.created_at }}</span>
      </div>
    </section>
  </div>
</template>

<style scoped>
.lucky {
  display: flex;
  flex-direction: column;
  gap: var(--sp-4);
  max-width: 640px;
  margin: 0 auto;
}
.hero {
  padding: 18px;
  border-radius: var(--radius-md);
  background: var(--color-surface-2);
  border: 1px solid var(--color-border);
  text-align: center;
}
.hero-title {
  font-size: var(--fs-title);
  font-weight: 800;
  color: var(--color-text);
  letter-spacing: 1px;
}

.section {
  padding: 16px;
  border-radius: var(--radius-md);
}
.sec-title {
  font-weight: 700;
  margin-bottom: 12px;
  font-size: var(--fs-lg);
}

.info-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--sp-2);
}
.info-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--sp-1);
  padding: 14px 8px;
  border-radius: var(--radius-sm);
  background: var(--color-surface-2);
  border: 1px solid var(--color-border);
}
.info-card b {
  font-size: var(--fs-xl);
  color: var(--color-primary);
}
.info-card small {
  font-size: 12px;
  color: var(--color-text-muted);
}

.prize-strip {
  display: flex;
  gap: var(--sp-2);
  overflow-x: auto;
  padding-bottom: 4px;
}
.prize-card {
  flex: 0 0 auto;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--sp-1);
  padding: 10px 12px;
  border-radius: var(--radius-sm);
  background: var(--color-surface-2);
  border: 1px solid var(--color-border);
  min-width: 70px;
}
.p-emoji {
  font-size: var(--fs-title);
}
.p-name {
  font-size: 12px;
  font-weight: 600;
  white-space: nowrap;
}

.ops {
  display: flex;
  gap: var(--sp-2);
  flex-wrap: wrap;
  justify-content: center;
}
.op-btn {
  min-height: 24px;
  padding: 9px 16px;
  border: 1px solid var(--color-primary);
  border-radius: var(--radius-sm);
  cursor: pointer;
  font-size: var(--fs-body);
  font-weight: 600;
  color: var(--color-primary);
  background: transparent;
  transition: background-color var(--dur-base) var(--ease), border-color var(--dur-base) var(--ease), color var(--dur-base) var(--ease), box-shadow var(--dur-base) var(--ease), opacity var(--dur-base) var(--ease);
}
.op-btn:hover:not(:disabled) {
  background: var(--color-primary-soft);
}
.op-btn.primary:hover:not(:disabled) {
  background: var(--color-primary-hover);
}
.op-btn.primary {
  color: var(--color-on-accent);
  background: var(--color-primary);
  border: 0;
  box-shadow: var(--shadow-card);
}
.op-btn:disabled {
  opacity: 0.6;
  cursor: wait;
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
  font-size: var(--fs-xl);
}
.r-list {
  display: flex;
  flex-wrap: wrap;
  gap: var(--sp-1);
}
.r-pill {
  padding: 3px 10px;
  border-radius: var(--radius-xs);
  font-size: var(--fs-xs);
  font-weight: 600;
  background: var(--pt-success-bg);
  color: var(--pt-success-fg);
}
.r-msg {
  font-size: var(--fs-body);
}

.rec-row {
  display: flex;
  gap: var(--sp-2);
  align-items: center;
  padding: 6px 0;
  font-size: var(--fs-body);
  border-bottom: 1px solid var(--color-border);
}
.rec-row .muted {
  color: var(--color-text-muted);
}
.rec-amt {
  color: var(--pt-success-fg);
  font-weight: 600;
}
.rec-time {
  margin-left: auto;
  font-size: 12px;
}
</style>
