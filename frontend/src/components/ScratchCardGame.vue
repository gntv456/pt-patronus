<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted } from 'vue'
import { entertainmentApi, type GameStats, type BuyOption, type PrizeDef, type GameRecord, type PlayResult } from '@/api/entertainment'

const props = defineProps<{ siteId: number }>()

const stats = ref<GameStats | null>(null)
const options = ref<BuyOption[]>([])
const prizes = ref<PrizeDef[]>([])
const history = ref<GameRecord[]>([])

const playing = ref(false)
const buying = ref(false)
const autoPlaying = ref(false)
const autoCount = ref(0)
const reveal = ref<PlayResult | null>(null) // 当前刮开的卡片结果
let autoTimer: ReturnType<typeof setInterval> | null = null

async function loadAll() {
  try {
    const [st, opt, pz, hi] = await Promise.all([
      entertainmentApi.stats(props.siteId, 'cspt_scratch').catch(() => null),
      entertainmentApi.buyOptions(props.siteId, 'cspt_scratch').catch(() => []),
      entertainmentApi.prizes(props.siteId, 'cspt_scratch').catch(() => []),
      entertainmentApi.history(props.siteId, 'cspt_scratch').catch(() => []),
    ])
    stats.value = st
    options.value = opt
    prizes.value = pz
    history.value = hi
  } catch {
    /* 静默 */
  }
}

async function refreshStats() {
  stats.value = await entertainmentApi.stats(props.siteId, 'cspt_scratch').catch(() => stats.value)
}

async function play() {
  if (playing.value) return
  playing.value = true
  reveal.value = null
  try {
    const res = await entertainmentApi.play(props.siteId, 'cspt_scratch')
    reveal.value = res
    if (res.success) ElMessage.success(res.prize ? `获得 ${res.prize}` : '刮卡成功')
    else ElMessage.warning(res.message || '刮卡失败')
    await refreshStats()
    history.value = await entertainmentApi.history(props.siteId, 'cspt_scratch').catch(() => history.value)
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '请求失败')
  } finally {
    playing.value = false
  }
}

function toggleAuto() {
  if (autoPlaying.value) stopAuto()
  else startAuto()
}
function startAuto() {
  if (autoPlaying.value) return
  autoPlaying.value = true
  autoCount.value = 0
  autoTimer = setInterval(async () => {
    if ((stats.value?.card_number ?? 0) <= 0 || playing.value) {
      stopAuto()
      return
    }
    await play()
    autoCount.value++
  }, 700)
}
function stopAuto() {
  autoPlaying.value = false
  if (autoTimer) clearInterval(autoTimer)
  autoTimer = null
}

async function buy(opt: BuyOption) {
  if (buying.value) return
  buying.value = true
  try {
    const res = await entertainmentApi.buy(props.siteId, 'cspt_scratch', opt.type)
    if (res.success) ElMessage.success(res.message || `购买 ${opt.buy_count} 张`)
    else ElMessage.warning(res.message || '购买失败')
    await refreshStats()
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '购买失败')
  } finally {
    buying.value = false
  }
}

function fmt(n: number) {
  return Math.round(n).toLocaleString('zh-CN')
}

onMounted(loadAll)
onUnmounted(stopAuto)
watch(() => props.siteId, loadAll)
</script>

<template>
  <div class="scratch">
    <!-- 标题 -->
    <div class="banner">
      <span class="banner-title">富豪刮刮乐</span>
      <span class="banner-sub">每次消耗 1 张刮刮卡 · 单卡 {{ fmt(stats?.cost_per_play ?? 2000) }} 金元宝</span>
    </div>

    <!-- 统计栏 -->
    <div class="stat-bar">
      <div class="stat-tile">
        <div class="stat-val">{{ stats?.card_number ?? '--' }}</div>
        <div class="stat-lbl">剩余刮刮卡</div>
      </div>
      <div class="stat-tile">
        <div class="stat-val">{{ stats ? fmt(stats.balance) : '--' }}</div>
        <div class="stat-lbl">金元宝余额</div>
      </div>
      <div class="stat-tile">
        <div class="stat-val">{{ autoPlaying ? autoCount : (history.length || 0) }}</div>
        <div class="stat-lbl">{{ autoPlaying ? '本轮已刮' : '近期记录' }}</div>
      </div>
    </div>

    <!-- 奖品展示 -->
    <div v-if="prizes.length" class="prizes">
      <div class="section-label">奖品展示</div>
      <div class="prize-row">
        <div v-for="p in prizes" :key="p.id" class="prize-chip">
          <span class="prize-icon">{{ p.name.slice(0, 1) }}</span>
          <span class="prize-name">{{ p.name }}</span>
        </div>
      </div>
    </div>

    <!-- 刮卡区 -->
    <div class="card-zone">
      <div class="scratch-card" :class="{ revealed: reveal, win: reveal?.success && (reveal?.amount ?? 0) > 0 }">
        <div v-if="!reveal" class="cover">
          <span class="cover-text">刮奖区</span>
          <span class="cover-hint">点击下方按钮刮开</span>
        </div>
        <div v-else class="prize-reveal">
          <div class="reveal-icon">{{ reveal.amount && reveal.amount > 0 ? '🎊' : '·' }}</div>
          <div class="reveal-name">{{ reveal.prize || (reveal.success ? '谢谢参与' : '未中奖') }}</div>
          <div v-if="reveal.amount && reveal.amount > 0" class="reveal-amount">×{{ fmt(reveal.amount) }}</div>
          <div v-if="reveal.message" class="reveal-msg">{{ reveal.message }}</div>
        </div>
      </div>
    </div>

    <!-- 操作按钮 -->
    <div class="actions">
      <button class="btn btn-gold" :disabled="playing || buying || (stats?.card_number ?? 0) <= 0" @click="play">
        {{ playing ? '刮卡中…' : '刮一张' }}
      </button>
      <button class="btn btn-auto" :class="{ active: autoPlaying }" @click="toggleAuto">
        {{ autoPlaying ? `停止 (${autoCount})` : '自动刮卡' }}
      </button>
    </div>

    <!-- 购卡 -->
    <div v-if="options.length" class="buy">
      <div class="section-label">购买刮刮卡</div>
      <div class="buy-row">
        <button
          v-for="opt in options"
          :key="opt.type"
          class="buy-btn"
          :disabled="buying"
          @click="buy(opt)"
        >
          <span class="buy-title">{{ opt.gift_count > 0 ? `买${opt.buy_count}送${opt.gift_count}` : `${opt.buy_count}张` }}</span>
          <span class="buy-price">{{ fmt(opt.buy_count * opt.unit_price) }} 元宝</span>
        </button>
      </div>
    </div>

    <!-- 抽奖记录 -->
    <div v-if="history.length" class="history">
      <div class="section-label">抽奖记录</div>
      <div class="hist-list">
        <div v-for="(r, i) in history.slice(0, 12)" :key="i" class="hist-item">
          <span class="hist-prize" :class="{ miss: !r.amount }">{{ r.prize_name || '谢谢参与' }}</span>
          <span v-if="r.amount > 0" class="hist-amt">×{{ fmt(r.amount) }}</span>
          <span class="hist-time">{{ r.created_at }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.scratch {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 560px;
  margin: 0 auto;
}
/* 古风横幅：金玉配色 */
.banner {
  text-align: center;
  padding: 16px;
  border-radius: 14px;
  background: linear-gradient(135deg, #1f6f4a 0%, #2e8b57 50%, #3a7d44 100%);
  border: 1px solid rgba(255, 200, 80, 0.5);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.15);
}
.banner-title {
  display: block;
  font-size: 24px;
  font-weight: 800;
  color: #ffd76a;
  letter-spacing: 4px;
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.4);
}
.banner-sub {
  display: block;
  margin-top: 4px;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.75);
}
/* 统计栏 */
.stat-bar {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 10px;
}
.stat-tile {
  padding: 12px 8px;
  border-radius: 12px;
  text-align: center;
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(255, 215, 106, 0.22);
}
.stat-val {
  font-size: 22px;
  font-weight: 800;
  color: #ffd76a;
}
.stat-lbl {
  margin-top: 2px;
  font-size: 11px;
  color: #9aa5b1;
}
/* 区块标题 */
.section-label {
  font-size: 13px;
  font-weight: 700;
  color: #ffd76a;
  margin-bottom: 8px;
}
/* 奖品展示 */
.prize-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.prize-chip {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 8px 10px;
  border-radius: 10px;
  background: rgba(255, 215, 106, 0.08);
  border: 1px solid rgba(255, 215, 106, 0.18);
  min-width: 64px;
}
.prize-icon {
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: linear-gradient(135deg, #ffd76a, #f59e0b);
  color: #4a2f00;
  font-weight: 800;
  font-size: 15px;
}
.prize-name {
  font-size: 11px;
  color: #cdd6e0;
}
/* 刮卡 */
.card-zone {
  display: flex;
  justify-content: center;
  padding: 8px 0;
}
.scratch-card {
  width: 240px;
  height: 150px;
  border-radius: 14px;
  position: relative;
  overflow: hidden;
  background: linear-gradient(135deg, #f4d67a, #e89b3c);
  border: 2px solid #ffd76a;
  box-shadow: 0 6px 18px rgba(0, 0, 0, 0.3);
  transition: all 0.3s ease;
}
.scratch-card.win {
  background: linear-gradient(135deg, #ffd76a, #f97316);
  animation: pulse 0.6s ease;
}
@keyframes pulse {
  0% { transform: scale(1); }
  50% { transform: scale(1.04); }
  100% { transform: scale(1); }
}
.cover {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  background: repeating-linear-gradient(45deg, #b8b8b8 0 8px, #cfcfcf 8px 16px);
  color: #6b4a00;
}
.cover-text {
  font-size: 28px;
  font-weight: 800;
  letter-spacing: 6px;
}
.cover-hint {
  font-size: 11px;
  opacity: 0.7;
}
.prize-reveal {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 2px;
  color: #4a2f00;
  padding: 8px;
  text-align: center;
}
.reveal-icon {
  font-size: 30px;
}
.reveal-name {
  font-size: 18px;
  font-weight: 800;
}
.reveal-amount {
  font-size: 14px;
  font-weight: 700;
}
.reveal-msg {
  margin-top: 4px;
  font-size: 11px;
  opacity: 0.8;
}
/* 按钮 */
.actions {
  display: flex;
  gap: 10px;
  justify-content: center;
}
.btn {
  flex: 1;
  padding: 12px;
  border-radius: 12px;
  border: none;
  font-size: 15px;
  font-weight: 700;
  cursor: pointer;
  transition: all 0.15s ease;
  color: #fff;
}
.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.btn-gold {
  background: linear-gradient(135deg, #ffd76a, #f59e0b);
  color: #4a2f00;
}
.btn-gold:hover:not(:disabled) {
  filter: brightness(1.08);
}
.btn-auto {
  background: linear-gradient(135deg, #1f6f4a, #2e8b57);
  border: 1px solid rgba(255, 215, 106, 0.4);
}
.btn-auto.active {
  background: linear-gradient(135deg, #c0392b, #e74c3c);
}
/* 购卡 */
.buy-row {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 8px;
}
.buy-btn {
  padding: 10px 6px;
  border-radius: 10px;
  border: 1px solid rgba(255, 215, 106, 0.3);
  background: rgba(255, 255, 255, 0.05);
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 2px;
  align-items: center;
  transition: all 0.15s ease;
}
.buy-btn:hover:not(:disabled) {
  background: rgba(255, 215, 106, 0.12);
}
.buy-btn:disabled {
  opacity: 0.5;
}
.buy-title {
  font-size: 13px;
  font-weight: 700;
  color: #ffd76a;
}
.buy-price {
  font-size: 10px;
  color: #9aa5b1;
}
/* 历史 */
.hist-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 220px;
  overflow-y: auto;
}
.hist-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 10px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.04);
  font-size: 12px;
}
.hist-prize {
  flex: 1;
  color: #cdd6e0;
}
.hist-prize.miss {
  color: #6b7280;
}
.hist-amt {
  color: #ffd76a;
  font-weight: 700;
}
.hist-time {
  color: #6b7280;
  font-size: 10px;
}
</style>
