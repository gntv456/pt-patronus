<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { toRef } from 'vue'
import { useScratchCardGame, fmt } from '@/composables/useScratchCardGame'

const { t } = useI18n()
const props = defineProps<{ siteId: number }>()

const { stats, options, prizes, history, playing, buying, autoPlaying, autoCount, reveal, play, toggleAuto, buy } =
  useScratchCardGame(toRef(props, 'siteId'))
</script>

<template>
  <div class="scratch">
    <!-- 标题 -->
    <div class="banner">
      <span class="banner-title">{{ t('sc.title') }}</span>
      <span class="banner-sub">{{ t('sc.sub', { n: fmt(stats?.cost_per_play ?? 2000) }) }}</span>
    </div>

    <!-- 统计栏 -->
    <div class="stat-bar">
      <div class="stat-tile">
        <div class="stat-val">{{ stats?.card_number ?? '--' }}</div>
        <div class="stat-lbl">{{ t('sc.left') }}</div>
      </div>
      <div class="stat-tile">
        <div class="stat-val">{{ stats ? fmt(stats.balance) : '--' }}</div>
        <div class="stat-lbl">{{ t('sc.ingot') }}</div>
      </div>
      <div class="stat-tile">
        <div class="stat-val">{{ autoPlaying ? autoCount : history.length || 0 }}</div>
        <div class="stat-lbl">{{ autoPlaying ? t('sc.roundDone') : t('sc.recent') }}</div>
      </div>
    </div>

    <!-- 奖品展示 -->
    <div v-if="prizes.length" class="prizes">
      <div class="section-label">{{ t('sc.prizes') }}</div>
      <div class="prize-row">
        <div v-for="p in prizes" :key="p.id" class="prize-chip">
          <span class="prize-icon" aria-hidden="true">{{ p.name.slice(0, 1) }}</span>
          <span class="prize-name">{{ p.name }}</span>
        </div>
      </div>
    </div>

    <!-- 刮卡区 -->
    <div class="card-zone">
      <div class="scratch-card" :class="{ revealed: reveal, win: reveal?.success && (reveal?.amount ?? 0) > 0 }">
        <div v-if="!reveal" class="cover">
          <span class="cover-text">{{ t('msg1.area') }}</span>
          <span class="cover-hint">{{ t('msg1.hint') }}</span>
        </div>
        <div v-else class="prize-reveal">
          <div class="reveal-icon">
            <SptIcon v-if="reveal.amount && reveal.amount > 0" name="party-popper" :size="28" aria-hidden="true" />
            <template v-else>·</template>
          </div>
          <div class="reveal-name">{{ reveal.prize || (reveal.success ? t('sp.thanks') : t('msg1.noWin')) }}</div>
          <div v-if="reveal.amount && reveal.amount > 0" class="reveal-amount">×{{ fmt(reveal.amount) }}</div>
          <div v-if="reveal.message" class="reveal-msg">{{ reveal.message }}</div>
        </div>
      </div>
    </div>

    <!-- 操作按钮 -->
    <div class="actions">
      <button class="btn btn-gold" :disabled="playing || buying || (stats?.card_number ?? 0) <= 0" @click="play">
        {{ playing ? t('msg1.scratching') : t('msg1.scratchOne') }}
      </button>
      <button class="btn btn-auto" :class="{ active: autoPlaying }" @click="toggleAuto">
        {{ autoPlaying ? t('sc.stopN', { n: autoCount }) : t('sc.auto') }}
      </button>
    </div>

    <!-- 购卡 -->
    <div v-if="options.length" class="buy">
      <div class="section-label">{{ t('sc.buy') }}</div>
      <div class="buy-row">
        <button v-for="opt in options" :key="opt.type" class="buy-btn" :disabled="buying" @click="buy(opt)">
          <span class="buy-title">{{
            opt.gift_count > 0
              ? t('sc.buyGift', { b: opt.buy_count, g: opt.gift_count })
              : t('sc.buyN', { n: opt.buy_count })
          }}</span>
          <span class="buy-price">{{ t('sc.ingotN', { n: fmt(opt.buy_count * opt.unit_price) }) }}</span>
        </button>
      </div>
    </div>

    <!-- 抽奖记录 -->
    <div v-if="history.length" class="history">
      <div class="section-label">{{ t('sc.history') }}</div>
      <div class="hist-list">
        <div v-for="(r, i) in history.slice(0, 12)" :key="i" class="hist-item">
          <span class="hist-prize" :class="{ miss: !r.amount }">{{ r.prize_name || t('sp.thanks') }}</span>
          <span v-if="r.amount > 0" class="hist-amt">×{{ fmt(r.amount) }}</span>
          <span class="hist-time">{{ r.created_at }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped src="./scratch-card.css"></style>
