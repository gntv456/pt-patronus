<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, onMounted, computed } from 'vue'
import { entertainmentApi, type BlackjackResult } from '@/api/entertainment'

// 二十一点状态机（批3）：new/hit/stand/double，渲染牌面与结果。
const { t } = useI18n()
const props = defineProps<{ siteId: number; game: string; name: string }>()

const state = ref<BlackjackResult | null>(null)
const acting = ref(false)

const finished = computed(() => state.value?.stage === 'finished')

async function act(action: string) {
  if (acting.value) return
  acting.value = true
  try {
    const res = await entertainmentApi.blackjack(props.siteId, props.game, action)
    state.value = res
    if (res.message) {
      if (res.result === 'win' || res.result === 'blackjack') ElMessage.success(res.message)
      else if (res.result === 'lose') ElMessage.warning(res.message)
      else ElMessage.info(res.message)
    }
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || t('common.requestFailed'))
  } finally {
    acting.value = false
  }
}

const resultText = computed(() => {
  switch (state.value?.result) {
    case 'win':
      return t('bj.win')
    case 'blackjack':
      return 'Blackjack!'
    case 'lose':
      return t('bj.lose')
    case 'push':
      return t('bj.draw')
    default:
      return ''
  }
})

onMounted(() => act('new'))
</script>

<template>
  <div class="bj">
    <div class="hero">
      <div class="hero-title"><SptIcon name="gamepad" :size="16" aria-hidden="true" /> {{ name }}</div>
      <div class="balance">
        {{ t('bj.balance') }} <b>{{ state?.balance?.toLocaleString() ?? '—' }}</b>
      </div>
    </div>

    <section v-if="state" class="glass section">
      <div class="dealer">
        <div class="who">{{ t('bj.dealer') }}</div>
        <div class="cards">
          <span v-for="(c, i) in state.dealer_cards" :key="'d' + i" class="card" aria-hidden="true">{{ c }}</span>
          <span v-if="!state.dealer_cards?.length" class="text-muted">—</span>
        </div>
      </div>
      <div class="player">
        <div class="who">{{ t('bj.player') }}</div>
        <div class="cards">
          <span v-for="(c, i) in state.player_cards" :key="'p' + i" class="card" aria-hidden="true">{{ c }}</span>
          <span v-if="!state.player_cards?.length" class="text-muted">—</span>
        </div>
      </div>
      <div v-if="resultText" class="result-tag" :class="state.result">{{ resultText }}</div>
      <div v-if="state.message" class="msg text-muted">{{ state.message }}</div>
    </section>

    <section class="glass section ops">
      <button class="op" :disabled="acting || !finished" @click="act('new')">{{ t('bj.newRound') }}</button>
      <button class="op hit" :disabled="acting || finished" @click="act('hit')">{{ t('bj.hit') }}</button>
      <button class="op stand" :disabled="acting || finished" @click="act('stand')">{{ t('bj.stand') }}</button>
      <button class="op double" :disabled="acting || finished" @click="act('double')">{{ t('bj.double') }}</button>
    </section>
  </div>
</template>

<style scoped>
.bj {
  display: flex;
  flex-direction: column;
  gap: var(--sp-4);
  max-width: 640px;
  margin: 0 auto;
}
.hero {
  padding: 18px;
  border-radius: var(--radius-md);
  background: var(--color-surface-2);
  border: 1px solid var(--color-border);
  text-align: center;
}
.hero-title {
  font-size: var(--fs-title);
  font-weight: 800;
  color: var(--color-text);
}
.balance {
  margin-top: 4px;
  font-size: var(--fs-body);
  color: var(--color-text-secondary);
}
.balance b {
  color: var(--pt-warn-fg);
  font-size: var(--fs-lg);
  font-variant-numeric: tabular-nums;
}

.section {
  padding: 16px;
  border-radius: var(--radius-md);
}
.dealer,
.player {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
  padding: 6px 0;
}
.player {
  border-top: 1px solid var(--color-border);
  margin-top: 6px;
  padding-top: 12px;
}
.who {
  width: 44px;
  font-size: var(--fs-body);
  color: var(--color-text-muted);
  font-weight: 600;
}
.cards {
  display: flex;
  gap: var(--sp-1);
  flex-wrap: wrap;
}
.card {
  display: inline-block;
  min-width: 30px;
  padding: 6px 8px;
  border-radius: var(--radius-xs);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  color: var(--color-text);
  font-weight: 700;
  font-size: var(--fs-lg);
  text-align: center;
  box-shadow: var(--shadow-card);
}

.result-tag {
  margin-top: 12px;
  display: inline-block;
  padding: 6px 16px;
  border-radius: var(--radius-sm);
  font-weight: 800;
  font-size: var(--fs-lg);
}
.result-tag.win,
.result-tag.blackjack {
  background: var(--color-success-soft);
  color: var(--pt-success-fg);
}
.result-tag.lose {
  background: var(--color-danger-soft);
  color: var(--pt-danger-fg);
}
.result-tag.push {
  background: var(--color-surface-2);
  color: var(--color-text-muted);
}
.msg {
  margin-top: 8px;
  font-size: var(--fs-body);
}

.ops {
  display: flex;
  gap: var(--sp-2);
  flex-wrap: wrap;
  justify-content: center;
}
.op {
  padding: 10px 18px;
  min-height: 24px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  cursor: pointer;
  font-weight: 600;
  font-size: var(--fs-body);
  color: var(--color-text);
  background: var(--color-surface-2);
  transition: border-color var(--dur-fast) var(--ease), background-color var(--dur-fast) var(--ease);
}
.op.hit {
  color: var(--color-on-accent);
  background: var(--color-primary);
  border: 0;
}
.op.stand {
  color: var(--color-on-accent);
  background: var(--color-danger);
  border: 0;
}
.op.double {
  color: var(--color-on-accent);
  background: var(--color-warning);
  border: 0;
}
.op:hover:not(:disabled) {
  border-color: var(--color-border-strong);
  box-shadow: var(--shadow-card);
}
.op:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
