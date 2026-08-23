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
  color: #fff;
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
  background: linear-gradient(135deg, var(--pt-primary-bg), var(--pt-primary-2-bg));
}
.rating {
  color: var(--pt-warn, #f39c12);
}
.overview {
  font-size: var(--fs-xs);
  line-height: 1.4;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

/* 影视卡 staggered 入场 + hover 水润反馈 */
.grid > * { animation: pt-fade-in-up var(--dur-slow) var(--ease-out-smooth) both; }
.grid > *:nth-child(1) { animation-delay: 0ms; }
.grid > *:nth-child(2) { animation-delay: 40ms; }
.grid > *:nth-child(3) { animation-delay: 80ms; }
.grid > *:nth-child(4) { animation-delay: 120ms; }
.grid > *:nth-child(5) { animation-delay: 160ms; }
.grid > *:nth-child(6) { animation-delay: 200ms; }
.grid > *:nth-child(7) { animation-delay: 240ms; }
.grid > *:nth-child(8) { animation-delay: 280ms; }
.grid > *:nth-child(n+9) { animation-delay: 320ms; }
.mcard { transition: transform var(--dur-base) var(--ease-out-spring), box-shadow var(--dur-base) var(--ease-out-smooth); }
.mcard:hover { transform: translateY(-4px); box-shadow: var(--shadow-lg); }
.mcard .poster img { transition: transform var(--dur-slow) var(--ease-out-smooth); }
.mcard:hover .poster img { transform: scale(1.06); }
</style>
