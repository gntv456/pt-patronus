<script setup lang="ts">
/**
 * 种子模式结果区：失败站折叠提示 + 流式进度 + 筛选条 + 结果表 + 渲染上限 + 空态。
 * 数据/筛选/状态均由父组件传入；下载与详情通过事件交还父组件处理。
 */
import SearchFilterBar from '@/components/SearchFilterBar.vue'
import EmptyState from '@/components/EmptyState.vue'
import { useSearchFilter, type SearchFilterHandle } from '@/composables/useSearchFilter'
import type { EnrichedMeta } from '@/utils/torrentMeta'
import type { SiteSearchResult, SiteTorrent } from '@/api/search'

// 扁平表行：种子原始字段 + 来源站点名 + 派生筛选 meta（enrichTorrent 后）。
type SearchRow = SiteTorrent & EnrichedMeta & { site_id: number; site_name: string }

const props = defineProps<{
  // useSearchFilter 句柄（filter/sort/options/filtered/...），由父组件创建并维护。
  filter: SearchFilterHandle
  // 派生 enriched 行（enrichTorrent 后）
  enrichedRows: SearchRow[]
  results: SiteSearchResult[]
  loading: boolean
  searched: boolean
  scopeSiteCount: number
}>()

const emit = defineEmits<{
  startPush: [row: SearchRow]
  openDetail: [row: SearchRow]
}>()

// 失败站折叠本地状态
import { ref, computed } from 'vue'
const showFailed = ref(false)
const failedSites = computed(() => props.results.filter((r) => r.error))
const streamDoneSites = computed(() => props.results.length)

// 失败站后端 JSON 里 torrents 可能是 null（Go slice nil），不能直接 .map。
// 渲染上限：跨 10+ 站可能返回数百行，一次性渲染卡顿；默认截断，提供「显示全部」展开。
const MAX_ROWS = 200
const showAll = ref(false)
const visibleRows = computed(() => {
  const s = props.filter.sorted.value
  return showAll.value ? s : s.slice(0, MAX_ROWS)
})
// showAll 需要随新搜索重置：父组件控制 loading/results，这里用 watch 不可靠（loading 不一定触发），
// 改由父组件在每次 search 重置。保留本地 ref 即可。
defineExpose({ resetShowAll: () => { showAll.value = false } })
</script>

<template>
  <div>
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
    <!-- 流式：有结果即显；仅空结果时全卡 loading，避免遮罩挡住「搜出即显」 -->
    <p v-if="loading" class="stream-progress text-muted">
      搜索中… 已返回 {{ streamDoneSites }} / {{ scopeSiteCount }} 站
      <template v-if="enrichedRows.length"> · 命中 {{ enrichedRows.length }} 条</template>
    </p>
    <SearchFilterBar
      v-if="enrichedRows.length"
      :filter="filter.filter"
      :sort="filter.sort"
      :options="filter.options.value"
      :active-tags="filter.activeTags.value"
      :has-active="filter.hasActive.value"
      :result-count="filter.filtered.value.length"
      :total-count="enrichedRows.length"
      :remove-filter="filter.removeFilter"
      :clear-all="filter.clearAll"
    />
    <div v-loading="loading && !enrichedRows.length" class="card glass">
      <el-table
        v-if="visibleRows.length"
        :data="visibleRows"
        style="width: 100%"
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
            <div v-if="row.promo || row.tags?.length" class="t-tags">
              <span v-if="row.promo" class="ttag promo" :class="`tone-${row.promo.tone}`">{{ row.promo.text }}</span>
              <span v-for="tag in row.tags" :key="tag.label" class="ttag" :class="`tone-${tag.tone}`">{{ tag.label }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="size" label="大小" width="90" />
        <el-table-column prop="seeders" label="做种" width="90" align="right">
          <template #default="{ row }">
            <span class="num up">↑ {{ row.seeders }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="leechers" label="吸血" width="90" align="right">
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
            <el-button size="small" type="primary" @click="emit('startPush', row as SearchRow)">下载</el-button>
            <el-button size="small" @click="emit('openDetail', row as SearchRow)">详情</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="filter.filtered.value.length > visibleRows.length" class="row-cap">
        已显示前 {{ visibleRows.length }} 条 · 筛选后 {{ filter.filtered.value.length }} 条（共 {{ enrichedRows.length }} 条） ·
        <el-button link type="primary" @click="showAll = true">显示全部</el-button>
      </div>
      <div v-else-if="showAll && filter.filtered.value.length > MAX_ROWS" class="row-cap">
        筛选后 {{ filter.filtered.value.length }} 条（共 {{ enrichedRows.length }} 条） ·
        <el-button link type="primary" @click="showAll = false">收起</el-button>
      </div>

      <!-- 空态：区分「未搜」/「搜了无果」/「筛选无结果」 -->
      <EmptyState
        v-else-if="!loading"
        :icon="searched ? (filter.hasActive.value ? '🔎' : '🔍') : '🌱'"
        :title="searched ? (filter.hasActive.value ? '筛选后无结果' : '没有找到相关种子') : '跨站搜索种子'"
        :description="searched ? (filter.hasActive.value ? '当前筛选条件太严格，试试清除部分筛选' : '试试更换关键词，或切换上方的分类筛选') : '输入关键词，聚合所有已登录站点的资源'"
      />
    </div>
  </div>
</template>

<style scoped>
.card {
  padding: 18px;
  min-height: 200px;
}
.site-tag {
  display: inline-block;
  padding: 2px 10px;
  border-radius: var(--radius-pill);
  background: var(--pt-primary-bg);
  color: var(--pt-primary, #6c5ce7);
  font-size: var(--fs-xs);
  white-space: nowrap;
}
.t-title {
  font-weight: 600;
  line-height: 1.4;
}
.t-sub {
  margin-top: 2px;
  font-size: var(--fs-xs);
  line-height: 1.4;
}
.t-tags {
  display: flex;
  flex-wrap: wrap;
  gap: var(--sp-1);
  margin-top: 3px;
}
.ttag {
  display: inline-block;
  padding: 0 5px;
  border-radius: var(--radius-2xs);
  font-size: var(--fs-2xs);
  font-weight: 600;
  color: #fff;
  line-height: 1.5;
}
.ttag.tone-res { background: #7b1fa2; }
.ttag.tone-src { background: #8000ff; }
.ttag.tone-codec { background: #ff9800; }
.ttag.tone-hdr { background: #e91e63; }
.ttag.tone-audio { background: #00897b; }
.ttag.tone-lang { background: #5c6bc0; }
.ttag.tone-misc { background: #607d8b; }
.ttag.promo { background: var(--pt-success, #2ecc71); }
.ttag.promo.tone-half { background: #ff5722; }
.ttag.promo.tone-double { background: #9c27b0; }
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
  border-radius: var(--radius-pill);
  background: var(--surface-muted);
  font-size: var(--fs-2xs);
  margin: 2px;
}
.chip.free {
  background: var(--pt-success-bg);
  color: var(--pt-success, #2ecc71);
  font-weight: 600;
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
  font-size: var(--fs-md);
  color: var(--text-strong);
}
.warn {
  color: var(--pt-warn, #f39c12);
  font-weight: 600;
}
.caret {
  font-size: var(--fs-xs);
  color: var(--text-muted);
}
.fail-list {
  margin: 10px 0 0;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
}
.fail-list li {
  font-size: var(--fs-body);
  display: flex;
  gap: var(--sp-2);
}
.row-cap { padding: var(--sp-2) var(--sp-3); font-size: var(--fs-sm); color: var(--text-muted); }

.stream-progress {
  margin: 0 0 10px;
  font-size: var(--fs-body);
  line-height: 1.4;
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
