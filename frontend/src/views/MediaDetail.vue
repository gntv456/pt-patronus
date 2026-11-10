<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

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
  route,
  router,
  detail,
  loading,
  error,
  subscribing,
  moreKind,
  CAST_PREVIEW_LIMIT,
  RECOMMENDATIONS_PREVIEW_LIMIT,
  visibleCast,
  visibleRecommendations,
  moreVisible,
  moreTitle,
  celVisible,
  celLoading,
  celProfile,
  load,
  searchResources,
  subscribe,
  openRec,
  openMore,
  openCelebrity,
} = useMediaDetail()

const TYPE_LABELS = computed<Record<string, string>>(() => ({
  movie: t('search.catMovie'),
  tv: t('search.catTv'),
  music: t('search.catMusic'),
}))
function typeBadge(k: string) {
  return TYPE_LABELS.value[k] || k
}
const SOURCE_LABELS = computed<Record<string, string>>(() => ({
  tmdb: 'TMDB',
  douban: t('search.srcDouban'),
  bangumi: 'Bangumi',
  musicbrainz: 'MB',
  cunguangji: t('search.srcCunguangji'),
  xingguangji: t('search.srcCunguangji'),
}))
function sourceBadge(s: string) {
  return SOURCE_LABELS.value[s] || s
}
function genreText(m: MediaInfo) {
  return [
    m.genres?.join(' / '),
    m.runtime ? t('mediadetail.runtimeMin', { n: m.runtime }) : '',
    m.countries?.join(' / '),
  ]
    .filter(Boolean)
    .join('  |  ')
}
function factText(m: MediaInfo) {
  const sourceID = m.source_id.replace(/^(movie|tv|music):/, '')
  return [
    m.release_date ? t('mediadetail.released', { date: m.release_date }) : '',
    m.vote_count ? t('mediadetail.voters', { n: m.vote_count.toLocaleString() }) : '',
    sourceID ? `${sourceBadge(m.source)} ID ${sourceID}` : '',
  ]
    .filter(Boolean)
    .join('  ·  ')
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
              <div
                v-if="detail.original_title && detail.original_title !== detail.title"
                class="original-title text-muted"
              >
                {{ detail.original_title }}
              </div>
              <div class="badges">
                <span class="badge type">{{ typeBadge(detail.type) }}</span>
                <span v-if="detail.rating" class="badge rating">⭐ {{ detail.rating.toFixed(1) }}</span>
                <span class="badge src">{{ sourceBadge(detail.source) }}</span>
              </div>
              <div v-if="genreText(detail)" class="genre-row text-muted">{{ genreText(detail) }}</div>
              <div v-if="detail.directors?.length" class="genre-row text-muted">
                {{ t('mediadetail.directors', { names: detail.directors.join(', ') }) }}
              </div>
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
            <div class="sec-title">
              {{ t('mediadetail.cast') }} <span class="sec-count">{{ detail.cast.length }}</span>
            </div>
            <el-button
              v-if="detail.cast.length > CAST_PREVIEW_LIMIT"
              link
              class="section-more"
              aria-haspopup="dialog"
              @click="openMore('cast')"
            >
              {{ t('mediadetail.more') }}
              <SptIcon name="chevron-right" :size="15" />
            </el-button>
          </div>
          <div id="media-detail-cast" class="cast">
            <div
              v-a11y-button
              v-for="(p, i) in visibleCast"
              :key="`${p.id || p.name}-${i}`"
              class="cast-item"
              :class="{ clickable: p.id }"
              @click="openCelebrity(p)"
            >
              <div class="avatar"><MediaPoster :url="p.avatar || ''" kind="person" /></div>
              <div class="cname">{{ p.name }}</div>
              <div v-if="p.role" class="crole text-muted">{{ p.role }}</div>
            </div>
          </div>
        </div>

        <!-- 推荐 -->
        <div v-if="detail.recommendations?.length" class="card glass section">
          <div class="section-head">
            <div class="sec-title">
              {{ t('mediadetail.recommendations') }} <span class="sec-count">{{ detail.recommendations.length }}</span>
            </div>
            <el-button
              v-if="detail.recommendations.length > RECOMMENDATIONS_PREVIEW_LIMIT"
              link
              class="section-more"
              aria-haspopup="dialog"
              @click="openMore('recommendations')"
            >
              {{ t('mediadetail.more') }}
              <SptIcon name="chevron-right" :size="15" />
            </el-button>
          </div>
          <div id="media-detail-recommendations" class="recs">
            <div
              v-a11y-button
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

        <el-dialog
          v-model="moreVisible"
          :title="moreTitle"
          width="min(960px, calc(100% - 32px))"
          class="more-dialog"
          append-to-body
        >
          <div v-if="moreKind === 'cast'" class="more-list cast">
            <div
              v-a11y-button
              v-for="(p, i) in detail.cast"
              :key="`${p.id || p.name}-${i}`"
              class="cast-item"
              :class="{ clickable: p.id }"
              @click="openCelebrity(p)"
            >
              <div class="avatar"><MediaPoster :url="p.avatar || ''" kind="person" /></div>
              <div class="cname">{{ p.name }}</div>
              <div v-if="p.role" class="crole text-muted">{{ p.role }}</div>
            </div>
          </div>
          <div v-else class="more-list recs">
            <div
              v-a11y-button
              v-for="(m, i) in detail.recommendations"
              :key="`${m.source}-${m.source_id}-${i}`"
              class="rec glass-hover"
              @click="openRec(m)"
            >
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

<style scoped src="./MediaDetail.css"></style>
