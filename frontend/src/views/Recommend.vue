<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { recommendApi, recommendSources, recommendCategories, type MediaInfo } from '@/api/recommend'
import { discoverApi } from '@/api/discover'
import EmptyState from '@/components/EmptyState.vue'
import SiteTorrentFindDialog from '@/components/SiteTorrentFindDialog.vue'
import { useFindOnSites } from '@/composables/useFindOnSites'

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
const { findVisible, findLoading, findTitle, findPoster, findResults, findOnSites, closeFindDialog, abortFind } = useFindOnSites()

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
    await recommendApi.feedStream(source.value, category.value, 1, refresh, (items) => {
      feedItems.value.push(...items)
    }, ctrl.signal)
    if (feedItems.value.length < PAGE_MIN_HINT) hasMore.value = false
  } catch (e: unknown) {
    // 切换筛选时 abort 上一次流，AbortError 是预期，不报错
    if ((e as Error).name !== 'AbortError') ElMessage.error((e as Error).message || '加载推荐失败')
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
    ElMessage.error((e as Error).message || '加载更多失败')
  } finally {
    loadingMore.value = false
  }
}

// 关键词跨源搜索（原「探索」页）：搜索态接管列表，沿用当前选中的源筛选。
async function doSearch() {
  const kw = keyword.value.trim()
  if (!kw) {
    ElMessage.warning('请输入关键词')
    return
  }
  searchLoading.value = true
  searchMode.value = true
  try {
    searchItems.value = await discoverApi.search(kw, 0, source.value)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '搜索失败')
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

onMounted(() => load())
onUnmounted(() => {
  abortCtrl?.abort()
  abortFind()
})
</script>

<template>
  <div class="page" v-loading="searchLoading">
    <div class="head">
      <h2 class="title">推荐</h2>
      <el-button :loading="loading || searchLoading" @click="refresh">刷新</el-button>
    </div>

    <div class="filter-bar glass">
      <div class="filter-group">
        <button v-for="s in recommendSources" :key="s.key" class="chip" :class="{ on: source === s.key }" @click="selectSource(s.key)">
          {{ s.label }}
        </button>
      </div>
      <div class="filter-group">
        <button v-for="c in recommendCategories" :key="c.key" class="chip" :class="{ on: category === c.key }" @click="selectCategory(c.key)">
          {{ c.label }}
        </button>
      </div>
      <!-- 搜索框：置于分类筛选右侧，原「探索」并入。回车/点按钮触发，清空回到信息流。 -->
      <div class="search-group">
        <el-input v-model="keyword" placeholder="搜索影视/动漫（多源）" clearable size="small" @keyup.enter="doSearch" @clear="clearSearch" />
        <el-button size="small" type="primary" :loading="searchLoading" @click="doSearch">搜索</el-button>
      </div>
    </div>

    <p v-if="loading && !items.length && !searchMode" class="sub">加载中…</p>

    <div class="cards">
      <div v-for="(m, i) in items" :key="m.source_id || i" class="rec-card glass clickable" @click="openMediaDetail(m)">
        <div class="poster"><MediaPoster :url="m.poster_url" :kind="m.type === 'music' ? 'music' : 'movie'" /></div>
        <div class="info">
          <div class="t-title">{{ m.title }}</div>
          <div class="t-meta">
            <span class="src" :data-src="m.source">{{ sourceLabel(m.source) }}</span>
            <span v-if="m.year">· {{ m.year }}</span>
            <span v-if="m.rating">· ⭐ {{ m.rating.toFixed(1) }}</span>
          </div>
          <div v-if="m.overview" class="t-overview">{{ m.overview }}</div>
          <el-button size="small" type="primary" plain class="find-btn" @click.stop="findOnSites(m)">在站点查找</el-button>
        </div>
      </div>
    </div>

    <EmptyState
      v-if="!items.length && !loading && !searchLoading"
      icon="🎬"
      :title="searchMode ? '无搜索结果' : '暂无推荐，点击刷新或切换源/分类'"
      :description="searchMode ? '换个关键词，或清空搜索框回到推荐信息流' : '点击右上角刷新重试，或在上方切换推荐源与分类'"
    >
      <el-button v-if="searchMode" @click="clearSearch">返回推荐</el-button>
      <el-button v-else :loading="loading" @click="refresh">刷新</el-button>
    </EmptyState>

    <!-- 分页：加载更多（仅信息流；搜索结果一次性返回） -->
    <div v-if="items.length && !searchMode" class="pager">
      <span class="page-info text-muted">第 {{ page }} 页 · 共 {{ items.length }} 条</span>
      <el-button v-if="hasMore" :loading="loadingMore" @click="loadMore">加载更多</el-button>
      <span v-else class="text-muted no-more">没有更多了</span>
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

<style scoped>
.page { display: flex; flex-direction: column; gap: var(--sp-4); }
.head { display: flex; align-items: center; justify-content: space-between; }
.title { margin: 4px 0; font-size: var(--fs-title); font-weight: 700; }
.filter-bar { display: flex; gap: var(--sp-6); padding: 10px 14px; flex-wrap: wrap; align-items: center; }
.filter-group { display: flex; gap: var(--sp-2); flex-wrap: wrap; }
/* 搜索框：靠最右，与分类 chip 同行；窄屏自动换行 */
.search-group { display: flex; gap: var(--sp-1); align-items: center; margin-left: auto; }
.search-group :deep(.el-input) { width: 220px; }
.chip { background: transparent; border: 1px solid transparent; border-radius: var(--radius-pill); padding: 4px 12px; cursor: pointer; font-size: var(--fs-body); color: var(--text-strong); transition: background var(--dur-base) var(--ease-out-smooth), border-color var(--dur-base) var(--ease-out-smooth), color var(--dur-base) var(--ease-out-smooth); }
.chip:hover { background: var(--surface-hover); }
.chip.on { background: var(--pt-primary-bg); border-color: var(--pt-primary-bg); }
.cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: var(--sp-3); }
.rec-card { display: flex; gap: var(--sp-3); padding: 12px; }
.clickable { cursor: pointer; transition: transform var(--dur-base) var(--ease-out-smooth), box-shadow var(--dur-base) var(--ease-out-smooth), border-color var(--dur-base) var(--ease-out-smooth); }
.clickable:hover { transform: translateY(-3px); box-shadow: var(--surface-shadow-hover); }
.poster { width: 92px; height: 132px; border-radius: var(--radius-xs); overflow: hidden; flex: none; background: var(--surface-muted); }
.info { flex: 1; min-width: 0; display: flex; flex-direction: column; }
.t-title { font-weight: 600; font-size: var(--fs-lg); }
.t-meta { font-size: var(--fs-xs); color: var(--text-muted); margin-top: 2px; }
.src { color: var(--pt-primary,#6c5ce7); font-weight: 600; }
.t-overview { font-size: var(--fs-xs); color: var(--text-muted); margin-top: 6px; flex: 1; display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
.find-btn { margin-top: 8px; align-self: flex-start; }
.empty { padding: 40px; text-align: center; }
.pager { display: flex; align-items: center; justify-content: center; gap: var(--sp-3); padding: 8px 0 4px; }
.pager .page-info { font-size: var(--fs-xs); }
.pager .no-more { font-size: var(--fs-xs); }
</style>
