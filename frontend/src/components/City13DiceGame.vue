<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { entertainmentApi, type PlayResult, type City13DiceState } from '@/api/entertainment'

// 13City 幸运骰子富组件（批12）：三骰滚动动画 + 大/小下注面板 + 开奖走势。
const { t } = useI18n()
const props = defineProps<{ siteId: number; game: string; name: string }>()

const state = ref<City13DiceState | null>(null)
const rolling = ref(false)
const dice = ref<[number, number, number]>([1, 1, 1])
const amount = ref(100)
const last = ref<PlayResult | null>(null)
const DICE_FACES = ['⚀', '⚁', '⚂', '⚃', '⚄', '⚅']

async function refresh() {
  try {
    state.value = await entertainmentApi.city13DiceState(props.siteId, props.game)
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '状态获取失败')
  }
}

async function bet(kind: 'big' | 'small') {
  if (rolling.value) return
  rolling.value = true
  try {
    // 滚动动画
    const timer = setInterval(() => {
      dice.value = dice.value.map(() => 1 + Math.floor(Math.random() * 6)) as [number, number, number]
    }, 80)
    const res = await entertainmentApi.bet(props.siteId, props.game, {
      selection: kind,
      amount: amount.value,
    })
    clearInterval(timer)
    last.value = res
    // 从结果解析真实骰面（res.prize 形如 "骰子: 3 5 6"）
    const m = /骰子[::]?\s*([\d\s]+)/.exec(res.prize || '')
    if (m) {
      const nums = m[1].trim().split(/\s+/).map(Number)
      if (nums.length >= 3) dice.value = [nums[0], nums[1], nums[2]]
    }
    if (res.success) ElMessage.success(`${res.prize || ''} ${res.message || ''}`)
    else ElMessage.warning(res.message || '下注失败')
    await refresh()
  } finally {
    rolling.value = false
  }
}

onMounted(refresh)
</script>

<template>
  <div class="c13d">
    <div class="hero">
      <div class="hero-title">🎲 {{ name }}</div>
      <div v-if="state" class="hero-sub">啤酒瓶 <b>{{ state.balance.toLocaleString() }}</b></div>
    </div>

    <section class="glass section dice-zone">
      <div class="dice-row">
        <div v-for="(d, i) in dice" :key="i" class="die" :class="{ rolling }">{{ DICE_FACES[d - 1] }}</div>
      </div>
      <div class="sum-line text-muted">
        合计 <b>{{ dice[0] + dice[1] + dice[2] }}</b> · {{ dice[0] + dice[1] + dice[2] >= 11 ? '大' : '小' }}
      </div>
      <div class="bet-panel">
        <div class="amt-row">
          <span>下注（啤酒瓶）</span>
          <el-input-number v-model="amount" :min="1" size="small" />
        </div>
        <div class="ops">
          <el-button type="danger" size="large" :loading="rolling" @click="bet('big')">押大 11-17 ×1</el-button>
          <el-button type="primary" size="large" :loading="rolling" @click="bet('small')">押小 4-10 ×1</el-button>
        </div>
      </div>
    </section>

    <section v-if="state?.recent?.length" class="glass section">
      <div class="sec-title">📜 最近开奖</div>
      <div class="recent">
        <div v-for="(r, i) in state.recent" :key="i" class="rec" :class="r.big ? 'big' : 'small'">
          {{ r.dice.join('+') }}={{ r.sum }} {{ r.big ? '大' : '小' }}
        </div>
      </div>
    </section>

    <div v-if="last" class="result" :class="last.success ? 'win' : 'lose'">
      <span class="badge">{{ last.success ? '✓' : '✗' }}</span>
      <div class="r-body">
        <span v-if="last.prize" class="r-prize">{{ last.prize }}</span>
        <span v-if="last.balance" class="r-bal">余 {{ last.balance.toLocaleString() }}</span>
        <div v-if="last.message" class="r-msg text-muted">{{ last.message }}</div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="css">
.c13d { display: flex; flex-direction: column; gap: var(--sp-4); max-width: 560px; margin: 0 auto; }
.hero { padding: 18px; border-radius: var(--radius-md); background: linear-gradient(135deg, #0e7490, #22d3ee); color: #fff; text-align: center; }
.hero-title { font-size: var(--fs-title); font-weight: 800; }
.hero-sub { margin-top: 6px; opacity: 0.92; }
.section { padding: 16px; }
.sec-title { font-weight: 700; margin-bottom: 10px; }
.dice-zone { display: flex; flex-direction: column; align-items: center; gap: 12px; }
.dice-row { display: flex; gap: 18px; }
.die { width: 76px; height: 76px; border-radius: 14px; background: #fff; border: 2px solid #e2e8f0; display: flex; align-items: center; justify-content: center; font-size: 56px; color: #0f172a; box-shadow: 0 4px 10px rgba(0,0,0,.12); }
.die.rolling { animation: shake .25s infinite; }
@keyframes shake { 0%,100% { transform: rotate(-6deg); } 50% { transform: rotate(6deg); } }
.sum-line b { font-size: 18px; }
.bet-panel { display: flex; flex-direction: column; gap: 12px; width: 100%; max-width: 360px; }
.amt-row { display: flex; align-items: center; gap: 10px; font-size: 13px; }
.ops { display: flex; gap: 12px; }
.ops .el-button { flex: 1; }
.recent { display: flex; flex-wrap: wrap; gap: 8px; }
.rec { padding: 4px 10px; border-radius: 999px; font-size: 12px; font-weight: 700; }
.rec.big { background: #fef2f2; color: #dc2626; }
.rec.small { background: #eff6ff; color: #2563eb; }
.result { display: flex; gap: 10px; padding: 12px 16px; border-radius: var(--radius-md); align-items: flex-start; }
.result.win { background: var(--success-bg, #f0f9eb); }
.result.lose { background: var(--danger-bg, #fef0f0); }
.badge { font-weight: 800; }
.r-prize { font-weight: 800; margin-right: 8px; }
</style>
