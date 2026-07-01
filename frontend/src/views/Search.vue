<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { searchApi, type SiteSearchResult, type SiteTorrent } from '@/api/search'
import { seedbrowseApi, type TorrentDetail } from '@/api/seedbrowse'
import { mediaApi, type MediaInfo } from '@/api/media'
import { downloaderApi, type Downloader } from '@/api/downloader'
import { copyText } from '@/utils/clipboard'
import MediaPoster from '@/components/MediaPoster.vue'
import EmptyState from '@/components/EmptyState.vue'
import DOMPurify from 'dompurify'

const route = useRoute()
const router = useRouter()

// 种子简介来自 PT 站原始 HTML，渲染前必须净化（防存储型 XSS：注入 onerror/script 窃 token）。
const sanitizeHtml = (html?: string) => (html ? DOMPurify.sanitize(html) : '')

// 统一搜索页：[种子 | 影视] 分段切换，共享关键词。
//  - 种子：合并自旧 Search（下载推送 + 单站失败容错）+ SeedBrowse（详情抽屉解析种子页）。
//  - 影视：豆瓣/TMDB/Bangumi 元数据 + 海报卡片（原 Media）。后端默认按 movie+tv 过滤，
//    不含音乐（与 MoviePilot 媒体搜索一致）。
type SearchMode = 'torrent' | 'media'
const mode = ref<SearchMode>('torrent')

// 扁平表行：种子字段 + 来源站点名（跨站合并到一张表）。
type SearchRow = SiteTorrent & { site_id: number; site_name: string }

const keyword = ref('')
const category = ref<'' | 'movie' | 'tv' | 'music'>('')
const results = ref<SiteSearchResult[]>([])
const loading = ref(false)
const error = ref('')
const searched = ref(false)

const rows = computed<SearchRow[]>(() =>
  results.value.flatMap((r) => r.torrents.map((t) => ({ ...t, site_id: r.site_id, site_name: r.site_name }))),
)
const failedSites = computed(() => results.value.filter((r) => r.error))
const showFailed = ref(false)

async function search() {
  const kw = keyword.value.trim()
  if (!kw) return
  loading.value = true
  error.value = ''
  try {
    if (mode.value === 'torrent') {
      results.value = await searchApi.all(kw, category.value)
    } else {
      mediaResults.value = await mediaApi.search(kw, year.value || 0)
    }
    searched.value = true
  } catch (e: unknown) {
    error.value = (e as Error).message || '搜索失败'
  } finally {
    loading.value = false
  }
}

function openDetail(url: string) {
  window.open(url, '_blank', 'noopener')
}

// —— 推送下载器（对齐移动端 search_page._download 闭环）——
const pushVisible = ref(false)
const pushRow = ref<SearchRow | null>(null)
const downloaders = ref<Downloader[]>([])
const selectedDl = ref<number | null>(null)
const pushLoading = ref(false)

async function startPush(row: SearchRow) {
  if (!row.download_url) {
    ElMessage.warning('该站点暂不支持直接下载')
    return
  }
  pushRow.value = row
  pushVisible.value = true
  pushLoading.value = true
  selectedDl.value = null
  try {
    downloaders.value = await downloaderApi.list()
    if (!downloaders.value.length) {
      ElMessage.warning('请先在「下载器」添加一个')
    } else {
      const def = downloaders.value.find((d) => d.is_default)
      selectedDl.value = def ? def.id : downloaders.value[0].id
    }
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载下载器失败')
  } finally {
    pushLoading.value = false
  }
}

async function confirmPush() {
  if (!pushRow.value || selectedDl.value == null) return
  const dl = downloaders.value.find((d) => d.id === selectedDl.value)
  pushLoading.value = true
  try {
    const r = await downloaderApi.add(selectedDl.value, pushRow.value.download_url)
    if (r.ok) {
      ElMessage.success(`已推送到 ${dl?.name ?? '下载器'}`)
      pushVisible.value = false
    } else {
      ElMessage.warning(r.error || '推送失败')
    }
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '推送失败')
  } finally {
    pushLoading.value = false
  }
}

// —— 种子详情抽屉：抓取并解析单条种子详情页（简介/截图/MediaInfo/文件列表）——
const detail = ref<TorrentDetail | null>(null)
const detailLoading = ref(false)
const detailVisible = ref(false)

async function openTorrentDetail(row: SearchRow) {
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

// 下载链接：打开 + 复制（详情页主操作，比小 el-link 更显眼）
function openDl() {
  if (detail.value?.download_url) window.open(detail.value.download_url, '_blank', 'noopener')
}
async function copyDl() {
  if (!detail.value?.download_url) return
  await copyText(detail.value.download_url, '已复制下载链接', '复制失败，请手动复制')
}
function imdbUrl(id: string) {
  return id ? `https://www.imdb.com/title/${id}/` : ''
}

// —— 影视模式：元数据 + 海报卡片 ——
const mediaResults = ref<MediaInfo[]>([])
const year = ref<number | undefined>(undefined)

function typeBadge(t: string) {
  return { movie: '电影', tv: '剧集', music: '音乐' }[t] || t
}
function sourceBadge(s: string) {
  return { tmdb: 'TMDB', douban: '豆瓣', musicbrainz: 'MB' }[s] || s
}
// 卡片点击 → 进入媒体详情页（对标 MoviePilot），不再跳外站。
function openMediaDetail(m: MediaInfo) {
  router.push({ path: '/media-detail', query: { source: m.source, id: m.source_id } })
}

// 接受跨页跳转带来的 keyword / mode（如详情页「搜索资源」按钮回跳）。
onMounted(() => {
  const q = route.query.keyword
  if (typeof q === 'string' && q) {
    keyword.value = q
  }
  const md = route.query.mode
  if (md === 'torrent' || md === 'media') {
    mode.value = md
  }
  if (keyword.value) search()
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
      <el-input
        v-model="keyword"
        :placeholder="mode === 'torrent'
          ? '输入电影/剧集名或关键词，跨启用站点搜索…'
          : '搜索影视（电影 / 剧集标题）'"
        clearable
        size="large"
        @keyup.enter="search"
      />
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

    <template v-else-if="mode === 'torrent'">
      <!-- 单站失败折叠提示（聚合容错：失败不影响已返回的结果） -->
      <div v-if="failedSites.length" class="fail-bar glass">
        <button class="fail-toggle" @click="showFailed = !showFailed">
          <span class="warn">⚠ {{ failedSites.length }} 个站点搜索失败</span>
          <span class="caret">{{ showFailed ? '收起' : '展开' }}</span>
        </button>
        <ul v-if="showFailed" class="fail-list">
          <li v-for="f in failedSites" :key="f.site_id">
            <strong>{{ f.site_name }}</strong>
            <span class="text-muted">{{ f.error }}</span>
          </li>
        </ul>
      </div>

      <!-- 种子结果表 -->
      <div v-loading="loading" class="card glass">
        <el-table
          v-if="rows.length"
          :data="rows"
          style="width: 100%"
          :default-sort="{ prop: 'seeders', order: 'descending' }"
          :row-style="{ background: 'transparent' }"
        >
          <el-table-column label="站点" width="120">
            <template #default="{ row }">
              <span class="site-tag">{{ row.site_name }}</span>
            </template>
          </el-table-column>
          <el-table-column label="标题" min-width="280" show-overflow-tooltip>
            <template #default="{ row }">
              <div class="t-title">{{ row.title }}</div>
              <div v-if="row.subtitle" class="t-sub text-muted">{{ row.subtitle }}</div>
            </template>
          </el-table-column>
          <el-table-column prop="size" label="大小" width="90" />
          <el-table-column prop="seeders" label="做种" width="90" sortable align="right">
            <template #default="{ row }">
              <span class="num up">↑ {{ row.seeders }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="leechers" label="吸血" width="90" sortable align="right">
            <template #default="{ row }">
              <span class="num">↓ {{ row.leechers }}</span>
            </template>
          </el-table-column>
          <el-table-column label="Free" width="70" align="center">
            <template #default="{ row }">
              <span v-if="row.free" class="chip free">FREE</span>
              <span v-else class="text-muted">—</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="160" fixed="right">
            <template #default="{ row }">
              <el-button size="small" type="primary" @click="startPush(row as SearchRow)">下载</el-button>
              <el-button size="small" @click="openTorrentDetail(row as SearchRow)">详情</el-button>
            </template>
          </el-table-column>
        </el-table>

        <!-- 空态：区分「未搜」与「搜了无果」，文案不再误导已登录站点的用户 -->
        <EmptyState
          v-else-if="!loading"
          :icon="searched ? '🔍' : '🌱'"
          :title="searched ? '没有找到相关种子' : '跨站搜索种子'"
          :description="searched ? '试试更换关键词，或切换上方的分类筛选' : '输入关键词，聚合所有已登录站点的资源'"
        />
      </div>
    </template>

    <!-- 影视模式：海报卡片 -->
    <template v-else>
      <div v-loading="loading" class="grid">
        <div
          v-for="(m, i) in mediaResults"
          :key="i"
          class="glass glass-hover mcard clickable"
          @click="openMediaDetail(m)"
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
        v-if="!mediaResults.length && !loading"
        icon="🎬"
        title="搜索影视"
        description="输入关键字查找影视元数据、海报与评分"
      />
    </template>

    <!-- 推送下载器对话框 -->
    <el-dialog v-model="pushVisible" title="推送到下载器" width="420">
      <div v-if="pushRow" class="push-target">
        <div class="t-title">{{ pushRow.title }}</div>
        <div class="text-muted">{{ pushRow.site_name }} · {{ pushRow.size }}</div>
      </div>
      <el-select
        v-if="downloaders.length"
        v-model="selectedDl"
        placeholder="选择下载器"
        style="width: 100%"
      >
        <el-option
          v-for="d in downloaders"
          :key="d.id"
          :label="`${d.name}（${d.type === 'tr' ? 'Transmission' : 'qBittorrent'}）`"
          :value="d.id"
        />
      </el-select>
      <div v-else-if="!pushLoading" class="empty text-muted">请先在「下载器」添加一个</div>
      <template #footer>
        <el-button @click="pushVisible = false">取消</el-button>
        <el-button
          type="primary"
          :loading="pushLoading"
          :disabled="!downloaders.length"
          @click="confirmPush"
        >
          推送
        </el-button>
      </template>
    </el-dialog>

    <!-- 种子详情抽屉 -->
    <el-drawer v-model="detailVisible" title="种子详情" size="50%" direction="rtl">
      <div v-loading="detailLoading" class="detail-body">
        <template v-if="detail">
          <!-- 头部：海报 + 标题/副标题 + 规格彩签 + 下载操作 -->
          <div class="detail-head glass">
            <div v-if="detail.poster_url" class="dposter"><MediaPoster :url="detail.poster_url" kind="movie" /></div>
            <div class="meta">
              <h3 class="dt-title">{{ detail.title }}</h3>
              <div v-if="detail.subtitle" class="dt-sub text-muted">{{ detail.subtitle }}</div>
              <div class="specs">
                <span v-if="detail.category" class="chip cat">{{ detail.category }}</span>
                <span v-if="detail.resolution" class="chip res">{{ detail.resolution }}</span>
                <span v-if="detail.codec" class="chip codec">{{ detail.codec }}</span>
                <span v-if="detail.source" class="chip src">{{ detail.source }}</span>
                <span v-if="detail.size" class="chip">{{ detail.size }}</span>
                <span v-if="detail.seeders !== undefined" class="chip seed">↑ {{ detail.seeders }}</span>
                <a
                  v-if="detail.imdb_id"
                  :href="imdbUrl(detail.imdb_id)"
                  target="_blank"
                  rel="noopener"
                  class="chip imdb"
                >{{ detail.imdb_id }}</a>
              </div>
              <div v-if="detail.download_url" class="dl-row">
                <el-button type="primary" @click="openDl">⬇ 下载</el-button>
                <el-button @click="copyDl">复制链接</el-button>
              </div>
            </div>
          </div>

          <div v-if="detail.screenshots?.length" class="card glass">
            <div class="sec-title">截图 <span class="text-muted">({{ detail.screenshots.length }})</span></div>
            <div class="shots">
              <a
                v-for="(s, i) in detail.screenshots"
                :key="i"
                :href="s"
                target="_blank"
                rel="noopener"
                class="shot"
              ><MediaPoster :url="s" kind="movie" /></a>
            </div>
          </div>

          <div v-if="detail.description" class="card glass">
            <div class="sec-title">简介</div>
            <div class="desc" v-html="sanitizeHtml(detail.description)"></div>
          </div>

          <div v-if="detail.mediainfo" class="card glass">
            <div class="sec-title">MediaInfo</div>
            <pre class="mediainfo">{{ detail.mediainfo }}</pre>
          </div>

          <div v-if="detail.file_list?.length" class="card glass">
            <div class="sec-title">文件列表 <span class="text-muted">({{ detail.file_list.length }})</span></div>
            <div class="file-list">
              <div v-for="(f, i) in detail.file_list" :key="i" class="file-row"><span class="file-ic">📄</span>{{ f }}</div>
            </div>
          </div>
        </template>
      </div>
    </el-drawer>
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
.search-bar :deep(.el-input) {
  flex: 1;
}
.card {
  padding: 18px;
  min-height: 200px;
}
.site-tag {
  display: inline-block;
  padding: 2px 10px;
  border-radius: 999px;
  background: rgba(108, 92, 231, 0.16);
  color: var(--pt-primary, #6c5ce7);
  font-size: 12px;
  white-space: nowrap;
}
.t-title {
  font-weight: 600;
  line-height: 1.4;
}
.t-sub {
  margin-top: 2px;
  font-size: 12px;
  line-height: 1.4;
}
.num {
  font-variant-numeric: tabular-nums;
}
.num.up {
  color: var(--pt-success, #2ecc71);
  font-weight: 600;
}
.chip {
  display: inline-block;
  padding: 1px 8px;
  border-radius: 999px;
  background: var(--surface-muted);
  font-size: 11px;
  margin: 2px;
}
.chip.free {
  background: rgba(46, 204, 113, 0.18);
  color: var(--pt-success, #2ecc71);
  font-weight: 600;
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
.fail-bar {
  padding: 10px 16px;
}
.fail-toggle {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  background: transparent;
  border: none;
  cursor: pointer;
  font-size: 14px;
  color: var(--text-strong);
}
.warn {
  color: var(--pt-warn, #f39c12);
  font-weight: 600;
}
.caret {
  font-size: 12px;
  color: var(--text-muted);
}
.fail-list {
  margin: 10px 0 0;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.fail-list li {
  font-size: 13px;
  display: flex;
  gap: 8px;
}
.push-target {
  margin-bottom: 16px;
}
/* 影视卡片网格 */
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 16px;
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
  border-radius: 8px;
  background: rgba(0, 0, 0, 0.5);
  color: #fff;
  font-size: 11px;
}
.meta {
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.mtitle {
  font-weight: 600;
  font-size: 14px;
  line-height: 1.3;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.sub {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
}
.badge {
  padding: 1px 7px;
  border-radius: 6px;
  background: linear-gradient(135deg, rgba(108, 92, 231, 0.3), rgba(66, 133, 244, 0.2));
}
.rating {
  color: var(--pt-warn, #f39c12);
}
.overview {
  font-size: 12px;
  line-height: 1.4;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
/* 种子详情抽屉 */
.detail-body {
  padding: 0 2px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.detail-head {
  display: flex;
  gap: 16px;
  padding: 16px;
  border-radius: 12px;
}
.dposter {
  width: 120px;
  height: 170px;
  border-radius: 10px;
  overflow: hidden;
  flex: none;
  background: var(--surface-muted);
  box-shadow: 0 6px 18px rgba(0, 0, 0, 0.2);
}
.detail-head .meta {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.dt-title {
  margin: 0;
  font-size: 18px;
  font-weight: 700;
  line-height: 1.35;
  word-break: break-word;
}
.dt-sub {
  font-size: 12px;
  line-height: 1.4;
}
.specs {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.specs .chip {
  margin: 0;
  font-weight: 600;
}
.specs .chip.cat {
  background: linear-gradient(135deg, rgba(108, 92, 231, 0.3), rgba(66, 133, 244, 0.2));
}
.specs .chip.res {
  background: rgba(66, 133, 244, 0.18);
  color: #4285f4;
}
.specs .chip.codec {
  background: rgba(156, 39, 176, 0.16);
  color: #9c27b0;
}
.specs .chip.src {
  background: rgba(0, 150, 136, 0.16);
  color: #009688;
}
.specs .chip.seed {
  background: rgba(46, 204, 113, 0.18);
  color: var(--pt-success, #2ecc71);
}
.specs .chip.imdb {
  background: rgba(247, 197, 41, 0.18);
  color: #c6a000;
  text-decoration: none;
  cursor: pointer;
}
.specs .chip.imdb:hover {
  filter: brightness(1.05);
}
.dl-row {
  display: flex;
  gap: 8px;
  margin-top: 4px;
}
.card {
  padding: 16px;
  border-radius: 12px;
}
.sec-title {
  font-weight: 700;
  font-size: 14px;
  margin-bottom: 10px;
  color: var(--text-strong);
}
.shots {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 8px;
}
.shot {
  display: block;
  aspect-ratio: 16/9;
  border-radius: 8px;
  overflow: hidden;
  background: var(--surface-muted);
  transition: transform 0.15s;
}
.shot:hover {
  transform: scale(1.02);
}
.desc {
  line-height: 1.7;
  font-size: 14px;
  max-height: 520px;
  overflow: auto;
  padding-right: 4px;
  word-break: break-word;
}
.desc :deep(img) {
  max-width: 100%;
  height: auto;
  border-radius: 6px;
  margin: 4px 0;
}
.desc :deep(a) {
  color: var(--pt-primary, #6c5ce7);
}
.desc :deep(table) {
  max-width: 100%;
  border-collapse: collapse;
}
.mediainfo {
  background: rgba(0, 0, 0, 0.78);
  color: #d4d4d4;
  padding: 12px;
  border-radius: 8px;
  font-size: 12px;
  line-height: 1.5;
  font-family: ui-monospace, 'Cascadia Code', monospace;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 320px;
  overflow: auto;
}
.file-list {
  max-height: 280px;
  overflow: auto;
  font-size: 12px;
  font-family: ui-monospace, monospace;
  border-radius: 8px;
  background: var(--surface-muted);
}
.file-row {
  padding: 5px 10px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
  display: flex;
  gap: 6px;
  align-items: baseline;
  word-break: break-all;
}
.file-row:last-child {
  border-bottom: none;
}
.file-ic {
  flex: none;
  opacity: 0.6;
}
:deep(.el-table),
:deep(.el-table tr),
:deep(.el-table td),
:deep(.el-table th) {
  background-color: transparent !important;
}
:deep(.el-table) {
  --el-table-border-color: rgba(0, 0, 0, 0.06);
}
</style>
