<script setup lang="ts">
// 媒体详情页（对标 MoviePilot 详情）：点搜索结果卡片进入，展示海报/背景/简介/演职/推荐，
// 并提供「搜索资源」一键跨到种子搜索（按标题在站点找片）。route: /media-detail?source=&id=
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { MediaInfo } from '@/api/media'
import MediaPoster from '@/components/MediaPoster.vue'
import SptIcon from '@/components/SptIcon.vue'
import CelebrityDialog from '@/components/CelebrityDialog.vue'
import { useMediaDetail } from '@/composables/useMediaDetail'

const { t } = useI18n()

const {
  route, router, detail, loading, error, subscribing, moreKind,
  CAST_PREVIEW_LIMIT, RECOMMENDATIONS_PREVIEW_LIMIT,
  visibleCast, visibleRecommendations, moreVisible, moreTitle,
  celVisible, celLoading, celProfile,
  load, searchResources, subscribe, openRec, openMore, openCelebrity,
} = useMediaDetail()

const TYPE_LABELS = computed<Record<string, string>>(() => ({ movie: t('search.catMovie'), tv: t('search.catTv'), music: t('search.catMusic') }))
function typeBadge(k: string) {
  return TYPE_LABELS.value[k] || k
}
const SOURCE_LABELS = computed<Record<string, string>>(() => ({ tmdb: 'TMDB', douban: t('search.srcDouban'), bangumi: 'Bangumi', musicbrainz: 'MB', cunguangji: t('search.srcCunguangji'), xingguangji: t('search.srcCunguangji') }))
function sourceBadge(s: string) {
  return SOURCE_LABELS.value[s] || s
}
function genreText(m: MediaInfo) {
  return [m.genres?.join(' / '), m.runtime ? t('mediadetail.runtimeMin', { n: m.runtime }) : '', m.countries?.join(' / ')]
    .filter(Boolean).join('  |  ')
}
function factText(m: MediaInfo) {
  const sourceID = m.source_id.replace(/^(movie|tv|music):/, '')
  return [
    m.release_date ? t('mediadetail.released', { date: m.release_date }) : '',
    m.vote_count ? t('mediadetail.voters', { n: m.vote_count.toLocaleString() }) : '',
    sourceID ? `${sourceBadge(m.source)} ID ${sourceID}` : '',
  ].filter(Boolean).join('  ·  ')
}
</script>

<template>
  <div class="page">
    <div class="back-row">
      <el-button size="small" @click="router.back()">← {{ t('mediadetail.back') }}</el-button>
    </div>

    <div v-if="error" class="empty glass error-state">
      <div class="err-text">⚠ {{ error }}</div>
      <el-button size="small" @click="load">{{ t('common.retry') }}</el-button>
    </div>

    <div v-loading="loading">
      <template v-if="detail">
        <!-- 顶部：背景图 + 海报 + 标题/评分/操作 -->
        <div class="hero glass">
          <div class="backdrop">
            <MediaPoster
              v-if="detail.backdrop_url || detail.poster_url"
              class="backdrop-image"
              :url="detail.backdrop_url || detail.poster_url"
              :kind="detail.type"
            />
            <div class="overlay"></div>
          </div>
          <div class="hero-body">
            <div class="poster"><MediaPoster :url="detail.poster_url" :kind="detail.type" /></div>
            <div class="hero-meta">
              <h1 class="t-title">
                {{ detail.title }}
                <span v-if="detail.year" class="year">({{ detail.year }})</span>
              </h1>
              <div v-if="detail.original_title && detail.original_title !== detail.title" class="original-title text-muted">
                {{ detail.original_title }}
              </div>
              <div class="badges">
                <span class="badge type">{{ typeBadge(detail.type) }}</span>
                <span v-if="detail.rating" class="badge rating">⭐ {{ detail.rating.toFixed(1) }}</span>
                <span class="badge src">{{ sourceBadge(detail.source) }}</span>
              </div>
              <div v-if="genreText(detail)" class="genre-row text-muted">{{ genreText(detail) }}</div>
              <div v-if="detail.directors?.length" class="genre-row text-muted">{{ t('mediadetail.directors', { names: detail.directors.join(', ') }) }}</div>
              <div v-if="factText(detail)" class="fact-row text-muted">{{ factText(detail) }}</div>
              <div class="actions">
                <el-button type="primary" @click="searchResources">🔍 {{ t('mediadetail.searchRes') }}</el-button>
                <el-button :loading="subscribing" @click="subscribe">❤ {{ t('mediadetail.subscribe') }}</el-button>
              </div>
            </div>
          </div>
        </div>

        <!-- 简介 -->
        <div v-if="detail.overview" class="card glass section">
          <div class="sec-title">{{ t('mediadetail.overview') }}</div>
          <div class="overview">{{ detail.overview }}</div>
        </div>

        <!-- 演员 -->
        <div v-if="detail.cast?.length" class="card glass section">
          <div class="section-head">
            <div class="sec-title">{{ t('mediadetail.cast') }} <span class="sec-count">{{ detail.cast.length }}</span></div>
            <el-button
              v-if="detail.cast.length > CAST_PREVIEW_LIMIT"
              link class="section-more" aria-haspopup="dialog"
              @click="openMore('cast')"
            >
              {{ t('mediadetail.more') }}
              <SptIcon name="chevron-right" :size="15" />
            </el-button>
          </div>
          <div id="media-detail-cast" class="cast">
            <div v-for="(p, i) in visibleCast" :key="`${p.id || p.name}-${i}`" class="cast-item" :class="{ clickable: p.id }" @click="openCelebrity(p)">
              <div class="avatar"><MediaPoster :url="p.avatar || ''" kind="person" /></div>
              <div class="cname">{{ p.name }}</div>
              <div v-if="p.role" class="crole text-muted">{{ p.role }}</div>
            </div>
          </div>
        </div>

        <!-- 推荐 -->
        <div v-if="detail.recommendations?.length" class="card glass section">
          <div class="section-head">
            <div class="sec-title">{{ t('mediadetail.recommendations') }} <span class="sec-count">{{ detail.recommendations.length }}</span></div>
            <el-button
              v-if="detail.recommendations.length > RECOMMENDATIONS_PREVIEW_LIMIT"
              link class="section-more" aria-haspopup="dialog"
              @click="openMore('recommendations')"
            >
              {{ t('mediadetail.more') }}
              <SptIcon name="chevron-right" :size="15" />
            </el-button>
          </div>
          <div id="media-detail-recommendations" class="recs">
            <div
              v-for="(m, i) in visibleRecommendations"
              :key="`${m.source}-${m.source_id}-${i}`"
              class="rec glass-hover"
              @click="openRec(m)"
            >
              <div class="rec-poster"><MediaPoster :url="m.poster_url" :kind="m.type" /></div>
              <div class="rec-title">{{ m.title }}</div>
              <div v-if="m.rating" class="rec-rating text-muted">⭐ {{ m.rating.toFixed(1) }}</div>
            </div>
          </div>
        </div>

        <el-dialog v-model="moreVisible" :title="moreTitle" width="min(960px, calc(100% - 32px))" class="more-dialog" append-to-body>
          <div v-if="moreKind === 'cast'" class="more-list cast">
            <div v-for="(p, i) in detail.cast" :key="`${p.id || p.name}-${i}`" class="cast-item" :class="{ clickable: p.id }" @click="openCelebrity(p)">
              <div class="avatar"><MediaPoster :url="p.avatar || ''" kind="person" /></div>
              <div class="cname">{{ p.name }}</div>
              <div v-if="p.role" class="crole text-muted">{{ p.role }}</div>
            </div>
          </div>
          <div v-else class="more-list recs">
            <div v-for="(m, i) in detail.recommendations" :key="`${m.source}-${m.source_id}-${i}`" class="rec glass-hover" @click="openRec(m)">
              <div class="rec-poster"><MediaPoster :url="m.poster_url" :kind="m.type" /></div>
              <div class="rec-title">{{ m.title }}</div>
              <div v-if="m.rating" class="rec-rating text-muted">⭐ {{ m.rating.toFixed(1) }}</div>
            </div>
          </div>
        </el-dialog>

        <!-- 演员简介弹窗（点 cast 头像） -->
        <CelebrityDialog v-model:visible="celVisible" v-model:loading="celLoading" v-model:profile="celProfile" />
      </template>
    </div>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: var(--sp-4); }
.back-row { display: flex; }
.t-title { font-size: var(--fs-title); font-weight: 700; margin: 0; }
.year { font-size: var(--fs-lg); font-weight: 400; color: var(--text-muted); margin-left: 6px; }
.original-title { font-size: var(--fs-body); line-height: 1.4; }
/* hero */
.hero { position: relative; padding: 0; overflow: hidden; }
.backdrop { position: absolute; inset: 0; overflow: hidden; background: var(--surface-muted); }
.backdrop :deep(.backdrop-image) { position: absolute; inset: 0; width: 100%; height: 100%; }
.backdrop :deep(img.backdrop-image) { object-fit: cover; filter: blur(8px) saturate(0.9); transform: scale(1.06); opacity: 0.72; }
.backdrop :deep(.no-poster.backdrop-image) { display: none; }
.overlay { position: absolute; inset: 0; background: linear-gradient(to top, var(--bg, #fff) 26%, transparent 72%); }
.hero-body { position: relative; z-index: 1; display: flex; gap: var(--sp-5); align-items: flex-end; padding: 48px 24px 24px; }
.poster { width: 150px; height: 220px; flex: none; border-radius: var(--radius-sm); overflow: hidden; box-shadow: var(--shadow-lg); background: var(--surface-muted); }
.hero-meta { flex: 1; display: flex; flex-direction: column; gap: var(--sp-2); }
.badges { display: flex; gap: var(--sp-2); flex-wrap: wrap; }
.badge { padding: 2px 10px; border-radius: var(--radius-pill); font-size: var(--fs-xs); }
.badge.type { background: linear-gradient(135deg, var(--pt-primary-bg), var(--pt-primary-2-bg)); }
.badge.rating { background: var(--pt-warn-bg); color: var(--pt-warn, #f39c12); }
.badge.src { background: var(--surface-muted); }
.genre-row { font-size: var(--fs-body); line-height: 1.6; }
.fact-row { font-size: var(--fs-xs); line-height: 1.6; }
.actions { margin-top: 6px; }
.section { padding: 18px; }
.section-head { min-height: 32px; display: flex; align-items: flex-start; justify-content: space-between; gap: var(--sp-3); margin-bottom: 12px; }
.sec-title { font-weight: 700; font-size: var(--fs-lg); line-height: 32px; margin-bottom: 12px; }
.section-head .sec-title { margin-bottom: 0; }
.sec-count { margin-left: 4px; color: var(--text-muted); font-size: var(--fs-xs); font-weight: 500; }
.section-more { flex: none; gap: var(--sp-1); }
.overview { font-size: var(--fs-md); line-height: 1.8; white-space: pre-line; }
/* cast */
.cast { display: grid; grid-template-columns: repeat(auto-fill, minmax(84px, 1fr)); gap: var(--sp-4); }
.cast-item { min-width: 0; text-align: center; }
.avatar { width: 84px; height: 84px; border-radius: 50%; overflow: hidden; background: var(--surface-muted); margin: 0 auto 6px; }
.cname { font-size: var(--fs-xs); font-weight: 600; line-height: 1.3; }
.crole { font-size: var(--fs-2xs); line-height: 1.3; }
/* recs */
.recs { display: grid; grid-template-columns: repeat(auto-fill, minmax(130px, 1fr)); gap: var(--sp-3); }
.rec { padding: 0; overflow: hidden; border-radius: var(--radius-sm); cursor: pointer; background: var(--surface-muted); }
.rec-poster { aspect-ratio: 2/3; background: var(--surface-muted); }
.rec-title { padding: 8px 10px 2px; font-size: var(--fs-body); font-weight: 600; line-height: 1.3; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
.rec-rating { padding: 0 10px 10px; font-size: var(--fs-xs); }
.more-list { max-height: min(68vh, 680px); overflow-y: auto; padding: 4px; }
.more-dialog :deep(.el-dialog__body) { padding-top: 4px; }
.empty { padding: 40px; text-align: center; }
.error-state { display: flex; flex-direction: column; align-items: center; gap: var(--sp-3); }
.err-text { color: var(--pt-danger); }
@media (max-width: 640px) {
  .hero-body { flex-direction: column; align-items: flex-start; padding: 32px 16px 16px; }
  .poster { width: 120px; height: 176px; }
  .hero-meta { padding-top: 12px; }
}
.cast-item.clickable { cursor: pointer; transition: transform var(--dur-fast) var(--ease-out-smooth); }
.cast-item.clickable:hover { transform: translateY(-2px); }
</style>
