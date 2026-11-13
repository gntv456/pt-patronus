<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { entertainmentApi, type PlayResult, type CbgSlotState } from '@/api/entertainment'

// 藏宝阁魔力老虎机富组件（批13）：三轴数字轮（1-9）滚动动画，results_index 定格，
// 净变动 = reward_mp - cost 展示。
const { t } = useI18n()
const props = defineProps<{ siteId: number; game: string; name: string }>()

const state = ref<CbgSlotState | null>(null)
const rolling = ref(false)
const reels = ref<[number, number, number]>([1, 2, 3])
const last = ref<PlayResult | null>(null)
const lastNet = ref<number | null>(null)

async function refresh() {
  try {
    state.value = await entertainmentApi.cbgSlotState(props.siteId, props.game)
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '状态获取失败')
  }
}

async function spin() {
  if (rolling.value) return
  rolling.value = true
  lastNet.value = null
  try {
    const timer = setInterval(() => {
      reels.value = reels.value.map(() => 1 + Math.floor(Math.random() * 9)) as [number, number, number]
    }, 70)
    const res = await entertainmentApi.play(props.siteId, props.game)
    clearInterval(timer)
    last.value = res
    // 解析 "转出: i | j | k" 定格
    const m = /转出[::]?\s*([\d\s|]+)/.exec(res.prize || '')
    if (m) {
      const nums = m[1].split('|').map((x) => parseInt(x.trim(), 10)).filter((n) => !isNaN(n))
      if (nums.length >= 3) reels.value = [nums[0], nums[1], nums[2]]
    }
    if (res.success) ElMessage.success(res.message || '转动完成')
    else ElMessage.warning(res.message || '转动失败')
    await refresh()
    // 净变动（reward_mp 在 raw 里）
    const reward = Number((res.raw as Record<string, unknown> | undefined)?.reward_mp ?? 0)
    if (res.success && state.value) lastNet.value = reward - state.value.cost
  } finally {
    rolling.value = false
  }
}

onMounted(refresh)
</script>

<template>
  <div class="cbgs">
    <div class="hero">
      <div class="hero-title"><SptIcon name="gamepad" :size="18" aria-hidden="true" /> {{ name }}</div>
      <div v-if="state" class="hero-sub">
        魔力 <b>{{ state.balance.toLocaleString() }}</b> · {{ state.cost }}/次
      </div>
    </div>

    <section class="glass section slot-zone">
      <div class="machine">
        <div class="reel-row">
          <div v-for="(r, i) in reels" :key="i" class="reel" :class="{ rolling }">{{ r }}</div>
        </div>
        <div class="net-line" v-if="lastNet !== null" :class="lastNet >= 0 ? 'up' : 'down'">
          {{ lastNet >= 0 ? '净赚' : '净亏' }} {{ Math.abs(lastNet).toLocaleString() }} 魔力
        </div>
      </div>
      <el-button type="primary" size="large" :loading="rolling" @click="spin">
        <SptIcon name="play" :size="16" aria-hidden="true" /> 转一把
      </el-button>
    </section>

    <section v-if="state?.recent?.length" class="glass section">
      <div class="sec-title"><SptIcon name="clock" :size="16" aria-hidden="true" /> 最近记录</div>
      <div class="recent">
        <el-tag v-for="(r, i) in state.recent" :key="i" size="small" :type="r.amount > 0 ? 'success' : 'info'">
          {{ r.prize_name || '—' }} {{ r.amount ? `+${r.amount}` : '' }}
        </el-tag>
      </div>
    </section>

    <div v-if="last" class="result" :class="last.success ? 'win' : 'lose'">
      <span class="badge"><SptIcon :name="last.success ? 'check' : 'x'" :size="16" aria-hidden="true" /></span>
      <div class="r-body">
        <span v-if="last.prize" class="r-prize">{{ last.prize }}</span>
        <span v-if="last.balance" class="r-bal">魔力 {{ last.balance.toLocaleString() }}</span>
        <div v-if="last.message" class="r-msg text-muted">{{ last.message }}</div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="css">
.cbgs { display: flex; flex-direction: column; gap: var(--sp-4); max-width: 560px; margin: 0 auto; }
.hero { padding: 18px; border-radius: var(--radius-md); background: var(--color-surface-2); border: 1px solid var(--color-border); color: var(--color-text); text-align: center; }
.hero-title { font-size: var(--fs-title); font-weight: 800; }
.hero-sub { margin-top: 6px; opacity: 0.92; }
.section { padding: 16px; }
.sec-title { font-weight: 700; margin-bottom: 10px; }
.slot-zone { display: flex; flex-direction: column; align-items: center; gap: 18px; }
.machine { display: flex; flex-direction: column; align-items: center; gap: 10px; padding: 18px; background: var(--color-surface-2); border: 1px solid var(--color-border); border-radius: var(--radius-panel); box-shadow: var(--shadow-card); }
.reel-row { display: flex; gap: 14px; }
.reel { width: 84px; height: 110px; border-radius: 10px; background: var(--color-surface); border: 1px solid var(--color-border-strong); display: flex; align-items: center; justify-content: center; font-size: 56px; font-weight: 900; color: var(--color-text); font-variant-numeric: tabular-nums; }
.reel.rolling { animation: blur .18s 3; }
@keyframes blur { 0%,100% { filter: blur(0.5px); } 50% { filter: blur(1.5px); } }
@media (prefers-reduced-motion: reduce) { .reel.rolling { animation: none; } }
.net-line { font-weight: 800; font-size: 15px; }
.net-line.up { color: var(--pt-success-fg); }
.net-line.down { color: var(--pt-danger-fg); }
.recent { display: flex; flex-wrap: wrap; gap: 6px; }
.result { display: flex; gap: 10px; padding: 12px 16px; border-radius: var(--radius-md); align-items: flex-start; }
.result.win { background: var(--color-success-soft); }
.result.lose { background: var(--color-danger-soft); }
.badge { font-weight: 800; }
.r-prize { font-weight: 800; margin-right: 8px; }
</style>
