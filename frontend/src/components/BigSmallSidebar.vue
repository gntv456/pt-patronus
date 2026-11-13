<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { BigSmallState, BigSmallTask, BigSmallProp, BigSmallRedpacket } from '@/api/entertainment'
import { fmt } from '@/composables/useBigSmallGame'

defineProps<{
  state: BigSmallState | null
  isHxpt: boolean
  rankTab: number
  rpAmount: number
  rpCount: number
  rpGreeting: string
  rpSending: boolean
}>()

const emit = defineEmits<{
  (e: 'update:rankTab', v: number): void
  (e: 'update:rpAmount', v: number): void
  (e: 'update:rpCount', v: number): void
  (e: 'update:rpGreeting', v: string): void
  (e: 'doTask', t: BigSmallTask, action: 'claim' | 'reward'): void
  (e: 'useRefund'): void
  (e: 'useProp', p: BigSmallProp): void
  (e: 'claimRed', r: BigSmallRedpacket): void
  (e: 'sendRed'): void
}>()

const { t: tr } = useI18n()

const topCls = ['gold', 'silver', 'bronze']
function top(i: number): string {
  return topCls[i] ?? ''
}
</script>

<template>
  <aside class="side">
    <section class="glass panel">
      <div class="sec-title"><SptIcon name="trophy" :size="16" aria-hidden="true" /> {{ tr('bss.rank') }}</div>
      <div v-if="!state?.rankings?.length" class="empty">{{ tr('bss.noRank') }}</div>
      <template v-else>
        <div class="tabs">
          <button
            v-for="(r, i) in state.rankings"
            :key="r.key"
            class="tab"
            :class="{ on: rankTab === i }"
            @click="emit('update:rankTab', i)"
          >
            {{ r.title }}
          </button>
        </div>
        <div class="rank-list">
          <div v-for="(e, i) in state.rankings[rankTab]?.entries ?? []" :key="i" class="rank-row">
            <span class="rank-no" :class="top(i)">{{ i + 1 }}</span>
            <span class="rank-name">{{ e.username }}</span>
            <span class="rank-amt">{{ fmt(Math.abs(e.amount)) }}</span>
            <span v-if="e.count" class="rank-cnt">{{ tr('bss.times', { n: e.count }) }}</span>
          </div>
          <div v-if="!state.rankings[rankTab]?.entries?.length" class="empty">{{ tr('common.noData') }}</div>
        </div>
      </template>
    </section>

    <!-- 任务 / 道具 -->
    <section v-if="state?.tasks?.length || state?.props?.length" class="glass panel">
      <div class="sec-title"><SptIcon name="target" :size="16" aria-hidden="true" /> {{ tr('bss.tasksProps') }}</div>
      <div v-for="t in state?.tasks ?? []" :key="t.id" class="task-row">
        <div class="task-info">
          <span class="task-title">{{ t.title }}</span>
          <span v-if="t.progress_text" class="task-prog">{{ t.progress_text }}</span>
        </div>
        <button v-if="t.status === 'rewardable'" class="mini ok" @click="emit('doTask', t, 'reward')">
          {{ tr('bss.claim') }}
        </button>
        <button v-else-if="t.status === 'idle'" class="mini" @click="emit('doTask', t, 'claim')">
          {{ tr('bss.accept') }}
        </button>
        <span v-else class="task-st">{{ t.status === 'doing' ? tr('bss.doing') : tr('bss.done') }}</span>
      </div>
      <!-- 好学亏返卡 -->
      <button
        v-if="isHxpt && state?.props?.some((p) => p.usable === false)"
        class="mini warn full"
        @click="emit('useRefund')"
      >
        <SptIcon name="package" :size="14" aria-hidden="true" /> {{ tr('bss.refundCard') }}
      </button>
      <!-- 熊猫道具独立使用 -->
      <div v-for="p in isHxpt ? [] : (state?.props ?? [])" :key="p.id" class="prop-row">
        <span class="prop-name">{{ p.name }} ×{{ p.quantity }}</span>
        <button class="mini" @click="emit('useProp', p)">{{ tr('bss.use') }}</button>
      </div>
    </section>

    <!-- 红包 -->
    <section class="glass panel">
      <div class="sec-title"><SptIcon name="gift" :size="16" aria-hidden="true" /> {{ tr('bg.redpack') }}</div>
      <div class="rp-form">
        <input
          :value="rpAmount"
          @input="emit('update:rpAmount', Number(($event.target as HTMLInputElement).value))"
          type="number"
          class="rp-amt"
          :placeholder="tr('bss.total')"
        />
        <input
          :value="rpCount"
          @input="emit('update:rpCount', Number(($event.target as HTMLInputElement).value))"
          type="number"
          class="rp-amt"
          :placeholder="tr('bss.count')"
        />
      </div>
      <input
        :value="rpGreeting"
        @input="emit('update:rpGreeting', ($event.target as HTMLInputElement).value)"
        class="rp-greet"
        :placeholder="tr('bss.greeting')"
      />
      <button class="mini ok full" :disabled="rpSending" @click="emit('sendRed')">
        {{ rpSending ? tr('bss.sending') : tr('bss.sendRed') }}
      </button>
      <div v-if="state?.redpackets?.length" class="rp-list">
        <button v-for="r in state.redpackets" :key="r.id" class="rp-item" @click="emit('claimRed', r)">
          <SptIcon name="gift" :size="14" aria-hidden="true" /> {{ tr('bss.redN', { n: r.count, v: fmt(r.total) }) }}
        </button>
      </div>
    </section>
  </aside>
</template>

<style scoped>
.side {
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
}
.panel {
  padding: 14px;
  border-radius: var(--radius-md);
}
.sec-title {
  font-weight: 700;
  font-size: var(--fs-lg);
  margin-bottom: 10px;
}
.empty {
  font-size: var(--fs-body);
  color: var(--color-text-muted);
  padding: 8px 0;
}
/* 排行 */
.tabs {
  display: flex;
  flex-wrap: wrap;
  gap: var(--sp-1);
  margin-bottom: 8px;
}
.tab {
  min-height: 24px;
  padding: 4px 10px;
  border-radius: var(--radius-xs);
  border: 1px solid var(--color-border);
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
  font-size: var(--fs-xs);
}
.tab.on {
  background: var(--color-primary);
  color: var(--color-on-accent);
  border-color: transparent;
}
.rank-list {
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
}
.rank-row {
  display: flex;
  align-items: center;
  gap: var(--sp-1);
  padding: 4px 0;
  font-size: var(--fs-xs);
  border-bottom: 1px solid var(--color-border);
}
.rank-no {
  width: 18px;
  height: 18px;
  line-height: 18px;
  text-align: center;
  border-radius: 50%;
  background: var(--color-surface-2);
  color: var(--color-text-secondary);
  font-size: 12px;
}
.rank-no.gold {
  background: var(--color-warning);
  color: var(--color-on-accent);
}
.rank-no.silver {
  background: var(--color-text-muted);
  color: var(--color-on-accent);
}
.rank-no.bronze {
  background: var(--chart-4);
  color: var(--color-on-accent);
}
.rank-name {
  flex: 1;
  font-weight: 600;
}
.rank-amt {
  color: var(--pt-success);
  font-weight: 600;
}
.rank-cnt {
  color: var(--color-text-muted);
  font-size: var(--fs-2xs);
}
/* 任务/道具/红包 */
.task-row,
.prop-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--sp-2);
  padding: 6px 0;
  font-size: var(--fs-xs);
  border-bottom: 1px solid var(--color-border);
}
.task-info {
  display: flex;
  flex-direction: column;
}
.task-title {
  font-weight: 600;
}
.task-prog {
  font-size: 12px;
  color: var(--color-text-muted);
}
.task-st {
  font-size: 12px;
  color: var(--color-text-muted);
}
.mini {
  min-height: 24px;
  padding: 4px 10px;
  border-radius: var(--radius-xs);
  border: 1px solid var(--color-border);
  background: var(--color-surface-2);
  color: inherit;
  cursor: pointer;
  font-size: 12px;
}
.mini.ok {
  background: var(--color-success);
  color: var(--color-on-accent);
  border-color: transparent;
}
.mini.warn {
  background: var(--color-warning);
  color: var(--color-on-accent);
  border-color: transparent;
}
.mini.full {
  width: 100%;
  margin-top: 6px;
  padding: 7px;
}
.rp-form {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--sp-1);
  margin-bottom: 6px;
}
.rp-amt {
  width: 100%;
  min-width: 0;
  box-sizing: border-box;
  padding: 6px;
  border-radius: var(--radius-xs);
  border: 1px solid var(--color-border);
  background: var(--color-surface-2);
  color: inherit;
  font-size: var(--fs-xs);
}
.rp-greet {
  width: 100%;
  padding: 6px;
  border-radius: var(--radius-xs);
  border: 1px solid var(--color-border);
  background: var(--color-surface-2);
  color: inherit;
  font-size: var(--fs-xs);
  margin-bottom: 6px;
  box-sizing: border-box;
}
.rp-list {
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
  margin-top: 8px;
}
.rp-item {
  padding: 6px;
  border-radius: var(--radius-xs);
  border: 1px solid var(--pt-danger);
  background: var(--pt-danger-bg);
  color: inherit;
  cursor: pointer;
  font-size: var(--fs-xs);
}
</style>
