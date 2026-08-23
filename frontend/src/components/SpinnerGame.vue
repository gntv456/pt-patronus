<script setup lang="ts">
import { ref, computed, watch, onMounted, nextTick } from 'vue'
import { entertainmentApi, type GameStats, type PrizeDef, type GameRecord, type PlayResult } from '@/api/entertainment'

// 通用抽奖组件（批1）：覆盖多站幸运转盘/老虎机/抽奖。按 game 调 play/playMulti/stats/prizes/history。
// 关键：永远画出可见转盘——有 prizes 画真扇区，没有画 8 格装饰扇区，保证所有站点都看得到转盘。
const props = defineProps<{ siteId: number; game: string; name: string }>()

// 12 色循环（对齐 LongPT 明亮配色）
const colors = ['#FF6B6B', '#FFD93D', '#6BCB77', '#4D96FF', '#9D65C9', '#FF9F43',
  '#1ABC9C', '#E84393', '#FDCB6E', '#74B9FF', '#A29BFE', '#55EFC4']
// 无奖品数据时的装饰扇区（保证转盘永远有内容）
const decorSegments = ['🎁', '🪙', '✨', '🎫', '🍀', '💎', '👑', '🎊']

const canvasRef = ref<HTMLCanvasElement | null>(null)
const stats = ref<GameStats | null>(null)
const prizes = ref<PrizeDef[]>([])
const records = ref<GameRecord[]>([])
const playing = ref(false)
const last = ref<PlayResult | null>(null)
const rotation = ref(0)

// 转盘扇区：有真奖品用奖品名，否则用装饰 emoji
const segments = computed(() =>
  prizes.value.length
    ? prizes.value.map((p) => p.name || '奖品')
    : decorSegments.slice(),
)

// 连抽结果按奖品名分组计数（对齐躺平站点「88 魔力 ×12」展示）
const groupedPrizes = computed(() => {
  const ps = last.value?.prizes ?? []
  if (!ps.length) return []
  const m = new Map<string, { name: string; n: number; win: boolean }>()
  for (const p of ps) {
    const key = p.prize_name || '未知'
    const e = m.get(key) ?? { name: key, n: 0, win: p.is_win }
    e.n++
    if (p.is_win) e.win = true
    m.set(key, e)
  }
  return [...m.values()].sort((a, b) => Number(b.win) - Number(a.win))
})

// hero 余额：stats 缺失时用最近一次结果余额兜底（部分站点 Stats 抠不到）
const heroBalance = computed(() => {
  const b = stats.value?.balance ?? last.value?.balance
  return b != null && b > 0 ? b.toLocaleString() : '—'
})

async function refreshStats() {
  stats.value = await entertainmentApi.stats(props.siteId, props.game).catch(() => stats.value)
}
async function refreshHistory() {
  records.value = await entertainmentApi.history(props.siteId, props.game).catch(() => [])
}

async function draw(count: number) {
  if (playing.value) return
  playing.value = true
  last.value = null
  try {
    const res = count === 1
      ? await entertainmentApi.play(props.siteId, props.game)
      : await entertainmentApi.playMulti(props.siteId, props.game, count)
    // 装饰旋转：5-7 圈 + 随机角度（不强求指针精确落点——各站 prize_id 不可靠，结果在结果条揭示）
    await animateSpins(5 + Math.floor(Math.random() * 3))
    last.value = res
    if (res.success) {
      // 连抽：按分组汇总展示（如「88 魔力 ×8, 谢谢参与 ×2」）；单抽：直接奖品名
      const groups = groupedPrizes.value
      if (groups.length > 1 || (groups.length === 1 && groups[0].n > 1)) {
        const wins = groups.filter((g) => g.win)
        const summary = groups.map((g) => `${g.name} ×${g.n}`).join('，')
        ElMessage.success(wins.length ? `🎉 ${summary}` : summary)
      } else {
        ElMessage.success(res.prize ? `🎉 ${res.prize}` : (res.message || '完成'))
      }
    } else {
      ElMessage.warning(res.message || '抽奖失败')
    }
    await Promise.all([refreshStats(), refreshHistory()])
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '请求失败')
  } finally {
    playing.value = false
  }
}

function animateSpins(turns: number) {
  return new Promise<void>((resolve) => {
    rotation.value = rotation.value + turns * 360 + Math.floor(Math.random() * 360)
    setTimeout(resolve, 4100) // 等 CSS transition（4s）结束
  })
}

// Canvas 绘制扇区 + 文字（静态；旋转靠外层 div CSS transition）
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
  const segs = segments.value
  const n = segs.length || 1
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
    ctx.save()
    ctx.translate(cx, cy)
    ctx.rotate(a0 + sector / 2)
    ctx.textAlign = 'right'
    ctx.fillStyle = '#3a2a1a'
    ctx.font = prizes.value.length ? 'bold 15px sans-serif' : 'bold 30px sans-serif'
    ctx.fillText(segs[i], R - 14, prizes.value.length ? 5 : 12)
    ctx.restore()
  }
  // 外金边
  ctx.beginPath()
  ctx.arc(cx, cy, R, 0, 2 * Math.PI)
  ctx.lineWidth = 6
  ctx.strokeStyle = '#FFD93D'
  ctx.stroke()
}

watch(segments, () => nextTick(drawWheel))

onMounted(async () => {
  prizes.value = await entertainmentApi.prizes(props.siteId, props.game).catch(() => [] as PrizeDef[])
  await Promise.all([refreshStats(), refreshHistory()])
  await nextTick(drawWheel)
})
</script>

<template>
  <div class="spinner">
    <div class="hero">
      <div class="hero-title">🎰 {{ name }}</div>
      <div class="balance">余额 <b>{{ heroBalance }}</b></div>
    </div>

    <div class="wheel-wrap">
      <div class="wheel" :style="{ transform: `rotate(${rotation}deg)` }">
        <canvas ref="canvasRef" class="canvas" width="560" height="560" />
      </div>
      <div class="pointer" />
      <button class="hub" :disabled="playing" @click="draw(1)">
        <span v-if="playing" class="spin-icon" />
        <template v-else>
          <span class="hub-emoji">🎲</span>
          <span class="hub-text">开始<br />抽奖</span>
        </template>
      </button>
    </div>

    <section class="glass section ops">
      <button class="op primary" :disabled="playing" @click="draw(1)">{{ playing ? '抽奖中…' : '立即抽奖' }}</button>
      <button class="op" :disabled="playing" @click="draw(10)">连抽 10 次</button>
      <button class="op" :disabled="playing" @click="draw(20)">连抽 20 次</button>
      <button class="op" :disabled="playing" @click="draw(50)">连抽 50 次</button>
    </section>

    <div v-if="last" class="result" :class="last.success ? 'win' : 'lose'">
      <span class="badge">{{ last.success ? '🎉' : '✗' }}</span>
      <div class="r-body">
        <template v-if="groupedPrizes.length">
          <div class="r-list">
            <span v-for="g in groupedPrizes" :key="g.name" class="r-pill" :class="{ nowin: !g.win }">
              {{ g.name }} ×{{ g.n }}
            </span>
          </div>
        </template>
        <template v-else>
          <span v-if="last.prize" class="r-prize">{{ last.prize }}</span>
        </template>
        <span v-if="last.balance" class="r-bal">余额 {{ last.balance.toLocaleString() }}</span>
        <div v-if="last.message" class="r-msg text-muted">{{ last.message }}</div>
      </div>
    </div>

    <section v-if="prizes.length" class="glass section">
      <div class="sec-title">奖品</div>
      <div class="prize-strip">
        <div v-for="p in prizes" :key="p.id" class="prize-card">{{ p.name }}</div>
      </div>
    </section>

    <section v-if="records.length" class="glass section">
      <div class="sec-title">抽奖记录</div>
      <div v-for="(r, i) in records.slice(0, 10)" :key="i" class="rec-row">
        <span :class="{ muted: r.prize_name === '未中奖' }">{{ r.prize_name }}</span>
        <span v-if="r.amount" class="rec-amt">×{{ r.amount }}</span>
        <span class="text-muted rec-time">{{ r.created_at }}</span>
      </div>
    </section>
  </div>
</template>

<style scoped>
.spinner { display: flex; flex-direction: column; gap: var(--sp-4); max-width: 640px; margin: 0 auto; }
.hero { padding: 18px; border-radius: var(--radius-md); background: linear-gradient(135deg, #6366f1, #8b5cf6); text-align: center; }
.hero-title { font-size: var(--fs-title); font-weight: 800; color: #fff; }
.balance { margin-top: 4px; font-size: var(--fs-body); color: rgba(255,255,255,0.92); }
.balance b { color: #fde68a; font-size: var(--fs-lg); }

.wheel-wrap { position: relative; width: 100%; max-width: 360px; aspect-ratio: 1; margin: 0 auto; }
.wheel { width: 100%; height: 100%; transition: transform 4s var(--ease-out-smooth); }
.canvas { width: 100%; height: 100%; display: block; }
.pointer {
  position: absolute; top: -6px; left: 50%; transform: translateX(-50%);
  width: 0; height: 0; border-left: 16px solid transparent; border-right: 16px solid transparent;
  border-top: 28px solid #ff3b3b; filter: drop-shadow(0 2px 3px rgba(0,0,0,0.3)); z-index: 3;
}
.hub {
  position: absolute; top: 50%; left: 50%; transform: translate(-50%, -50%);
  width: 26%; height: 26%; border-radius: 50%; border: 4px solid #fff;
  background: linear-gradient(135deg, #6366f1, #8b5cf6); color: #fff; cursor: pointer;
  display: flex; flex-direction: column; align-items: center; justify-content: center; z-index: 2;
  box-shadow: 0 4px 14px rgba(99, 102, 241, 0.5);
}
.hub:disabled { cursor: wait; opacity: 0.85; }
.hub-emoji { font-size: var(--fs-title); }
.hub-text { font-size: var(--fs-xs); font-weight: 700; line-height: 1.2; margin-top: 2px; }
.spin-icon { width: 22px; height: 22px; border: 3px solid rgba(255,255,255,0.4); border-top-color: #fff; border-radius: 50%; animation: sp 0.8s linear infinite; }
@keyframes sp { to { transform: rotate(360deg); } }

.section { padding: 16px; border-radius: var(--radius-md); }
.sec-title { font-weight: 700; margin-bottom: 10px; font-size: var(--fs-lg); }
.ops { display: flex; gap: var(--sp-2); flex-wrap: wrap; justify-content: center; }
.op { padding: 9px 16px; border: 1px solid #6366f1; border-radius: var(--radius-sm); cursor: pointer; font-size: var(--fs-body); font-weight: 600; color: #6366f1; background: transparent; }
.op.primary { color: #fff; background: linear-gradient(135deg, #6366f1, #8b5cf6); border: 0; box-shadow: 0 4px 12px rgba(99,102,241,0.4); }
.op:disabled { opacity: 0.6; cursor: wait; }

.result { padding: 12px; border-radius: var(--radius-sm); display: flex; gap: var(--sp-2); align-items: flex-start; }
.result.win { background: var(--pt-success-bg); }
.result.lose { background: var(--pt-danger-bg); }
.badge { font-size: var(--fs-xl); }
.r-body { display: flex; flex-direction: column; gap: var(--sp-1); flex: 1; }
.r-prize { font-weight: 700; color: var(--pt-success); }
.r-bal { font-size: var(--fs-xs); color: var(--text-muted, #999); }
.r-msg { font-size: var(--fs-xs); }
.r-list { display: flex; flex-wrap: wrap; gap: var(--sp-1); }
.r-pill { padding: 3px 10px; border-radius: var(--radius-xs); font-size: var(--fs-xs); font-weight: 600; background: var(--pt-success-bg); color: var(--pt-success); }
.r-pill.nowin { background: rgba(255,255,255,0.08); color: var(--text-muted, #999); font-weight: 500; }

.prize-strip { display: flex; gap: var(--sp-2); overflow-x: auto; padding-bottom: 4px; }
.prize-card { flex: 0 0 auto; padding: 8px 14px; border-radius: var(--radius-sm); background: var(--glass-bg, rgba(255,255,255,0.06)); border: 1px solid var(--border-soft, rgba(255,255,255,0.08)); font-size: var(--fs-body); font-weight: 600; }

.rec-row { display: flex; gap: var(--sp-2); align-items: center; padding: 6px 0; font-size: var(--fs-body); border-bottom: 1px solid var(--border-soft, rgba(255,255,255,0.05)); }
.rec-row .muted { color: var(--text-muted, #999); }
.rec-amt { color: var(--pt-success); font-weight: 600; }
.rec-time { margin-left: auto; font-size: var(--fs-2xs); }
</style>
