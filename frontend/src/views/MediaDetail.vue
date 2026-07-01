<script setup lang="ts">
// 媒体详情页（对标 MoviePilot 详情）：点搜索结果卡片进入，展示海报/背景/简介/演职/推荐，
// 并提供「搜索资源」一键跨到种子搜索（按标题在站点找片）。route: /media-detail?source=&id=
import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { mediaApi, type MediaInfo } from '@/api/media'
import { subscribeApi } from '@/api/subscribe'
import MediaPoster from '@/components/MediaPoster.vue'

const route = useRoute()
const router = useRouter()
const detail = ref<MediaInfo | null>(null)
const loading = ref(false)
const error = ref('')
const subscribing = ref(false)

function typeBadge(t: string) {
  return ({ movie: '电影', tv: '剧集', music: '音乐' } as Record<string, string>)[t] || t
}
function sourceBadge(s: string) {
  return ({ tmdb: 'TMDB', douban: '豆瓣', bangumi: 'Bangumi', musicbrainz: 'MB', xingguangji: '星光集' } as Record<string, string>)[s] || s
}
function genreText(m: MediaInfo) {
  return [m.genres?.join(' / '), m.runtime ? `${m.runtime} 分钟` : '', m.countries?.join(' / ')]
    .filter(Boolean).join('  |  ')
}

async function load() {
  const source = route.query.source as string
  const id = route.query.id as string
  if (!source || !id) {
    error.value = '缺少 source/id'
    return
  }
  loading.value = true
  error.value = ''
  try {
    detail.value = await mediaApi.detail(source, id)
  } catch (e: unknown) {
    error.value = (e as Error).message || '加载失败'
  } finally {
    loading.value = false
  }
}

// 一键用标题跨站搜种子（跳种子搜索 tab，预填关键词）
function searchResources() {
  if (!detail.value) return
  router.push({ path: '/search', query: { keyword: detail.value.title, mode: 'torrent' } })
}

// 订阅：创建后 cron 自动搜索+下载（命中推送到默认下载器 + 通知）。同源同 id 已订阅则幂等。
async function subscribe() {
  if (!detail.value) return
  subscribing.value = true
  try {
    await subscribeApi.create({
      source: detail.value.source,
      source_id: detail.value.source_id,
      title: detail.value.title,
      year: detail.value.year,
      type: detail.value.type,
      category: detail.value.type === 'tv' ? 'tv' : 'movie',
      poster_url: detail.value.poster_url,
    })
    ElMessage.success('已订阅，将自动搜索并下载到默认下载器')
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '订阅失败')
  } finally {
    subscribing.value = false
  }
}

// 点推荐项 → 换源/id 重载本页
function openRec(m: MediaInfo) {
  router.push({ path: '/media-detail', query: { source: m.source, id: m.source_id } })
}

watch(() => [route.query.source, route.query.id], load, { immediate: true })
</script>

<template>
  <div class="page">
    <div class="back-row">
      <el-button size="small" @click="router.back()">← 返回</el-button>
    </div>

    <div v-if="error" class="empty glass error-state">
      <div class="err-text">⚠ {{ error }}</div>
      <el-button size="small" @click="load">重试</el-button>
    </div>

    <div v-loading="loading">
      <template v-if="detail">
        <!-- 顶部：背景图 + 海报 + 标题/评分/操作 -->
        <div class="hero glass">
          <div class="backdrop">
            <img v-if="detail.backdrop_url" :src="detail.backdrop_url" alt="" />
            <div class="overlay"></div>
          </div>
          <div class="hero-body">
            <div class="poster"><MediaPoster :url="detail.poster_url" :kind="detail.type" /></div>
            <div class="hero-meta">
              <h1 class="t-title">
                {{ detail.title }}
                <span v-if="detail.year" class="year">({{ detail.year }})</span>
              </h1>
              <div class="badges">
                <span class="badge type">{{ typeBadge(detail.type) }}</span>
                <span v-if="detail.rating" class="badge rating">⭐ {{ detail.rating.toFixed(1) }}</span>
                <span class="badge src">{{ sourceBadge(detail.source) }}</span>
              </div>
              <div v-if="genreText(detail)" class="genre-row text-muted">{{ genreText(detail) }}</div>
              <div v-if="detail.directors?.length" class="genre-row text-muted">导演：{{ detail.directors.join('、') }}</div>
              <div class="actions">
                <el-button type="primary" @click="searchResources">🔍 搜索资源</el-button>
                <el-button :loading="subscribing" @click="subscribe">❤ 订阅</el-button>
              </div>
            </div>
          </div>
        </div>

        <!-- 简介 -->
        <div v-if="detail.overview" class="card glass section">
          <div class="sec-title">简介</div>
          <div class="overview">{{ detail.overview }}</div>
        </div>

        <!-- 演员 -->
        <div v-if="detail.cast?.length" class="card glass section">
          <div class="sec-title">演员阵容</div>
          <div class="cast">
            <div v-for="(p, i) in detail.cast" :key="i" class="cast-item">
              <div class="avatar"><MediaPoster :url="p.avatar || ''" kind="movie" /></div>
              <div class="cname">{{ p.name }}</div>
              <div v-if="p.role" class="crole text-muted">{{ p.role }}</div>
            </div>
          </div>
        </div>

        <!-- 推荐 -->
        <div v-if="detail.recommendations?.length" class="card glass section">
          <div class="sec-title">推荐</div>
          <div class="recs">
            <div
              v-for="(m, i) in detail.recommendations"
              :key="i"
              class="rec glass-hover"
              @click="openRec(m)"
            >
              <div class="rec-poster"><MediaPoster :url="m.poster_url" :kind="m.type" /></div>
              <div class="rec-title">{{ m.title }}</div>
              <div v-if="m.rating" class="rec-rating text-muted">⭐ {{ m.rating.toFixed(1) }}</div>
            </div>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: 16px; }
.back-row { display: flex; }
.t-title { font-size: 24px; font-weight: 700; margin: 0; }
.year { font-size: 16px; font-weight: 400; color: var(--text-muted); margin-left: 6px; }

/* hero */
.hero { padding: 0; overflow: hidden; }
.backdrop { position: relative; height: 240px; background: var(--surface-muted); }
.backdrop img { width: 100%; height: 100%; object-fit: cover; display: block; }
.overlay { position: absolute; inset: 0; background: linear-gradient(to top, var(--bg, #fff) 8%, transparent 60%); }
.hero-body { display: flex; gap: 20px; padding: 0 24px 24px; margin-top: -90px; position: relative; }
.poster { width: 150px; height: 220px; flex: none; border-radius: 12px; overflow: hidden; box-shadow: 0 8px 24px rgba(0,0,0,0.25); background: var(--surface-muted); }
.hero-meta { flex: 1; padding-top: 100px; display: flex; flex-direction: column; gap: 8px; }
.badges { display: flex; gap: 8px; flex-wrap: wrap; }
.badge { padding: 2px 10px; border-radius: 999px; font-size: 12px; }
.badge.type { background: linear-gradient(135deg, rgba(108,92,231,0.3), rgba(66,133,244,0.2)); }
.badge.rating { background: rgba(243,156,18,0.16); color: var(--pt-warn, #f39c12); }
.badge.src { background: var(--surface-muted); }
.genre-row { font-size: 13px; line-height: 1.6; }
.actions { margin-top: 6px; }

.section { padding: 18px; }
.sec-title { font-weight: 700; font-size: 15px; margin-bottom: 12px; }
.overview { font-size: 14px; line-height: 1.8; white-space: pre-line; }

/* cast */
.cast { display: flex; gap: 16px; overflow-x: auto; padding-bottom: 6px; }
.cast-item { flex: none; width: 84px; text-align: center; }
.avatar { width: 84px; height: 84px; border-radius: 50%; overflow: hidden; background: var(--surface-muted); margin-bottom: 6px; }
.cname { font-size: 12px; font-weight: 600; line-height: 1.3; }
.crole { font-size: 11px; line-height: 1.3; }

/* recs */
.recs { display: grid; grid-template-columns: repeat(auto-fill, minmax(130px, 1fr)); gap: 14px; }
.rec { padding: 0; overflow: hidden; border-radius: 10px; cursor: pointer; background: var(--surface-muted); }
.rec-poster { aspect-ratio: 2/3; background: var(--surface-muted); }
.rec-title { padding: 8px 10px 2px; font-size: 13px; font-weight: 600; line-height: 1.3; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
.rec-rating { padding: 0 10px 10px; font-size: 12px; }

.empty { padding: 40px; text-align: center; }
.error-state { display: flex; flex-direction: column; align-items: center; gap: 12px; }
.err-text { color: var(--pt-danger); }

@media (max-width: 640px) {
  .hero-body { flex-direction: column; margin-top: -60px; padding: 0 16px 16px; }
  .poster { width: 120px; height: 176px; }
  .hero-meta { padding-top: 12px; }
}
</style>
