<script setup lang="ts">
import { useRoute } from 'vue-router'
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { safeUrl } from '@/utils/safeUrl'
import { ElMessage } from 'element-plus'
import { freeleechApi, type FreeleechResult, type FreeleechStatus } from '@/api/freeleech'
import EmptyState from '@/components/EmptyState.vue'
import SiteFilterDialog from '@/components/SiteFilterDialog.vue'

const { t } = useI18n()
const results = ref<FreeleechResult[]>([])
const loading = ref(false)
const syncingId = ref<number | null>(null)

// 站点的归类状态（汇总与筛选共用同一判定，避免两处分类逻辑漂移）：
// error/failed→failed，否则取首条 notice 状态，缺失（未开启检测/空）→none。
type FilterKey = 'all' | FreeleechStatus
function siteStatus(r: FreeleechResult): FreeleechStatus {
  if (r.error || r.notices[0]?.status === 'failed') return 'failed'
  const top = r.notices[0]?.status
  if (top === 'active') return 'active'
  if (top === 'upcoming') return 'upcoming'
  return 'none'
}

// 状态汇总（顶层筛选 chip 的数值）
const summary = computed(() => {
  let active = 0,
    upcoming = 0,
    none = 0,
    failed = 0
  for (const r of results.value) {
    switch (siteStatus(r)) {
      case 'active':
        active++
        break
      case 'upcoming':
        upcoming++
        break
      case 'failed':
        failed++
        break
      default:
        none++
    }
  }
  return { total: results.value.length, active, upcoming, none, failed }
})

// 顶层筛选：点 chip 按状态过滤站点卡片；再点同一个 = 取消（回 all）。
const filter = ref<FilterKey>('all')
// 站点筛选：空=全部，非空=仅选中站点（多选，纯前端筛选）。UI 用 SiteFilterDialog 组件。
const route = useRoute()
function parseSiteIdsFromQuery(): number[] {
  const raw = route.query.site_id
  const parts = Array.isArray(raw) ? raw : raw != null && raw !== '' ? [raw] : []
  return parts.map((x) => Number(x)).filter((n) => Number.isFinite(n) && n > 0)
}
const selectedSiteIds = ref<number[]>(parseSiteIdsFromQuery())
const focusFromQuery = computed(() => selectedSiteIds.value.length === 1)
const focusLabel = computed(() => {
  if (!focusFromQuery.value) return ''
  const id = selectedSiteIds.value[0]
  const fromCard = results.value.find((r) => r.site_id === id)?.site_name
  const q = route.query.site_name
  return fromCard || (typeof q === 'string' ? q : '') || `site_id=${id}`
})
const sites = computed(() => {
  const m = new Map<number, string>()
  for (const r of results.value) if (!m.has(r.site_id)) m.set(r.site_id, r.site_name)
  return [...m].map(([id, name]) => ({ id, name }))
})
const filteredResults = computed(() => {
  let list = results.value
  if (filter.value !== 'all') list = list.filter((r) => siteStatus(r) === filter.value)
  if (selectedSiteIds.value.length) list = list.filter((r) => selectedSiteIds.value.includes(r.site_id))
  return list
})
function toggleFilter(key: FilterKey) {
  filter.value = filter.value === key ? 'all' : key
}
// 汇总条 5 个 tile（站点=全部 + 4 状态），模板 v-for 渲染。color 走 statusColor（懒引用）。
const tiles = computed(() => [
  { key: 'all' as FilterKey, label: t('freeleech.tile.sites'), value: summary.value.total, color: '' },
  { key: 'active' as FilterKey, label: t('freeleech.status.active'), value: summary.value.active, color: statusColor.active },
  { key: 'upcoming' as FilterKey, label: t('freeleech.status.upcoming'), value: summary.value.upcoming, color: statusColor.upcoming },
  { key: 'none' as FilterKey, label: t('freeleech.tile.none'), value: summary.value.none, color: statusColor.none },
  { key: 'failed' as FilterKey, label: t('freeleech.tile.failed'), value: summary.value.failed, color: statusColor.failed },
])

// 展开每站全部公告（可能多条横幅）
const expanded = ref<Record<number, boolean>>({})

// 流式逐站渲染：清空后每站到达即 upsert（按 site_id），边加载边显示；切页/重新加载中断上一次。
// fetch 无整体 timeout，慢站逐个到达不再导致首屏「等很久显示失败」（缓存命中的站秒到）。
let abortCtrl: AbortController | null = null

async function load(refresh = false) {
  abortCtrl?.abort()
  const ac = new AbortController()
  abortCtrl = ac
  loading.value = true
  results.value = [] // 清空，逐站重新填充（缓存命中的站秒级到达，几乎不闪）
  try {
    await freeleechApi.streamAll(
      refresh,
      (r) => {
        const idx = results.value.findIndex((x) => x.site_id === r.site_id)
        if (idx >= 0) results.value[idx] = r
        else results.value.push(r)
      },
      ac.signal,
    )
  } catch (e: unknown) {
    if (ac.signal.aborted) return // 切页/重载中断，静默
    ElMessage.error((e as Error).message || t('freeleech.loadFailed'))
  } finally {
    if (abortCtrl === ac) {
      loading.value = false
      abortCtrl = null
    }
  }
}

async function syncOne(r: FreeleechResult) {
  syncingId.value = r.site_id
  try {
    const one = await freeleechApi.syncOne(r.site_id)
    const idx = results.value.findIndex((x) => x.site_id === r.site_id)
    if (idx >= 0) results.value[idx] = one
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('freeleech.refreshFailed'))
  } finally {
    syncingId.value = null
  }
}

const statusColor: Record<FreeleechStatus, string> = {
  active: '#2ecc71',
  upcoming: '#3498db',
  none: '#95a5a6',
  failed: '#f56c6c',
}
const statusLabel = computed<Record<FreeleechStatus, string>>(() => ({
  active: t('freeleech.status.active'),
  upcoming: t('freeleech.status.upcoming'),
  none: t('freeleech.status.none'),
  failed: t('freeleech.status.failed'),
}))

onMounted(() => load())
onUnmounted(() => abortCtrl?.abort())
</script>

<template>
  <div class="page">
    <div class="head">
      <h2 class="title">{{ t('freeleech.title') }}</h2>
      <div class="head-tools">
        <SiteFilterDialog v-model="selectedSiteIds" :sites="sites" :title="t('freeleech.siteFilterTitle')" />
        <el-button :loading="loading" @click="load(true)">{{ t('announcements.refreshAll') }}</el-button>
      </div>
    </div>

    <!-- 汇总筛选条（点 tile 按状态过滤；再点取消）-->
    <div class="summary glass">
      <div
        v-for="tile in tiles"
        :key="tile.key"
        class="tile"
        :class="{ active: filter === tile.key }"
        :title="filter === tile.key ? t('freeleech.filterOff') : t('freeleech.filterOn')"
        @click="toggleFilter(tile.key)"
      >
        <span class="num" :style="tile.color ? { color: tile.color } : undefined">{{ tile.value }}</span>
        <span class="lbl">{{ tile.label }}</span>
      </div>
    </div>

    <el-alert
      v-if="focusFromQuery && !loading"
      type="info"
      :closable="false"
      show-icon
      style="margin-bottom: 12px"
      :title="t('assessment.focusTitle', { name: focusLabel })"
    />

    <EmptyState
      v-if="!results.length && !loading"
      icon="🎉"
      :title="t('announcements.emptyTitle')"
      :description="t('freeleech.emptyDesc')"
    />
    <EmptyState
      v-else-if="!filteredResults.length"
      icon="🔍"
      :title="t('assessment.emptyFilterTitle')"
      :description="t('freeleech.emptyFilterDesc')"
    >
      <el-button @click="filter = 'all'">{{ t('freeleech.showAll') }}</el-button>
    </EmptyState>

    <div class="cards">
      <div v-for="r in filteredResults" :key="r.site_id" class="site-card glass">
        <div class="site-head">
          <span class="site-name">{{ r.site_name }}</span>
          <span
            v-if="r.notices[0]"
            class="chip"
            :style="{ background: statusColor[r.notices[0].status] + '22', color: statusColor[r.notices[0].status] }"
          >
            {{ statusLabel[r.notices[0].status] }}
          </span>
          <span class="spacer"></span>
          <el-button size="small" :loading="syncingId === r.site_id" @click="syncOne(r)">{{ t('common.refresh') }}</el-button>
        </div>
        <div v-if="r.error" class="err">{{ r.error }}</div>
        <div v-else-if="!r.notices.length" class="text-muted">{{ t('freeleech.notEnabled') }}</div>
        <div v-else class="notices">
          <div
            v-for="(n, i) in expanded[r.site_id] ? r.notices : r.notices.slice(0, 1)"
            :key="i"
            class="notice"
          >
            <div class="notice-title">{{ n.title }}</div>
            <div class="notice-content">{{ n.content || (n.error ?? '—') }}</div>
            <a v-if="n.link" :href="safeUrl(n.link)" target="_blank" rel="noopener" class="notice-link">{{ t('freeleech.goTo') }}</a>
          </div>
          <button
            v-if="r.notices.length > 1"
            class="expand"
            @click="expanded[r.site_id] = !expanded[r.site_id]"
          >
            {{ expanded[r.site_id] ? t('announcements.collapse') : t('freeleech.expandAll', { n: r.notices.length }) }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: var(--sp-4); }
.head { display: flex; align-items: center; justify-content: space-between; }
.head-tools { display: flex; align-items: center; gap: var(--sp-2); }
.title { margin: 4px 0; font-size: var(--fs-title); font-weight: 700; }
.summary { display: flex; gap: var(--sp-2); padding: 14px; flex-wrap: wrap; }
.tile {
  display: flex; flex-direction: column; align-items: center; flex: 1; min-width: 80px;
  padding: 8px 4px; border-radius: var(--radius-sm); cursor: pointer; user-select: none;
  transition: background var(--dur-fast) var(--ease-out-smooth), box-shadow var(--dur-fast);
}
.tile:hover { background: var(--pt-primary-bg); }
.tile.active { background: var(--pt-primary-bg); box-shadow: inset 0 0 0 1.5px var(--pt-primary, #6c5ce7); }
.tile .num { font-size: var(--fs-title); font-weight: 700; font-variant-numeric: tabular-nums; }
.tile .lbl { font-size: var(--fs-xs); color: var(--text-muted); margin-top: 2px; }
.reset-link { color: var(--pt-primary, #6c5ce7); cursor: pointer; }
.cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: var(--sp-3); }
.site-card { padding: 16px; }
.site-head { display: flex; align-items: center; gap: var(--sp-2); }
.site-name { font-weight: 600; }
.spacer { flex: 1; }
.chip { padding: 2px 10px; border-radius: var(--radius-pill); font-size: var(--fs-xs); white-space: nowrap; }
.err { color: var(--pt-danger); font-size: var(--fs-body); margin-top: 8px; }
.notices { margin-top: 10px; display: flex; flex-direction: column; gap: var(--sp-2); }
.notice { padding: 8px 10px; border-radius: var(--radius-sm, 8px); background: var(--pt-primary-bg); }
.notice-title { font-weight: 600; font-size: var(--fs-md); }
.notice-content { font-size: var(--fs-body); margin-top: 4px; line-height: 1.5; word-break: break-word; }
.notice-link { font-size: var(--fs-xs); color: var(--pt-primary, #6c5ce7); }
.expand { margin-top: 4px; background: transparent; border: none; cursor: pointer; color: var(--pt-primary, #6c5ce7); font-size: var(--fs-xs); }
.empty { padding: 40px; text-align: center; }
/* 免费 seed 卡 staggered 入场 */
.cards > * { animation: pt-fade-in-up var(--dur-slow) var(--ease-out-smooth) both; }
.cards > *:nth-child(1) { animation-delay: 0ms; }
.cards > *:nth-child(2) { animation-delay: 50ms; }
.cards > *:nth-child(3) { animation-delay: 100ms; }
.cards > *:nth-child(4) { animation-delay: 150ms; }
.cards > *:nth-child(5) { animation-delay: 200ms; }
.cards > *:nth-child(6) { animation-delay: 250ms; }
.cards > *:nth-child(n+7) { animation-delay: 300ms; }

</style>
