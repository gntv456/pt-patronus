<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  recommendApi,
  recommendSources,
  recommendCategories,
  type MediaInfo,
} from '@/api/recommend'
import { discoverApi } from '@/api/discover'
import { searchApi, type SiteSearchResult } from '@/api/search'
import EmptyState from '@/components/EmptyState.vue'

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

// find-on-sites 弹窗
const findVisible = ref(false)
const findLoading = ref(false)
const findTitle = ref('')
const findResults = ref<SiteSearchResult[]>([])

async function load(refresh = false) {
  loading.value = true
  searchMode.value = false
  page.value = 1
  hasMore.value = true
  try {
    feedItems.value = await recommendApi.feed(source.value, category.value, 1, refresh)
    if (feedItems.value.length < PAGE_MIN_HINT) hasMore.value = false
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载推荐失败')
  } finally {
    loading.value = false
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

async function findOnSites(m: MediaInfo) {
  findVisible.value = true
  findLoading.value = true
  findTitle.value = m.title
  findResults.value = []
  try {
    findResults.value = await searchApi.all(m.title, m.type)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '站点查找失败')
  } finally {
    findLoading.value = false
  }
}

// 关键词跨源搜索（原「探索」页）：搜索态接管列表，沿用当前选中的源筛选。
async function doSearch() {
  const kw = keyword.value.trim()
  if (!kw) return
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

// 命中站点数 / 总种子数
function hitSummary() {
  let sites = 0
  let total = 0
  for (const s of findResults.value) {
    if (!s.error && s.torrents.length) {
      sites++
      total += s.torrents.length
    }
  }
  return { sites, total }
}

const sourceLabel = (key: string) => recommendSources.find((s) => s.key === key)?.label ?? key

// 星光集↔短剧互斥联动：星光集只产短剧、短剧只在星光集。选其一时自动校正另一筛选，
// 避免「星光集 + 热映电影」「豆瓣 + 短剧」这类必然空态组合。后端 CategoryCapable 亦会兜底
// 跳过不支持当前分类的源，但这里联动让选中态始终自洽。
function selectSource(k: string) {
  source.value = k
  if (k === 'xingguangji') {
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
  } else if (source.value === 'xingguangji') {
    source.value = ''
  }
  load()
}

// 卡片点击 → 进入媒体详情页（对标 MoviePilot），与搜索页影视卡片一致。
function openMediaDetail(m: MediaInfo) {
  router.push({ path: '/media-detail', query: { source: m.source, id: m.source_id } })
}

onMounted(() => load())
</script>

<template>
  <div class="page" v-loading="loading || searchLoading">
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

    <el-dialog v-model="findVisible" :title="`在站点查找：${findTitle}`" width="640">
      <div v-loading="findLoading">
        <div v-if="!findLoading" class="find-summary">
          命中 <b>{{ hitSummary().sites }}</b> 个站点，共 <b>{{ hitSummary().total }}</b> 条种子
        </div>
        <div v-for="s in findResults" :key="s.site_id" class="find-site">
          <div class="find-site-name">
            <span class="site-tag">{{ s.site_name }}</span>
            <span v-if="s.error" class="err">{{ s.error }}</span>
            <span v-else-if="!s.torrents.length" class="text-muted">未命中</span>
            <span v-else class="text-muted">{{ s.torrents.length }} 条</span>
          </div>
          <div v-for="(t, j) in s.torrents.slice(0, 5)" :key="j" class="find-torr">
            <span class="ft-title">{{ t.title }}</span>
            <span class="ft-meta">{{ t.size }} · {{ t.seeders }} 种</span>
          </div>
        </div>
      </div>
      <template #footer><el-button @click="findVisible = false">关闭</el-button></template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: 16px; }
.head { display: flex; align-items: center; justify-content: space-between; }
.title { margin: 4px 0; font-size: 22px; font-weight: 700; }
.filter-bar { display: flex; gap: 24px; padding: 10px 14px; flex-wrap: wrap; align-items: center; }
.filter-group { display: flex; gap: 8px; flex-wrap: wrap; }
/* 搜索框：靠最右，与分类 chip 同行；窄屏自动换行 */
.search-group { display: flex; gap: 6px; align-items: center; margin-left: auto; }
.search-group :deep(.el-input) { width: 220px; }
.chip { background: transparent; border: 1px solid transparent; border-radius: 999px; padding: 4px 12px; cursor: pointer; font-size: 13px; color: var(--text-strong); }
.chip.on { background: rgba(108,92,231,0.14); border-color: rgba(108,92,231,0.3); }
.cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 12px; }
.rec-card { display: flex; gap: 12px; padding: 12px; }
.clickable { cursor: pointer; transition: transform 0.15s, box-shadow 0.15s; }
.clickable:hover { transform: translateY(-2px); }
.poster { width: 92px; height: 132px; border-radius: 8px; overflow: hidden; flex: none; background: var(--surface-muted); }
.info { flex: 1; min-width: 0; display: flex; flex-direction: column; }
.t-title { font-weight: 600; font-size: 15px; }
.t-meta { font-size: 12px; color: var(--text-muted); margin-top: 2px; }
.src { color: var(--pt-primary,#6c5ce7); font-weight: 600; }
.t-overview { font-size: 12px; color: var(--text-muted); margin-top: 6px; flex: 1; display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
.find-btn { margin-top: 8px; align-self: flex-start; }
.empty { padding: 40px; text-align: center; }
.pager { display: flex; align-items: center; justify-content: center; gap: 12px; padding: 8px 0 4px; }
.pager .page-info { font-size: 12px; }
.pager .no-more { font-size: 12px; }
.find-summary { padding: 4px 0 12px; font-size: 13px; }
.find-site { padding: 8px 0; border-top: 1px solid rgba(0,0,0,0.06); }
.find-site-name { display: flex; align-items: center; gap: 8px; margin-bottom: 4px; }
.site-tag { display: inline-block; padding: 2px 10px; border-radius: 999px; background: rgba(108,92,231,0.16); color: var(--pt-primary,#6c5ce7); font-size: 12px; }
.err { color: var(--pt-danger); font-size: 12px; }
.find-torr { display: flex; justify-content: space-between; gap: 8px; font-size: 12px; padding: 2px 0; }
.ft-title { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ft-meta { color: var(--text-muted); flex: none; }
</style>
