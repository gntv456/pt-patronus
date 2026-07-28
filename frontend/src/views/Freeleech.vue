<script setup lang="ts">
import { useRoute } from 'vue-router'
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { safeUrl } from '@/utils/safeUrl'
import { ElMessage } from 'element-plus'
import { freeleechApi, type FreeleechResult, type FreeleechStatus } from '@/api/freeleech'
import EmptyState from '@/components/EmptyState.vue'
import SiteFilterDialog from '@/components/SiteFilterDialog.vue'

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
  { key: 'all' as FilterKey, label: '站点', value: summary.value.total, color: '' },
  { key: 'active' as FilterKey, label: '正在站免', value: summary.value.active, color: statusColor.active },
  { key: 'upcoming' as FilterKey, label: '即将站免', value: summary.value.upcoming, color: statusColor.upcoming },
  { key: 'none' as FilterKey, label: '暂无', value: summary.value.none, color: statusColor.none },
  { key: 'failed' as FilterKey, label: '失败', value: summary.value.failed, color: statusColor.failed },
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
    ElMessage.error((e as Error).message || '加载站免失败')
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
    ElMessage.error((e as Error).message || '刷新失败')
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
const statusLabel: Record<FreeleechStatus, string> = {
  active: '正在站免',
  upcoming: '即将站免',
  none: '暂无站免',
  failed: '读取失败',
}

onMounted(() => load())
onUnmounted(() => abortCtrl?.abort())
</script>

<template>
  <div class="page" v-loading="loading">
    <div class="head">
      <h2 class="title">站免</h2>
      <div class="head-tools">
        <SiteFilterDialog v-model="selectedSiteIds" :sites="sites" title="选择站免站点" />
        <el-button :loading="loading" @click="load(true)">刷新全部</el-button>
      </div>
    </div>

    <!-- 汇总筛选条（点 tile 按状态过滤；再点取消）-->
    <div class="summary glass">
      <div
        v-for="t in tiles"
        :key="t.key"
        class="tile"
        :class="{ active: filter === t.key }"
        :title="filter === t.key ? '点击取消筛选' : '点击按此状态筛选'"
        @click="toggleFilter(t.key)"
      >
        <span class="num" :style="t.color ? { color: t.color } : undefined">{{ t.value }}</span>
        <span class="lbl">{{ t.label }}</span>
      </div>
    </div>

    <el-alert
      v-if="focusFromQuery && !loading"
      type="info"
      :closable="false"
      show-icon
      style="margin-bottom: 12px"
      :title="`已按站点详情筛选：${focusLabel}`"
    />

    <EmptyState
      v-if="!results.length && !loading"
      icon="🎉"
      title="暂无数据，请先在「站点」添加并登录站点"
      description="添加并启用 PT 站点后，这里会自动汇总各站正在/即将开始的免费活动"
    />
    <EmptyState
      v-else-if="!filteredResults.length"
      icon="🔍"
      title="当前筛选下无站点"
      description="切换或取消上方的状态筛选，显示全部站点"
    >
      <el-button @click="filter = 'all'">显示全部</el-button>
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
          <el-button size="small" :loading="syncingId === r.site_id" @click="syncOne(r)">刷新</el-button>
        </div>
        <div v-if="r.error" class="err">{{ r.error }}</div>
        <div v-else-if="!r.notices.length" class="text-muted">该站点未开启站免检测</div>
        <div v-else class="notices">
          <div
            v-for="(n, i) in expanded[r.site_id] ? r.notices : r.notices.slice(0, 1)"
            :key="i"
            class="notice"
          >
            <div class="notice-title">{{ n.title }}</div>
            <div class="notice-content">{{ n.content || (n.error ?? '—') }}</div>
            <a v-if="n.link" :href="safeUrl(n.link)" target="_blank" rel="noopener" class="notice-link">前往 →</a>
          </div>
          <button
            v-if="r.notices.length > 1"
            class="expand"
            @click="expanded[r.site_id] = !expanded[r.site_id]"
          >
            {{ expanded[r.site_id] ? '收起' : `展开全部（${r.notices.length}）` }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: 16px; }
.head { display: flex; align-items: center; justify-content: space-between; }
.head-tools { display: flex; align-items: center; gap: 10px; }
.title { margin: 4px 0; font-size: 22px; font-weight: 700; }
.summary { display: flex; gap: 8px; padding: 14px; flex-wrap: wrap; }
.tile {
  display: flex; flex-direction: column; align-items: center; flex: 1; min-width: 80px;
  padding: 8px 4px; border-radius: 10px; cursor: pointer; user-select: none;
  transition: background 0.15s, box-shadow 0.15s;
}
.tile:hover { background: rgba(108, 92, 231, 0.08); }
.tile.active { background: rgba(108, 92, 231, 0.14); box-shadow: inset 0 0 0 1.5px var(--pt-primary, #6c5ce7); }
.tile .num { font-size: 24px; font-weight: 700; font-variant-numeric: tabular-nums; }
.tile .lbl { font-size: 12px; color: var(--text-muted); margin-top: 2px; }
.reset-link { color: var(--pt-primary, #6c5ce7); cursor: pointer; }
.cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: 14px; }
.site-card { padding: 16px; }
.site-head { display: flex; align-items: center; gap: 8px; }
.site-name { font-weight: 600; }
.spacer { flex: 1; }
.chip { padding: 2px 10px; border-radius: 999px; font-size: 12px; white-space: nowrap; }
.err { color: var(--pt-danger); font-size: 13px; margin-top: 8px; }
.notices { margin-top: 10px; display: flex; flex-direction: column; gap: 8px; }
.notice { padding: 8px 10px; border-radius: var(--radius-sm, 8px); background: rgba(108,92,231,0.06); }
.notice-title { font-weight: 600; font-size: 14px; }
.notice-content { font-size: 13px; margin-top: 4px; line-height: 1.5; word-break: break-word; }
.notice-link { font-size: 12px; color: var(--pt-primary, #6c5ce7); }
.expand { margin-top: 4px; background: transparent; border: none; cursor: pointer; color: var(--pt-primary, #6c5ce7); font-size: 12px; }
.empty { padding: 40px; text-align: center; }
</style>
