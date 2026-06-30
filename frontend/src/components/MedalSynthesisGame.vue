<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import {
  entertainmentApi,
  type GameStats,
  type SynthesisTarget,
  type GameRecord,
  type PlayResult,
} from '@/api/entertainment'

const props = defineProps<{ siteId: number }>()

const targets = ref<SynthesisTarget[]>([])
const stats = ref<GameStats | null>(null)
const selectedId = ref<number | null>(null)
const drawing = ref(0) // 正在抽取的次数（0=空闲）
const synthesizing = ref(false)
const lastDraw = ref<{ count: number; frags: { name: string; count: number }[]; msg: string } | null>(null)
const showHistory = ref(false)
const history = ref<GameRecord[]>([])
const historyLoading = ref(false)

const selected = computed(() => targets.value.find((t) => t.id === selectedId.value) || null)
const progressPct = computed(() => {
  if (!selected.value || selected.value.need === 0) return 0
  return Math.min(100, Math.round((selected.value.got / selected.value.need) * 100))
})

async function loadAll() {
  try {
    const [ts, st] = await Promise.all([
      entertainmentApi.syntheses(props.siteId, 'medal_synthesis').catch(() => []),
      entertainmentApi.stats(props.siteId, 'medal_synthesis').catch(() => null),
    ])
    targets.value = ts
    stats.value = st
    if (selectedId.value === null || !ts.find((t) => t.id === selectedId.value)) {
      selectedId.value = ts[0]?.id ?? null
    }
  } catch {
    /* 静默 */
  }
}

function parseFrags(prize?: string) {
  if (!prize) return []
  return prize
    .split(/[、,]/)
    .map((s) => {
      const m = s.trim().match(/^(.+?)×(\d+)$/)
      return m ? { name: m[1], count: Number(m[2]) } : { name: s.trim(), count: 1 }
    })
    .filter((f) => f.name)
}

async function draw(count: number) {
  if (!selected.value || drawing.value) return
  drawing.value = count
  lastDraw.value = null
  try {
    const res: PlayResult = await entertainmentApi.draw(
      props.siteId,
      'medal_synthesis',
      selected.value.id,
      count,
    )
    if (res.success) {
      lastDraw.value = { count, frags: parseFrags(res.prize), msg: res.message || '' }
      ElMessage.success(res.message || `${count} 连抽取完成`)
    } else {
      ElMessage.warning(res.message || '抽取失败')
    }
    await loadAll() // 刷新碎片库存 + 火花
    if (showHistory.value) {
      entertainmentApi.history(props.siteId, 'medal_synthesis').then((h) => (history.value = h)).catch(() => {})
    }
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '请求失败')
  } finally {
    drawing.value = 0
  }
}

async function synthesize() {
  if (!selected.value || synthesizing.value) return
  synthesizing.value = true
  try {
    const res = await entertainmentApi.synthesize(props.siteId, 'medal_synthesis', selected.value.id)
    if (res.success) ElMessage.success(res.prize ? `合成成功：${res.prize}` : '合成成功')
    else ElMessage.warning(res.message || '合成失败')
    await loadAll()
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '请求失败')
  } finally {
    synthesizing.value = false
  }
}

async function toggleHistory() {
  showHistory.value = !showHistory.value
  if (showHistory.value && !history.value.length) {
    historyLoading.value = true
    try {
      history.value = await entertainmentApi.history(props.siteId, 'medal_synthesis')
    } catch {
      ElMessage.warning('记录加载失败')
    } finally {
      historyLoading.value = false
    }
  }
}

function fmt(n: number) {
  return Math.round(n).toLocaleString('zh-CN')
}

onMounted(loadAll)
</script>

<template>
  <div class="card-synth">
    <!-- 顶部栏 -->
    <div class="topbar">
      <span class="title">卡牌合成</span>
      <span class="balance">当前火花：{{ stats ? fmt(stats.balance) : '--' }}</span>
    </div>

    <div v-if="!targets.length" class="empty text-muted">
      未解析到合成目标（可能未登录或页面结构变化，见后端日志 <code>entertainment raw response</code>）
    </div>

    <template v-else>
      <!-- 目标选择 -->
      <div v-if="targets.length > 1" class="target-tabs">
        <button
          v-for="t in targets"
          :key="t.id"
          class="tab"
          :class="{ active: t.id === selectedId }"
          @click="selectedId = t.id"
        >
          {{ t.name }}
        </button>
      </div>

      <div v-if="selected" class="panel">
        <!-- 左：目标卡 -->
        <div class="target-card">
          <img v-if="selected.image" :src="selected.image" :alt="selected.name" class="t-img" @error="(e) => ((e.target as HTMLImageElement).style.display = 'none')" />
          <div v-else class="t-img placeholder">🏅</div>
          <div class="t-name">{{ selected.name }}</div>
          <div class="t-progress">
            <div class="bar"><div class="fill" :style="{ width: progressPct + '%' }" /></div>
            <span class="pct">{{ selected.got }}/{{ selected.need }} · {{ progressPct }}%</span>
          </div>
          <div v-if="selected.pity" class="t-pity">保底 {{ selected.pity }}</div>
        </div>

        <!-- 右：碎片网格 + 操作 -->
        <div class="right">
          <div class="frag-grid">
            <div
              v-for="(f, i) in selected.fragments"
              :key="i"
              class="frag"
              :class="{ collected: f.current >= f.required && f.required > 0, rare: f.is_rare }"
            >
              <img v-if="f.image" :src="f.image" class="frag-img" @error="(e) => ((e.target as HTMLImageElement).style.display = 'none')" />
              <div v-else class="frag-img placeholder">💠</div>
              <div class="frag-name">{{ f.name || '碎片' }}</div>
              <div class="frag-count">
                <span class="cur">{{ f.current }}</span>/<span class="req">{{ f.required }}</span>
              </div>
            </div>
            <div v-if="!selected.fragments.length" class="text-muted frag-empty">无碎片数据</div>
          </div>

          <!-- 操作按钮 -->
          <div class="ops">
            <button class="op draw1" :disabled="!!drawing" @click="draw(1)">
              {{ drawing === 1 ? '…' : '单抽' }}<small>{{ fmt(selected.draw_price) }}</small>
            </button>
            <button class="op draw10" :disabled="!!drawing" @click="draw(10)">
              {{ drawing === 10 ? '…' : '十连' }}<small>{{ fmt(selected.ten_price) }}</small>
            </button>
            <button class="op draw100" :disabled="!!drawing" @click="draw(100)">
              {{ drawing === 100 ? '…' : '百连' }}<small>{{ fmt(selected.hundred_price) }}</small>
            </button>
            <button
              class="op synth"
              :disabled="synthesizing || !selected.can_synthesize"
              @click="synthesize"
            >
              {{ synthesizing ? '…' : '合成' }}
            </button>
            <button class="op hist" :class="{ active: showHistory }" @click="toggleHistory">记录</button>
          </div>

          <!-- 上次抽取结果 -->
          <div v-if="lastDraw" class="last-draw">
            <span class="ld-label">{{ lastDraw.count }} 连抽结果：</span>
            <span v-for="(f, i) in lastDraw.frags" :key="i" class="ld-frag">{{ f.name }}×{{ f.count }}</span>
            <span v-if="!lastDraw.frags.length" class="text-muted">{{ lastDraw.msg || '完成' }}</span>
          </div>

          <!-- 抽取记录 -->
          <div v-if="showHistory" class="history" v-loading="historyLoading">
            <div class="hd-label">抽取记录（{{ history.length }}）</div>
            <div v-if="!history.length" class="text-muted hist-empty">暂无记录</div>
            <div v-else class="hist-list">
              <div v-for="(r, i) in history" :key="i" class="hist-row">
                <span class="h-frag">{{ r.prize_name || '—' }}</span>
                <span class="h-syn text-muted">{{ r.prize_type }}</span>
                <span v-if="r.cost > 0" class="h-cost">-{{ fmt(r.cost) }}</span>
                <span class="h-time text-muted">{{ r.created_at }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.card-synth {
  display: flex;
  flex-direction: column;
  gap: 14px;
  max-width: 720px;
  margin: 0 auto;
  background: #fff5f7;
  border-radius: 16px;
  padding: 16px;
  color: #4a2f35;
}
.topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.title {
  font-size: 20px;
  font-weight: 800;
  color: #ff69b4;
  letter-spacing: 2px;
}
.balance {
  padding: 4px 12px;
  border-radius: 999px;
  background: #ffc107;
  color: #4a2f00;
  font-weight: 700;
  font-size: 13px;
}
.empty {
  padding: 30px;
  text-align: center;
  font-size: 13px;
}
/* 目标 tabs */
.target-tabs {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
.tab {
  padding: 5px 12px;
  border-radius: 999px;
  border: 1px solid rgba(106, 90, 205, 0.3);
  background: rgba(255, 255, 255, 0.6);
  cursor: pointer;
  font-size: 12px;
  color: #6a5acd;
}
.tab.active {
  background: #6a5acd;
  color: #fff;
  border-color: #6a5acd;
}
/* 面板 */
.panel {
  display: grid;
  grid-template-columns: 160px 1fr;
  gap: 14px;
}
@media (max-width: 520px) {
  .panel { grid-template-columns: 1fr; }
}
.target-card {
  background: linear-gradient(135deg, #6a5acd, #8e6fd0);
  border-radius: 12px;
  padding: 12px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  color: #fff;
}
.t-img {
  width: 100%;
  aspect-ratio: 1;
  object-fit: cover;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.15);
}
.t-img.placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 40px;
}
.t-name {
  font-weight: 700;
  font-size: 15px;
}
.t-progress {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 3px;
  align-items: center;
  font-size: 11px;
}
.bar {
  width: 100%;
  height: 6px;
  background: rgba(255, 255, 255, 0.25);
  border-radius: 3px;
  overflow: hidden;
}
.fill {
  height: 100%;
  background: linear-gradient(90deg, #ffc107, #ff69b4);
  transition: width 0.4s ease;
}
.t-pity {
  font-size: 10px;
  color: rgba(255, 255, 255, 0.8);
}
/* 碎片网格 */
.right {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.frag-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(58px, 1fr));
  gap: 6px;
}
.frag {
  padding: 5px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.7);
  border: 2px solid #e0c0c8;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  text-align: center;
}
.frag.collected {
  border-color: #4caf50;
  background: rgba(76, 175, 80, 0.12);
}
.frag.rare {
  border-color: #a855f7;
}
.frag-img {
  width: 34px;
  height: 34px;
  object-fit: cover;
  border-radius: 6px;
}
.frag-img.placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  background: rgba(106, 90, 205, 0.1);
}
.frag-name {
  font-size: 10px;
  color: #6a5acd;
  line-height: 1.1;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.frag-count {
  font-size: 11px;
  font-weight: 700;
}
.frag-count .cur {
  color: #ff69b4;
}
.frag-count .req {
  color: #555;
}
.frag-empty {
  grid-column: 1 / -1;
  padding: 16px;
  text-align: center;
  font-size: 12px;
}
/* 操作按钮 */
.ops {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 6px;
}
.op {
  padding: 8px 4px;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  color: #fff;
  font-weight: 700;
  font-size: 13px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1px;
  transition: filter 0.15s ease;
}
.op small {
  font-size: 9px;
  font-weight: 600;
  opacity: 0.9;
}
.op:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.op:hover:not(:disabled) {
  filter: brightness(1.08);
}
.draw1 { background: #ff69b4; }
.draw10 { background: #ffc107; color: #4a2f00; }
.draw100 { background: #f44336; }
.synth { background: #ffebee; color: #ff69b4; border: 1px solid #ff69b4; }
.hist { background: #2196f3; }
.hist.active { background: #6a5acd; }
/* 上次结果 */
.last-draw {
  padding: 8px 10px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.6);
  font-size: 12px;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}
.ld-label {
  font-weight: 700;
  color: #6a5acd;
}
.ld-frag {
  padding: 1px 8px;
  border-radius: 999px;
  background: #6a5acd;
  color: #fff;
  font-size: 11px;
  font-weight: 600;
}
/* 记录 */
.history {
  margin-top: 4px;
  padding: 8px 10px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.6);
}
.hd-label {
  font-size: 12px;
  font-weight: 700;
  color: #6a5acd;
  margin-bottom: 6px;
}
.hist-empty {
  padding: 12px;
  text-align: center;
  font-size: 12px;
}
.hist-list {
  display: flex;
  flex-direction: column;
  gap: 3px;
  max-height: 220px;
  overflow-y: auto;
}
.hist-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 6px;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.5);
  font-size: 11.5px;
}
.hist-row .h-frag {
  font-weight: 600;
  color: #ff69b4;
  flex: 1;
}
.hist-row .h-syn {
  font-size: 10px;
  max-width: 30%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.hist-row .h-cost {
  color: #e67e22;
  font-weight: 600;
}
.hist-row .h-time {
  font-size: 10px;
  white-space: nowrap;
}
</style>
