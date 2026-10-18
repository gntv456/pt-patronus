<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, onMounted } from 'vue'
import { entertainmentApi, type GameStats, type PlayResult } from '@/api/entertainment'
import { entFail } from '@/utils/entFail'

const { t } = useI18n()
const props = defineProps<{ siteId: number }>()

const stats = ref<GameStats | null>(null)
const playing = ref(false)
const reveal = ref<PlayResult | null>(null)

async function refreshStats() {
  stats.value = await entertainmentApi.stats(props.siteId, 'magic_scratch').catch(entFail('stats', stats.value))
}

async function scratch() {
  if (playing.value) return
  playing.value = true
  reveal.value = null
  try {
    const res = await entertainmentApi.play(props.siteId, 'magic_scratch')
    reveal.value = res
    if (res.success && (res.amount ?? 0) > 0)
      ElMessage.success(t('msg1.won', { prize: res.prize || '', n: res.amount }))
    else if (res.success) ElMessage.info(res.message || t('msg1.noWin'))
    else ElMessage.warning(res.message || t('msg1.scratchFailed'))
    await refreshStats()
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || t('common.requestFailed'))
  } finally {
    playing.value = false
  }
}

function fmt(n: number) {
  return Math.round(n).toLocaleString('zh-CN')
}

onMounted(refreshStats)
</script>

<template>
  <div class="ms">
    <div class="banner">
      <span class="banner-title">{{ t('msg1.title') }}</span>
      <span class="banner-sub">{{ t('msg1.sub', { n: fmt(stats?.cost_per_play ?? 5000) }) }}</span>
    </div>

    <!-- 统计栏 -->
    <div class="stat-bar">
      <div class="stat-tile">
        <div class="stat-val">{{ stats ? fmt(stats.balance) : '--' }}</div>
        <div class="stat-lbl">{{ t('msg1.balance') }}</div>
      </div>
      <div class="stat-tile">
        <div class="stat-val">{{ stats ? `${stats.today_spins}/${stats.max_daily || '∞'}` : '--' }}</div>
        <div class="stat-lbl">{{ t('msg1.todayCount') }}</div>
      </div>
    </div>

    <!-- 刮卡 -->
    <div class="card-zone">
      <div class="scratch-card" :class="{ revealed: reveal, win: reveal?.success && (reveal?.amount ?? 0) > 0 }">
        <div v-if="!reveal" class="cover">
          <span class="cover-text">{{ t('msg1.area') }}</span>
          <span class="cover-hint">{{ t('msg1.hint') }}</span>
        </div>
        <div v-else class="prize-reveal">
          <div class="reveal-icon">{{ reveal.amount && reveal.amount > 0 ? '✨' : '·' }}</div>
          <div class="reveal-name">{{ reveal.prize || (reveal.success ? t('msg1.noWin') : t('common.failed')) }}</div>
          <div v-if="reveal.amount && reveal.amount > 0" class="reveal-amount">
            +{{ t('msg1.sparkN', { n: fmt(reveal.amount) }) }}
          </div>
          <div v-if="reveal.message" class="reveal-msg">{{ reveal.message }}</div>
        </div>
      </div>
    </div>

    <div class="actions">
      <button class="btn" :disabled="playing" @click="scratch">
        {{ playing ? t('msg1.scratching') : t('msg1.scratchOne') }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.ms {
  display: flex;
  flex-direction: column;
  gap: var(--sp-4);
  max-width: 480px;
  margin: 0 auto;
}
.banner {
  text-align: center;
  padding: 14px;
  border-radius: var(--radius-md);
  background: linear-gradient(135deg, #f7971e, #ff5858);
}
.banner-title {
  display: block;
  font-size: var(--fs-title);
  font-weight: 800;
  color: #fff;
  letter-spacing: 3px;
}
.banner-sub {
  display: block;
  margin-top: 3px;
  font-size: var(--fs-xs);
  color: rgba(255, 255, 255, 0.85);
}
.stat-bar {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: var(--sp-2);
}
.stat-tile {
  padding: 12px 8px;
  border-radius: var(--radius-sm);
  text-align: center;
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(247, 151, 30, 0.25);
}
.stat-val {
  font-size: var(--fs-title);
  font-weight: 800;
  color: #ffb74d;
}
.stat-lbl {
  margin-top: 2px;
  font-size: var(--fs-2xs);
  color: #9aa5b1;
}
.card-zone {
  display: flex;
  justify-content: center;
  padding: 8px 0;
}
.scratch-card {
  width: 240px;
  height: 150px;
  border-radius: var(--radius-md);
  position: relative;
  overflow: hidden;
  background: linear-gradient(135deg, #ffd180, #ff8a3d);
  border: 2px solid #ffb74d;
  box-shadow: 0 6px 18px rgba(0, 0, 0, 0.3);
  transition: all var(--dur-slow) var(--ease-out-smooth);
}
.scratch-card.win {
  animation: pulse 0.6s ease;
}
@keyframes pulse {
  0% {
    transform: scale(1);
  }
  50% {
    transform: scale(1.04);
  }
  100% {
    transform: scale(1);
  }
}
.cover {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--sp-1);
  background: repeating-linear-gradient(45deg, #c9a06a 0 8px, #e0c08a 8px 16px);
  color: #4a2f00;
}
.cover-text {
  font-size: var(--fs-hero);
  font-weight: 800;
  letter-spacing: 6px;
}
.cover-hint {
  font-size: var(--fs-2xs);
  opacity: 0.7;
}
.prize-reveal {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--sp-1);
  color: #4a2f00;
  padding: 8px;
  text-align: center;
}
.reveal-icon {
  font-size: var(--fs-hero);
}
.reveal-name {
  font-size: var(--fs-xl);
  font-weight: 800;
}
.reveal-amount {
  font-size: var(--fs-md);
  font-weight: 700;
}
.reveal-msg {
  margin-top: 4px;
  font-size: var(--fs-2xs);
  opacity: 0.8;
}
.actions {
  display: flex;
  justify-content: center;
}
.btn {
  flex: 1;
  padding: 12px;
  border-radius: var(--radius-sm);
  border: none;
  font-size: var(--fs-lg);
  font-weight: 700;
  cursor: pointer;
  color: #fff;
  background: linear-gradient(135deg, #f7971e, #ff5858);
  transition: all var(--dur-fast) var(--ease-out-smooth);
}
.btn:disabled {
  opacity: 0.5;
  cursor: progress;
}
.btn:hover:not(:disabled) {
  filter: brightness(1.08);
}
</style>
