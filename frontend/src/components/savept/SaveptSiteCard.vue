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
const emit = defineEmits<{ (e: 'celebrationRef', key: string, el: Element | null): void }>()

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
  </a>
</template>

<style scoped>
.spt-card {
  position: relative;
  display: flex;
  border: 1px solid rgba(var(--spt-on-surface), 0.08);
  border-radius: var(--radius-sm);
  overflow: hidden;
  text-decoration: none;
  transition:
    border-color var(--dur-fast) var(--ease-out-smooth),
    box-shadow var(--dur-fast) var(--ease-out-smooth),
    transform var(--dur-fast) var(--ease-out-smooth);
}
.spt-card:hover {
  border-color: rgba(var(--spt-on-surface), 0.18);
  box-shadow: var(--shadow-md);
  transform: translateY(-1px);
}
.spt-card--healthy {
  background: var(--pt-success-bg);
}
.spt-card--critical {
  background: var(--pt-danger-bg);
}
.spt-card--closed {
  opacity: 0.7;
}
.spt-card--closed:hover {
  opacity: 0.88;
}
.spt-card--closed .spt-card__icon {
  filter: grayscale(1) saturate(0.2);
}
.spt-card--closed .spt-tag {
  background: rgba(148, 163, 184, 0.1);
  color: rgba(var(--spt-on-surface), 0.5);
}
.spt-card--closed .spt-card__status-icon {
  color: var(--spt-color-closed);
}
.spt-card__body {
  min-width: 0;
  padding: 10px 12px;
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--sp-1);
}
.spt-card__badges {
  display: flex;
  align-items: center;
  gap: var(--sp-1);
  min-height: 18px;
}
.spt-card__badges-left {
  display: flex;
  align-items: center;
  gap: var(--sp-1);
  flex-wrap: wrap;
}
.spt-tag {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--sp-1);
  padding: 2px 6px;
  border-radius: var(--radius-2xs);
  font-size: 0.66rem;
  font-weight: 600;
  line-height: 1.4;
  max-width: 100%;
}
.spt-tag__text {
  display: block;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.spt-tag :deep(svg) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.spt-tag--type {
  background: rgba(var(--spt-on-surface), 0.06);
  color: rgba(var(--spt-on-surface), 0.55);
}
.spt-tag--mp {
  width: fit-content;
  max-width: 100%;
  padding-inline: 7px;
}
.spt-tag--mp-owned {
  background: var(--pt-success-bg);
  color: var(--pt-success);
}
.spt-tag--mp-available {
  background: var(--pt-primary-2-bg);
  color: var(--pt-primary-2);
}
.spt-tag--mp-unsupported {
  background: rgba(148, 163, 184, 0.14);
  color: rgba(var(--spt-on-surface), 0.5);
}
.spt-tag--duration {
  background: rgba(var(--spt-primary), 0.08);
  color: rgb(var(--spt-primary));
}
.spt-tag--anniversary {
  background: var(--pt-warn-bg);
  color: var(--spt-color-anniv);
}
.spt-tag--anniversary-today {
  background: var(--pt-warn-bg);
  border: 1px solid var(--pt-warn-bg);
}

.spt-card--anniversary-today {
  border-color: var(--pt-warn-bg);
  box-shadow:
    0 0 0 1px var(--pt-warn-bg),
    0 4px 16px var(--pt-warn-bg);
}
.spt-card__celebration {
  position: absolute;
  inset: 0;
  pointer-events: none;
  z-index: 0;
}
.spt-card__celebration-canvas {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
}
.spt-card__spark {
  position: absolute;
  pointer-events: none;
  will-change: transform, opacity;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #fbbf24;
  opacity: 0;
  animation: spt-celebration-spark 2.4s ease-in-out infinite;
}
.spt-card__spark--one {
  top: 10px;
  left: 92px;
  animation-delay: 0s;
}
.spt-card__spark--two {
  top: 16px;
  left: 136px;
  width: 5px;
  height: 5px;
  animation-delay: 0.5s;
}
.spt-card__spark--three {
  top: 30px;
  right: 46px;
  animation-delay: 1.1s;
}
.spt-card__spark--four {
  top: 42px;
  right: 90px;
  width: 4px;
  height: 4px;
  animation-delay: 1.6s;
}
@keyframes spt-celebration-spark {
  0%,
  100% {
    opacity: 0;
    transform: scale(0.6);
  }
  50% {
    opacity: 1;
    transform: scale(1.3);
  }
}
@media (prefers-reduced-motion: reduce) {
  .spt-card__spark {
    animation: none !important;
    opacity: 0 !important;
  }
}

.spt-card__main {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
}
.spt-card__icon {
  width: 36px;
  height: 36px;
  object-fit: contain;
  border-radius: var(--radius-xs);
  flex-shrink: 0;
}
.spt-card__info {
  flex: 1;
  min-width: 0;
}
.spt-card__name-row {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  min-width: 0;
}
.spt-card__name-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}
.spt-card__name {
  font-size: 0.88rem;
  font-weight: 600;
  color: rgb(var(--spt-on-surface));
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.spt-card--closed .spt-card__name {
  color: rgba(var(--spt-on-surface), 0.5);
}
.spt-card--closed .spt-card__name-dot {
  background: var(--spt-color-closed);
}
.spt-card__sub {
  font-size: 0.75rem;
  color: rgba(var(--spt-on-surface), 0.55);
  line-height: 1.4;
}
.spt-card__fault {
  color: var(--spt-color-critical);
  font-weight: 600;
}
.spt-card__sub--mp {
  display: flex;
  align-items: center;
  gap: var(--sp-1);
  flex-wrap: wrap;
}
.spt-card__mp-text {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.72rem;
  color: rgba(var(--spt-on-surface), 0.45);
}
.spt-card--closed .spt-card__sub {
  color: rgba(var(--spt-on-surface), 0.4);
}
.spt-card__status-icon {
  position: absolute;
  top: 12px;
  right: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  z-index: 1;
  transition: transform var(--dur-base) var(--ease-out-smooth);
}
.spt-card:hover .spt-card__status-icon {
  transform: scale(1.15);
}

@media (max-width: 1200px) {
  /* grid handled by parent */
}
@media (max-width: 768px) {
  .spt-card__body {
    padding: 10px 36px 10px 12px;
    gap: var(--sp-1);
  }
  .spt-card__icon {
    width: 32px;
    height: 32px;
  }
  .spt-card__name {
    font-size: 0.84rem;
  }
  .spt-card__sub {
    font-size: 0.72rem;
  }
  .spt-card__status-icon {
    top: 11px;
    right: 10px;
    width: 18px;
    height: 18px;
  }
}
</style>
