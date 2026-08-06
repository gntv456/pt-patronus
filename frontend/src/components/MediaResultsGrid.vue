<script setup lang="ts">
/**
 * 影视搜索结果海报卡片网格（原 Search.vue 影视模式区块）。
 * 卡片点击 → 进入媒体详情页（对标 MoviePilot），由父组件处理跳转。
 */
import MediaPoster from '@/components/MediaPoster.vue'
import EmptyState from '@/components/EmptyState.vue'
import type { MediaInfo } from '@/api/media'

defineProps<{
  results: MediaInfo[]
  loading: boolean
}>()

const emit = defineEmits<{ open: [m: MediaInfo] }>()

function typeBadge(t: string) {
  return ({ movie: '电影', tv: '剧集', music: '音乐' } as Record<string, string>)[t] || t
}
function sourceBadge(s: string) {
  return ({ tmdb: 'TMDB', douban: '豆瓣', musicbrainz: 'MB' } as Record<string, string>)[s] || s
}
</script>

<template>
  <div v-loading="loading" class="grid">
    <div
      v-for="(m, i) in results"
      :key="i"
      class="glass glass-hover mcard clickable"
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
          <span v-if="m.rating" class="rating">⭐ {{ m.rating.toFixed(1) }}</span>
        </div>
        <div v-if="m.overview" class="overview text-muted">{{ m.overview }}</div>
      </div>
    </div>
  </div>
  <EmptyState
    v-if="!results.length && !loading"
    icon="🎬"
    title="搜索影视"
    description="输入关键字查找影视元数据、海报与评分"
  />
</template>

<style scoped>
/* 影视卡片网格 */
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 16px;
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
  border-radius: 8px;
  background: rgba(0, 0, 0, 0.5);
  color: #fff;
  font-size: 11px;
}
.meta {
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.mtitle {
  font-weight: 600;
  font-size: 14px;
  line-height: 1.3;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.sub {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
}
.badge {
  padding: 1px 7px;
  border-radius: 6px;
  background: linear-gradient(135deg, rgba(108, 92, 231, 0.3), rgba(66, 133, 244, 0.2));
}
.rating {
  color: var(--pt-warn, #f39c12);
}
.overview {
  font-size: 12px;
  line-height: 1.4;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
