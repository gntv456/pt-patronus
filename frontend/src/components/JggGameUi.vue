<script setup lang="ts">
import { onMounted, toRef } from 'vue'
import { useI18n } from 'vue-i18n'
import { useJggGame, prizes, clockwise, layout } from '@/composables/useJggGame'

const { t } = useI18n()
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
      <span class="banner-title">{{ t('jgg.title') }}</span>
      <span class="banner-sub">{{ t('jgg.costLine', { n: Math.round(stats?.cost_per_play ?? 3000) }) }}</span>
    </div>

    <!-- 统计栏 -->
    <div class="stat-bar">
      <div class="stat-tile">
        <div class="stat-val">{{ stats ? `${stats.today_spins}/${stats.max_daily || '∞'}` : '--' }}</div>
        <div class="stat-lbl">{{ t('jgg.todayCount') }}</div>
      </div>
      <div class="stat-tile">
        <div class="stat-val">{{ result ? Math.round(result.balance ?? 0) : (stats ? '—' : '--') }}</div>
        <div class="stat-lbl">{{ result ? t('jgg.curSpark') : t('jgg.sparkBalance') }}</div>
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
              <span class="center-text">{{ t('sp.hub') }}</span>
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
      <span class="badge">{{ result.success ? '🎉 ' + t('wh.win') : '✗ ' + t('dl.fail') }}</span>
      <span v-if="result.prize" class="r-prize">{{ result.prize }}</span>
      <span v-if="result.amount" class="r-amt">×{{ result.amount }}</span>
      <div v-if="result.message" class="r-msg text-muted">{{ result.message }}</div>
    </div>

    <!-- 连抽（站点 /jgg/magic_grid_bulk.php 仅接受 10/20/50/100） -->
    <div class="multi">
      <button class="mbtn" :disabled="spinning || bulkSpinning" @click="drawMulti(10)"><span>🎀</span> {{ t('jgg.multi', { n: 10 }) }}</button>
      <button class="mbtn" :disabled="spinning || bulkSpinning" @click="drawMulti(20)"><span>💖</span> {{ t('jgg.multi', { n: 20 }) }}</button>
      <button class="mbtn" :disabled="spinning || bulkSpinning" @click="drawMulti(50)"><span>✨</span> {{ t('jgg.multi', { n: 50 }) }}</button>
      <button class="mbtn" :disabled="spinning || bulkSpinning" @click="drawMulti(100)"><span>🌸</span> {{ t('jgg.multi', { n: 100 }) }}</button>
    </div>

    <!-- 连抽结果（按奖品名分组统计） -->
    <div v-if="bulk" class="bulk">
      <div class="bulk-head">🎉 {{ t('jgg.bulkDone', { n: bulkCount, won: (bulk.prizes || []).filter((p) => p.is_win).length }) }}</div>
      <div v-for="(s, i) in bulkStats" :key="i" class="bulk-row" :class="{ muted: s.name === t('jgg.thanks') }">
        <span>{{ s.name }}</span>
        <span class="bulk-cnt">×{{ s.count }}</span>
      </div>
      <div class="bulk-foot text-muted">
        {{ t('jgg.bulkFoot', { cost: bulkCount * Math.round(stats?.cost_per_play ?? 3000), balance: Math.round(bulk.balance ?? 0) }) }}
      </div>
    </div>

    <!-- 抽奖历史 -->
    <section v-if="records.length" class="history">
      <div class="hist-title">📜 {{ t('jgg.history') }}</div>
      <div v-for="(r, i) in records.slice(0, 20)" :key="i" class="hist-row">
        <span :class="{ muted: r.prize_name === t('jgg.thanks') }">{{ r.prize_name }}</span>
        <span class="hist-cost">-{{ Math.round(r.cost) }}</span>
        <span class="hist-time text-muted">{{ r.created_at }}</span>
      </div>
    </section>
  </div>
</template>

<style scoped src="./jgg-game.css"></style>
