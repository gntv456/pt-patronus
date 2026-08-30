<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { entertainmentApi, type FlipState, type FlipTier } from '@/api/entertainment'
import { authApi } from '@/api/auth'

// PandaPT 翻面夺宝：10×10 卡片网格 + 奖项/规则侧栏 + 本局记录。
// 点格子→bet(selection=index, extra.round_number)→翻牌动画→刷新 FlipState。
// 他人翻格实时同步：后端桥接站点 wss://host/wss，前端连 /flip-ws 收 tileFlip 事件（不做轮询）。
const props = defineProps<{ siteId: number }>()
const GAME = 'panda_flip'

const state = ref<FlipState | null>(null)
const { t } = useI18n()
const pfTileN = (n: number) => t('pf.tileN', { n })
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
      ElMessage.error(msg || t('pf.loadFailed'))
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
    if (ev.type === 'tileFlip' && ev.tile_index)
      applyTileFlip(ev.tile_index, ev.tier_id ?? '', ev.tier_name ?? '', ev.player ?? '')
    else if (ev.type === 'special') ElMessage.warning(t('pf.special'))
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
  const label = t('pf.othersFlip', { player: player || '?', index }) + (tierName ? ` · ${tierName}` : '')
  state.value.history = [
    { prize_name: label, prize_type: tierId, amount: 0, cost: 0, created_at: '' },
    ...(state.value.history ?? []),
  ].slice(0, 50)
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
      if (res.prize && !res.prize.includes(t('pf.noWinShort'))) {
        lastTier.value = res.prize
        ElMessage.success(`🎉 ${res.prize}`)
      } else {
        ElMessage.info(res.message || t('pf.noWin'))
      }
    } else {
      ElMessage.warning(res.message || t('pf.flipFailed'))
    }
    await load()
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || t('common.requestFailed'))
  } finally {
    playing.value = null
  }
}

const RULES = computed(() => [t('pf.r1'), t('pf.r2'), t('pf.r3'), t('pf.r4'), t('pf.r5'), t('pf.r6'), t('pf.r7')])
</script>

<template>
  <div class="flip">
    <div class="hero">
      <div class="hero-title">🃏 {{ t('pf.title') }}</div>
      <div class="balance">
        {{ t('bs.balance') }} <b>{{ state?.balance?.toLocaleString() ?? '—' }}</b>
      </div>
    </div>

    <div class="stat-bar">
      <div class="stat-tile">
        <div class="stat-val">{{ state?.flipped_count ?? 0 }}/{{ state?.total_tiles ?? 100 }}</div>
        <div class="stat-lbl">{{ t('pf.gameState') }}</div>
      </div>
      <div class="stat-tile">
        <div class="stat-val warn">{{ state?.cost_per_flip ? Math.round(state.cost_per_flip) : '—' }}</div>
        <div class="stat-lbl">{{ t('pf.costPer') }}</div>
      </div>
      <div class="stat-tile">
        <div class="stat-val">{{ t('bs.roundN', { n: state?.round_number ?? '?' }) }}</div>
        <div class="stat-lbl">{{ t('pf.curRound') }}</div>
      </div>
      <button class="refresh-btn" :disabled="refreshing" @click="manualRefresh" :title="t('pf.refreshTiles')">
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
          :title="t.flipped && t.tierId ? tierNameOf(t.tierId) : pfTileN(t.index)"
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
          <div class="sec-title">{{ t('pf.tiers') }}</div>
          <div v-if="!tiers.length" class="text-muted empty">{{ t('common.loading') }}</div>
          <div v-for="(tier, i) in tiers" :key="tier.id" class="tier-row">
            <span class="tier-bar" :style="{ background: tier.color || tierPalette[i % tierPalette.length] }" />
            <span class="tier-name">{{ tier.name }}</span>
            <span class="tier-reward"
              >{{ t('pf.magicN', { n: Math.round(tier.reward) }) }} × {{ t('pf.cardsN', { n: tier.count }) }}</span
            >
          </div>
        </section>

        <section class="glass panel">
          <div class="sec-title">{{ t('pf.rules') }}</div>
          <ol class="rules">
            <li v-for="(r, i) in RULES" :key="i">{{ r }}</li>
          </ol>
        </section>
      </aside>
    </div>

    <!-- 本局翻牌记录 -->
    <section v-if="state?.history?.length" class="glass section">
      <div class="sec-title">{{ t('pf.history') }}</div>
      <div v-for="(r, i) in state.history.slice(0, 12)" :key="i" class="rec-row">
        <span class="rec-prize">{{ r.prize_name }}</span>
        <span v-if="r.amount" class="rec-amt">+{{ Math.round(r.amount) }}</span>
        <span v-if="r.created_at" class="text-muted rec-time">{{ r.created_at }}</span>
      </div>
    </section>
  </div>
</template>

<style scoped src="./PandaFlipGame.css"></style>
