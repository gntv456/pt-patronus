<script setup lang="ts">
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

const topCls = ['gold', 'silver', 'bronze']
function top(i: number): string {
  return topCls[i] ?? ''
}
</script>

<template>
  <aside class="side">
    <section class="glass panel">
      <div class="sec-title">🏆 用户排行</div>
      <div v-if="!state?.rankings?.length" class="empty">暂无排行</div>
      <template v-else>
        <div class="tabs">
          <button v-for="(r, i) in state.rankings" :key="r.key" class="tab" :class="{ on: rankTab === i }" @click="emit('update:rankTab', i)">{{ r.title }}</button>
        </div>
        <div class="rank-list">
          <div v-for="(e, i) in state.rankings[rankTab]?.entries ?? []" :key="i" class="rank-row">
            <span class="rank-no" :class="top(i)">{{ i + 1 }}</span>
            <span class="rank-name">{{ e.username }}</span>
            <span class="rank-amt">{{ fmt(Math.abs(e.amount)) }}</span>
            <span v-if="e.count" class="rank-cnt">{{ e.count }}次</span>
          </div>
          <div v-if="!(state.rankings[rankTab]?.entries?.length)" class="empty">暂无数据</div>
        </div>
      </template>

<style scoped>
.side { display: flex; flex-direction: column; gap: 14px; }
.panel { padding: 14px; border-radius: 14px; }
.sec-title { font-weight: 700; font-size: 15px; margin-bottom: 10px; }
.empty { font-size: 13px; color: var(--text-muted, #999); padding: 8px 0; }
/* 排行 */
.tabs { display: flex; flex-wrap: wrap; gap: 4px; margin-bottom: 8px; }
.tab { padding: 4px 10px; border-radius: 8px; border: 1px solid var(--border-soft, rgba(255,255,255,0.15)); background: transparent; color: var(--text-muted, #999); cursor: pointer; font-size: 12px; }
.tab.on { background: var(--pt-primary, #6c5ce7); color: #fff; border-color: transparent; }
.rank-list { display: flex; flex-direction: column; gap: 4px; }
.rank-row { display: flex; align-items: center; gap: 6px; padding: 4px 0; font-size: 12px; border-bottom: 1px solid var(--border-soft, rgba(255,255,255,0.05)); }
.rank-no { width: 18px; height: 18px; line-height: 18px; text-align: center; border-radius: 50%; background: rgba(255,255,255,0.1); font-size: 10px; }
.rank-no.gold { background: #f59e0b; color: #fff; }
.rank-no.silver { background: #9ca3af; color: #fff; }
.rank-no.bronze { background: #b45309; color: #fff; }
.rank-name { flex: 1; font-weight: 600; }
.rank-amt { color: #16a34a; font-weight: 600; }
.rank-cnt { color: var(--text-muted, #999); font-size: 10px; }
/* 任务/道具/红包 */
.task-row, .prop-row { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 6px 0; font-size: 12px; border-bottom: 1px solid var(--border-soft, rgba(255,255,255,0.05)); }
.task-info { display: flex; flex-direction: column; }
.task-title { font-weight: 600; }
.task-prog { font-size: 10px; color: var(--text-muted, #999); }
.task-st { font-size: 11px; color: var(--text-muted, #999); }
.mini { padding: 4px 10px; border-radius: 7px; border: 1px solid var(--border-soft, rgba(255,255,255,0.2)); background: var(--glass-bg, rgba(255,255,255,0.06)); color: inherit; cursor: pointer; font-size: 11px; }
.mini.ok { background: #16a34a; color: #fff; border-color: transparent; }
.mini.warn { background: #f59e0b; color: #fff; border-color: transparent; }
.mini.full { width: 100%; margin-top: 6px; padding: 7px; }
.rp-form { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 6px; margin-bottom: 6px; }
.rp-amt { width: 100%; min-width: 0; box-sizing: border-box; padding: 6px; border-radius: 7px; border: 1px solid var(--border-soft, rgba(255,255,255,0.15)); background: var(--glass-bg, rgba(255,255,255,0.06)); color: inherit; font-size: 12px; }
.rp-greet { width: 100%; padding: 6px; border-radius: 7px; border: 1px solid var(--border-soft, rgba(255,255,255,0.15)); background: var(--glass-bg, rgba(255,255,255,0.06)); color: inherit; font-size: 12px; margin-bottom: 6px; box-sizing: border-box; }
.rp-list { display: flex; flex-direction: column; gap: 4px; margin-top: 8px; }
.rp-item { padding: 6px; border-radius: 7px; border: 1px solid #dc2626; background: rgba(220,38,38,0.1); color: inherit; cursor: pointer; font-size: 12px; }
</style>
    </section>

    <!-- 任务 / 道具 -->
    <section v-if="state?.tasks?.length || state?.props?.length" class="glass panel">
      <div class="sec-title">🎯 任务 / 道具</div>
      <div v-for="t in state?.tasks ?? []" :key="t.id" class="task-row">
        <div class="task-info">
          <span class="task-title">{{ t.title }}</span>
          <span v-if="t.progress_text" class="task-prog">{{ t.progress_text }}</span>
        </div>
        <button v-if="t.status === 'rewardable'" class="mini ok" @click="emit('doTask', t, 'reward')">领奖</button>
        <button v-else-if="t.status === 'idle'" class="mini" @click="emit('doTask', t, 'claim')">接取</button>
        <span v-else class="task-st">{{ t.status === 'doing' ? '进行中' : '已完成' }}</span>
      </div>
      <!-- 好学亏返卡 -->
      <button v-if="isHxpt && state?.props?.some((p) => p.usable === false)" class="mini warn full" @click="emit('useRefund')">💳 使用亏返卡</button>
      <!-- 熊猫道具独立使用 -->
      <div v-for="p in isHxpt ? [] : state?.props ?? []" :key="p.id" class="prop-row">
        <span class="prop-name">{{ p.name }} ×{{ p.quantity }}</span>
        <button class="mini" @click="emit('useProp', p)">使用</button>
      </div>
    </section>

    <!-- 红包 -->
    <section class="glass panel">
      <div class="sec-title">🧧 红包</div>
      <div class="rp-form">
        <input :value="rpAmount" @input="emit('update:rpAmount', Number(($event.target as HTMLInputElement).value))" type="number" class="rp-amt" placeholder="总额" />
        <input :value="rpCount" @input="emit('update:rpCount', Number(($event.target as HTMLInputElement).value))" type="number" class="rp-amt" placeholder="个数" />
      </div>
      <input :value="rpGreeting" @input="emit('update:rpGreeting', ($event.target as HTMLInputElement).value)" class="rp-greet" placeholder="祝福语" />
      <button class="mini ok full" :disabled="rpSending" @click="emit('sendRed')">{{ rpSending ? '发送中…' : '发红包' }}</button>
      <div v-if="state?.redpackets?.length" class="rp-list">
        <button v-for="r in state.redpackets" :key="r.id" class="rp-item" @click="emit('claimRed', r)">
          🧧 {{ r.count }}个 / {{ fmt(r.total) }}
        </button>
      </div>
    </section>
  </aside>
</template>
