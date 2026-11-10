<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, onMounted } from 'vue'
import { entertainmentApi, type GameStats, type PrizeDef, type GameRecord, type PlayResult } from '@/api/entertainment'
import { entFail } from '@/utils/entFail'

const props = defineProps<{ siteId: number }>()

const { t } = useI18n()
const GAME = 'longpt_daily'

const stats = ref<GameStats | null>(null)
const prizes = ref<PrizeDef[]>([])
const records = ref<GameRecord[]>([])
const joining = ref(false)
const joined = ref<PlayResult | null>(null)

// 每日奖品图标按名称关键词推断（奖品名来自站点 desc 字段，如"333333魔力"）
function emoji(name: string) {
  if (/\u9b54\u529b|\u706b\u82b1/.test(name)) return '🪙'
  if (/\u4e0a\u4f20/.test(name)) return '📤'
  if (/VIP|\u4f1a\u5458/.test(name)) return '👑'
  if (/\u9080\u8bf7/.test(name)) return '✉️'
  if (/\u52cb\u7ae0/.test(name)) return '🎖️'
  if (/\u5f69\u8679/.test(name)) return '🌈'
  return '🎁'
}

async function refreshStats() {
  stats.value = await entertainmentApi.stats(props.siteId, GAME).catch(entFail('stats', stats.value))
}
async function refreshHistory() {
  records.value = await entertainmentApi.history(props.siteId, GAME).catch(entFail('history', []))
}

async function join() {
  if (joining.value) return
  joining.value = true
  try {
    const res = await entertainmentApi.play(props.siteId, GAME)
    joined.value = res
    if (res.success) ElMessage.success(res.message || t('dl.joinedOk'))
    else ElMessage.warning(res.message || t('dl.joinFailed'))
    await Promise.all([refreshStats(), refreshHistory()])
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || t('common.requestFailed'))
  } finally {
    joining.value = false
  }
}

onMounted(async () => {
  prizes.value = await entertainmentApi.prizes(props.siteId, GAME).catch(entFail('prizes', [] as PrizeDef[]))
  await Promise.all([refreshStats(), refreshHistory()])
})
</script>

<template>
  <div class="lpt-daily">
    <div class="hero">
      <div class="hero-title">🎊 {{ t('dl.title') }}</div>
      <div class="hero-sub">
        {{ t('dl.sub', { time: stats?.open_time ? t('dl.openAt', { v: stats.open_time }) : t('dl.sameDay') }) }}
      </div>
    </div>

    <div class="board">
      <!-- 今日奖品 -->
      <div class="prizes">
        <div class="section-title">{{ t('dl.todayPrizes') }}</div>
        <div class="prize-grid">
          <div v-for="p in prizes" :key="p.id" class="prize-card">
            <span class="p-emoji">{{ emoji(p.name) }}</span>
            <span class="p-name">{{ p.name }}</span>
          </div>
          <div v-if="!prizes.length" class="text-muted empty-tip">{{ t('dl.noPrizes') }}</div>
        </div>
      </div>

      <!-- 立即参与 -->
      <div class="join">
        <div class="section-title">{{ t('dl.joinNow') }}</div>
        <div class="join-circle-wrap">
          <button class="join-circle" :disabled="joining" @click="join">
            <span v-if="joining" class="spinner" />
            <template v-else>
              <span class="jc-icon">🎁</span>
              <span class="jc-text">{{ joined && joined.success ? t('dl.joined') : t('dl.joinNow2') }}</span>
            </template>
          </button>
        </div>
        <div class="join-meta">
          <div v-if="stats?.participants !== undefined" class="meta-row">
            <span>👥</span><span>{{ t('dl.participants', { n: stats.participants }) }}</span>
          </div>
          <div v-if="stats?.start_time" class="meta-row">
            <span>🕒</span><span>{{ t('dl.joinWindow', { s: stats.start_time, e: stats.end_time }) }}</span>
          </div>
          <div v-if="stats?.open_time" class="meta-row">
            <span>🔔</span><span>{{ t('dl.openTime', { v: stats.open_time }) }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 参与提示 -->
    <div v-if="joined" class="result" :class="joined.success ? 'win' : 'lose'">
      <span class="badge">{{ joined.success ? '✓ ' + t('dl.joined') : '✗ ' + t('dl.fail') }}</span>
      <span v-if="joined.message" class="r-msg">{{ joined.message }}</span>
    </div>

    <!-- 获奖记录 -->
    <div v-if="records.length" class="glass records">
      <div class="rec-title">{{ t('dl.winRecords') }}</div>
      <div v-for="(r, i) in records.slice(0, 10)" :key="i" class="rec-row">
        <span class="rec-prize" :class="{ muted: r.prize_name === t('sp.noWin') }">{{ r.prize_name }}</span>
        <span v-if="r.amount" class="rec-amt">×{{ r.amount }}</span>
        <span class="rec-time text-muted">{{ r.created_at }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.lpt-daily {
  display: flex;
  flex-direction: column;
  gap: var(--sp-4);
  max-width: 640px;
  margin: 0 auto;
}
.hero {
  padding: 16px;
  border-radius: var(--radius-md);
  background: linear-gradient(135deg, #1e3a8a, #3b5bdb);
  text-align: center;
}
.hero-title {
  font-size: var(--fs-title);
  font-weight: 800;
  color: #fff;
  letter-spacing: 2px;
}
.hero-sub {
  margin-top: 4px;
  font-size: var(--fs-xs);
  color: rgba(255, 255, 255, 0.8);
}

.board {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--sp-4);
}
@media (max-width: 520px) {
  .board {
    grid-template-columns: 1fr;
  }
}
.section-title {
  font-weight: 700;
  margin-bottom: 10px;
  font-size: var(--fs-lg);
}

.prize-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--sp-2);
}
@media (max-width: 520px) {
  .prize-grid {
    grid-template-columns: repeat(3, 1fr);
  }
}
.prize-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--sp-1);
  padding: 12px 6px;
  border-radius: var(--radius-sm);
  background: var(--glass-bg, rgba(255, 255, 255, 0.06));
  border: 1px solid var(--border-soft, rgba(255, 255, 255, 0.08));
}
.p-emoji {
  font-size: var(--fs-hero);
}
.p-name {
  font-size: var(--fs-xs);
  font-weight: 600;
  text-align: center;
}
.empty-tip {
  grid-column: 1 / -1;
  text-align: center;
  padding: 20px;
  font-size: var(--fs-body);
}

.join {
  display: flex;
  flex-direction: column;
  align-items: center;
}
.join-circle-wrap {
  padding: 8px 0 14px;
}
.join-circle {
  width: 120px;
  height: 120px;
  border-radius: 50%;
  border: 4px solid #fff;
  cursor: pointer;
  background: linear-gradient(135deg, #22c55e, #16a34a);
  color: #fff;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  box-shadow: 0 6px 18px var(--pt-success-bg);
  transition: transform var(--dur-fast) var(--ease-out-smooth);
}
.join-circle:hover:not(:disabled) {
  transform: scale(1.04);
}
.join-circle:disabled {
  cursor: wait;
  opacity: 0.85;
}
.jc-icon {
  font-size: var(--fs-hero);
}
.jc-text {
  font-size: var(--fs-md);
  font-weight: 700;
  margin-top: 4px;
}
.spinner {
  width: 28px;
  height: 28px;
  border: 3px solid rgba(255, 255, 255, 0.4);
  border-top-color: #fff;
  border-radius: 50%;
  animation: sp 0.8s linear infinite;
}
@keyframes sp {
  to {
    transform: rotate(360deg);
  }
}

.join-meta {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
}
.meta-row {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  padding: 6px 12px;
  border-radius: var(--radius-sm);
  background: rgba(30, 58, 138, 0.12);
  font-size: var(--fs-xs);
}

.result {
  padding: 12px;
  border-radius: var(--radius-sm);
  display: flex;
  gap: var(--sp-2);
  align-items: center;
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
.r-msg {
  font-size: var(--fs-body);
}

.records {
  padding: 14px;
  border-radius: var(--radius-sm);
}
.rec-title {
  font-weight: 700;
  margin-bottom: 8px;
}
.rec-row {
  display: flex;
  gap: var(--sp-2);
  align-items: center;
  padding: 5px 0;
  font-size: var(--fs-body);
  border-bottom: 1px solid var(--border-soft, rgba(255, 255, 255, 0.05));
}
.rec-prize {
  font-weight: 600;
}
.rec-prize.muted {
  color: var(--text-muted, #999);
  font-weight: 400;
}
.rec-amt {
  color: var(--pt-warn, #f59e0b);
}
.rec-time {
  margin-left: auto;
  font-size: var(--fs-2xs);
}
</style>
