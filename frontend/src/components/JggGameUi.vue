<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { entertainmentApi, type GameStats, type PlayResult, type GameRecord } from '@/api/entertainment'

const props = defineProps<{ siteId: number }>()

// 9 个奖品（源码 jgg/magic_grid_config.php 固定配置；id 1-8 占外圈，id9 归到 VIP1天格）。
interface Prize { id: number; name: string; icon: string; color: string }
const prizes: Prize[] = [
  { id: 1, name: '勋章', icon: '🎊', color: '#4CAF50' },
  { id: 2, name: '临时邀请码', icon: '⏱️', color: '#2196F3' },
  { id: 3, name: '1GB上传量', icon: '💾', color: '#9C27B0' },
  { id: 4, name: '10GB上传量', icon: '💾', color: '#FF9800' },
  { id: 5, name: '10GB下载量', icon: '📥', color: '#00BCD4' },
  { id: 6, name: 'VIP(1天)', icon: '👑', color: '#FF5722' },
  { id: 7, name: '邀请码', icon: '✉️', color: '#795548' },
  { id: 8, name: '谢谢参与', icon: '⭐', color: '#607D8B' },
  { id: 9, name: 'VIP(7天)', icon: '👑', color: '#FFD700' },
]
// 外圈顺时针（id 1-8 → 索引 0-7）；网格行主序布局，-1 = 中心按钮。
const clockwise = prizes.slice(0, 8) // id 1-8
const layout = [
  [0, 1, 2],
  [7, -1, 3],
  [6, 5, 4],
]
// 奖品 id → 外圈位置（0-7）。id9 归到 pos5（VIP1天格）。
function mapIdToPos(id: number): number {
  if (id >= 1 && id <= 8) return id - 1
  if (id === 9) return 5
  return 7
}
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

const stats = ref<GameStats | null>(null)
const spinning = ref(false)
const highlight = ref(-1) // 当前高亮的外圈索引；-1 无
const result = ref<PlayResult | null>(null)
// 连抽（10/20/50/100）与抽奖历史
const bulk = ref<PlayResult | null>(null)
const bulkCount = ref(0)
const bulkSpinning = ref(false)
const records = ref<GameRecord[]>([])

// 连抽结果按奖品名分组统计（等价于站点 prize_stats）。
const bulkStats = computed(() => {
  const map = new Map<string, number>()
  for (const p of bulk.value?.prizes ?? []) {
    map.set(p.prize_name, (map.get(p.prize_name) ?? 0) + 1)
  }
  return [...map.entries()]
    .map(([name, count]) => ({ name, count }))
    .sort((a, b) => b.count - a.count)
})

async function refreshStats() {
  stats.value = await entertainmentApi.stats(props.siteId, 'jgg').catch(() => stats.value)
}

async function refreshHistory() {
  records.value = await entertainmentApi.history(props.siteId, 'jgg').catch(() => records.value)
}

async function spin() {
  if (spinning.value || bulkSpinning.value) return
  spinning.value = true
  result.value = null
  bulk.value = null
  highlight.value = -1
  try {
    const res = await entertainmentApi.play(props.siteId, 'jgg')
    if (res.success) {
      // 转 3 圈 + 到目标格，逐步减速（对齐 Kotlin 节奏）。
      const target = mapIdToPos(res.prize_id || 0)
      const total = 3 * 8 + target
      let delay = 70
      for (let i = 0; i <= total; i++) {
        highlight.value = i % 8
        await sleep(delay)
        delay = Math.min(delay + 8, 280)
      }
      result.value = res
      const win = res.prize && res.prize !== '谢谢参与'
      if (win) ElMessage.success(`🎉 获得 ${res.prize}`)
      else ElMessage.info(res.message || '谢谢参与')
    } else {
      result.value = res
      ElMessage.warning(res.message || '抽奖失败')
    }
    await Promise.all([refreshStats(), refreshHistory()])
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '请求失败')
  } finally {
    spinning.value = false
  }
}

// 连抽：站点 /jgg/magic_grid_bulk.php 一次返回 N 份结果，无单一中奖格，故仅做装饰性流转反馈。
async function drawMulti(count: number) {
  if (spinning.value || bulkSpinning.value) return
  bulkSpinning.value = true
  bulk.value = null
  result.value = null
  highlight.value = -1
  try {
    let delay = 55
    for (let i = 0; i < 18; i++) {
      highlight.value = i % 8
      await sleep(delay)
    }
    highlight.value = -1
    const res = await entertainmentApi.playMulti(props.siteId, 'jgg', count)
    if (res.success) {
      bulk.value = res
      bulkCount.value = count
      const wins = (res.prizes || []).filter((p) => p.is_win).length
      ElMessage.success(`🎉 ${count}连抽完成，中奖 ${wins}/${count}`)
    } else {
      ElMessage.warning(res.message || '连抽失败')
    }
    await Promise.all([refreshStats(), refreshHistory()])
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '请求失败')
  } finally {
    bulkSpinning.value = false
  }
}

onMounted(() => {
  refreshStats()
  refreshHistory()
})
</script>

<template>
  <div class="jgg">
    <!-- 标题 -->
    <div class="banner">
      <span class="banner-title">九宫格抽奖</span>
      <span class="banner-sub">单次消耗 {{ Math.round(stats?.cost_per_play ?? 3000) }} 火花</span>
    </div>

    <!-- 统计栏 -->
    <div class="stat-bar">
      <div class="stat-tile">
        <div class="stat-val">{{ stats ? `${stats.today_spins}/${stats.max_daily || '∞'}` : '--' }}</div>
        <div class="stat-lbl">今日次数</div>
      </div>
      <div class="stat-tile">
        <div class="stat-val">{{ result ? Math.round(result.balance ?? 0) : (stats ? '—' : '--') }}</div>
        <div class="stat-lbl">{{ result ? '当前火花' : '火花余额' }}</div>
      </div>
    </div>

    <!-- 3×3 九宫格 -->
    <div class="grid">
      <template v-for="(row, ri) in layout" :key="ri">
        <template v-for="(pos, ci) in row" :key="ri + '-' + ci">
          <!-- 中心按钮 -->
          <button
            v-if="pos === -1"
            class="cell center"
            :disabled="spinning || bulkSpinning"
            @click="spin"
          >
            <span v-if="spinning || bulkSpinning" class="spinner" />
            <template v-else>
              <span class="center-icon">⚡</span>
              <span class="center-text">开始<br />抽奖</span>
            </template>
          </button>
          <!-- 奖品格 -->
          <div
            v-else
            class="cell prize"
            :class="{ lit: highlight === pos, win: result && highlight === pos && !spinning }"
            :style="highlight === pos && pos < clockwise.length ? { '--pc': clockwise[pos].color } : {}"
          >
            <span class="p-icon">{{ clockwise[pos]?.icon }}</span>
            <span class="p-name">{{ clockwise[pos]?.name }}</span>
          </div>
        </template>
      </template>
    </div>

    <!-- 结果 -->
    <div v-if="result" class="result" :class="result.success ? 'win' : 'lose'">
      <span class="badge">{{ result.success ? '🎉 中奖' : '✗ 失败' }}</span>
      <span v-if="result.prize" class="r-prize">{{ result.prize }}</span>
      <span v-if="result.amount" class="r-amt">×{{ result.amount }}</span>
      <div v-if="result.message" class="r-msg text-muted">{{ result.message }}</div>
    </div>

    <!-- 连抽（站点 /jgg/magic_grid_bulk.php 仅接受 10/20/50/100） -->
    <div class="multi">
      <button class="mbtn" :disabled="spinning || bulkSpinning" @click="drawMulti(10)"><span>🎀</span> 10连抽</button>
      <button class="mbtn" :disabled="spinning || bulkSpinning" @click="drawMulti(20)"><span>💖</span> 20连抽</button>
      <button class="mbtn" :disabled="spinning || bulkSpinning" @click="drawMulti(50)"><span>✨</span> 50连抽</button>
      <button class="mbtn" :disabled="spinning || bulkSpinning" @click="drawMulti(100)"><span>🌸</span> 100连抽</button>
    </div>

    <!-- 连抽结果（按奖品名分组统计） -->
    <div v-if="bulk" class="bulk">
      <div class="bulk-head">🎉 {{ bulkCount }} 连抽完成 · 中奖 {{ (bulk.prizes || []).filter((p) => p.is_win).length }}/{{ bulkCount }}</div>
      <div v-for="(s, i) in bulkStats" :key="i" class="bulk-row" :class="{ muted: s.name === '谢谢参与' }">
        <span>{{ s.name }}</span>
        <span class="bulk-cnt">×{{ s.count }}</span>
      </div>
      <div class="bulk-foot text-muted">
        消耗 {{ bulkCount * Math.round(stats?.cost_per_play ?? 3000) }} 火花 · 余额 {{ Math.round(bulk.balance ?? 0) }}
      </div>
    </div>

    <!-- 抽奖历史 -->
    <section v-if="records.length" class="history">
      <div class="hist-title">📜 抽奖历史</div>
      <div v-for="(r, i) in records.slice(0, 20)" :key="i" class="hist-row">
        <span :class="{ muted: r.prize_name === '谢谢参与' }">{{ r.prize_name }}</span>
        <span class="hist-cost">-{{ Math.round(r.cost) }}</span>
        <span class="hist-time text-muted">{{ r.created_at }}</span>
      </div>
    </section>
  </div>
</template>

<style scoped>
.jgg {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 480px;
  margin: 0 auto;
}
.banner {
  text-align: center;
  padding: 14px;
  border-radius: 14px;
  background: linear-gradient(135deg, #6c5ce7, #8e44ad);
  border: 1px solid rgba(255, 215, 106, 0.4);
}
.banner-title {
  display: block;
  font-size: 22px;
  font-weight: 800;
  color: #ffd76a;
  letter-spacing: 3px;
}
.banner-sub {
  display: block;
  margin-top: 3px;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.75);
}
.stat-bar {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 10px;
}
.stat-tile {
  padding: 10px 8px;
  border-radius: 12px;
  text-align: center;
  background: rgba(255, 255, 255, 0.7);
  border: 1px solid rgba(255, 255, 255, 0.9);
  box-shadow: 0 2px 8px rgba(31, 38, 135, 0.08);
}
.stat-val {
  font-size: 20px;
  font-weight: 800;
  color: #ffd76a;
}
.stat-lbl {
  margin-top: 2px;
  font-size: 11px;
  color: #4b5563;
}
/* 九宫格 */
.grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 8px;
}
.cell {
  aspect-ratio: 1 / 1;
  border-radius: 12px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: 6px;
  text-align: center;
  transition: all 0.12s ease;
}
.prize {
  background: rgba(255, 255, 255, 0.72);
  border: 1px solid rgba(255, 255, 255, 0.9);
  box-shadow: 0 2px 8px rgba(31, 38, 135, 0.08);
}
.prize.lit {
  opacity: 1;
  background: var(--pc, #6c5ce7);
  border-color: #ffd76a;
  transform: scale(1.04);
  box-shadow: 0 0 16px var(--pc, #6c5ce7);
}
.prize.win {
  animation: winpop 0.5s ease;
}
@keyframes winpop {
  0% { transform: scale(1.04); }
  50% { transform: scale(1.12); }
  100% { transform: scale(1.06); }
}
.p-icon {
  font-size: 26px;
}
.p-name {
  font-size: 11px;
  color: #fff;
  font-weight: 600;
  line-height: 1.2;
}
.prize:not(.lit) .p-name {
  color: #374151;
}
/* 中心按钮 */
.center {
  border: none;
  cursor: pointer;
  background: linear-gradient(135deg, #6c5ce7, #4285f4);
  color: #fff;
  box-shadow: 0 4px 14px rgba(108, 92, 231, 0.4);
}
.center:hover:not(:disabled) {
  filter: brightness(1.1);
}
.center:disabled {
  cursor: progress;
}
.center-icon {
  font-size: 26px;
}
.center-text {
  font-size: 13px;
  font-weight: 800;
  letter-spacing: 1px;
}
.spinner {
  width: 28px;
  height: 28px;
  border: 3px solid rgba(255, 255, 255, 0.35);
  border-top-color: #fff;
  border-radius: 50%;
  animation: rot 0.8s linear infinite;
}
@keyframes rot {
  to { transform: rotate(360deg); }
}
/* 结果 */
.result {
  padding: 12px;
  border-radius: 12px;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  font-size: 13px;
}
.result.win { background: rgba(46, 204, 113, 0.12); }
.result.lose { background: rgba(231, 76, 60, 0.1); }
.badge { font-weight: 800; }
.r-prize { color: #ffd76a; font-weight: 700; }
.r-amt { color: #f59e0b; font-weight: 700; }
.r-msg { width: 100%; }

/* 连抽按钮 */
.multi {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 8px;
}
.mbtn {
  padding: 10px 6px;
  border-radius: 12px;
  border: 1px solid rgba(108, 92, 231, 0.4);
  background: rgba(255, 255, 255, 0.72);
  color: #4b5563;
  font-size: 13px;
  font-weight: 700;
  cursor: pointer;
  transition: all 0.12s ease;
  box-shadow: 0 2px 8px rgba(31, 38, 135, 0.08);
}
.mbtn span {
  margin-right: 4px;
  font-size: 15px;
}
.mbtn:hover:not(:disabled) {
  background: linear-gradient(135deg, #6c5ce7, #4285f4);
  color: #fff;
  border-color: transparent;
  transform: translateY(-1px);
}
.mbtn:disabled {
  opacity: 0.5;
  cursor: progress;
}

/* 连抽结果 */
.bulk {
  padding: 12px;
  border-radius: 12px;
  background: rgba(46, 204, 113, 0.1);
  border: 1px solid rgba(46, 204, 113, 0.3);
}
.bulk-head { font-weight: 800; font-size: 13px; margin-bottom: 8px; color: #2ecc71; }
.bulk-row {
  display: flex;
  justify-content: space-between;
  padding: 3px 2px;
  font-size: 13px;
}
.bulk-row.muted { color: #9aa5b1; }
.bulk-cnt { font-weight: 800; color: #ffd76a; }
.bulk-foot { margin-top: 8px; font-size: 11px; }

/* 抽奖历史 */
.history {
  padding: 12px;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.62);
  border: 1px solid rgba(255, 255, 255, 0.9);
  box-shadow: 0 2px 8px rgba(31, 38, 135, 0.06);
}
.hist-title { font-weight: 800; font-size: 13px; margin-bottom: 8px; }
.hist-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 2px;
  font-size: 12px;
  border-bottom: 1px dashed rgba(0, 0, 0, 0.06);
}
.hist-row:last-child { border-bottom: none; }
.hist-row .muted { color: #9aa5b1; }
.hist-cost { margin-left: auto; color: #e74c3c; font-weight: 700; }
.hist-time { font-size: 11px; white-space: nowrap; }

/* 暗色主题：还原深色玻璃风格（亮色液态玻璃为默认基准） */
:root.dark .stat-tile {
  background: rgba(255, 255, 255, 0.06);
  border-color: rgba(255, 215, 106, 0.18);
  box-shadow: none;
}
:root.dark .stat-lbl {
  color: #9aa5b1;
}
:root.dark .prize {
  background: rgba(255, 255, 255, 0.05);
  border-color: rgba(255, 255, 255, 0.08);
  box-shadow: none;
  opacity: 0.55;
}
:root.dark .p-name {
  color: #fff;
}
:root.dark .prize:not(.lit) .p-name {
  color: #cdd6e0;
}
:root.dark .mbtn {
  background: rgba(255, 255, 255, 0.05);
  border-color: rgba(255, 215, 106, 0.18);
  color: #cdd6e0;
  box-shadow: none;
}
:root.dark .bulk {
  background: rgba(46, 204, 113, 0.08);
  border-color: rgba(46, 204, 113, 0.22);
}
:root.dark .history {
  background: rgba(255, 255, 255, 0.04);
  border-color: rgba(255, 255, 255, 0.08);
  box-shadow: none;
}
:root.dark .hist-row {
  border-bottom-color: rgba(255, 255, 255, 0.06);
}
</style>
