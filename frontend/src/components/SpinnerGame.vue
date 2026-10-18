<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, computed, watch, onMounted, nextTick } from 'vue'
import { entertainmentApi, type GameStats, type PrizeDef, type GameRecord, type PlayResult } from '@/api/entertainment'
import { entFail } from '@/utils/entFail'

// 通用抽奖组件（批1）：覆盖多站幸运转盘/老虎机/抽奖。按 game 调 play/playMulti/stats/prizes/history。
// 关键：永远画出可见转盘——有 prizes 画真扇区，没有画 8 格装饰扇区，保证所有站点都看得到转盘。
const props = defineProps<{ siteId: number; game: string; name: string }>()

// 12 色循环（对齐 LongPT 明亮配色）
const colors = [
  '#FF6B6B',
  '#FFD93D',
  '#6BCB77',
  '#4D96FF',
  '#9D65C9',
  '#FF9F43',
  '#1ABC9C',
  '#E84393',
  '#FDCB6E',
  '#74B9FF',
  '#A29BFE',
  '#55EFC4',
]
// 无奖品数据时的装饰扇区（保证转盘永远有内容）
const decorSegments = ['🎁', '🪙', '✨', '🎫', '🍀', '💎', '👑', '🎊']

const canvasRef = ref<HTMLCanvasElement | null>(null)
const stats = ref<GameStats | null>(null)
const { t } = useI18n()
const prizes = ref<PrizeDef[]>([])
const records = ref<GameRecord[]>([])
const playing = ref(false)
const last = ref<PlayResult | null>(null)
const rotation = ref(0)

// 转盘扇区：有真奖品用奖品名，否则用装饰 emoji
const segments = computed(() =>
  prizes.value.length ? prizes.value.map((p) => p.name || t('sp.prize')) : decorSegments.slice(),
)

// 连抽结果按奖品名分组计数（对齐躺平站点「88 魔力 ×12」展示）
const groupedPrizes = computed(() => {
  const ps = last.value?.prizes ?? []
  if (!ps.length) return []
  const m = new Map<string, { name: string; n: number; win: boolean }>()
  for (const p of ps) {
    const key = p.prize_name || t('sp.unknown')
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
  stats.value = await entertainmentApi.stats(props.siteId, props.game).catch(entFail('stats', stats.value))
}
async function refreshHistory() {
  records.value = await entertainmentApi.history(props.siteId, props.game).catch(entFail('history', []))
}

async function draw(count: number) {
  if (playing.value) return
  playing.value = true
  last.value = null
  try {
    const res =
      count === 1
        ? await entertainmentApi.play(props.siteId, props.game)
        : await entertainmentApi.playMulti(props.siteId, props.game, count)
    // 单抽且有奖品名时：转盘定位到匹配扇区（与结果一致的可信动画）；
    // 连抽/匹配不到则装饰旋转（5-7 圈随机角，结果在结果条揭示）
    let stopName: string | null = null
    if (count === 1 && res.success && res.prize) stopName = res.prize
    await animateSpins(5 + Math.floor(Math.random() * 3), stopName)
    last.value = res
    if (res.success) {
      // 连抽：按分组汇总展示（如「88 魔力 ×8, 谢谢参与 ×2」）；单抽：直接奖品名
      const groups = groupedPrizes.value
      if (groups.length > 1 || (groups.length === 1 && groups[0].n > 1)) {
        const wins = groups.filter((g) => g.win)
        const summary = groups.map((g) => `${g.name} ×${g.n}`).join('，')
        ElMessage.success(wins.length ? `🎉 ${summary}` : summary)
      } else {
        ElMessage.success(res.prize ? `🎉 ${res.prize}` : res.message || t('sp.done'))
      }
    } else {
      ElMessage.warning(res.message || t('sp.drawFailed'))
    }
    await Promise.all([refreshStats(), refreshHistory()])
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || t('common.requestFailed'))
  } finally {
    playing.value = false
  }
}

function animateSpins(turns: number, stopName?: string | null) {
  return new Promise<void>((resolve) => {
    let extra: number
    const segs = segments.value
    if (stopName && segs.length) {
      // 找到奖品所在扇区 index（含子串匹配兜底——站点奖品名可能带数量后缀）
      let idx = segs.findIndex((nm) => nm === stopName)
      if (idx < 0) idx = segs.findIndex((nm) => stopName.includes(nm) || nm.includes(stopName))
      if (idx >= 0) {
        // 指针在正上方（-90deg 起）：转满 turns 圈后落在该扇区中心
        const sector = 360 / segs.length
        const target = 360 - (idx + 0.5) * sector
        extra = turns * 360 + ((target - (rotation.value % 360) + 360) % 360)
      } else {
        extra = turns * 360 + Math.floor(Math.random() * 360)
      }
    } else {
      extra = turns * 360 + Math.floor(Math.random() * 360)
    }
    rotation.value = rotation.value + extra
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
  prizes.value = await entertainmentApi.prizes(props.siteId, props.game).catch(entFail('prizes', [] as PrizeDef[]))
  await Promise.all([refreshStats(), refreshHistory()])
  await nextTick(drawWheel)
})
</script>

<template>
  <div class="spinner">
    <div class="hero">
      <div class="hero-title">🎰 {{ name }}</div>
      <div class="balance">
        {{ t('bs.balance') }} <b>{{ heroBalance }}</b>
        <span v-if="stats?.cost_per_play" class="cost">· {{ stats.cost_per_play.toLocaleString() }}/次</span>
      </div>
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
          <span class="hub-text">{{ t('sp.hub') }}</span>
        </template>
      </button>
    </div>

    <section class="glass section ops">
      <button class="op primary" :disabled="playing" @click="draw(1)">
        {{ playing ? t('sp.drawing') : t('sp.drawNow') }}
      </button>
      <button class="op" :disabled="playing" @click="draw(10)">{{ t('sp.multi', { n: 10 }) }}</button>
      <button class="op" :disabled="playing" @click="draw(20)">{{ t('sp.multi', { n: 20 }) }}</button>
      <button class="op" :disabled="playing" @click="draw(50)">{{ t('sp.multi', { n: 50 }) }}</button>
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
        <span v-if="last.balance" class="r-bal">{{ t('bs.balance') }} {{ last.balance.toLocaleString() }}</span>
        <div v-if="last.message" class="r-msg text-muted">{{ last.message }}</div>
      </div>
    </div>

    <section v-if="prizes.length" class="glass section">
      <div class="sec-title">{{ t('sp.prizes') }}</div>
      <div class="prize-strip">
        <div v-for="p in prizes" :key="p.id" class="prize-card">{{ p.name }}</div>
      </div>
    </section>

    <section v-if="records.length" class="glass section">
      <div class="sec-title">{{ t('sp.records') }}</div>
      <div v-for="(r, i) in records.slice(0, 10)" :key="i" class="rec-row">
        <span :class="{ muted: r.prize_name === t('sp.noWin') }">{{ r.prize_name }}</span>
        <span v-if="r.amount" class="rec-amt">×{{ r.amount }}</span>
        <span class="text-muted rec-time">{{ r.created_at }}</span>
      </div>
    </section>
  </div>
</template>

<style scoped src="./SpinnerGame.css">
.cost { font-size: 12px; opacity: 0.85; margin-left: 4px; }
</style>
