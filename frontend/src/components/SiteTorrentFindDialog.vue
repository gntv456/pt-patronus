<script setup lang="ts">
/**
 * MoviePilot-style "find on sites" dialog.
 * Card grid + "更多来源" grouping, progressive results while parent streams.
 */
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import type { SiteSearchResult, SiteTorrent } from '@/api/search'
import { siteApi, type Site } from '@/api/site'
import { downloaderApi, type Downloader } from '@/api/downloader'
import SiteFavicon from '@/components/SiteFavicon.vue'
import MediaPoster from '@/components/MediaPoster.vue'
import {
  cardGroupKey,
  extractTorrentTags,
  formatTorrentSize,
  relativeTime,
  volumeFactorLabel,
  type TorrentTag,
} from '@/utils/torrentMeta'

export type FindRow = SiteTorrent & {
  site_id: number
  site_name: string
  base_url?: string
}

type CardGroup = {
  key: string
  primary: FindRow
  more: FindRow[]
  tags: TorrentTag[]
  promo: ReturnType<typeof volumeFactorLabel>
  timeText: string
}

const props = withDefaults(
  defineProps<{
    modelValue: boolean
    title: string
    loading: boolean
    results: SiteSearchResult[]
    posterUrl?: string
  }>(),
  { posterUrl: '' },
)

const emit = defineEmits<{
  'update:modelValue': [v: boolean]
  close: []
}>()

const router = useRouter()
const sites = ref<Site[]>([])
const siteMap = computed(() => {
  const m = new Map<number, Site>()
  for (const s of sites.value) m.set(s.id, s)
  return m
})

onMounted(() => {
  siteApi.list().then((list) => { sites.value = list }).catch(() => {})
})

const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => {
    emit('update:modelValue', v)
    if (!v) emit('close')
  },
})

const flatRows = computed<FindRow[]>(() =>
  props.results.flatMap((r) =>
    (r.torrents ?? []).map((t) => ({
      ...t,
      site_id: r.site_id,
      site_name: r.site_name,
      base_url: siteMap.value.get(r.site_id)?.base_url || '',
    })),
  ),
)

const hitSites = computed(() =>
  props.results.filter((r) => !r.error && (r.torrents?.length ?? 0) > 0).length,
)
const hitTotal = computed(() => flatRows.value.length)
const failedSites = computed(() => props.results.filter((r) => r.error))
const showFailed = ref(false)

const cards = computed<CardGroup[]>(() => {
  const map = new Map<string, FindRow[]>()
  for (const row of flatRows.value) {
    const key = cardGroupKey(row)
    const list = map.get(key)
    if (list) list.push(row)
    else map.set(key, [row])
  }
  const groups: CardGroup[] = []
  for (const [key, list] of map) {
    list.sort((a, b) => (b.seeders || 0) - (a.seeders || 0))
    const primary = list[0]
    // 副标题只参与规则识别，不作为 raw tag（避免卡片上副标题显示两遍）
    const tags = extractTorrentTags(
      `${primary.title} ${primary.subtitle || ''}`,
      [...(primary.tags || []), ...(primary.labels || []), primary.category || ''].filter(Boolean),
    )
    groups.push({
      key,
      primary,
      more: list.slice(1),
      tags,
      promo: volumeFactorLabel(
        primary.uploadvolumefactor,
        primary.downloadvolumefactor,
        primary.free,
        [...(primary.tags || []), ...(primary.labels || [])],
      ),
      timeText: relativeTime(primary.time || primary.pub_date),
    })
  }
  groups.sort((a, b) => {
    const promoRank = (g: CardGroup) => {
      if (!g.promo) return 2
      if (g.promo.tone === 'free') return 0
      return 1
    }
    const d = promoRank(a) - promoRank(b)
    if (d !== 0) return d
    return (b.primary.seeders || 0) - (a.primary.seeders || 0)
  })
  return groups
})

// —— 更多来源 ——
const moreVisible = ref(false)
const moreItems = ref<FindRow[]>([])

function openMore(group: CardGroup) {
  moreItems.value = group.more
  moreVisible.value = true
}

function promoOf(row: FindRow) {
  return volumeFactorLabel(
    row.uploadvolumefactor,
    row.downloadvolumefactor,
    row.free,
    [...(row.tags || []), ...(row.labels || [])],
  )
}

function openDetail(row: FindRow) {
  if (row.detail_url) window.open(row.detail_url, '_blank', 'noopener')
}

// —— 推送下载器 ——
const pushVisible = ref(false)
const pushRow = ref<FindRow | null>(null)
const downloaders = ref<Downloader[]>([])
const selectedDl = ref<number | null>(null)
const categories = ref<string[]>([])
const selectedCategory = ref('')
const savePath = ref('')
const showAdvanced = ref(false)
const paused = ref(false)
const pushLoading = ref(false)
const pushedDlName = ref('')
const pushedDlId = ref<number | null>(null)

const pushSubtitle = computed(() => {
  const row = pushRow.value
  if (!row) return ''
  return [row.site_name, props.title].filter(Boolean).join(' · ')
})

async function loadCategories(dlId: number | null) {
  categories.value = []
  selectedCategory.value = ''
  if (dlId == null) return
  try {
    const r = await downloaderApi.categories(dlId)
    categories.value = r.categories ?? []
  } catch {
    categories.value = []
  }
}

watch(selectedDl, (id) => {
  void loadCategories(id)
})

const dlTypeLabel: Record<string, string> = {
  qb: 'qBittorrent',
  tr: 'Transmission',
  ut: 'uTorrent',
  dl: 'Deluge',
  ar: 'Aria2',
  rt: 'ruTorrent',
  fl: 'Flood',
}

async function startPush(row: FindRow) {
  if (!row.download_url) {
    if (row.detail_url) {
      openDetail(row)
      return
    }
    ElMessage.warning('该站点暂不支持直接下载')
    return
  }
  pushRow.value = row
  pushVisible.value = true
  pushLoading.value = true
  selectedDl.value = null
  selectedCategory.value = ''
  savePath.value = ''
  showAdvanced.value = false
  paused.value = false
  categories.value = []
  pushedDlName.value = ''
  pushedDlId.value = null
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
    const r = await downloaderApi.add(selectedDl.value, pushRow.value.download_url, {
      category: selectedCategory.value || undefined,
      save_path: savePath.value.trim() || undefined,
      paused: paused.value || undefined,
    })
    if (r.ok) {
      ElMessage.success(`已推送到 ${dl?.name ?? '下载器'}`)
      pushedDlName.value = dl?.name ?? '下载器'
      pushedDlId.value = selectedDl.value
    } else {
      ElMessage.warning(r.error || '推送失败')
    }
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '推送失败')
  } finally {
    pushLoading.value = false
  }
}

function gotoDownloaderTasks() {
  const id = pushedDlId.value
  closePush()
  if (id != null) router.push({ name: 'downloaders', query: { dl: String(id) } })
  else router.push({ name: 'downloaders' })
}

function closePush() {
  pushVisible.value = false
  pushRow.value = null
  pushedDlName.value = ''
  pushedDlId.value = null
  showAdvanced.value = false
}

function close() {
  visible.value = false
}

watch(
  () => props.modelValue,
  (v) => {
    if (!v) {
      moreVisible.value = false
      showFailed.value = false
      closePush()
    }
  },
)
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="`在站点查找：${title}`"
    width="min(1560px, 98vw)"
    class="site-find-dialog"
    append-to-body
    align-center
    destroy-on-close
    @closed="emit('close')"
  >
    <div class="sf-body">
      <div class="sf-status">
        <template v-if="loading">
          <span class="sf-spinner" aria-hidden="true" />
          <span>
            正在跨站搜索…
            <template v-if="results.length">
              已返回 <b>{{ results.length }}</b> 站
              <template v-if="hitTotal"> · 命中 <b>{{ hitTotal }}</b> 条</template>
            </template>
          </span>
        </template>
        <template v-else>
          命中 <b>{{ hitSites }}</b> 个站点，共 <b>{{ hitTotal }}</b> 条种子
          <span v-if="results.length" class="text-muted">（共查询 {{ results.length }} 站 · 展示 {{ cards.length }} 组）</span>
        </template>
      </div>

      <div v-if="failedSites.length" class="sf-fail">
        <button type="button" class="sf-fail-toggle" @click="showFailed = !showFailed">
          <span>⚠ {{ failedSites.length }} 个站点搜索失败</span>
          <span class="text-muted">{{ showFailed ? '收起' : '展开' }}</span>
        </button>
        <ul v-if="showFailed" class="sf-fail-list">
          <li v-for="f in failedSites" :key="f.site_id">
            <strong>{{ f.site_name }}</strong>
            <span class="text-muted">{{ f.error }}</span>
          </li>
        </ul>
      </div>

      <div v-if="!loading && !flatRows.length" class="sf-empty text-muted">
        未从任何站点返回结果
      </div>

      <div v-else class="sf-grid">
        <article
          v-for="g in cards"
          :key="g.key"
          class="sf-card"
          :class="{ 'has-poster': !!posterUrl }"
          @click="startPush(g.primary)"
        >
          <div v-if="posterUrl" class="sf-card-bg" aria-hidden="true">
            <MediaPoster :url="posterUrl" kind="movie" />
          </div>
          <div class="sf-card-veil" />

          <div
            v-if="g.promo"
            class="sf-promo"
            :class="`tone-${g.promo.tone}`"
          >{{ g.promo.text }}</div>

          <header class="sf-card-head">
            <div class="sf-media-title" :title="title">{{ title }}</div>
            <div class="sf-site-row">
              <div class="sf-site">
                <SiteFavicon :name="g.primary.site_name" :base-url="g.primary.base_url" :size="18" />
                <span class="sf-site-name">{{ g.primary.site_name }}</span>
              </div>
              <div class="sf-peers">
                <span class="up">↑ {{ g.primary.seeders ?? 0 }}</span>
                <span class="down">↓ {{ g.primary.leechers ?? 0 }}</span>
              </div>
            </div>
          </header>

          <div class="sf-card-body">
            <div class="sf-t-title" :title="g.primary.title">{{ g.primary.title }}</div>
            <div
              v-if="g.primary.subtitle"
              class="sf-t-sub text-muted"
              :title="g.primary.subtitle"
            >{{ g.primary.subtitle }}</div>
            <div v-if="g.timeText" class="sf-time text-muted">⏱ {{ g.timeText }}</div>
            <div v-if="g.tags.length" class="sf-tags">
              <span
                v-for="tag in g.tags"
                :key="tag.label"
                class="sf-tag"
                :class="`tone-${tag.tone}`"
              >{{ tag.label }}</span>
            </div>
          </div>

          <footer class="sf-card-foot">
            <button
              v-if="g.more.length"
              type="button"
              class="sf-more"
              @click.stop="openMore(g)"
            >
              ▾ 更多来源 ({{ g.more.length }})
            </button>
            <span v-else />
            <div class="sf-foot-right">
              <span v-if="formatTorrentSize(g.primary.size)" class="sf-size">{{ formatTorrentSize(g.primary.size) }}</span>
              <button
                v-if="g.primary.detail_url"
                type="button"
                class="sf-info"
                title="打开详情"
                @click.stop="openDetail(g.primary)"
              >ⓘ</button>
            </div>
          </footer>
        </article>
      </div>
    </div>

    <template #footer>
      <el-button v-if="loading" @click="close">取消</el-button>
      <el-button v-else type="primary" @click="close">关闭</el-button>
    </template>
  </el-dialog>

  <!-- 其他来源 -->
  <el-dialog
    v-model="moreVisible"
    title="其他来源"
    width="min(420px, 92vw)"
    append-to-body
    align-center
    class="sf-more-dialog"
  >
    <div class="sf-more-list">
      <button
        v-for="(item, i) in moreItems"
        :key="`${item.site_id}-${item.id || i}`"
        type="button"
        class="sf-more-row"
        @click="startPush(item)"
      >
        <div class="sf-more-left">
          <SiteFavicon :name="item.site_name" :base-url="item.base_url" :size="16" />
          <span class="sf-more-name">{{ item.site_name }}</span>
          <span
            v-if="promoOf(item)"
            class="sf-promo-chip"
            :class="`tone-${promoOf(item)!.tone}`"
          >{{ promoOf(item)!.text }}</span>
        </div>
        <div class="sf-more-right">
          <span v-if="formatTorrentSize(item.size)" class="sf-size">{{ formatTorrentSize(item.size) }}</span>
          <span class="up">↑ {{ item.seeders ?? 0 }}</span>
          <span class="down">↓ {{ item.leechers ?? 0 }}</span>
          <span
            v-if="item.detail_url"
            class="sf-ext"
            title="详情"
            @click.stop="openDetail(item)"
          >↗</span>
        </div>
      </button>
    </div>
  </el-dialog>

  <!-- 确认下载（对齐 MoviePilot） -->
  <el-dialog
    v-model="pushVisible"
    width="min(520px, 94vw)"
    append-to-body
    align-center
    class="sf-push-dialog"
    @closed="closePush"
  >
    <template #header>
      <div class="sf-push-header">
        <div class="sf-push-header-text">
          <div class="sf-push-title-main">确认下载</div>
          <div v-if="pushSubtitle" class="sf-push-title-sub text-muted">{{ pushSubtitle }}</div>
        </div>
      </div>
    </template>

    <div v-if="pushedDlName" class="sf-push-ok">
      <p>已推送到 <b>{{ pushedDlName }}</b></p>
      <div class="sf-push-actions">
        <el-button type="primary" @click="gotoDownloaderTasks">查看任务</el-button>
        <el-button @click="closePush">关闭</el-button>
      </div>
    </div>

    <div v-else v-loading="pushLoading" class="sf-push-body">
      <div class="sf-push-media">
        <div v-if="posterUrl" class="sf-push-poster">
          <MediaPoster :url="posterUrl" kind="movie" />
        </div>
        <div class="sf-push-meta">
          <div v-if="pushRow" class="sf-push-t-title" :title="pushRow.title">
            {{ pushRow.title }}
            <span class="up">↑{{ pushRow.seeders ?? 0 }}</span>
            <span class="down">↓{{ pushRow.leechers ?? 0 }}</span>
          </div>
          <div
            v-if="pushRow?.subtitle"
            class="sf-push-t-sub text-muted"
            :title="pushRow.subtitle"
          >{{ pushRow.subtitle }}</div>
          <div v-if="pushRow?.size" class="sf-push-size-row">
            <span v-if="formatTorrentSize(pushRow.size)" class="sf-size">{{ formatTorrentSize(pushRow.size) }}</span>
            <span v-if="pushRow.category" class="sf-push-cat text-muted">{{ pushRow.category }}</span>
          </div>
        </div>
      </div>

      <div class="sf-push-fields">
        <div class="sf-push-field">
          <label>下载器</label>
          <el-select v-model="selectedDl" placeholder="选择下载器" style="width: 100%">
            <el-option
              v-for="d in downloaders"
              :key="d.id"
              :label="`${d.name}${d.is_default ? '（默认）' : ''} · ${dlTypeLabel[d.type] || d.type}`"
              :value="d.id"
            />
          </el-select>
        </div>
        <div class="sf-push-field">
          <label>保存目录</label>
          <el-select
            v-model="selectedCategory"
            clearable
            filterable
            allow-create
            default-first-option
            placeholder="自动（下载器默认）"
            style="width: 100%"
          >
            <el-option label="自动（下载器默认）" value="" />
            <el-option v-for="c in categories" :key="c" :label="c" :value="c" />
          </el-select>
        </div>
      </div>

      <button type="button" class="sf-adv-toggle" @click="showAdvanced = !showAdvanced">
        {{ showAdvanced ? '▾ 收起高级选项' : '▸ 显示高级选项' }}
      </button>
      <div v-if="showAdvanced" class="sf-push-advanced">
        <div class="sf-push-field">
          <label>自定义保存路径</label>
          <el-input v-model="savePath" placeholder="留空则用分类/下载器默认路径" clearable />
        </div>
        <div class="sf-push-field row">
          <label>添加后暂停</label>
          <el-switch v-model="paused" />
        </div>
      </div>

      <div class="sf-push-actions primary">
        <el-button
          type="primary"
          size="large"
          class="sf-start-btn"
          :disabled="selectedDl == null || !pushRow?.download_url"
          :loading="pushLoading"
          @click="confirmPush"
        >
          ⬇ 开始下载
        </el-button>
      </div>
    </div>
  </el-dialog>
</template>

<style scoped>
.sf-body {
  min-height: 160px;
  max-height: min(82vh, 900px);
  overflow-y: auto;
  padding-right: 2px;
}
.sf-status {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  font-size: 13px;
  color: var(--text-strong);
}
.sf-spinner {
  width: 14px;
  height: 14px;
  border: 2px solid rgba(108, 92, 231, 0.25);
  border-top-color: var(--pt-primary, #6c5ce7);
  border-radius: 50%;
  animation: sf-spin 0.7s linear infinite;
  flex: none;
}
@keyframes sf-spin {
  to { transform: rotate(360deg); }
}
.sf-fail {
  margin-bottom: 12px;
  padding: 8px 12px;
  border-radius: 10px;
  background: rgba(231, 76, 60, 0.08);
  border: 1px solid rgba(231, 76, 60, 0.18);
}
.sf-fail-toggle {
  display: flex;
  width: 100%;
  justify-content: space-between;
  gap: 8px;
  background: transparent;
  border: 0;
  cursor: pointer;
  font-size: 13px;
  color: var(--text-strong);
  padding: 0;
}
.sf-fail-list {
  margin: 8px 0 0;
  padding-left: 18px;
  font-size: 12px;
}
.sf-fail-list li {
  margin: 2px 0;
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.sf-empty {
  padding: 40px 8px;
  text-align: center;
}
.sf-grid {
  display: grid;
  /* 宽屏约 6 列；窄屏 auto-fill 自动减列 */
  /* 宽屏约 5 列 */
  grid-template-columns: repeat(auto-fill, minmax(290px, 1fr));
  gap: 12px;
}
.sf-card {
  position: relative;
  display: flex;
  flex-direction: column;
  min-height: 190px;
  padding: 12px 12px 10px;
  border-radius: 12px;
  background: var(--surface-1, #fff);
  border: 1px solid var(--surface-border, rgba(15, 23, 42, 0.1));
  box-shadow: var(--surface-shadow, 0 1px 2px rgba(15, 23, 42, 0.05));
  cursor: pointer;
  transition: transform 0.15s ease, box-shadow 0.15s ease, border-color 0.15s ease;
  overflow: hidden;
}
.sf-card:hover {
  transform: translateY(-3px);
  border-color: var(--surface-border-strong, rgba(108, 92, 231, 0.45));
  box-shadow: var(--surface-shadow-hover, 0 8px 20px rgba(108, 92, 231, 0.12));
}
.sf-card-bg {
  position: absolute;
  inset: 0;
  z-index: 0;
  pointer-events: none;
  /* 清晰海报：不磨砂，略提高饱和度 */
  opacity: 0.78;
  transform: scale(1.02);
  filter: saturate(1.08) contrast(1.02);
}
.sf-card-bg :deep(img),
.sf-card-bg :deep(.no-poster) {
  width: 100%;
  height: 100%;
  object-fit: cover;
  object-position: center 20%;
}
.sf-card-veil {
  position: absolute;
  inset: 0;
  z-index: 0;
  pointer-events: none;
  /* 左侧/上方略遮罩保可读，右侧/下方透出海报 */
  background:
    linear-gradient(
      115deg,
      color-mix(in srgb, var(--surface-1, #fff) 86%, transparent) 0%,
      color-mix(in srgb, var(--surface-1, #fff) 62%, transparent) 38%,
      color-mix(in srgb, var(--surface-1, #fff) 28%, transparent) 68%,
      color-mix(in srgb, var(--surface-1, #fff) 12%, transparent) 100%
    ),
    linear-gradient(
      180deg,
      color-mix(in srgb, var(--surface-1, #fff) 35%, transparent) 0%,
      transparent 42%,
      color-mix(in srgb, var(--surface-1, #fff) 40%, transparent) 100%
    );
}
.sf-card > :not(.sf-card-bg):not(.sf-card-veil) {
  position: relative;
  z-index: 1;
}
.sf-promo {
  position: absolute;
  top: 0;
  right: 0;
  z-index: 2;
  padding: 2px 8px;
  border-bottom-left-radius: 8px;
  color: #fff;
  font-size: 11px;
  font-weight: 700;
  line-height: 1.4;
}
.sf-promo.tone-free { background: #4caf50; }
.sf-promo.tone-half { background: #ff5722; }
.sf-promo.tone-double { background: #9c27b0; }
.sf-promo.tone-plain { background: #607d8b; }
.sf-card-head { margin-bottom: 4px; padding-right: 38px; }
.sf-media-title {
  font-size: 13px;
  font-weight: 700;
  color: var(--text-strong);
  margin-bottom: 4px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  text-shadow: 0 1px 2px color-mix(in srgb, var(--surface-1, #fff) 70%, transparent);
}
.sf-site-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.sf-site {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
.sf-site-name {
  font-size: 11px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sf-peers {
  display: inline-flex;
  gap: 5px;
  font-size: 10.5px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  flex: none;
}
.sf-peers .up { color: var(--pt-success, #2ecc71); }
.sf-peers .down { color: #f39c12; }
.sf-card-body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.sf-t-title {
  font-size: 11.5px;
  font-weight: 600;
  line-height: 1.4;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  word-break: break-all;
  text-shadow: 0 1px 2px color-mix(in srgb, var(--surface-1, #fff) 75%, transparent);
}
.sf-t-sub {
  font-size: 10.5px;
  line-height: 1.35;
  color: var(--text-muted);
  display: -webkit-box;
  -webkit-line-clamp: 1;
  -webkit-box-orient: vertical;
  overflow: hidden;
  word-break: break-all;
}
.sf-time {
  font-size: 11px;
  margin-top: 1px;
}
.sf-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 3px;
  margin-top: 4px;
}
.sf-tag {
  display: inline-block;
  padding: 0 5px;
  border-radius: 3px;
  font-size: 10px;
  font-weight: 600;
  color: #fff;
  line-height: 1.5;
}
.sf-tag.tone-res { background: #7b1fa2; }
.sf-tag.tone-src { background: #8000ff; }
.sf-tag.tone-codec { background: #ff9800; }
.sf-tag.tone-hdr { background: #e91e63; }
.sf-tag.tone-audio { background: #00897b; }
.sf-tag.tone-lang { background: #5c6bc0; }
.sf-tag.tone-misc { background: #607d8b; }
.sf-card-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-top: 5px;
  padding-top: 5px;
  border-top: 1px solid var(--divider, rgba(0, 0, 0, 0.06));
}
.sf-more {
  background: transparent;
  border: 0;
  color: var(--pt-primary, #6c5ce7);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  padding: 0;
}
.sf-more:hover { text-decoration: underline; }
.sf-foot-right {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.sf-size {
  display: inline-block;
  padding: 0 6px;
  border-radius: 3px;
  background: rgba(108, 92, 231, 0.14);
  color: var(--pt-primary, #6c5ce7);
  font-size: 10px;
  font-weight: 700;
}
.sf-info {
  width: 26px;
  height: 26px;
  border-radius: 50%;
  border: 0;
  background: transparent;
  color: var(--pt-primary, #6c5ce7);
  cursor: pointer;
  font-size: 14px;
  line-height: 1;
}
.sf-info:hover { background: rgba(108, 92, 231, 0.1); }

.sf-more-list {
  max-height: 60vh;
  overflow-y: auto;
}
.sf-more-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  width: 100%;
  padding: 10px 8px;
  border: 0;
  border-bottom: 1px solid var(--divider, rgba(0, 0, 0, 0.06));
  background: transparent;
  cursor: pointer;
  text-align: left;
}
.sf-more-row:hover { background: rgba(108, 92, 231, 0.06); }
.sf-more-left,
.sf-more-right {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
.sf-more-name {
  font-size: 13px;
  font-weight: 600;
}
.sf-promo-chip {
  display: inline-block;
  padding: 0 6px;
  border-radius: 4px;
  color: #fff;
  font-size: 11px;
  font-weight: 700;
}
.sf-promo-chip.tone-free { background: #4caf50; }
.sf-promo-chip.tone-half { background: #ff5722; }
.sf-promo-chip.tone-double { background: #9c27b0; }
.sf-promo-chip.tone-plain { background: #607d8b; }
.sf-more-right .up {
  color: var(--pt-success, #2ecc71);
  font-size: 12px;
  font-weight: 700;
}
.sf-ext {
  color: var(--text-muted);
  font-size: 13px;
  padding: 0 2px;
}
.sf-push-header-text { min-width: 0; }
.sf-push-title-main {
  font-size: 16px;
  font-weight: 700;
  line-height: 1.3;
}
.sf-push-title-sub {
  margin-top: 2px;
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sf-push-body { display: flex; flex-direction: column; gap: 14px; }
.sf-push-media {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}
.sf-push-poster {
  width: 72px;
  height: 108px;
  border-radius: 8px;
  overflow: hidden;
  flex: none;
  background: var(--surface-muted, #f0f0f0);
  border: 1px solid var(--divider, rgba(0,0,0,0.06));
}
.sf-push-meta {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.sf-push-t-title {
  font-size: 13px;
  font-weight: 600;
  line-height: 1.45;
  word-break: break-word;
}
.sf-push-t-title .up,
.sf-push-t-title .down {
  margin-left: 8px;
  font-size: 12px;
  font-weight: 700;
  white-space: nowrap;
}
.sf-push-t-sub {
  font-size: 12px;
  line-height: 1.45;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
  word-break: break-word;
}
.sf-push-size-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.sf-push-cat { font-size: 12px; }
.sf-push-fields {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}
@media (max-width: 520px) {
  .sf-push-fields { grid-template-columns: 1fr; }
}
.sf-push-field label {
  display: block;
  font-size: 12px;
  color: var(--text-muted);
  margin-bottom: 4px;
}
.sf-push-field.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.sf-push-field.row label { margin: 0; }
.sf-adv-toggle {
  align-self: flex-start;
  background: transparent;
  border: 0;
  color: var(--pt-primary, #6c5ce7);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  padding: 0;
}
.sf-push-advanced {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 10px;
  background: var(--surface-muted, rgba(0,0,0,0.03));
}
.sf-push-ok p { margin: 0 0 14px; }
.sf-push-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
.sf-push-actions.primary {
  justify-content: stretch;
  margin-top: 2px;
}
.sf-start-btn {
  width: 100%;
  font-weight: 700;
}
</style>
