<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
/**
 * 统一搜索页：[种子 | 影视] 分段切换，共享关键词。
 *  - 种子：跨站 SSE 流式聚合 + 前端筛选排序；结果表/失败容错见 TorrentResultsTable；
 *    下载推送见 SearchPushDialog；详情抽屉见 TorrentDetailDrawer。
 *  - 影视：豆瓣/TMDB/Bangumi 元数据 + 海报卡片（MediaResultsGrid）。后端默认按 movie+tv 过滤，
 *    不含音乐。
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

const { t } = useI18n()
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
  results.value.flatMap((r) => (r.torrents ?? []).map((t) => ({ ...t, site_id: r.site_id, site_name: r.site_name }))),
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
    ElMessage.warning(t('search.emptyKeyword'))
    return
  }
  if (scopeSiteCount.value === 0) {
    // 真机实测的形状：一个启用站点都没有，点「搜索」照常开一条 SSE 流，
    // 流立刻结束、表格空、页面一动不动——界面上只剩角标那句「将搜索 0 个站点」，
    // 用户既不知道是没站点以为搜索坏了，也会反复点。这里直接说明下一步该去哪。
    ElMessage.warning(t('search.noSitesInScope'))
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
        error.value = (e as Error).message || t('search.searchFailed')
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
    error.value = (e as Error).message || t('search.searchFailed')
  } finally {
    loading.value = false
  }
}

// —— 确认下载：逻辑在 composable（解构后 ref 在模板自动解包）——
const {
  visible: pushVisible,
  row: pushRow,
  subtitle: pushSubtitle,
  downloaders: pushDownloaders,
  selectedDl,
  categories: pushCategories,
  selectedCategory,
  savePath,
  showAdvanced,
  paused,
  loading: pushLoading,
  pushedDlName,
  start: startPush,
  confirm: confirmPush,
  gotoDownloaderTasks,
  gotoDownloaderConfig,
  close: closePush,
} = useSearchPush()

// —— 种子详情抽屉：抓取并解析单条种子详情页 ——
const detail = ref<TorrentDetail | null>(null)
const detailLoading = ref(false)
const detailVisible = ref(false)

const resultsTable = ref<InstanceType<typeof TorrentResultsTable> | null>(null)

async function openTorrentDetail(row: { site_id: number; detail_url?: string }) {
  if (!row.detail_url) {
    ElMessage.warning(t('search.noDetail'))
    return
  }
  detailVisible.value = true
  detail.value = null
  detailLoading.value = true
  try {
    const r = await seedbrowseApi.detail(row.site_id, row.detail_url)
    detail.value = r.detail ?? null
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('search.loadDetailFailed'))
    detailVisible.value = false
  } finally {
    detailLoading.value = false
  }
}

// —— 影视模式：元数据 + 海报卡片 ——
const mediaResults = ref<MediaInfo[]>([])
const year = ref<number | undefined>(undefined)

// 卡片点击 → 进入媒体详情页，不再跳外站。
function openMediaDetail(m: MediaInfo) {
  router.push({ path: '/media-detail', query: { source: m.source, id: m.source_id } })
}

// 接受跨页跳转带来的 keyword / mode（如详情页「搜索资源」按钮回跳）。
onMounted(() => {
  siteApi
    .list()
    .then((l) => {
      sites.value = l
    })
    .catch(() => {})
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
    const next = typeof kw === 'string' && kw ? kw : typeof q === 'string' ? q : ''
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
      <h2 class="title">{{ t('search.title') }}</h2>
      <div class="head-right">
        <el-segmented
          v-model="mode"
          :options="[
            { label: t('search.modeTorrent'), value: 'torrent' },
            { label: t('search.modeMedia'), value: 'media' },
          ]"
        />
        <router-link v-if="mode === 'media'" to="/media-settings" class="src-settings-link">{{
          t('search.mediaSettingsLink')
        }}</router-link>
      </div>
    </div>

    <div class="search-bar solid-card">
      <div class="search-main">
        <el-input
          v-model="keyword"
          :placeholder="mode === 'torrent' ? t('search.torrentPlaceholder') : t('search.mediaPlaceholder')"
          clearable
          size="large"
          @keyup.enter="search"
        />
        <div v-if="mode === 'torrent'" class="scope-line">
          <span>{{ t('search.scope', { n: scopeSiteCount }) }}</span>
          <SiteFilterDialog v-model="selectedSiteIds" :sites="activeSites" :title="t('search.siteFilterTitle')" />
        </div>
      </div>
      <el-input-number
        v-if="mode === 'media'"
        v-model="year"
        :min="1900"
        :max="2099"
        :placeholder="t('search.year')"
        controls-position="right"
      />
      <el-select v-if="mode === 'torrent'" v-model="category" :placeholder="t('search.category')" style="width: 110px">
        <el-option :label="t('search.catAll')" value="" />
        <el-option :label="t('search.catMovie')" value="movie" />
        <el-option :label="t('search.catTv')" value="tv" />
        <el-option :label="t('search.catMusic')" value="music" />
      </el-select>
      <el-button type="primary" size="large" :loading="loading" @click="search">{{ t('search.submit') }}</el-button>
    </div>

    <!-- 顶层请求失败 -->
    <div v-if="error" class="empty solid-card error-state">
      <div class="err-text" role="alert"><SptIcon name="alert-triangle" :size="16" aria-hidden="true" /> {{ error }}</div>
      <el-button size="small" @click="search">{{ t('search.retry') }}</el-button>
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
    <MediaResultsGrid v-else :results="mediaResults" :loading="loading" @open="openMediaDetail" />

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
      @goto-downloader-config="gotoDownloaderConfig"
      @close="closePush"
    />

    <TorrentDetailDrawer v-model:visible="detailVisible" :detail="detail" :loading="detailLoading" />
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: var(--sp-4);
}
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--sp-3);
}
.head-right {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
}
.src-settings-link {
  font-size: var(--fs-body);
  color: var(--pt-primary);
  text-decoration: none;
  white-space: nowrap;
}
.src-settings-link:hover {
  text-decoration: underline;
}
.title {
  margin: 4px 0;
  font-size: var(--fs-title);
  font-weight: 700;
}
.search-bar {
  display: flex;
  gap: var(--sp-3);
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
  gap: var(--sp-2);
  margin-top: 7px;
  color: var(--text-muted);
  font-size: var(--fs-xs);
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
  gap: var(--sp-3);
}
.err-text {
  color: var(--pt-danger);
  font-size: var(--fs-md);
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
