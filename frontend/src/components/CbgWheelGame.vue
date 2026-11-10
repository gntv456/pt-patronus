<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, onMounted, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { entertainmentApi, type PlayResult, type CbgWheelState } from '@/api/entertainment'

// 藏宝阁幸运大转盘富组件（批13）：10 格奖品环（站点 INITIAL_PRIZES）+ 旋转动画 + 连抽。
const { t } = useI18n()
const props = defineProps<{ siteId: number; game: string; name: string }>()

const state = ref<CbgWheelState | null>(null)
const spinning = ref(false)
const angle = ref(0)
const last = ref<PlayResult | null>(null)
const multiCount = ref(5)

const slice = computed(() => 360 / (state.value?.prizes.length || 10))
const wheelStyle = computed(() => {
  const n = state.value?.prizes.length || 10
  const colors = ['#7c3aed', '#f59e0b', '#10b981', '#0ea5e9', '#ef4444', '#8b5cf6', '#14b8a6', '#f97316', '#06b6d4', '#a855f7']
  const parts: string[] = []
  for (let i = 0; i < n; i++) parts.push(`${colors[i % colors.length]} 0deg ${(360 / n) * (i + 1)}deg`)
  return {
    background: `conic-gradient(from -90deg, ${parts.join(',')})`,
    transform: `rotate(${angle.value}deg)`,
    transition: spinning.value ? 'transform 3.2s cubic-bezier(0.15, 0.85, 0.25, 1)' : 'none',
  }
})

async function refresh() {
  try {
    state.value = await entertainmentApi.cbgWheelState(props.siteId, props.game)
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '状态获取失败')
  }
}

async function draw(times: number) {
  if (spinning.value) return
  spinning.value = true
  last.value = null
  try {
    const res =
      times > 1
        ? await entertainmentApi.playMulti(props.siteId, props.game, times)
        : await entertainmentApi.play(props.siteId, props.game)
    last.value = res
    // prize_ids 最后一项 → 转到对应扇区（站点顺序即扇区序）
    const prizes = state.value?.prizes ?? []
    const lastId = res.prizes?.length ? res.prizes[res.prizes.length - 1].prize_id : null
    let target = Math.floor(Math.random() * prizes.length)
    if (lastId != null) {
      const idx = prizes.findIndex((p) => p.id === lastId)
      if (idx >= 0) target = idx
    }
    angle.value += 360 * 5 + ((360 - (target + 0.5) * slice.value) - (angle.value % 360))
    await new Promise((r) => setTimeout(r, 3300))
    if (res.success) ElMessage.success(res.prize || res.message || '中奖！')
    else ElMessage.warning(res.message || '抽奖失败')
    await refresh()
  } finally {
    spinning.value = false
  }
}

onMounted(refresh)
</script>

<template>
  <div class="cbgw">
    <div class="hero">
      <div class="hero-title">🎡 {{ name }}</div>
      <div v-if="state" class="hero-sub">
        魔力 <b>{{ state.balance.toLocaleString() }}</b> · {{ state.cost }}/次
      </div>
    </div>

    <section class="glass section wheel-zone">
      <div class="wheel-wrap">
        <div class="wheel" :style="wheelStyle">
          <div
            v-for="(p, i) in state?.prizes ?? []"
            :key="p.id"
            class="slice-label"
            :style="{ transform: `rotate(${(i + 0.5) * slice}deg)` }"
          >
            {{ p.name?.slice(0, 7) }}
          </div>
        </div>
        <div class="hub">🎁</div>
        <div class="pointer">▼</div>
      </div>
      <div class="ops">
        <el-button type="primary" size="large" :loading="spinning" @click="draw(1)">🎡 抽一次</el-button>
        <div class="multi">
          <el-input-number v-model="multiCount" :min="2" :max="10" size="small" />
          <el-button :loading="spinning" @click="draw(multiCount)">连抽 {{ multiCount }}</el-button>
        </div>
      </div>
    </section>

    <section v-if="state?.recent?.length" class="glass section">
      <div class="sec-title">📜 最近中奖</div>
      <div class="recent">
        <el-tag v-for="(r, i) in state.recent" :key="i" size="small" :type="r.amount > 0 ? 'success' : 'info'">
          {{ r.prize_name || '—' }}
        </el-tag>
      </div>
    </section>

    <div v-if="last" class="result" :class="last.success ? 'win' : 'lose'">
      <span class="badge">{{ last.success ? '✓' : '✗' }}</span>
      <div class="r-body">
        <span v-if="last.prize" class="r-prize">{{ last.prize }}</span>
        <span v-if="last.balance" class="r-bal">魔力 {{ last.balance.toLocaleString() }}</span>
        <div v-if="last.message" class="r-msg text-muted">{{ last.message }}</div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="css">
.cbgw { display: flex; flex-direction: column; gap: var(--sp-4); max-width: 560px; margin: 0 auto; }
.hero { padding: 18px; border-radius: var(--radius-md); background: linear-gradient(135deg, #6d28d9, #a855f7); color: #fff; text-align: center; }
.hero-title { font-size: var(--fs-title); font-weight: 800; }
.hero-sub { margin-top: 6px; opacity: 0.92; }
.section { padding: 16px; }
.sec-title { font-weight: 700; margin-bottom: 10px; }
.wheel-zone { display: flex; flex-direction: column; align-items: center; gap: 18px; }
.wheel-wrap { position: relative; width: 270px; height: 270px; }
.wheel { position: absolute; inset: 0; border-radius: 50%; border: 6px solid #fff; box-shadow: 0 8px 24px rgba(0,0,0,.25); overflow: hidden; }
.slice-label { position: absolute; left: 50%; top: 10px; transform-origin: 0 120px; font-size: 11px; font-weight: 700; color: #fff; text-shadow: 0 1px 2px rgba(0,0,0,.55); white-space: nowrap; }
.hub { position: absolute; left: 50%; top: 50%; transform: translate(-50%, -50%); width: 56px; height: 56px; border-radius: 50%; background: #fff; display: flex; align-items: center; justify-content: center; font-size: 26px; box-shadow: 0 2px 8px rgba(0,0,0,.3); z-index: 2; }
.pointer { position: absolute; top: -12px; left: 50%; transform: translateX(-50%); font-size: 24px; color: #dc2626; z-index: 3; text-shadow: 0 1px 2px #fff; }
.ops { display: flex; gap: 12px; align-items: center; }
.multi { display: flex; gap: 6px; align-items: center; }
.recent { display: flex; flex-wrap: wrap; gap: 6px; }
.result { display: flex; gap: 10px; padding: 12px 16px; border-radius: var(--radius-md); align-items: flex-start; }
.result.win { background: var(--success-bg, #f0f9eb); }
.result.lose { background: var(--danger-bg, #fef0f0); }
.badge { font-weight: 800; }
.r-prize { font-weight: 800; margin-right: 8px; }
</style>
