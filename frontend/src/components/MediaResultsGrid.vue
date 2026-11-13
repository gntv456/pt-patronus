<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
/**
 * 影视搜索结果海报卡片网格（原 Search.vue 影视模式区块）。
 * 卡片点击 → 进入媒体详情页，由父组件处理跳转。
 */
import { useI18n } from 'vue-i18n'
import MediaPoster from '@/components/MediaPoster.vue'
import EmptyState from '@/components/EmptyState.vue'
import type { MediaInfo } from '@/api/media'

const { t } = useI18n()

defineProps<{
  results: MediaInfo[]
  loading: boolean
}>()

const emit = defineEmits<{ open: [m: MediaInfo] }>()

function typeBadge(v: string) {
  return ({ movie: t('mrg.movie'), tv: t('mrg.tv'), music: t('mrg.music') } as Record<string, string>)[v] || v
}
function sourceBadge(v: string) {
  return ({ tmdb: 'TMDB', douban: t('mrg.douban'), musicbrainz: 'MB' } as Record<string, string>)[v] || v
}
</script>

<template>
  <div v-loading="loading" :aria-busy="loading" class="grid">
    <template v-if="loading">
      <PtSkeleton v-for="i in 8" :key="i" height="280px" />
    </template>
    <div
      v-a11y-button
      v-for="(m, i) in results"
      :key="i"
      class="solid-card mcard clickable"
      @click="emit('open', m)"
    >
      <div class="poster">
        <MediaPoster :url="m.poster_url" :kind="m.type" />
        <span class="src">{{ sourceBadge(m.source) }}</span>
      </div>
      <div class="meta">
        <div class="mtitle">{{ m.title }}</div>
        <div class="sub">
          <span class="badge">{{ typeBadge(m.type) }}</span>
          <span v-if="m.year" class="year">{{ m.year }}</span>
          <span v-if="m.rating" class="rating"><SptIcon name="star" :size="14" aria-hidden="true" /> {{ m.rating.toFixed(1) }}</span>
        </div>
        <div v-if="m.overview" class="overview text-muted">{{ m.overview }}</div>
      </div>
    </div>
  </div>
  <EmptyState v-if="!results.length && !loading"  :title="t('mrg.title')" :description="t('mrg.desc')" />
</template>

<style scoped>
/* 影视卡片网格 */
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: var(--sp-4);
  min-height: 120px;
}
.mcard {
  padding: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}
.mcard.clickable {
  cursor: pointer;
}
.mcard .poster {
  position: relative;
  aspect-ratio: 2/3;
  background: var(--surface-muted);
}
.src {
  position: absolute;
  top: 8px;
  right: 8px;
  padding: 2px 8px;
  border-radius: var(--radius-xs);
  background: rgba(0, 0, 0, 0.5);
  color: var(--color-text-inverse);
  font-size: var(--fs-2xs);
}
.meta {
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
}
.mtitle {
  font-weight: 600;
  font-size: var(--fs-md);
  line-height: 1.3;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.sub {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  font-size: var(--fs-xs);
}
.badge {
  padding: 1px 7px;
  border-radius: var(--radius-xs);
  background: var(--color-primary-soft);
  color: var(--color-primary);
}
.rating {
  color: var(--pt-warn-fg);
}
.overview {
  font-size: var(--fs-xs);
  line-height: 1.4;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

/* 入场动画、hover 位移与图片缩放已移除（去 AI 感） */
</style>
