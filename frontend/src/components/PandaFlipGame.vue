<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { entertainmentApi, type FlipState, type FlipTier } from '@/api/entertainment'
import { authApi } from '@/api/auth'

// PandaPT 翻面夺宝：10×10 卡片网格 + 奖项/规则侧栏 + 本局记录。
// 点格子→bet(selection=index, extra.round_number)→翻牌动画→刷新 FlipState。
// 他人翻格实时同步：后端桥接站点 wss://host/wss，前端连 /flip-ws 收 tileFlip 事件（不做轮询）。
const props = defineProps<{ siteId: number }>()
const GAME = 'panda_flip'

const state = ref<FlipState | null>(null)
const playing = ref<number | null>(null) // 正在翻的格子 index
const lastTier = ref<string>('') // 最近翻出的奖项（高亮提示）

// 兜底配色（站点 tier.color 缺失时按 tier 顺序）
const tierPalette = ['#facc15', '#9ca3af', '#b45309', '#3b82f6', '#22c55e', '#a855f7', '#ec4899']

const tiers = computed<FlipTier[]>(() => state.value?.tiers ?? [])
const tiles = computed(() => state.value?.tiles ?? [])

// tiles 可能稀疏（仅已翻的）；补齐 1..100 完整网格
const grid = computed(() => {
  const byIndex = new Map<number, { index: number; flipped: boolean; tierId: string; byUser: string }>()
  for (const t of tiles.value) {
    byIndex.set(t.index, { index: t.index, flipped: t.flipped, tierId: t.tier_id ?? '', byUser: t.by_user ?? '' })
  }
  const total = state.value?.total_tiles ?? 100
  const out = []
  for (let i = 1; i <= total; i++) {
    out.push(byIndex.get(i) ?? { index: i, flipped: false, tierId: '', byUser: '' })
  }
  return out
})

const tierColorOf = (tierId: string) => {
  if (!tierId) return ''
  const idx = tiers.value.findIndex((t) => t.id === tierId)
  if (idx < 0) return '#6366f1'
  const t = tiers.value[idx]
  return t.color || tierPalette[idx % tierPalette.length]
}
const tierNameOf = (tierId: string) => tiers.value.find((t) => t.id === tierId)?.name ?? ''

async function load(silent = false) {
  try {
    state.value = await entertainmentApi.flipState(props.siteId, GAME)
  } catch (e: unknown) {
    if (!silent) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
      ElMessage.error(msg || '加载翻面夺宝失败')
    }
  }
}

// 不做后台轮询（持续自动请求站点易被判为攻击/机器人）。仅在挂载时加载一次、
// 翻牌后刷新、以及用户手动点「刷新」时请求。他人翻格走 websocket 桥接（见下）。
const refreshing = ref(false)
async function manualRefresh() {
  if (refreshing.value) return
  refreshing.value = true
  await load()
  refreshing.value = false
}

// ── websocket 桥接：收站点 tileFlip 推送，实时标记他人翻的格 ──
let flipWs: WebSocket | null = null
let wsStopped = false
async function connectFlipWs() {
  if (wsStopped) return
  let ticket: string
  try {
    ticket = (await authApi.wsTicket()).ticket
  } catch {
    return
  }
  if (wsStopped) return
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  const url = `${proto}://${location.host}/api/v1/entertainment/${props.siteId}/games/${GAME}/flip-ws?ticket=${encodeURIComponent(ticket)}`
  try {
    flipWs = new WebSocket(url)
  } catch {
    return
  }
  flipWs.onmessage = (e) => {
    let ev: { type: string; tile_index?: number; tier_id?: string; tier_name?: string; player?: string }
    try {
      ev = JSON.parse(e.data)
    } catch {
      return
    }
    if (ev.type === 'tileFlip' && ev.tile_index) applyTileFlip(ev.tile_index, ev.tier_id ?? '', ev.tier_name ?? '', ev.player ?? '')
    else if (ev.type === 'special') ElMessage.warning('🎉 特等奖出现，新局即将开始')
  }
  flipWs.onclose = () => {
    // 下游偶发断开 → 几秒后重连（上游在后端自愈）；卸载后不重连
    if (!wsStopped) setTimeout(connectFlipWs, 3000)
  }
}

// 收到他人翻格事件：标记该格已翻 + 记入本局记录（已存在则跳过，去重）
function applyTileFlip(index: number, tierId: string, tierName: string, player: string) {
  if (!state.value) return
  if (!state.value.tiles) state.value.tiles = []
  if (state.value.tiles.some((t) => t.index === index)) return
  state.value.tiles.push({ index, flipped: true, tier_id: tierId, by_user: player })
  state.value.flipped_count = (state.value.flipped_count ?? 0) + 1
  const label = `${player || '?'} 翻了 #${index}` + (tierName ? ` · ${tierName}` : '')
  state.value.history = [{ prize_name: label, prize_type: tierId, amount: 0, cost: 0, created_at: '' }, ...(state.value.history ?? [])].slice(0, 50)
}

onMounted(() => {
  load()
  connectFlipWs()
})
onBeforeUnmount(() => {
  wsStopped = true
  if (flipWs) flipWs.onclose = null
  flipWs?.close()
})

async function flip(index: number) {
  if (playing.value !== null || !state.value) return
  const tile = grid.value.find((t) => t.index === index)
  if (tile?.flipped) return
  playing.value = index
  try {
    const res = await entertainmentApi.bet(props.siteId, GAME, {
      selection: String(index),
      amount: 0,
      extra: { round_number: String(state.value.round_number || 1) },
    })
    if (res.success) {
      if (res.prize && !res.prize.includes('未中奖')) {
        lastTier.value = res.prize
        ElMessage.success(`🎉 ${res.prize}`)
      } else {
        ElMessage.info(res.message || '未中奖，继续加油')
      }
    } else {
      ElMessage.warning(res.message || '翻牌失败')
    }
    await load()
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '请求失败')
  } finally {
    playing.value = null
  }
}

const RULES = [
  '点击任意卡片翻面',
  '每次翻面消耗对应魔力',
  '翻中对应奖项即获奖励',
  '特等奖/全翻完开启新一轮',
  '奖项随机分布',
  '翻面过快可能卡顿（非 bug）',
  '最终解释权归站点所有',
]
</script>

<template>
  <div class="flip">
    <div class="hero">
      <div class="hero-title">🃏 翻面夺宝</div>
      <div class="balance">余额 <b>{{ state?.balance?.toLocaleString() ?? '—' }}</b></div>
    </div>

    <div class="stat-bar">
      <div class="stat-tile">
        <div class="stat-val">{{ state?.flipped_count ?? 0 }}/{{ state?.total_tiles ?? 100 }}</div>
        <div class="stat-lbl">游戏状态</div>
      </div>
      <div class="stat-tile">
        <div class="stat-val warn">{{ state?.cost_per_flip ? Math.round(state.cost_per_flip) : '—' }}</div>
        <div class="stat-lbl">翻牌成本 / 次</div>
      </div>
      <div class="stat-tile">
        <div class="stat-val">第 {{ state?.round_number ?? '?' }} 局</div>
        <div class="stat-lbl">当前轮次</div>
      </div>
      <button class="refresh-btn" :disabled="refreshing" @click="manualRefresh" title="刷新他人翻的格子">
        {{ refreshing ? '…' : '🔄' }}
      </button>
    </div>

    <div class="main">
      <!-- 10×10 卡片网格 -->
      <div class="grid">
        <button
          v-for="t in grid"
          :key="t.index"
          class="tile"
          :class="{ flipped: t.flipped, active: playing === t.index }"
          :style="t.flipped && t.tierId ? { background: tierColorOf(t.tierId) } : {}"
          :disabled="playing !== null || t.flipped"
          :title="t.flipped && t.tierId ? tierNameOf(t.tierId) : `第 ${t.index} 格`"
          @click="flip(t.index)"
        >
          <template v-if="t.flipped && t.tierId">
            <span class="t-tier">{{ tierNameOf(t.tierId) }}</span>
          </template>
          <template v-else-if="t.flipped">
            <span class="t-num">{{ t.index }}</span>
          </template>
          <template v-else>
            <span class="t-q">?</span>
            <span class="t-num small">{{ t.index }}</span>
          </template>
        </button>
      </div>

      <!-- 侧栏：奖项详情 + 规则 -->
      <aside class="side">
        <section class="glass panel">
          <div class="sec-title">奖项详情</div>
          <div v-if="!tiers.length" class="text-muted empty">加载中…</div>
          <div v-for="(t, i) in tiers" :key="t.id" class="tier-row">
            <span class="tier-bar" :style="{ background: t.color || tierPalette[i % tierPalette.length] }" />
            <span class="tier-name">{{ t.name }}</span>
            <span class="tier-reward">{{ Math.round(t.reward) }} 魔力 × {{ t.count }} 张</span>
          </div>
        </section>

        <section class="glass panel">
          <div class="sec-title">游戏规则</div>
          <ol class="rules">
            <li v-for="(r, i) in RULES" :key="i">{{ r }}</li>
          </ol>
        </section>
      </aside>
    </div>

    <!-- 本局翻牌记录 -->
    <section v-if="state?.history?.length" class="glass section">
      <div class="sec-title">本局翻牌记录</div>
      <div v-for="(r, i) in state.history.slice(0, 12)" :key="i" class="rec-row">
        <span class="rec-prize">{{ r.prize_name }}</span>
        <span v-if="r.amount" class="rec-amt">+{{ Math.round(r.amount) }}</span>
        <span v-if="r.created_at" class="text-muted rec-time">{{ r.created_at }}</span>
      </div>
    </section>
  </div>
</template>

<style scoped>
.flip { display: flex; flex-direction: column; gap: 16px; max-width: 760px; margin: 0 auto; }
.hero { padding: 18px; border-radius: 14px; background: linear-gradient(135deg, #db2777, #f59e0b); text-align: center; }
.hero-title { font-size: 22px; font-weight: 800; color: #fff; }
.balance { margin-top: 4px; font-size: 13px; color: rgba(255,255,255,0.92); }
.balance b { color: #fde68a; font-size: 16px; }

.stat-bar { display: flex; gap: 10px; }
.stat-tile { flex: 1; text-align: center; padding: 12px 6px; border-radius: 12px; background: var(--glass-bg, rgba(255,255,255,0.06)); border: 1px solid var(--border-soft, rgba(255,255,255,0.08)); }
.stat-val { font-size: 18px; font-weight: 800; color: var(--pt-primary, #6c5ce7); }
.stat-val.warn { color: var(--pt-warn, #f59e0b); }
.stat-lbl { font-size: 11px; color: var(--text-muted, #999); margin-top: 2px; }
.refresh-btn { flex: 0 0 auto; width: 44px; align-self: stretch; border: 1px solid var(--border-soft, rgba(255,255,255,0.08)); border-radius: 12px; background: var(--glass-bg, rgba(255,255,255,0.06)); font-size: 18px; cursor: pointer; transition: transform 0.15s; }
.refresh-btn:hover:not(:disabled) { transform: rotate(90deg); }
.refresh-btn:disabled { opacity: 0.5; cursor: wait; }

.main { display: grid; grid-template-columns: 1fr; gap: 16px; }
@media (min-width: 640px) { .main { grid-template-columns: 1fr 240px; } }

.grid { display: grid; grid-template-columns: repeat(10, 1fr); gap: 5px; padding: 12px; border-radius: 14px; background: var(--glass-bg, rgba(255,255,255,0.04)); }
.tile {
  aspect-ratio: 1; border: 0; border-radius: 8px; cursor: pointer; padding: 2px;
  display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 1px;
  background: linear-gradient(135deg, #7c3aed, #6366f1); color: #fff;
  transition: transform 0.15s, box-shadow 0.15s; position: relative;
}
.tile:hover:not(:disabled) { transform: scale(1.08); box-shadow: 0 3px 10px rgba(124,58,237,0.5); z-index: 2; }
.tile:disabled { cursor: default; }
.tile.flipped { background: rgba(255,255,255,0.1); }
.tile.active { animation: pulse 0.6s infinite; }
@keyframes pulse { 0%,100% { transform: scale(1); } 50% { transform: scale(0.92); } }
.t-q { font-size: clamp(14px, 3vw, 20px); font-weight: 800; }
.t-num { font-size: 10px; opacity: 0.7; }
.t-num.small { font-size: 9px; opacity: 0.5; }
.t-tier { font-size: clamp(8px, 1.6vw, 11px); font-weight: 700; text-align: center; line-height: 1.1; text-shadow: 0 1px 2px rgba(0,0,0,0.4); }

.side { display: flex; flex-direction: column; gap: 16px; }
.panel { padding: 14px; border-radius: 14px; }
.sec-title { font-weight: 700; margin-bottom: 10px; font-size: 15px; }
.empty { font-size: 13px; }
.tier-row { display: flex; align-items: center; gap: 8px; padding: 5px 0; font-size: 12px; border-bottom: 1px solid var(--border-soft, rgba(255,255,255,0.05)); }
.tier-bar { width: 6px; height: 18px; border-radius: 3px; flex: 0 0 auto; }
.tier-name { font-weight: 700; flex: 0 0 auto; }
.tier-reward { margin-left: auto; color: var(--text-muted, #999); font-size: 11px; }
.rules { margin: 0; padding-left: 18px; font-size: 12px; line-height: 1.7; color: var(--text-muted, #aaa); }

.section { padding: 14px; border-radius: 14px; }
.rec-row { display: flex; gap: 10px; align-items: center; padding: 5px 0; font-size: 13px; border-bottom: 1px solid var(--border-soft, rgba(255,255,255,0.05)); }
.rec-prize { font-weight: 600; }
.rec-amt { color: #22c55e; font-weight: 600; }
.rec-time { margin-left: auto; font-size: 11px; }
</style>
