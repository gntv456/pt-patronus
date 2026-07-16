<script setup lang="ts">
// 媒体详情页（对标 MoviePilot 详情）：点搜索结果卡片进入，展示海报/背景/简介/演职/推荐，
// 并提供「搜索资源」一键跨到种子搜索（按标题在站点找片）。route: /media-detail?source=&id=
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { mediaApi, type MediaInfo, type CreditPerson, type CelebrityProfile } from '@/api/media'
import { subscribeApi } from '@/api/subscribe'
import MediaPoster from '@/components/MediaPoster.vue'
import SptIcon from '@/components/SptIcon.vue'

const route = useRoute()
const router = useRouter()
const detail = ref<MediaInfo | null>(null)
const loading = ref(false)
const error = ref('')
const subscribing = ref(false)
const moreKind = ref<'cast' | 'recommendations' | null>(null)

const CAST_PREVIEW_LIMIT = 12
const RECOMMENDATIONS_PREVIEW_LIMIT = 8
const visibleCast = computed(() => (detail.value?.cast || []).slice(0, CAST_PREVIEW_LIMIT))
const visibleRecommendations = computed(() => (detail.value?.recommendations || []).slice(0, RECOMMENDATIONS_PREVIEW_LIMIT))
const moreVisible = computed({
  get: () => moreKind.value !== null,
  set: (visible: boolean) => {
    if (!visible) moreKind.value = null
  },
})
const moreTitle = computed(() => (moreKind.value === 'cast' ? '演员阵容' : '推荐'))

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
function factText(m: MediaInfo) {
  const sourceID = m.source_id.replace(/^(movie|tv|music):/, '')
  return [
    m.release_date ? `上映 ${m.release_date}` : '',
    m.vote_count ? `${m.vote_count.toLocaleString('zh-CN')} 人评分` : '',
    sourceID ? `${sourceBadge(m.source)} ID ${sourceID}` : '',
  ].filter(Boolean).join('  ·  ')
}

async function load() {
  const source = route.query.source as string
  const id = route.query.id as string
  if (!source || !id) {
    error.value = '缺少 source/id'
    return
  }
  moreKind.value = null
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

function openMore(kind: 'cast' | 'recommendations') {
  moreKind.value = kind
}

// 点演员头像 → 拉演员简介弹窗（豆瓣 /elessar/subject）
const celVisible = ref(false)
const celLoading = ref(false)
const celProfile = ref<CelebrityProfile | null>(null)
async function openCelebrity(p: CreditPerson) {
  if (!p.id || !detail.value) return
  celLoading.value = true
  celProfile.value = null
  celVisible.value = true
  try {
    celProfile.value = await mediaApi.celebrity(detail.value.source, p.id)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载演员简介失败')
    celVisible.value = false
  } finally {
    celLoading.value = false
  }
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
              <div v-if="detail.directors?.length" class="genre-row text-muted">导演：{{ detail.directors.join('、') }}</div>
              <div v-if="factText(detail)" class="fact-row text-muted">{{ factText(detail) }}</div>
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
          <div class="section-head">
            <div class="sec-title">演员阵容 <span class="sec-count">{{ detail.cast.length }}</span></div>
            <el-button
              v-if="detail.cast.length > CAST_PREVIEW_LIMIT"
              link
              class="section-more"
              aria-haspopup="dialog"
              @click="openMore('cast')"
            >
              更多
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
            <div class="sec-title">推荐 <span class="sec-count">{{ detail.recommendations.length }}</span></div>
            <el-button
              v-if="detail.recommendations.length > RECOMMENDATIONS_PREVIEW_LIMIT"
              link
              class="section-more"
              aria-haspopup="dialog"
              @click="openMore('recommendations')"
            >
              更多
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
        <el-dialog v-model="celVisible" :title="celProfile?.name || '演员简介'" width="560px" class="cel-dialog" align-center append-to-body>
          <div v-loading="celLoading" class="cel-body">
            <template v-if="celProfile">
              <div class="cel-head">
                <div class="cel-avatar"><MediaPoster :url="celProfile.avatar || ''" kind="person" /></div>
                <div class="cel-headinfo">
                  <div class="cel-name">{{ celProfile.name }}</div>
                  <div v-if="celProfile.latin_name" class="cel-latin text-muted">{{ celProfile.latin_name }}</div>
                  <div v-if="celProfile.short_info" class="cel-short text-muted">{{ celProfile.short_info }}</div>
                  <div v-if="celProfile.followed_count" class="cel-follow text-muted">{{ celProfile.followed_count.toLocaleString('zh-CN') }} 人关注</div>
                </div>
              </div>
              <div v-if="celProfile.info?.length" class="cel-info">
                <div v-for="(kv, i) in celProfile.info" :key="i" class="cel-inforow">
                  <span class="cel-label text-muted">{{ kv[0] }}</span>
                  <span class="cel-val">{{ kv[1] }}</span>
                </div>
              </div>
              <div v-if="celProfile.bio?.length" class="cel-section">
                <div class="cel-stitle">生平</div>
                <p v-for="(para, i) in celProfile.bio" :key="i" class="cel-bio">{{ para }}</p>
              </div>
              <div v-if="celProfile.works?.length" class="cel-section">
                <div class="cel-stitle">参演作品</div>
                <div class="cel-works">
                  <a v-for="(w, i) in celProfile.works" :key="i" :href="w.url" target="_blank" rel="noopener" class="cel-work">
                    <span class="cel-wtitle">{{ w.title }}</span>
                    <span v-if="w.role" class="cel-wrole text-muted">{{ w.role }}</span>
                  </a>
                </div>
              </div>
              <div v-if="celProfile.douban_url" class="cel-footer">
                <a :href="celProfile.douban_url" target="_blank" rel="noopener" class="text-muted">查看豆瓣页面 ↗</a>
              </div>
            </template>
          </div>
        </el-dialog>
      </template>
    </div>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: 16px; }
.back-row { display: flex; }
.t-title { font-size: 24px; font-weight: 700; margin: 0; }
.year { font-size: 16px; font-weight: 400; color: var(--text-muted); margin-left: 6px; }
.original-title { font-size: 13px; line-height: 1.4; }

/* hero */
.hero { position: relative; padding: 0; overflow: hidden; }
.backdrop { position: absolute; inset: 0; overflow: hidden; background: var(--surface-muted); }
.backdrop :deep(.backdrop-image) { position: absolute; inset: 0; width: 100%; height: 100%; }
.backdrop :deep(img.backdrop-image) { object-fit: cover; filter: blur(8px) saturate(0.9); transform: scale(1.06); opacity: 0.72; }
.backdrop :deep(.no-poster.backdrop-image) { display: none; }
.overlay { position: absolute; inset: 0; background: linear-gradient(to top, var(--bg, #fff) 26%, transparent 72%); }
.hero-body { position: relative; z-index: 1; display: flex; gap: 20px; align-items: flex-end; padding: 48px 24px 24px; }
.poster { width: 150px; height: 220px; flex: none; border-radius: 12px; overflow: hidden; box-shadow: 0 8px 24px rgba(0,0,0,0.25); background: var(--surface-muted); }
.hero-meta { flex: 1; display: flex; flex-direction: column; gap: 8px; }
.badges { display: flex; gap: 8px; flex-wrap: wrap; }
.badge { padding: 2px 10px; border-radius: 999px; font-size: 12px; }
.badge.type { background: linear-gradient(135deg, rgba(108,92,231,0.3), rgba(66,133,244,0.2)); }
.badge.rating { background: rgba(243,156,18,0.16); color: var(--pt-warn, #f39c12); }
.badge.src { background: var(--surface-muted); }
.genre-row { font-size: 13px; line-height: 1.6; }
.fact-row { font-size: 12px; line-height: 1.6; }
.actions { margin-top: 6px; }

.section { padding: 18px; }
.section-head { min-height: 32px; display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; margin-bottom: 12px; }
.sec-title { font-weight: 700; font-size: 15px; line-height: 32px; margin-bottom: 12px; }
.section-head .sec-title { margin-bottom: 0; }
.sec-count { margin-left: 4px; color: var(--text-muted); font-size: 12px; font-weight: 500; }
.section-more { flex: none; gap: 3px; }
.overview { font-size: 14px; line-height: 1.8; white-space: pre-line; }

/* cast */
.cast { display: grid; grid-template-columns: repeat(auto-fill, minmax(84px, 1fr)); gap: 16px; }
.cast-item { min-width: 0; text-align: center; }
.avatar { width: 84px; height: 84px; border-radius: 50%; overflow: hidden; background: var(--surface-muted); margin: 0 auto 6px; }
.cname { font-size: 12px; font-weight: 600; line-height: 1.3; }
.crole { font-size: 11px; line-height: 1.3; }

/* recs */
.recs { display: grid; grid-template-columns: repeat(auto-fill, minmax(130px, 1fr)); gap: 14px; }
.rec { padding: 0; overflow: hidden; border-radius: 10px; cursor: pointer; background: var(--surface-muted); }
.rec-poster { aspect-ratio: 2/3; background: var(--surface-muted); }
.rec-title { padding: 8px 10px 2px; font-size: 13px; font-weight: 600; line-height: 1.3; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
.rec-rating { padding: 0 10px 10px; font-size: 12px; }

.more-list { max-height: min(68vh, 680px); overflow-y: auto; padding: 4px; }
.more-dialog :deep(.el-dialog__body) { padding-top: 4px; }

.empty { padding: 40px; text-align: center; }
.error-state { display: flex; flex-direction: column; align-items: center; gap: 12px; }
.err-text { color: var(--pt-danger); }

@media (max-width: 640px) {
  .hero-body { flex-direction: column; align-items: flex-start; padding: 32px 16px 16px; }
  .poster { width: 120px; height: 176px; }
  .hero-meta { padding-top: 12px; }
}

.cast-item.clickable { cursor: pointer; transition: transform 0.15s; }
.cast-item.clickable:hover { transform: translateY(-2px); }

/* 演员简介弹窗 */
.cel-body { min-height: 120px; }
.cel-head { display: flex; gap: 16px; margin-bottom: 16px; }
.cel-avatar { width: 96px; height: 96px; flex: none; border-radius: 50%; overflow: hidden; background: var(--surface-muted); }
.cel-headinfo { flex: 1; min-width: 0; }
.cel-name { font-size: 18px; font-weight: 700; }
.cel-latin { font-size: 13px; margin-top: 2px; }
.cel-short { font-size: 13px; line-height: 1.5; margin-top: 8px; }
.cel-follow { font-size: 12px; margin-top: 6px; }
.cel-info { padding: 12px 0; border-top: 1px solid var(--border, rgba(128,128,128,0.15)); border-bottom: 1px solid var(--border, rgba(128,128,128,0.15)); }
.cel-inforow { font-size: 13px; line-height: 1.6; display: flex; gap: 12px; }
.cel-label { flex: none; width: 5em; color: var(--text-muted); }
.cel-val { flex: 1; min-width: 0; word-break: break-word; }
.cel-section { margin-top: 16px; }
.cel-stitle { font-weight: 700; font-size: 14px; margin-bottom: 8px; }
.cel-bio { font-size: 13px; line-height: 1.8; color: var(--text-muted); margin: 0 0 8px; }
.cel-works { display: flex; flex-direction: column; gap: 2px; }
.cel-work { display: flex; justify-content: space-between; gap: 12px; padding: 6px 8px; border-radius: 6px; font-size: 13px; text-decoration: none; color: inherit; }
.cel-work:hover { background: var(--surface-muted); }
.cel-wtitle { font-weight: 600; }
.cel-wrole { font-size: 12px; text-align: right; color: var(--text-muted); }
.cel-footer { margin-top: 16px; text-align: right; font-size: 13px; }
.cel-footer a { text-decoration: none; }
.cel-footer a:hover { text-decoration: underline; }
</style>
