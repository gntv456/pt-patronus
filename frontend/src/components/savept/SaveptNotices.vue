<script setup lang="ts">
// PT监护室结构化公告区：按严重级别分区展示告警/消息，站名高亮。
import { computed } from 'vue'
import { buildNoticeSections, tokenizeNoticeText } from '@/utils/savept'
import type { SaveptResult, SaveptSiteCard } from '@/api/savept'

const props = defineProps<{
  alerts: SaveptResult['alerts']
  sites: SaveptSiteCard[]
}>()

const noticeSections = computed(() => buildNoticeSections(props.alerts ?? []))
</script>

<template>
      <div class="spt-notices">
        <div v-for="section in noticeSections" :key="section.key" class="spt-notice-card" :class="`spt-notice-card--${section.key}`">
          <div class="spt-notice-card__title">{{ section.title }}</div>
          <div v-if="section.items.length" class="spt-notice-card__content">
            <div v-for="(item, i) in section.items" :key="`${section.key}-${i}`" class="spt-notice-card__item">
              <template v-for="(token, ti) in tokenizeNoticeText(item, sites)" :key="`${section.key}-${i}-${ti}`">
                <span v-if="token.type === 'site'" class="spt-notice-tag" :class="`spt-notice-tag--${section.key}`">{{ token.text }}</span>
                <span v-else>{{ token.text }}</span>
              </template>
            </div>
          </div>
          <div v-else class="spt-notice-card__empty">暂无公告</div>
        </div>
      </div>
</template>

<style scoped>
.spt-notices { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--spt-gap-sm); }
.spt-notice-card {
  border-radius: var(--spt-radius);
  border: 1px solid rgba(var(--spt-border), var(--spt-border-opacity));
  background: rgb(var(--spt-surface));
  padding: 12px 14px; display: flex; flex-direction: column; gap: 8px;
}
.spt-notice-card__title { font-size: 0.84rem; font-weight: 700; line-height: 1.35; }
.spt-notice-card__content { display: flex; flex-direction: column; gap: 6px; }
.spt-notice-card__item { font-size: 0.8rem; line-height: 1.55; color: rgba(var(--spt-on-surface), 0.68); }
.spt-notice-tag { display: inline-flex; align-items: center; padding: 1px 8px; margin: 0 3px; border-radius: var(--spt-radius-pill); font-size: 0.72rem; font-weight: 600; line-height: 1.4; vertical-align: baseline; }
.spt-notice-tag--critical { background: rgba(239, 68, 68, 0.12); color: var(--spt-color-critical); }
.spt-notice-tag--success { background: rgba(34, 197, 94, 0.12); color: var(--spt-color-healthy); }
.spt-notice-tag--info { background: rgba(245, 158, 11, 0.14); color: #d97706; }
.spt-notice-tag--error { background: rgba(148, 163, 184, 0.16); color: rgba(var(--spt-on-surface), 0.68); }
.spt-notice-card__empty { font-size: 0.78rem; color: rgba(var(--spt-on-surface), 0.38); }
.spt-notice-card--critical { background: rgba(239, 68, 68, 0.04); border-color: rgba(239, 68, 68, 0.14); }
.spt-notice-card--critical .spt-notice-card__title { color: var(--spt-color-critical); }
.spt-notice-card--success { background: rgba(34, 197, 94, 0.04); border-color: rgba(34, 197, 94, 0.14); }
.spt-notice-card--success .spt-notice-card__title { color: var(--spt-color-healthy); }
.spt-notice-card--info { background: rgba(245, 158, 11, 0.05); border-color: rgba(245, 158, 11, 0.16); }
.spt-notice-card--info .spt-notice-card__title { color: var(--spt-color-anniv); }
.spt-notice-card--error { background: rgba(148, 163, 184, 0.06); border-color: rgba(148, 163, 184, 0.16); }
.spt-notice-card--error .spt-notice-card__title { color: rgba(var(--spt-on-surface), 0.58); }

</style>
