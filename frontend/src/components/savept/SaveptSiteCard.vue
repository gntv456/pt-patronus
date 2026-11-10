<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// PT监护室单张站点卡片：状态徽标、图标、站庆彩带 canvas。
import { useI18n } from 'vue-i18n'
import { safeUrl } from '@/utils/safeUrl'
import {
  getAnniversaryDisplayText,
  getMpStatusMeta,
  getStatusMeta,
  isTodayAnniversaryText,
  siteTypeLabel,
} from '@/utils/savept'
import SptIcon from '@/components/SptIcon.vue'
import type { SaveptSiteCard as SiteCard } from '@/api/savept'

const { t } = useI18n()
// 服务端 open/close_time 缺失时下发的占位值（savept.ts 同款），判断时须比对原始值而非本地化文案
const sscUnknownSentinel = '\u672a\u77e5' // 未知：服务端占位值，须比对原始值而非本地化文案
const props = defineProps<{ card: SiteCard; year: string }>()
const emit = defineEmits<{
  (e: 'celebrationRef', key: string, el: Element | null): void
  (e: 'detail', card: SiteCard): void
}>()

function statusMeta(card: SiteCard) {
  return getStatusMeta(card.status)
}
function mpMeta(card: SiteCard) {
  return getMpStatusMeta(card.match_type)
}
</script>

<template>
  <a
    :href="safeUrl(props.card.url)"
    target="_blank"
    rel="noreferrer noopener"
    class="spt-card"
    :class="[
      `spt-card--${props.card.status || 'unknown'}`,
      { 'spt-card--anniversary-today': isTodayAnniversaryText(props.card.anniversary_text) },
    ]"
  >
    <div v-if="isTodayAnniversaryText(card.anniversary_text)" class="spt-card__celebration" aria-hidden="true">
      <canvas
        :ref="(el) => emit('celebrationRef', `${year}-${card.name || card.url}`, el as Element | null)"
        class="spt-card__celebration-canvas"
      />
      <span class="spt-card__spark spt-card__spark--one" />
      <span class="spt-card__spark spt-card__spark--two" />
      <span class="spt-card__spark spt-card__spark--three" />
      <span class="spt-card__spark spt-card__spark--four" />
    </div>
    <div class="spt-card__body">
      <div class="spt-card__badges">
        <div class="spt-card__badges-left">
          <span class="spt-tag spt-tag--type">{{ siteTypeLabel(card.site_type) }}</span>
          <span v-if="card.duration_text" class="spt-tag spt-tag--duration">
            <SptIcon name="clock" :size="12" />
            <span class="spt-tag__text">{{ card.duration_text }}</span>
          </span>
          <span
            v-if="card.anniversary_text"
            class="spt-tag spt-tag--anniversary"
            :class="{ 'spt-tag--anniversary-today': isTodayAnniversaryText(card.anniversary_text) }"
          >
            <SptIcon v-if="!isTodayAnniversaryText(card.anniversary_text)" name="party-popper" :size="12" />
            <span class="spt-tag__text">{{ getAnniversaryDisplayText(card.anniversary_text) }}</span>
          </span>
        </div>
      </div>

      <div class="spt-card__main">
        <img v-if="card.icon" class="spt-card__icon" :src="card.icon" :alt="card.name" loading="lazy" />
        <div class="spt-card__info">
          <div class="spt-card__name-row">
            <span class="spt-card__name-dot" :style="{ backgroundColor: statusMeta(card).color }" />
            <div class="spt-card__name">{{ card.name }}</div>
          </div>
          <div class="spt-card__sub">
            {{ t('ssc.open', { v: card.open_time || t('ssc.unknown') })
            }}<template v-if="card.close_time && card.close_time !== sscUnknownSentinel">
              {{ t('ssc.close', { v: card.close_time }) }}</template
            >
          </div>
          <div v-if="card.fault_duration" class="spt-card__sub spt-card__fault">
            ⏱ {{ t('ssc.rescue', { v: card.fault_duration }) }}
          </div>
          <div class="spt-card__sub spt-card__sub--mp">
            <span v-if="card.local_site_name" class="spt-card__mp-text">{{ card.local_site_name }}</span>
            <span class="spt-tag spt-tag--mp" :class="`spt-tag--mp-${mpMeta(card).chipClass}`">
              <SptIcon :name="mpMeta(card).icon" :size="12" />
              {{ mpMeta(card).label }}
            </span>
          </div>
        </div>
      </div>
    </div>

    <div v-if="statusMeta(card).icon" class="spt-card__status-icon" :style="{ color: statusMeta(card).color }">
      <SptIcon :name="statusMeta(card).icon" :size="18" />
    </div>

    <!-- 右下角详情按钮：点击弹出站点详情弹窗 -->
    <button
      class="spt-card__detail-btn"
      type="button"
      :aria-label="t('ssc.dTitle')"
      @click.prevent.stop="emit('detail', card)"
    >
      <SptIcon name="info" :size="13" />
      <span>{{ t('ssc.detailBtn') }}</span>
    </button>
  </a>
</template>

<style scoped src="./SaveptSiteCard.css"></style>
