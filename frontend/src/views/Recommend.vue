<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  recommendApi,
  recommendSources,
  recommendCategories,
  type MediaInfo,
  type CalendarEpisode,
} from '@/api/recommend'
import { discoverApi } from '@/api/discover'
import EmptyState from '@/components/EmptyState.vue'
import SiteTorrentFindDialog from '@/components/SiteTorrentFindDialog.vue'
import { useFindOnSites } from '@/composables/useFindOnSites'

const { t } = useI18n()
const router = useRouter()

// 两种数据态：信息流（默认）+ 关键词搜索结果（原「探索」并入后共用本页）。
// items 按是否处于搜索态返回对应数组；搜索结果一次性返回，不参与分页。
const feedItems = ref<MediaInfo[]>([])
const searchItems = ref<MediaInfo[]>([])
const searchMode = ref(false)
const items = computed<MediaInfo[]>(() => (searchMode.value ? searchItems.value : feedItems.value))

const loading = ref(false)
const searchLoading = ref(false)
const source = ref('')
const category = ref('movie')
const keyword = ref('')

// 分页：首页加载后用「加载更多」追加下一页。不同源每页条数不一，故用最小阈值
// 推断是否还有下一页（单页返回不足阈值 → 视为末页）。
const page = ref(1)
const loadingMore = ref(false)
const hasMore = ref(true)
const PAGE_MIN_HINT = 8

// find-on-sites 弹窗（composable 封装）
const { findVisible, findLoading, findTitle, findPoster, findResults, findOnSites, closeFindDialog, abortFind } =
  useFindOnSites()

let abortCtrl: AbortController | null = null

async function load(refresh = false) {
  abortCtrl?.abort()
  const ctrl = new AbortController()
  abortCtrl = ctrl
  loading.value = true
  searchMode.value = false
  page.value = 1
  hasMore.value = true
  feedItems.value = []
  try {
    await recommendApi.feedStream(
      source.value,
      category.value,
      1,
      refresh,
      (items) => {
        feedItems.value.push(...items)
      },
      ctrl.signal,
    )
    if (feedItems.value.length < PAGE_MIN_HINT) hasMore.value = false
  } catch (e: unknown) {
    // 切换筛选时 abort 上一次流，AbortError 是预期，不报错
    if ((e as Error).name !== 'AbortError') ElMessage.error((e as Error).message || t('recommend.loadFailed'))
  } finally {
    if (abortCtrl === ctrl) {
      loading.value = false
      abortCtrl = null
    }
  }
}

async function loadMore() {
  if (loadingMore.value || !hasMore.value) return
  loadingMore.value = true
  const next = page.value + 1
  try {
    const more = await recommendApi.feed(source.value, category.value, next, false)
    if (!more.length) {
      hasMore.value = false
    } else {
      feedItems.value.push(...more)
      page.value = next
      if (more.length < PAGE_MIN_HINT) hasMore.value = false
    }
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('recommend.loadMoreFailed'))
  } finally {
    loadingMore.value = false
  }
}

// 关键词跨源搜索（原「探索」页）：搜索态接管列表，沿用当前选中的源筛选。
async function doSearch() {
  const kw = keyword.value.trim()
  if (!kw) {
    ElMessage.warning(t('search.emptyKeyword'))
    return
  }
  searchLoading.value = true
  searchMode.value = true
  try {
    searchItems.value = await discoverApi.search(kw, 0, source.value)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('search.searchFailed'))
    searchMode.value = false
  } finally {
    searchLoading.value = false
  }
}

// 清空关键词 → 退出搜索态回到信息流。
function clearSearch() {
  if (searchMode.value) {
    searchMode.value = false
    searchItems.value = []
  }
}

// 顶部刷新：搜索态重搜，否则刷信息流。
function refresh() {
  if (searchMode.value) doSearch()
  else load(true)
}

const sourceLabel = (key: string) => recommendSources.find((s) => s.key === key)?.label ?? key

// 与搜索页 MediaResultsGrid 同款徽标：类型/来源统一中文展示。
const TYPE_LABELS = computed<Record<string, string>>(() => ({
  movie: t('search.catMovie'),
  tv: t('search.catTv'),
  music: t('search.catMusic'),
}))
function typeBadge(t: string) {
  return TYPE_LABELS.value[t] ?? ''
}
function genreText(m: MediaInfo) {
  return (m.genres?.length ?? 0) > 0 || (m.countries?.length ?? 0) > 0
}

// 寸光集↔短剧互斥联动：寸光集只产短剧、短剧只在寸光集。选其一时自动校正另一筛选，
// 避免「寸光集 + 热映电影」「豆瓣 + 短剧」这类必然空态组合。后端 CategoryCapable 亦会兜底
// 跳过不支持当前分类的源，但这里联动让选中态始终自洽。
function selectSource(k: string) {
  source.value = k
  if (k === 'cunguangji') {
    category.value = 'duanju'
  } else if (category.value === 'duanju') {
    category.value = 'movie'
  }
  load()
}

function selectCategory(k: string) {
  category.value = k
  if (k === 'duanju') {
    source.value = ''
  } else if (source.value === 'cunguangji') {
    source.value = ''
  }
  load()
}

// 卡片点击 → 进入媒体详情页（对标 MoviePilot），与搜索页影视卡片一致。
function openMediaDetail(m: MediaInfo) {
  router.push({ path: '/media-detail', query: { source: m.source, id: m.source_id } })
}

// ---- 开播日历 / 订阅日历（对标 MSaber 开播日历；第三视图，与信息流/搜索并列） ----
const calMode = ref<'' | 'tv' | 'movie' | 'mine'>('')
const calLoading = ref(false)
const calData = ref<Record<string, CalendarEpisode[]>>({})
const calDays = ref(14)
const calDates = computed(() => Object.keys(calData.value).sort())
function fmtDateLabel(d: string) {
  const dt = new Date(d + 'T00:00:00')
  const weekday = t('recommend.weekday.' + dt.getDay())
  return t('recommend.dateLabel', { m: dt.getMonth() + 1, d: dt.getDate(), w: weekday })
}
async function selectCalendar(mode: '' | 'tv' | 'movie' | 'mine') {
  calMode.value = mode
  calData.value = {}
  if (mode === '') return
  calLoading.value = true
  try {
    calData.value =
      mode === 'mine' ? await recommendApi.subscriptionCalendar(30) : await recommendApi.calendar(mode, calDays.value)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('recommend.calendarLoadFailed'))
  } finally {
    calLoading.value = false
  }
}
function openCalendarDetail(ep: CalendarEpisode) {
  // tmdb_id 形如 "tv:123"/"movie:456"（tmdb 命名空间下的类型前缀）；source 必须是
  // "tmdb"——把前缀当 source 传给 /media/detail 会报 unknown media source。
  const [ns, id] = ep.tmdb_id.split(':')
  if (ns && id) router.push({ path: '/media-detail', query: { source: 'tmdb', id: `${ns}:${id}` } })
}

onMounted(() => load())
onUnmounted(() => {
  abortCtrl?.abort()
  abortFind()
})
</script>

<template>
  <div class="page" v-loading="searchLoading">
    <div class="head">
      <h2 class="title">{{ t('recommend.title') }}</h2>
      <el-button :loading="loading || searchLoading" @click="refresh">{{ t('common.refresh') }}</el-button>
    </div>

    <div class="filter-bar glass">
      <div class="filter-group">
        <button
          v-for="s in recommendSources"
          :key="s.key"
          class="chip"
          :class="{ on: source === s.key }"
          @click="selectSource(s.key)"
        >
          {{ s.label }}
        </button>
      </div>
      <div class="filter-group">
        <button
          v-for="c in recommendCategories"
          :key="c.key"
          class="chip"
          :class="{ on: category === c.key && !calMode }"
          @click="selectCategory(c.key)"
        >
          {{ c.label }}
        </button>
      </div>
      <!-- 开播日历视图：剧集/电影日历 + 我的订阅日历（单集粒度） -->
      <div class="filter-group">
        <button class="chip" :class="{ on: calMode === 'tv' }" @click="selectCalendar(calMode === 'tv' ? '' : 'tv')">
          📅 {{ t('recommend.tvCalendar') }}
        </button>
        <button
          class="chip"
          :class="{ on: calMode === 'movie' }"
          @click="selectCalendar(calMode === 'movie' ? '' : 'movie')"
        >
          📅 {{ t('recommend.movieCalendar') }}
        </button>
        <button
          class="chip"
          :class="{ on: calMode === 'mine' }"
          @click="selectCalendar(calMode === 'mine' ? '' : 'mine')"
        >
          🔖 {{ t('recommend.myCalendar') }}
        </button>
      </div>
      <!-- 搜索框：置于分类筛选右侧，原「探索」并入。回车/点按钮触发，清空回到信息流。 -->
      <div class="search-group">
        <el-input
          v-model="keyword"
          :placeholder="t('recommend.searchPlaceholder')"
          clearable
          size="small"
          @keyup.enter="doSearch"
          @clear="clearSearch"
        />
        <el-button size="small" type="primary" :loading="searchLoading" @click="doSearch">{{
          t('common.search')
        }}</el-button>
      </div>
    </div>

    <p v-if="loading && !items.length && !searchMode && !calMode" class="sub">{{ t('common.loading') }}</p>

    <!-- 日历视图：按日期分组的开播/上映条目 -->
    <div v-if="calMode" v-loading="calLoading" class="calendar-view">
      <template v-if="calDates.length">
        <div v-for="d in calDates" :key="d" class="cal-day card glass">
          <div class="cal-date">
            {{ fmtDateLabel(d) }}<span class="text-muted small">（{{ d }}）</span>
          </div>
          <div
            v-a11y-button
            v-for="(ep, i) in calData[d]"
            :key="i"
            class="cal-ep clickable"
            @click="openCalendarDetail(ep)"
          >
            <span v-if="ep.poster_url" class="cal-poster"><MediaPoster :url="ep.poster_url" kind="movie" /></span>
            <span v-if="ep.season" class="chip res"
              >S{{ String(ep.season).padStart(2, '0') }}E{{ String(ep.episode ?? 0).padStart(2, '0') }}</span
            >
            <span class="cal-title">{{ ep.title }}</span>
            <span v-if="ep.overview" class="text-muted small cal-overview">{{ ep.overview.slice(0, 60) }}</span>
          </div>
        </div>
      </template>
      <EmptyState
        v-else-if="!calLoading"
        icon="📅"
        :title="calMode === 'mine' ? t('recommend.calEmptyMine') : t('recommend.calEmpty')"
        :description="calMode === 'mine' ? t('recommend.calEmptyMineDesc') : t('recommend.calEmptyDesc')"
      />
    </div>

    <div v-else class="cards">
      <div
        v-a11y-button
        v-for="(m, i) in items"
        :key="m.source_id || i"
        class="rec-card glass clickable"
        @click="openMediaDetail(m)"
      >
        <div class="poster"><MediaPoster :url="m.poster_url" :kind="m.type === 'music' ? 'music' : 'movie'" /></div>
        <div class="info">
          <div class="t-title">{{ m.title }}</div>
          <div class="t-meta">
            <span v-if="typeBadge(m.type)" class="badge">{{ typeBadge(m.type) }}</span>
            <span class="src" :data-src="m.source">{{ sourceLabel(m.source) }}</span>
            <span v-if="m.year">· {{ m.year }}</span>
            <span v-if="m.rating">· ⭐ {{ m.rating.toFixed(1) }}</span>
          </div>
          <!-- 类型/产地：豆瓣榜单自 card_subtitle 结构化，TMDB/寸光集自带；有则展示，与详情页一致 -->
          <div v-if="genreText(m)" class="t-tags">
            <el-tag v-for="g in (m.genres || []).slice(0, 3)" :key="g" size="small" effect="plain">{{ g }}</el-tag>
            <span v-for="cA in (m.countries || []).slice(0, 1)" :key="cA" class="t-country">{{ cA }}</span>
          </div>
          <div v-if="m.overview" class="t-overview">{{ m.overview }}</div>
          <el-button size="small" type="primary" plain class="find-btn" @click.stop="findOnSites(m)">{{
            t('recommend.findOnSites')
          }}</el-button>
        </div>
      </div>
    </div>

    <EmptyState
      v-if="!items.length && !loading && !searchLoading && !calMode"
      icon="🎬"
      :title="searchMode ? t('recommend.noSearchResults') : t('recommend.emptyFeed')"
      :description="searchMode ? t('recommend.noSearchResultsDesc') : t('recommend.emptyFeedDesc')"
    >
      <el-button v-if="searchMode" @click="clearSearch">{{ t('recommend.backToFeed') }}</el-button>
      <el-button v-else :loading="loading" @click="refresh">{{ t('common.refresh') }}</el-button>
    </EmptyState>

    <!-- 分页：加载更多（仅信息流；搜索结果一次性返回） -->
    <div v-if="items.length && !searchMode && !calMode" class="pager">
      <span class="page-info text-muted">{{ t('recommend.pageInfo', { page: page, n: items.length }) }}</span>
      <el-button v-if="hasMore" :loading="loadingMore" @click="loadMore">{{ t('recommend.loadMore') }}</el-button>
      <span v-else class="text-muted no-more">{{ t('recommend.noMore') }}</span>
    </div>

    <SiteTorrentFindDialog
      v-model="findVisible"
      :title="findTitle"
      :poster-url="findPoster"
      :loading="findLoading"
      :results="findResults"
      @close="closeFindDialog"
    />
  </div>
</template>

<style scoped src="./Recommend.css"></style>
