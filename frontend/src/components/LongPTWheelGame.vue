<script setup lang="ts">
import { ref, computed, watch, onMounted, nextTick } from 'vue'
import { entertainmentApi, type GameStats, type PlayResult, type PrizeDef, type GameRecord } from '@/api/entertainment'

const props = defineProps<{ siteId: number }>()

const GAME = 'longpt_wheel'
const MULTI = 10

// 12 色循环（对齐 LongPT 原图明亮配色）
const colors = ['#FF6B6B', '#FFD93D', '#6BCB77', '#4D96FF', '#9D65C9', '#FF9F43',
  '#1ABC9C', '#E84393', '#FDCB6E', '#74B9FF', '#A29BFE', '#55EFC4']

const canvasRef = ref<HTMLCanvasElement | null>(null)
const stats = ref<GameStats | null>(null)
const prizes = ref<PrizeDef[]>([])
const records = ref<GameRecord[]>([])
const spinning = ref(false)
const result = ref<PlayResult | null>(null)
const rotation = ref(0) // 转盘累计旋转角度（deg）
const lastMode = ref<1 | typeof MULTI>(1)

const idToIndex = computed(() => {
  const m = new Map<number, number>()
  prizes.value.forEach((p, i) => m.set(p.id, i))
  return m
})

// 连抽结果聚合（按 prize_name 分组计数）
const multiSummary = computed(() => {
  const ps = result.value?.prizes ?? []
  const m = new Map<string, { name: string; n: number; type: string; win: boolean }>()
  for (const p of ps) {
    const key = p.prize_name || '未知'
    const e = m.get(key) ?? { name: key, n: 0, type: p.prize_type, win: p.is_win }
    e.n++
    m.set(key, e)
  }
  return [...m.values()].sort((a, b) => Number(b.win) - Number(a.win))
})

async function refreshStats() {
  stats.value = await entertainmentApi.stats(props.siteId, GAME).catch(() => stats.value)
}
async function refreshHistory() {
  records.value = await entertainmentApi.history(props.siteId, GAME).catch(() => [])
}

// 计算让第 index 个扇区转到指针（顶部 12 点）下所需的累计角度
function targetRotationForIndex(index: number): number {
  const n = prizes.value.length || 1
  const sector = 360 / n
  const center = index * sector + sector / 2
  const need = (360 - (center % 360)) % 360
  const cur = ((rotation.value % 360) + 360) % 360
  let delta = need - cur
  if (delta <= 0) delta += 360
  return rotation.value + 5 * 360 + delta
}

function animateToIndex(index: number) {
  return new Promise<void>((resolve) => {
    rotation.value = targetRotationForIndex(index)
    setTimeout(resolve, 4100) // 等 CSS transition（4s）结束
  })
}

function animateSpins(turns: number) {
  return new Promise<void>((resolve) => {
    rotation.value = rotation.value + turns * 360
    setTimeout(resolve, 4100)
  })
}

async function spin(count: 1 | typeof MULTI) {
  if (spinning.value) return
  spinning.value = true
  result.value = null
  lastMode.value = count
  try {
    const res = count === 1
      ? await entertainmentApi.play(props.siteId, GAME)
      : await entertainmentApi.playMulti(props.siteId, GAME, count)
    if (res.success) {
      if (count === 1) {
        await animateToIndex(idToIndex.value.get(res.prize_id ?? 0) ?? 0)
        result.value = res
        const win = res.prize && res.prize !== '未中奖'
        if (win) ElMessage.success(`🎉 ${res.prize}`)
        else ElMessage.info(res.message || '未中奖')
      } else {
        await animateSpins(3)
        result.value = res
        const won = (res.prizes ?? []).filter((p) => p.is_win).length
        ElMessage.success(won > 0 ? `${count} 连抽中奖 ${won} 次` : `${count} 连抽未中奖`)
      }
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

// Canvas 绘制转盘扇区 + 文字（静态；旋转靠外层 div CSS transition）
function drawWheel() {
  const cv = canvasRef.value
  if (!cv) return
  const ctx = cv.getContext('2d')
  if (!ctx) return
  const size = cv.width
  const cx = size / 2
  const cy = size / 2
  const R = size / 2 - 8
  ctx.clearRect(0, 0, size, size)
  const n = prizes.value.length || 1
  const sector = (2 * Math.PI) / n
  for (let i = 0; i < n; i++) {
    const a0 = -Math.PI / 2 + i * sector
    const a1 = a0 + sector
    ctx.beginPath()
    ctx.moveTo(cx, cy)
    ctx.arc(cx, cy, R, a0, a1)
    ctx.closePath()
    ctx.fillStyle = colors[i % colors.length]
    ctx.fill()
    ctx.strokeStyle = 'rgba(255,255,255,0.5)'
    ctx.lineWidth = 1
    ctx.stroke()
    // 文字沿径向
    ctx.save()
    ctx.translate(cx, cy)
    ctx.rotate(a0 + sector / 2)
    ctx.textAlign = 'right'
    ctx.fillStyle = '#3a2a1a'
    ctx.font = 'bold 15px sans-serif'
    ctx.fillText(prizes.value[i]?.name ?? '', R - 14, 5)
    ctx.restore()
  }
  // 外金边
  ctx.beginPath()
  ctx.arc(cx, cy, R, 0, 2 * Math.PI)
  ctx.lineWidth = 6
  ctx.strokeStyle = '#FFD93D'
  ctx.stroke()
}

watch(prizes, () => nextTick(drawWheel))

onMounted(async () => {
  prizes.value = await entertainmentApi.prizes(props.siteId, GAME).catch(() => [] as PrizeDef[])
  await Promise.all([refreshStats(), refreshHistory()])
  await nextTick(drawWheel)
})
</script>

<template>
  <div class="lpt-wheel">
    <div class="banner">
      <span class="banner-title">🎰 幸运大转盘</span>
      <span class="banner-sub">单次消耗 {{ Math.round(stats?.cost_per_play ?? 0) }} 魔力{{ stats?.discount ? ` · 连抽${stats.discount / 10}折` : '' }}</span>
    </div>

    <div class="stat-bar">
      <div class="stat-tile">
        <div class="stat-val">{{ stats ? Math.round(stats.balance) : '--' }}</div>
        <div class="stat-lbl">魔力余额</div>
      </div>
      <div class="stat-tile">
        <div class="stat-val">{{ stats?.discount ? stats.discount / 10 : '—' }}</div>
        <div class="stat-lbl">连抽折扣</div>
      </div>
    </div>

    <div class="wheel-wrap">
      <div class="wheel" :style="{ transform: `rotate(${rotation}deg)` }">
        <canvas ref="canvasRef" class="canvas" width="560" height="560" />
      </div>
      <div class="pointer" />
      <button class="hub" :disabled="spinning" @click="spin(1)">
        <span v-if="spinning" class="spinner" />
        <template v-else>
          <span class="hub-icon">🎲</span>
          <span class="hub-text">开始<br />抽奖</span>
        </template>
      </button>
    </div>

    <div class="actions">
      <el-button type="primary" :loading="spinning && lastMode === 1" @click="spin(1)">单抽（消耗{{ Math.round(stats?.cost_per_play ?? 0) }}魔力）</el-button>
      <el-button type="warning" :loading="spinning && lastMode === MULTI" @click="spin(MULTI)">{{ MULTI }} 连抽</el-button>
    </div>

    <!-- 结果 -->
    <div v-if="result" class="result" :class="result.success ? 'win' : 'lose'">
      <template v-if="lastMode === 1">
        <span class="badge">{{ result.prize && result.prize !== '未中奖' ? '🎉 中奖' : '💫 未中奖' }}</span>
        <span v-if="result.prize" class="r-prize">{{ result.prize }}</span>
        <span v-if="result.amount" class="r-amt">×{{ result.amount }}</span>
        <div v-if="result.message" class="r-msg text-muted">{{ result.message }}</div>
      </template>
      <template v-else>
        <span class="badge">{{ MULTI }} 连抽结果</span>
        <div class="multi-list">
          <span v-for="it in multiSummary" :key="it.name" class="multi-item" :class="{ nowin: !it.win }">
            {{ it.name }} ×{{ it.n }}
          </span>
        </div>
      </template>
    </div>

    <!-- 抽奖记录 -->
    <div v-if="records.length" class="glass records">
      <div class="rec-title">抽奖记录</div>
      <div v-for="(r, i) in records.slice(0, 8)" :key="i" class="rec-row">
        <span class="rec-prize" :class="{ muted: r.prize_name === '未中奖' }">{{ r.prize_name }}</span>
        <span v-if="r.amount" class="rec-amt">×{{ r.amount }}</span>
        <span class="rec-time text-muted">{{ r.created_at }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.lpt-wheel { display: flex; flex-direction: column; gap: 16px; max-width: 560px; margin: 0 auto; }
.banner { text-align: center; padding: 14px; border-radius: 14px; background: linear-gradient(135deg, #4D96FF, #9D65C9); }
.banner-title { display: block; font-size: 22px; font-weight: 800; color: #fff; letter-spacing: 2px; }
.banner-sub { display: block; margin-top: 3px; font-size: 12px; color: rgba(255, 255, 255, 0.8); }
.stat-bar { display: flex; gap: 12px; }
.stat-tile { flex: 1; text-align: center; padding: 12px; border-radius: 12px; background: var(--glass-bg, rgba(255,255,255,0.06)); }
.stat-val { font-size: 22px; font-weight: 800; color: var(--pt-primary, #6c5ce7); }
.stat-lbl { font-size: 12px; color: var(--text-muted, #999); margin-top: 2px; }

.wheel-wrap { position: relative; width: 100%; max-width: 420px; aspect-ratio: 1; margin: 0 auto; }
.wheel { width: 100%; height: 100%; transition: transform 4s cubic-bezier(0.17, 0.67, 0.21, 0.99); }
.canvas { width: 100%; height: 100%; display: block; }
.pointer {
  position: absolute; top: -6px; left: 50%; transform: translateX(-50%);
  width: 0; height: 0; border-left: 16px solid transparent; border-right: 16px solid transparent;
  border-top: 28px solid #ff3b3b; filter: drop-shadow(0 2px 3px rgba(0,0,0,0.3)); z-index: 3;
}
.hub {
  position: absolute; top: 50%; left: 50%; transform: translate(-50%, -50%);
  width: 26%; height: 26%; border-radius: 50%; border: 4px solid #fff;
  background: linear-gradient(135deg, #4D96FF, #2563eb); color: #fff; cursor: pointer;
  display: flex; flex-direction: column; align-items: center; justify-content: center; z-index: 2;
  box-shadow: 0 4px 14px rgba(37, 99, 235, 0.5);
}
.hub:disabled { cursor: wait; opacity: 0.85; }
.hub-icon { font-size: 22px; }
.hub-text { font-size: 12px; font-weight: 700; line-height: 1.2; margin-top: 2px; }
.spinner { width: 22px; height: 22px; border: 3px solid rgba(255,255,255,0.4); border-top-color: #fff; border-radius: 50%; animation: sp 0.8s linear infinite; }
@keyframes sp { to { transform: rotate(360deg); } }

.actions { display: flex; gap: 12px; justify-content: center; }
.result { padding: 12px; border-radius: 12px; }
.result.win { background: rgba(46, 204, 113, 0.12); }
.result.lose { background: rgba(231, 76, 60, 0.1); }
.badge { font-weight: 700; margin-right: 8px; }
.r-prize { color: var(--pt-primary, #6c5ce7); font-weight: 700; }
.r-amt { color: var(--pt-warn, #f59e0b); font-weight: 700; margin-left: 6px; }
.r-msg { margin-top: 6px; font-size: 12px; }
.multi-list { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 8px; }
.multi-item { padding: 3px 10px; border-radius: 8px; background: rgba(255, 215, 106, 0.2); font-weight: 600; font-size: 13px; }
.multi-item.nowin { background: rgba(255,255,255,0.08); color: var(--text-muted, #999); }

.records { padding: 14px; border-radius: 12px; }
.rec-title { font-weight: 700; margin-bottom: 8px; }
.rec-row { display: flex; gap: 10px; align-items: center; padding: 5px 0; font-size: 13px; border-bottom: 1px solid var(--border-soft, rgba(255,255,255,0.05)); }
.rec-prize { font-weight: 600; }
.rec-prize.muted { color: var(--text-muted, #999); font-weight: 400; }
.rec-amt { color: var(--pt-warn, #f59e0b); }
.rec-time { margin-left: auto; font-size: 11px; }
</style>
