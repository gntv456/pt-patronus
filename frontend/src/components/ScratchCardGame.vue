<script setup lang="ts">
import { toRef } from 'vue'
import { useScratchCardGame, fmt } from '@/composables/useScratchCardGame'

const props = defineProps<{ siteId: number }>()

const {
  stats, options, prizes, history,
  playing, buying, autoPlaying, autoCount, reveal,
  play, toggleAuto, buy,
} = useScratchCardGame(toRef(props, 'siteId'))
</script>

<template>
  <div class="scratch">
    <!-- 标题 -->
    <div class="banner">
      <span class="banner-title">富豪刮刮乐</span>
      <span class="banner-sub">每次消耗 1 张刮刮卡 · 单卡 {{ fmt(stats?.cost_per_play ?? 2000) }} 金元宝</span>
    </div>

    <!-- 统计栏 -->
    <div class="stat-bar">
      <div class="stat-tile">
        <div class="stat-val">{{ stats?.card_number ?? '--' }}</div>
        <div class="stat-lbl">剩余刮刮卡</div>
      </div>
      <div class="stat-tile">
        <div class="stat-val">{{ stats ? fmt(stats.balance) : '--' }}</div>
        <div class="stat-lbl">金元宝余额</div>
      </div>
      <div class="stat-tile">
        <div class="stat-val">{{ autoPlaying ? autoCount : (history.length || 0) }}</div>
        <div class="stat-lbl">{{ autoPlaying ? '本轮已刮' : '近期记录' }}</div>
      </div>
    </div>

    <!-- 奖品展示 -->
    <div v-if="prizes.length" class="prizes">
      <div class="section-label">奖品展示</div>
      <div class="prize-row">
        <div v-for="p in prizes" :key="p.id" class="prize-chip">
          <span class="prize-icon">{{ p.name.slice(0, 1) }}</span>
          <span class="prize-name">{{ p.name }}</span>
        </div>
      </div>
    </div>

    <!-- 刮卡区 -->
    <div class="card-zone">
      <div class="scratch-card" :class="{ revealed: reveal, win: reveal?.success && (reveal?.amount ?? 0) > 0 }">
        <div v-if="!reveal" class="cover">
          <span class="cover-text">刮奖区</span>
          <span class="cover-hint">点击下方按钮刮开</span>
        </div>
        <div v-else class="prize-reveal">
          <div class="reveal-icon">{{ reveal.amount && reveal.amount > 0 ? '🎊' : '·' }}</div>
          <div class="reveal-name">{{ reveal.prize || (reveal.success ? '谢谢参与' : '未中奖') }}</div>
          <div v-if="reveal.amount && reveal.amount > 0" class="reveal-amount">×{{ fmt(reveal.amount) }}</div>
          <div v-if="reveal.message" class="reveal-msg">{{ reveal.message }}</div>
        </div>
      </div>
    </div>

    <!-- 操作按钮 -->
    <div class="actions">
      <button class="btn btn-gold" :disabled="playing || buying || (stats?.card_number ?? 0) <= 0" @click="play">
        {{ playing ? '刮卡中…' : '刮一张' }}
      </button>
      <button class="btn btn-auto" :class="{ active: autoPlaying }" @click="toggleAuto">
        {{ autoPlaying ? `停止 (${autoCount})` : '自动刮卡' }}
      </button>
    </div>

    <!-- 购卡 -->
    <div v-if="options.length" class="buy">
      <div class="section-label">购买刮刮卡</div>
      <div class="buy-row">
        <button
          v-for="opt in options"
          :key="opt.type"
          class="buy-btn"
          :disabled="buying"
          @click="buy(opt)"
        >
          <span class="buy-title">{{ opt.gift_count > 0 ? `买${opt.buy_count}送${opt.gift_count}` : `${opt.buy_count}张` }}</span>
          <span class="buy-price">{{ fmt(opt.buy_count * opt.unit_price) }} 元宝</span>
        </button>
      </div>
    </div>

    <!-- 抽奖记录 -->
    <div v-if="history.length" class="history">
      <div class="section-label">抽奖记录</div>
      <div class="hist-list">
        <div v-for="(r, i) in history.slice(0, 12)" :key="i" class="hist-item">
          <span class="hist-prize" :class="{ miss: !r.amount }">{{ r.prize_name || '谢谢参与' }}</span>
          <span v-if="r.amount > 0" class="hist-amt">×{{ fmt(r.amount) }}</span>
          <span class="hist-time">{{ r.created_at }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped src="./scratch-card.css"></style>
