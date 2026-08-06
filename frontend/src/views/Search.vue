<script setup lang="ts">
/**
 * 统一搜索页：[种子 | 影视] 分段切换，共享关键词。
 *  - 种子：跨站 SSE 流式聚合 + 前端筛选排序；结果表/失败容错见 TorrentResultsTable；
 *    下载推送见 SearchPushDialog；详情抽屉见 TorrentDetailDrawer。
 *  - 影视：豆瓣/TMDB/Bangumi 元数据 + 海报卡片（MediaResultsGrid）。后端默认按 movie+tv 过滤，
 *    不含音乐（与 MoviePilot 媒体搜索一致）。
 * 本文件仅做编排：搜索状态/请求 + 生命周期 + 顶部搜索栏，具体 UI 拆到子组件。
 */
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { searchApi, type SiteSearchResult } from '@/api/search'
import { seedbrowseApi, type TorrentDetail } from '@/api/seedbrowse'
import { mediaApi, type MediaInfo } from '@/api/media'
import { siteApi, type Site } from '@/api/site'
import { enrichTorrent } from '@/utils/torrentMeta'
import { useSearchFilter } from '@/composables/useSearchFilter'
import { useSearchPush } from '@/composables/useSearchPush'
import SiteFilterDialog from '@/components/SiteFilterDialog.vue'
import TorrentResultsTable from '@/components/TorrentResultsTable.vue'
import MediaResultsGrid from '@/components/MediaResultsGrid.vue'
import SearchPushDialog from '@/components/SearchPushDialog.vue'
import TorrentDetailDrawer from '@/components/TorrentDetailDrawer.vue'

const route = useRoute()
const router = useRouter()

type SearchMode = 'torrent' | 'media'
const mode = ref<SearchMode>('torrent')

const keyword = ref('')
const category = ref<'' | 'movie' | 'tv' | 'music'>('')
const results = ref<SiteSearchResult[]>([])
const sites = ref<Site[]>([])
const loading = ref(false)
const error = ref('')
const searched = ref(false)

// 扁平表行：种子字段 + 来源站点名（跨站合并到一张表）。
const rows = computed(() =>
  results.value.flatMap((r) =>
    (r.torrents ?? []).map((t) => ({ ...t, site_id: r.site_id, site_name: r.site_name })),
  ),
)
// 派生筛选 meta + 前端过滤/排序 composable。
const enrichedRows = computed(() => rows.value.map((r) => enrichTorrent(r)))
const filter = useSearchFilter(enrichedRows)

// 站点筛选：空=搜全部启用站；非空=仅搜选中站点。UI/逻辑内聚在 SiteFilterDialog 组件。
const selectedSiteIds = ref<number[]>([])
const activeSites = computed(() => sites.value.filter((s) => s.is_active))
const scopeSiteCount = computed(() =>
  selectedSiteIds.value.length ? selectedSiteIds.value.length : activeSites.value.length,
)

// 种子跨站 SSE 流：切换关键词/重复搜索时中断上一流，避免乱序覆盖。
let torrentAbort: AbortController | null = null

async function search() {
  const kw = keyword.value.trim()
  if (!kw) {
    ElMessage.warning('请输入关键词')
    return
  }
  if (mode.value === 'torrent') {
    torrentAbort?.abort()
    const ctrl = new AbortController()
    torrentAbort = ctrl
    loading.value = true
    error.value = ''
    results.value = []
    searched.value = true
    resultsTable.value?.resetShowAll()
    try {
      await searchApi.allStream(
        kw,
        category.value,
        selectedSiteIds.value,
        (r) => {
          if (torrentAbort !== ctrl) return
          results.value = [...results.value, { ...r, torrents: r.torrents ?? [] }]
        },
        ctrl.signal,
      )
    } catch (e: unknown) {
      if ((e as Error).name !== 'AbortError') {
        error.value = (e as Error).message || '搜索失败'
      }
    } finally {
      if (torrentAbort === ctrl) {
        loading.value = false
        torrentAbort = null
      }
    }
    return
  }
  loading.value = true
  error.value = ''
  try {
    mediaResults.value = await mediaApi.search(kw, year.value || 0)
    searched.value = true
  } catch (e: unknown) {
    error.value = (e as Error).message || '搜索失败'
  } finally {
    loading.value = false
  }
}

// —— 确认下载：逻辑在 composable（解构后 ref 在模板自动解包）——
const {
  visible: pushVisible, row: pushRow, subtitle: pushSubtitle, downloaders: pushDownloaders,
  selectedDl, categories: pushCategories, selectedCategory, savePath, showAdvanced, paused,
  loading: pushLoading, pushedDlName,
  start: startPush, confirm: confirmPush, gotoDownloaderTasks, close: closePush,
} = useSearchPush()

// —— 种子详情抽屉：抓取并解析单条种子详情页 ——
const detail = ref<TorrentDetail | null>(null)
const detailLoading = ref(false)
const detailVisible = ref(false)

const resultsTable = ref<InstanceType<typeof TorrentResultsTable> | null>(null)

async function openTorrentDetail(row: { site_id: number; detail_url?: string }) {
  if (!row.detail_url) {
    ElMessage.warning('该种子无详情页链接')
    return
  }
  detailVisible.value = true
  detail.value = null
  detailLoading.value = true
  try {
    const r = await seedbrowseApi.detail(row.site_id, row.detail_url)
    if (r.error) {
      ElMessage.error(r.error)
      detailVisible.value = false
    } else {
      detail.value = r.detail ?? null
    }
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载详情失败')
    detailVisible.value = false
  } finally {
    detailLoading.value = false
  }
}

// —— 影视模式：元数据 + 海报卡片 ——
const mediaResults = ref<MediaInfo[]>([])
const year = ref<number | undefined>(undefined)

// 卡片点击 → 进入媒体详情页（对标 MoviePilot），不再跳外站。
function openMediaDetail(m: MediaInfo) {
  router.push({ path: '/media-detail', query: { source: m.source, id: m.source_id } })
}

// 接受跨页跳转带来的 keyword / mode（如详情页「搜索资源」按钮回跳）。
onMounted(() => {
  siteApi.list().then((l) => { sites.value = l }).catch(() => {})
  // 兼容详情页 keyword 与命令面板 q
  const q = route.query.keyword ?? route.query.q
  if (typeof q === 'string' && q) {
    keyword.value = q
  }
  const md = route.query.mode
  if (md === 'torrent' || md === 'media') {
    mode.value = md
  }
  if (keyword.value) search()
})

// 已在搜索页时命令面板再次带 q/keyword 跳入
watch(
  () => [route.query.keyword, route.query.q, route.query.mode] as const,
  ([kw, q, md]) => {
    const next = (typeof kw === 'string' && kw) ? kw : (typeof q === 'string' ? q : '')
    if (next && next !== keyword.value) {
      keyword.value = next
      search()
    }
    if (md === 'torrent' || md === 'media') mode.value = md
  },
)

onUnmounted(() => {
  torrentAbort?.abort()
  torrentAbort = null
})
</script>

<template>
  <div class="page">
    <div class="head">
      <h2 class="title">搜索</h2>
      <div class="head-right">
        <el-segmented v-model="mode" :options="[
          { label: '种子', value: 'torrent' },
          { label: '影视', value: 'media' },
        ]" />
        <router-link v-if="mode === 'media'" to="/media-settings" class="src-settings-link">⚙ 媒体源设置</router-link>
      </div>
    </div>

    <div class="search-bar glass">
      <div class="search-main">
        <el-input
          v-model="keyword"
          :placeholder="mode === 'torrent'
            ? '输入电影/剧集名或关键词，跨启用站点搜索…'
            : '搜索影视（电影 / 剧集标题）'"
          clearable
          size="large"
          @keyup.enter="search"
        />
        <div v-if="mode === 'torrent'" class="scope-line">
          <span>将搜索 {{ scopeSiteCount }} 个站点</span>
          <SiteFilterDialog v-model="selectedSiteIds" :sites="activeSites" title="选择搜索站点" />
        </div>
      </div>
      <el-input-number
        v-if="mode === 'media'"
        v-model="year"
        :min="1900"
        :max="2099"
        placeholder="年份"
        controls-position="right"
      />
      <el-select
        v-if="mode === 'torrent'"
        v-model="category"
        placeholder="分类"
        style="width: 110px"
      >
        <el-option label="全部" value="" />
        <el-option label="电影" value="movie" />
        <el-option label="剧集" value="tv" />
        <el-option label="音乐" value="music" />
      </el-select>
      <el-button type="primary" size="large" :loading="loading" @click="search">搜索</el-button>
    </div>

    <!-- 顶层请求失败 -->
    <div v-if="error" class="empty glass error-state">
      <div class="err-text">⚠ {{ error }}</div>
      <el-button size="small" @click="search">重试</el-button>
    </div>

    <TorrentResultsTable
      v-else-if="mode === 'torrent'"
      ref="resultsTable"
      :filter="filter"
      :enriched-rows="enrichedRows"
      :results="results"
      :loading="loading"
      :searched="searched"
      :scope-site-count="scopeSiteCount"
      @start-push="startPush"
      @open-detail="openTorrentDetail"
    />

    <!-- 影视模式：海报卡片 -->
    <MediaResultsGrid
      v-else
      :results="mediaResults"
      :loading="loading"
      @open="openMediaDetail"
    />

    <SearchPushDialog
      v-model="pushVisible"
      v-model:selected-dl="selectedDl"
      v-model:selected-category="selectedCategory"
      v-model:save-path="savePath"
      v-model:show-advanced="showAdvanced"
      v-model:paused="paused"
      :row="pushRow"
      :subtitle="pushSubtitle"
      :downloaders="pushDownloaders"
      :categories="pushCategories"
      :loading="pushLoading"
      :pushed-dl-name="pushedDlName"
      @confirm="confirmPush"
      @goto-downloader-tasks="gotoDownloaderTasks"
      @close="closePush"
    />

    <TorrentDetailDrawer
      v-model:visible="detailVisible"
      :detail="detail"
      :loading="detailLoading"
    />
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.head-right {
  display: flex;
  align-items: center;
  gap: 12px;
}
.src-settings-link {
  font-size: 13px;
  color: var(--pt-primary, #6c5ce7);
  text-decoration: none;
  white-space: nowrap;
}
.src-settings-link:hover {
  text-decoration: underline;
}
.title {
  margin: 4px 0;
  font-size: 22px;
  font-weight: 700;
}
.search-bar {
  display: flex;
  gap: 12px;
  padding: 14px;
  align-items: center;
}
.search-main {
  flex: 1;
  min-width: 0;
}
.scope-line {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 7px;
  color: var(--text-muted);
  font-size: 12px;
}
.scope-line a {
  color: var(--pt-primary);
  text-decoration: none;
}
.scope-line a:hover {
  text-decoration: underline;
}
.empty {
  padding: 40px;
  text-align: center;
}
.error-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
}
.err-text {
  color: var(--pt-danger);
  font-size: 14px;
}
@media (max-width: 720px) {
  .head,
  .search-bar {
    align-items: stretch;
    flex-direction: column;
  }
  .head-right {
    justify-content: space-between;
  }
  .search-bar :deep(.el-select),
  .search-bar :deep(.el-button) {
    width: 100%;
  }
}
</style>
