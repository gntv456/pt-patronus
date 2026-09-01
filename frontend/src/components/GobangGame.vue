<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { entertainmentApi, type GobangMatch, type GobangState } from '@/api/entertainment'

// 蜂巢五子棋人机挑战：全景 = 进行中一局 + 最近完成一局 + 周榜 + 蜂蜜汇总。
// 动作全部 POST gobang-action（create/choose-color/move/undo/forfeit/retry-ai），完成后重拉全景。
const { t } = useI18n()
const props = defineProps<{ siteId: number; game: string; name: string }>()

const SIZE = 15
const state = ref<GobangState | null>(null)
const loading = ref(false)
const acting = ref(false)
const errorMsg = ref('')
const difficulty = ref('WARMUP')
const lbTab = ref('STANDARD')
let pollTimer: ReturnType<typeof setTimeout> | null = null

const difficulties = [
  { id: 'WARMUP', label: '热身', desc: '免费 · 不计榜' },
  { id: 'STANDARD', label: '标准', desc: '门票 2 · 返奖 3' },
  { id: 'HARD', label: '困难', desc: '门票 3 · 返奖 5' },
  { id: 'NIGHTMARE', label: '噩梦', desc: '门票 5 · 返奖 10 · 限时' },
]

const ongoing = computed(() => state.value?.matches.find((m) => m.status === 'ONGOING') ?? null)
const finished = computed(() => state.value?.matches.find((m) => m.status === 'FINISHED') ?? null)
const board = computed(() => {
  const b = ongoing.value?.board
  if (b && b.length) return b
  return Array.from({ length: SIZE }, () => Array(SIZE).fill(0))
})
const lastMove = computed(() => {
  const mv = ongoing.value?.moves
  return mv?.length ? mv[mv.length - 1] : null
})
const myTurn = computed(() => ongoing.value?.current_side === 'PLAYER' && !ongoing.value?.opening_choice_pending)
// 悔棋条件与站点一致：标准/困难 + 未用 + 轮到玩家 + 平衡开局后至少两手
const canUndo = computed(
  () =>
    ongoing.value &&
    !ongoing.value.opening_choice_pending &&
    ['STANDARD', 'HARD'].includes(ongoing.value.difficulty) &&
    !ongoing.value.undo_used &&
    myTurn.value &&
    (ongoing.value.moves?.length ?? 0) >= (ongoing.value.opening_move_count ?? 0) + 2,
)
const boardRows = computed(() =>
  board.value.slice(0, SIZE).map((row, r) => ({ r, cells: Array.from({ length: SIZE }, (_, c) => row?.[c] ?? 0) })),
)
const summaryLine = computed(() => {
  const s = state.value?.summary
  if (!s) return ''
  const warmup = s.warmup_remaining ?? s.free_remaining ?? 0
  return `${s.point_name || '蜂蜜'} ${s.points} · 今日 ${s.total_used}/${s.total_limit} · 热身剩 ${warmup}`
})
const statusLine = computed(() => {
  const m = ongoing.value
  if (!m) return finished.value ? resultText(finished.value) : t('entertainment.done')
  if (m.opening_choice_pending) return '请在倒计时内选择执黑或执白'
  return m.current_side === 'PLAYER' ? '轮到你落子' : 'AI 正在思考…'
})

function resultText(m: GobangMatch | null): string {
  if (!m) return ''
  if (m.interruption_voided_at) return '上一局：服务保护，门票与次数已恢复'
  if (m.winner_id === null || m.winner_id === undefined) {
    if (m.finish_reason === 'TIMEOUT') return '上一局：超时负'
    if (m.finish_reason === 'FORFEIT') return '上一局：已认输'
    return m.status === 'FINISHED' ? '上一局：平局' : ''
  }
  return m.winner_id === 0 ? '上一局：AI 获胜' : `上一局：你赢了${m.payout_amount ? `，返奖 ${m.payout_amount}` : ''}`
}

async function loadState() {
  loading.value = true
  try {
    state.value = await entertainmentApi.gobangState(props.siteId, props.game)
    if (ongoing.value) difficulty.value = ongoing.value.difficulty
    errorMsg.value = ''
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    errorMsg.value = msg || t('common.requestFailed')
  } finally {
    loading.value = false
    schedulePoll()
  }
}

// AI 回合轮询：有进行中局面且轮到 AI 时 5s 拉一次，回到玩家回合即停。
function schedulePoll() {
  if (pollTimer) clearTimeout(pollTimer)
  if (ongoing.value && ongoing.value.current_side === 'AI' && !ongoing.value.opening_choice_pending) {
    pollTimer = setTimeout(loadState, 5000)
  }
}

async function act(action: string, params: Record<string, unknown> = {}) {
  if (acting.value) return
  acting.value = true
  errorMsg.value = ''
  try {
    const res = await entertainmentApi.gobangAction(props.siteId, props.game, action, params)
    if (res.message) ElMessage.info(res.message)
    await loadState()
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    errorMsg.value = msg || t('common.requestFailed')
    // 站点限速（429 落子过于频繁，~1s/手）：1.5s 后自动重拉一次全景
    if (msg && msg.includes('过于频繁')) setTimeout(() => loadState(), 1500)
  } finally {
    acting.value = false
  }
}

function create() {
  act('create', { difficulty: difficulty.value })
}
function chooseColor(color: string) {
  if (ongoing.value) act('choose-color', { matchId: ongoing.value.id, color })
}
function drop(r: number, c: number) {
  if (!myTurn.value || board.value[r]?.[c]) return
  act('move', { matchId: ongoing.value?.id, x: c, y: r })
}
function undo() {
  if (ongoing.value) act('undo', { matchId: ongoing.value.id })
}
function forfeit() {
  if (ongoing.value) act('forfeit', { matchId: ongoing.value.id })
}
function retryAI() {
  if (ongoing.value) act('retry-ai', { matchId: ongoing.value.id })
}

function fmtDur(sec: number): string {
  const s = Math.max(0, Math.round(sec))
  const m = Math.floor(s / 60)
  if (m >= 60) return `${Math.floor(m / 60)} 时 ${m % 60} 分`
  return m > 0 ? `${m} 分 ${s % 60} 秒` : `${s} 秒`
}

onMounted(loadState)
onBeforeUnmount(() => {
  if (pollTimer) clearTimeout(pollTimer)
})
</script>

<template>
  <div class="gobang">
    <div class="hero">
      <div class="hero-title">♟️ {{ name }}</div>
      <div class="balance">{{ summaryLine || '五子棋 · 人机挑战' }}</div>
    </div>

    <div v-if="errorMsg" class="err">{{ errorMsg }}</div>

    <!-- 开局面板：选难度开一局；有进行中局面则不显示 -->
    <section v-if="!ongoing" class="glass section start">
      <div class="diff-rail">
        <button
          v-for="d in difficulties"
          :key="d.id"
          class="diff"
          :class="{ active: difficulty === d.id }"
          @click="difficulty = d.id"
        >
          <b>{{ d.label }}</b>
          <small>{{ d.desc }}</small>
        </button>
      </div>
      <el-button type="primary" :loading="acting || loading" @click="create">
        {{ finished ? '再来一局' : '开始挑战' }}
      </el-button>
      <div v-if="finished" class="last-result text-muted">{{ resultText(finished) }}</div>
    </section>

    <!-- 对局中 -->
    <section v-else class="glass section arena">
      <div class="arena-head">
        <span class="tag" :class="ongoing?.difficulty.toLowerCase()">{{ difficulties.find((d) => d.id === ongoing?.difficulty)?.label || ongoing?.difficulty }}</span>
        <span class="status" :class="{ my: myTurn }">{{ statusLine }}</span>
        <span v-if="ongoing.player_color" class="side">你执{{ ongoing.player_color === 'WHITE' ? '白' : '黑' }}</span>
      </div>

      <!-- 平衡开局选色（困难/噩梦） -->
      <div v-if="ongoing.opening_choice_pending" class="choose">
        <el-button :disabled="acting" @click="chooseColor('BLACK')">执黑先行</el-button>
        <el-button :disabled="acting" @click="chooseColor('WHITE')">执白后行</el-button>
      </div>

      <div class="board" :class="{ locked: !myTurn }">
        <div v-for="row in boardRows" :key="row.r" class="brow">
          <button
            v-for="cell in row.cells"
            :key="row.r + '-' + cell.c"
            class="pt"
            type="button"
            :disabled="acting || cell.v !== 0 || !myTurn"
            @click="drop(row.r, cell.c)"
          >
            <span
              v-if="cell.v !== 0"
              class="stone"
              :class="cell.v === 1 ? 'black' : 'white'"
              :data-last="lastMove && lastMove.x === cell.c && lastMove.y === row.r"
            />
          </button>
        </div>
      </div>
      <div class="coords text-muted">15 路棋盘 · 服务端棋局，禁止双开</div>

      <div class="ops">
        <el-button v-if="canUndo" :disabled="acting" @click="undo">
          悔棋{{ ongoing.undo_cost ? ` · ${ongoing.undo_cost}` : '' }}
        </el-button>
        <el-button v-if="ongoing.current_side === 'AI'" :disabled="acting" @click="retryAI">重试 AI 落子</el-button>
        <el-button type="danger" plain :disabled="acting" @click="forfeit">认输</el-button>
      </div>

      <div v-if="finished" class="last-result">{{ resultText(finished) }}</div>
    </section>

    <!-- 周榜 -->
    <section class="glass section">
      <div class="sec-title">各难度周榜</div>
      <el-tabs v-if="state?.leaderboards" v-model="lbTab">
        <el-tab-pane v-for="d in ['STANDARD', 'HARD', 'NIGHTMARE']" :key="d" :label="difficulties.find((x) => x.id === d)?.label || d" :name="d" />
      </el-tabs>
      <div v-if="(state?.leaderboards?.[lbTab] ?? []).length" class="lb">
        <div class="lb-row head">
          <span>#</span><span class="lb-name">玩家</span><span>战绩</span><span>胜率</span><span>榜分</span>
        </div>
        <div v-for="e in (state?.leaderboards?.[lbTab] ?? []).slice(0, 10)" :key="e.user_id" class="lb-row">
          <span class="lb-rank" :class="{ top: e.rank <= 3 }">{{ e.rank }}</span>
          <span class="lb-name">{{ e.nickname || e.username }}</span>
          <span>{{ e.wins }}胜{{ e.losses }}负</span>
          <span>{{ e.win_rate }}%</span>
          <span class="lb-score">{{ e.score > 0 ? '+' : '' }}{{ e.score }}</span>
        </div>
      </div>
      <div v-else class="text-muted empty-hint">本周还没有完成计榜对局的玩家</div>
    </section>
  </div>
</template>

<style scoped src="./gobang-game.css"></style>
