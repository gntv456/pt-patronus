<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, onMounted, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { entertainmentApi, type FishingState, type PlayResult } from '@/api/entertainment'

// 思齐钓鱼富组件（批8）：钓次进度/装备/图鉴网格，抛竿走 play-multi。
const { t } = useI18n()
const props = defineProps<{ siteId: number; game: string; name: string }>()

const state = ref<FishingState | null>(null)
const fishing = ref(false)
const last = ref<PlayResult | null>(null)

const rarityOrder: Record<string, number> = { common: 0, uncommon: 1, rare: 2, epic: 3, legendary: 4 }
const codexSorted = computed(() => {
  if (!state.value) return []
  return [...state.value.codex].sort((a, b) =>
    (rarityOrder[a.rarity] ?? 9) - (rarityOrder[b.rarity] ?? 9) || a.code - b.code
  )
})

async function refresh() {
  try {
    state.value = await entertainmentApi.fishingState(props.siteId, props.game)
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '状态获取失败')
  }
}

async function equip(action: 'select_rod' | 'select_lure', key: string) {
  if (fishing.value) return
  fishing.value = true
  try {
    const extraKey = action === 'select_rod' ? 'rod_key' : 'lure_key'
    const res = await entertainmentApi.bet(props.siteId, props.game, {
      selection: action,
      amount: 0,
      extra: { [extraKey]: key },
    })
    if (res.success) ElMessage.success(res.message || '已切换')
    else ElMessage.warning(res.message || '操作失败')
    await refresh()
  } finally {
    fishing.value = false
  }
}

async function buyLure(key: string, price: number) {
  if (fishing.value) return
  fishing.value = true
  try {
    const res = await entertainmentApi.bet(props.siteId, props.game, {
      selection: 'buy_lure',
      amount: 0,
      extra: { lure_key: key, quantity: '1' },
    })
    if (res.success) ElMessage.success(`已购买（${price} 魔力）：${res.message || 'OK'}`)
    else ElMessage.warning(res.message || '购买失败')
    await refresh()
  } finally {
    fishing.value = false
  }
}

async function cast(times: number) {
  if (fishing.value) return
  fishing.value = true
  try {
    const res = await entertainmentApi.playMulti(props.siteId, props.game, times)
    last.value = res
    if (res.success) ElMessage.success(res.message || '收竿！')
    else ElMessage.warning(res.message || '抛竿失败')
    await refresh()
  } finally {
    fishing.value = false
  }
}

onMounted(refresh)
</script>

<template>
  <div class="fishgame">
    <div class="hero">
      <div class="hero-title">🎣 {{ name }}</div>
      <div v-if="state" class="hero-sub">
        魔力 <b>{{ state.balance.toLocaleString() }}</b> · 鱼竿 {{ state.selected_rod || '—' }}
        <template v-if="state.selected_lure"> · 鱼饵 {{ state.selected_lure }}</template>
      </div>
    </div>

    <template v-if="state">
      <section class="glass section">
        <div class="sec-title">今日钓次</div>
        <el-progress
          :percentage="state.attempts_limit ? Math.min(100, Math.round((state.attempts_used / state.attempts_limit) * 100)) : 0"
          :format="() => `${state!.attempts_used}/${state!.attempts_limit}`"
        />
        <div class="casts">
          <el-button type="primary" :loading="fishing" :disabled="state.attempts_used >= state.attempts_limit" @click="cast(1)">
            🎣 抛竿一次
          </el-button>
          <el-button :loading="fishing" :disabled="state.attempts_used >= state.attempts_limit" @click="cast(5)">
            连抛 5 次
          </el-button>
          <span class="text-muted caught">今日钓获 {{ state.caught_today }} 尾</span>
        </div>
      </section>

      <section v-if="state.rods?.length || state.lures?.length" class="glass section">
        <div class="sec-title">🎣 装备</div>
        <div v-if="state.rods?.length" class="equip-group">
          <div class="equip-label">鱼竿</div>
          <div class="equip-grid">
            <button
              v-for="rod in state.rods"
              :key="rod.key"
              class="equip rod"
              :class="{ sel: rod.selected, locked: !rod.unlocked }"
              :disabled="fishing || !rod.unlocked || rod.selected"
              :title="rod.description + (rod.unlock_hint ? `（${rod.unlock_hint}）` : '')"
              @click="equip('select_rod', rod.key)"
            >
              <span class="e-name">{{ rod.name }}</span>
              <span class="e-meta">{{ rod.selected ? '使用中' : rod.unlocked ? '点击换上' : rod.unlock_hint || '未解锁' }}</span>
            </button>
          </div>
        </div>
        <div v-if="state.lures?.length" class="equip-group">
          <div class="equip-label">鱼饵</div>
          <div class="equip-grid">
            <div v-for="lure in state.lures" :key="lure.key" class="equip lure" :class="{ sel: lure.selected }">
              <button
                class="e-main"
                :disabled="fishing || lure.selected || lure.owned <= 0"
                :title="lure.description"
                @click="equip('select_lure', lure.key)"
              >
                <span class="e-name">{{ lure.name }}</span>
                <span class="e-meta">{{ lure.selected ? '挂饵中' : lure.owned > 0 ? `持有 ${lure.owned} · 点击挂上` : '无库存' }}</span>
              </button>
              <button class="e-buy" :disabled="fishing" :title="`购买 1 个（${lure.price} 魔力）`" @click="buyLure(lure.key, lure.price)">
                买 {{ lure.price }}
              </button>
            </div>
          </div>
        </div>
      </section>

      <section class="glass section">
        <div class="sec-title">🐟 图鉴 {{ state.discovered_count }}/{{ state.total_species }}</div>
        <div class="codex">
          <div
            v-for="f in codexSorted"
            :key="f.code"
            class="fish"
            :class="[f.rarity, { unknown: !f.discovered }]"
            :title="f.discovered ? `${f.name} ×${f.quantity}` : '未解锁'"
          >
            <div class="f-name">{{ f.discovered ? f.name : '???' }}</div>
            <div class="f-meta">{{ f.rarity_label }}</div>
            <div v-if="f.discovered" class="f-qty">×{{ f.quantity }}</div>
          </div>
        </div>
      </section>
    </template>

    <div v-if="last" class="result" :class="last.success ? 'win' : 'lose'">
      <span class="badge">{{ last.success ? '✓' : '✗' }}</span>
      <div class="r-body">
        <div v-if="last.message" class="r-msg text-muted">{{ last.message }}</div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="css">
.fishgame { display: flex; flex-direction: column; gap: var(--sp-4); max-width: 720px; margin: 0 auto; }
.hero { padding: 18px; border-radius: var(--radius-md); background: linear-gradient(135deg, #0ea5e9, #22c55e); color: #fff; text-align: center; }
.hero-title { font-size: var(--fs-title); font-weight: 800; }
.hero-sub { margin-top: 6px; opacity: 0.92; }
.section { padding: 16px; }
.sec-title { font-weight: 700; margin-bottom: 10px; }
.casts { display: flex; gap: 10px; align-items: center; margin-top: 12px; }
.caught { margin-left: auto; font-size: 13px; }
.codex { display: grid; grid-template-columns: repeat(auto-fill, minmax(96px, 1fr)); gap: 8px; }
.fish { border: 1px solid var(--border-light, #e5e7eb); border-radius: var(--radius-sm, 6px); padding: 8px; text-align: center; font-size: 12px; }
.fish.unknown { opacity: 0.45; filter: grayscale(0.8); }
.fish .f-name { font-weight: 700; }
.fish .f-meta { color: var(--text-muted, #909399); margin-top: 2px; }
.fish .f-qty { margin-top: 2px; }
.fish.rare { border-color: #60a5fa; }
.fish.epic { border-color: #a78bfa; }
.fish.legendary { border-color: #f59e0b; box-shadow: 0 0 6px rgba(245, 158, 11, 0.5); }
.result { display: flex; gap: 10px; padding: 12px 16px; border-radius: var(--radius-md); align-items: flex-start; }
.result.win { background: var(--success-bg, #f0f9eb); }
.result.lose { background: var(--danger-bg, #fef0f0); }
.badge { font-weight: 800; }

.equip-group { margin-bottom: 14px; }
.equip-group:last-child { margin-bottom: 0; }
.equip-label { font-size: 12px; color: var(--text-muted); margin-bottom: 8px; font-weight: 600; }
.equip-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(170px, 1fr)); gap: 8px; }
.equip { border: 1.5px solid var(--border-light, #e5e7eb); border-radius: 10px; padding: 10px; text-align: left; background: var(--bg-card, #fff); cursor: pointer; transition: border-color .15s, box-shadow .15s; }
.equip:hover:not(:disabled) { border-color: var(--primary, #409eff); }
.equip.sel { border-color: #f59e0b; box-shadow: 0 0 0 2px rgba(245, 158, 11, .35); }
.equip.locked { opacity: .55; }
.equip.rod, .equip.lure { display: block; }
.equip .e-main { all: unset; display: block; width: 100%; cursor: pointer; }
.equip .e-main:disabled { cursor: not-allowed; }
.e-name { display: block; font-weight: 700; font-size: 13px; }
.e-meta { display: block; font-size: 11px; color: var(--text-muted); margin-top: 2px; }
.e-buy { margin-top: 6px; width: 100%; border: none; border-radius: 6px; background: #f59e0b; color: #fff; font-size: 12px; font-weight: 700; padding: 4px 0; cursor: pointer; }
.e-buy:hover:not(:disabled) { background: #d97706; }
.e-buy:disabled { background: #d1d5db; cursor: not-allowed; }
</style>
