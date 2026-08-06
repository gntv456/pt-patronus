<script setup lang="ts">
import { onMounted, toRef } from 'vue'
import { useJggGame, prizes, clockwise, layout } from '@/composables/useJggGame'

const props = defineProps<{ siteId: number }>()

const {
  stats, spinning, highlight, result, bulk, bulkCount, bulkSpinning, records, bulkStats,
  refreshStats, refreshHistory, spin, drawMulti,
} = useJggGame(toRef(props, 'siteId'))

onMounted(() => {
  refreshStats()
  refreshHistory()
})
</script>

<template>
  <div class="jgg">
    <!-- 标题 -->
    <div class="banner">
      <span class="banner-title">九宫格抽奖</span>
      <span class="banner-sub">单次消耗 {{ Math.round(stats?.cost_per_play ?? 3000) }} 火花</span>
    </div>

    <!-- 统计栏 -->
    <div class="stat-bar">
      <div class="stat-tile">
        <div class="stat-val">{{ stats ? `${stats.today_spins}/${stats.max_daily || '∞'}` : '--' }}</div>
        <div class="stat-lbl">今日次数</div>
      </div>
      <div class="stat-tile">
        <div class="stat-val">{{ result ? Math.round(result.balance ?? 0) : (stats ? '—' : '--') }}</div>
        <div class="stat-lbl">{{ result ? '当前火花' : '火花余额' }}</div>
      </div>
    </div>

    <!-- 3×3 九宫格 -->
    <div class="grid">
      <template v-for="(row, ri) in layout" :key="ri">
        <template v-for="(pos, ci) in row" :key="ri + '-' + ci">
          <!-- 中心按钮 -->
          <button
            v-if="pos === -1"
            class="cell center"
            :disabled="spinning || bulkSpinning"
            @click="spin"
          >
            <span v-if="spinning || bulkSpinning" class="spinner" />
            <template v-else>
              <span class="center-icon">⚡</span>
              <span class="center-text">开始<br />抽奖</span>
            </template>
          </button>
          <!-- 奖品格 -->
          <div
            v-else
            class="cell prize"
            :class="{ lit: highlight === pos, win: result && highlight === pos && !spinning }"
            :style="highlight === pos && pos < clockwise.length ? { '--pc': clockwise[pos].color } : {}"
          >
            <span class="p-icon">{{ clockwise[pos]?.icon }}</span>
            <span class="p-name">{{ clockwise[pos]?.name }}</span>
          </div>
        </template>
      </template>
    </div>

    <!-- 结果 -->
    <div v-if="result" class="result" :class="result.success ? 'win' : 'lose'">
      <span class="badge">{{ result.success ? '🎉 中奖' : '✗ 失败' }}</span>
      <span v-if="result.prize" class="r-prize">{{ result.prize }}</span>
      <span v-if="result.amount" class="r-amt">×{{ result.amount }}</span>
      <div v-if="result.message" class="r-msg text-muted">{{ result.message }}</div>
    </div>

    <!-- 连抽（站点 /jgg/magic_grid_bulk.php 仅接受 10/20/50/100） -->
    <div class="multi">
      <button class="mbtn" :disabled="spinning || bulkSpinning" @click="drawMulti(10)"><span>🎀</span> 10连抽</button>
      <button class="mbtn" :disabled="spinning || bulkSpinning" @click="drawMulti(20)"><span>💖</span> 20连抽</button>
      <button class="mbtn" :disabled="spinning || bulkSpinning" @click="drawMulti(50)"><span>✨</span> 50连抽</button>
      <button class="mbtn" :disabled="spinning || bulkSpinning" @click="drawMulti(100)"><span>🌸</span> 100连抽</button>
    </div>

    <!-- 连抽结果（按奖品名分组统计） -->
    <div v-if="bulk" class="bulk">
      <div class="bulk-head">🎉 {{ bulkCount }} 连抽完成 · 中奖 {{ (bulk.prizes || []).filter((p) => p.is_win).length }}/{{ bulkCount }}</div>
      <div v-for="(s, i) in bulkStats" :key="i" class="bulk-row" :class="{ muted: s.name === '谢谢参与' }">
        <span>{{ s.name }}</span>
        <span class="bulk-cnt">×{{ s.count }}</span>
      </div>
      <div class="bulk-foot text-muted">
        消耗 {{ bulkCount * Math.round(stats?.cost_per_play ?? 3000) }} 火花 · 余额 {{ Math.round(bulk.balance ?? 0) }}
      </div>
    </div>

    <!-- 抽奖历史 -->
    <section v-if="records.length" class="history">
      <div class="hist-title">📜 抽奖历史</div>
      <div v-for="(r, i) in records.slice(0, 20)" :key="i" class="hist-row">
        <span :class="{ muted: r.prize_name === '谢谢参与' }">{{ r.prize_name }}</span>
        <span class="hist-cost">-{{ Math.round(r.cost) }}</span>
        <span class="hist-time text-muted">{{ r.created_at }}</span>
      </div>
    </section>
  </div>
</template>

<style scoped src="./jgg-game.css"></style>
