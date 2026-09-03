<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, onMounted, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { entertainmentApi, type GameStats, type PlayResult } from '@/api/entertainment'

// 双色球选球器富组件（批14）：红区 33 球选 6 + 蓝区 16 球选 1，机选/清空，
// 覆盖麒麟 hdkylin_lotto / 织梦 zmpt_lotto（Selection="r,r,r,r,r,r|b"）
// 与蟹黄堡 crab_unionlotto（extra.reds/blue）。
const { t } = useI18n()
const props = defineProps<{ siteId: number; game: string; name: string }>()

const stats = ref<GameStats | null>(null)
const playing = ref(false)
const last = ref<PlayResult | null>(null)
const selectedRed = ref<number[]>([])
const selectedBlue = ref<number | null>(null)

const reds = Array.from({ length: 33 }, (_, i) => i + 1)
const blues = Array.from({ length: 16 }, (_, i) => i + 1)

const ready = computed(() => selectedRed.value.length === 6 && selectedBlue.value !== null)
const nextPoolDraw = ref('')

function toggleRed(n: number) {
  const i = selectedRed.value.indexOf(n)
  if (i >= 0) selectedRed.value.splice(i, 1)
  else if (selectedRed.value.length < 6) selectedRed.value.push(n)
  else ElMessage.warning('红球最多选 6 个')
}
function toggleBlue(n: number) {
  selectedBlue.value = selectedBlue.value === n ? null : n
}
function quickPick() {
  const pool = [...reds]
  const pick: number[] = []
  for (let i = 0; i < 6; i++) pick.push(pool.splice(Math.floor(Math.random() * pool.length), 1)[0])
  selectedRed.value = pick.sort((a, b) => a - b)
  selectedBlue.value = 1 + Math.floor(Math.random() * 16)
}
function clearAll() {
  selectedRed.value = []
  selectedBlue.value = null
}

async function refreshStats() {
  stats.value = await entertainmentApi.stats(props.siteId, props.game).catch(() => stats.value)
}

async function bet() {
  if (playing.value || !ready.value) return
  playing.value = true
  try {
    const redStr = [...selectedRed.value].sort((a, b) => a - b).join(',')
    const blue = selectedBlue.value!
    let res: PlayResult
    if (props.game === 'crab_unionlotto') {
      res = await entertainmentApi.bet(props.siteId, props.game, {
        selection: '',
        amount: 166,
        extra: { reds: redStr, blue: String(blue) },
      })
    } else {
      // 麒麟/织梦：Selection 直传 "6红|1蓝"
      res = await entertainmentApi.bet(props.siteId, props.game, {
        selection: `${redStr}|${blue}`,
        amount: 0,
      })
    }
    last.value = res
    if (res.success) ElMessage.success(res.prize || res.message || '投注成功')
    else ElMessage.warning(res.message || '投注失败')
    await refreshStats()
  } finally {
    playing.value = false
  }
}

onMounted(() => {
  refreshStats()
  // 开奖信息（麒麟/织梦 Play 只读同步会带开奖时间）
  entertainmentApi
    .play(props.siteId, props.game)
    .then((r) => {
      if (r.success && r.message) nextPoolDraw.value = r.message
    })
    .catch(() => {})
  quickPick()
})
</script>

<template>
  <div class="lotto">
    <div class="hero">
      <div class="hero-title">🎱 {{ name }}</div>
      <div class="hero-sub">
        {{ t('bj.balance') }} <b>{{ stats?.balance?.toLocaleString() ?? '—' }}</b>
        <template v-if="nextPoolDraw"> · {{ nextPoolDraw }}</template>
      </div>
    </div>

    <section class="glass section">
      <div class="sec-title">
        红球 <b class="cnt" :class="{ ok: selectedRed.length === 6 }">{{ selectedRed.length }}/6</b>
        <span class="hint text-muted">选 6 个（1-33）</span>
      </div>
      <div class="balls red">
        <button
          v-for="n in reds"
          :key="n"
          class="ball red-ball"
          :class="{ on: selectedRed.includes(n) }"
          :disabled="playing"
          @click="toggleRed(n)"
        >
          {{ n }}
        </button>
      </div>

      <div class="sec-title blue-title">
        蓝球 <b class="cnt" :class="{ ok: selectedBlue !== null }">{{ selectedBlue !== null ? 1 : 0 }}/1</b>
        <span class="hint text-muted">选 1 个（1-16）</span>
      </div>
      <div class="balls blue">
        <button
          v-for="n in blues"
          :key="n"
          class="ball blue-ball"
          :class="{ on: selectedBlue === n }"
          :disabled="playing"
          @click="toggleBlue(n)"
        >
          {{ n }}
        </button>
      </div>

      <div class="ops">
        <el-button :disabled="playing" @click="quickPick">🎲 机选一注</el-button>
        <el-button :disabled="playing" @click="clearAll">清空</el-button>
        <el-button type="primary" size="large" :loading="playing" :disabled="!ready" @click="bet">
          {{ playing ? '投注中…' : '投注' }}
        </el-button>
      </div>
      <div class="preview text-muted" v-if="selectedRed.length">
        当前：
        <span class="pv-red">{{ [...selectedRed].sort((a, b) => a - b).join(' ') }}</span>
        <span v-if="selectedBlue !== null" class="pv-blue"> + {{ selectedBlue }}</span>
      </div>
    </section>

    <div v-if="last" class="result" :class="last.success ? 'win' : 'lose'">
      <span class="badge">{{ last.success ? '✓' : '✗' }}</span>
      <div class="r-body">
        <span v-if="last.prize" class="r-prize">{{ last.prize }}</span>
        <div v-if="last.message" class="r-msg text-muted">{{ last.message }}</div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="css">
.lotto { display: flex; flex-direction: column; gap: var(--sp-4); max-width: 680px; margin: 0 auto; }
.hero { padding: 18px; border-radius: var(--radius-md); background: linear-gradient(135deg, #b91c1c, #f59e0b); color: #fff; text-align: center; }
.hero-title { font-size: var(--fs-title); font-weight: 800; }
.hero-sub { margin-top: 6px; opacity: 0.92; font-size: 13px; }
.section { padding: 16px; }
.sec-title { font-weight: 700; margin: 4px 0 10px; display: flex; align-items: baseline; gap: 8px; }
.sec-title .hint { font-weight: 400; font-size: 12px; }
.cnt { font-size: 14px; }
.cnt.ok { color: var(--success, #67c23a); }
.blue-title { margin-top: 16px; }
.balls { display: flex; flex-wrap: wrap; gap: 8px; }
.ball { width: 40px; height: 40px; border-radius: 50%; border: 2px solid transparent; font-size: 14px; font-weight: 800; cursor: pointer; color: #fff; transition: transform .12s; }
.ball:hover { transform: scale(1.12); }
.ball:disabled { opacity: .55; cursor: not-allowed; }
.red-ball { background: linear-gradient(160deg, #ef4444, #b91c1c); }
.blue-ball { background: linear-gradient(160deg, #3b82f6, #1d4ed8); }
.ball.on { border-color: #fbbf24; box-shadow: 0 0 0 3px rgba(251, 191, 36, .45); transform: scale(1.08); }
.ops { display: flex; gap: 10px; margin-top: 16px; align-items: center; }
.ops .el-button--primary { margin-left: auto; }
.preview { margin-top: 12px; font-size: 14px; }
.pv-red { color: #dc2626; font-weight: 800; letter-spacing: 2px; }
.pv-blue { color: #2563eb; font-weight: 800; margin-left: 4px; }
.result { display: flex; gap: 10px; padding: 12px 16px; border-radius: var(--radius-md); align-items: flex-start; }
.result.win { background: var(--success-bg, #f0f9eb); }
.result.lose { background: var(--danger-bg, #fef0f0); }
.badge { font-weight: 800; }
.r-prize { font-weight: 800; margin-right: 8px; }
</style>
