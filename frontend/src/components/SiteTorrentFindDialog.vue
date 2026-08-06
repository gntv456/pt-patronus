<script setup lang="ts">
/**
 * MoviePilot-style "find on sites" dialog.
 * Card grid + "更多来源" grouping, progressive results while parent streams.
 */
import { computed, onMounted, ref, watch } from 'vue'
import type { SiteSearchResult, SiteTorrent } from '@/api/search'
import { siteApi, type Site } from '@/api/site'
import SearchFilterBar from '@/components/SearchFilterBar.vue'
import TorrentCard from '@/components/search/TorrentCard.vue'
import TorrentFindMoreDialog from '@/components/search/TorrentFindMoreDialog.vue'
import TorrentPushDialog from '@/components/search/TorrentPushDialog.vue'
import { useTorrentFindCards } from '@/composables/useTorrentFindCards'
import { useTorrentFindPush } from '@/composables/useTorrentFindPush'

export type FindRow = SiteTorrent & {
  site_id: number
  site_name: string
  base_url?: string
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

const resultsComputed = computed(() => props.results)
const { flatRows, filter, sort, options, filtered, hasActive, activeTags, removeFilter, clearAll,
  hitSites, hitTotal, failedSites, showFailed, cards, moreVisible, moreItems,
  openMore, openDetail,
} = useTorrentFindCards(resultsComputed, siteMap)

const { visible: pushVisible, row: pushRow, downloaders, selectedDl, categories, selectedCategory,
  savePath, showAdvanced, paused, loading: pushLoading, pushedDlName, subtitle: pushSubtitle,
  start: startPush, confirm: confirmPush, gotoDownloaderTasks, close: closePush,
} = useTorrentFindPush(() => props.title)

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
      clearAll()
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
          <template v-if="hasActive"> · 筛选后 <b>{{ filtered.length }}</b> 条</template>
          <span v-if="results.length" class="text-muted">（共查询 {{ results.length }} 站 · 展示 {{ cards.length }} 组）</span>
        </template>
      </div>

      <SearchFilterBar
        v-if="flatRows.length"
        :filter="filter"
        :sort="sort"
        :options="options"
        :active-tags="activeTags"
        :has-active="hasActive"
        :result-count="filtered.length"
        :total-count="hitTotal"
        :sortable="false"
        :remove-filter="removeFilter"
        :clear-all="clearAll"
      />

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
      <div v-else-if="hasActive && !cards.length" class="sf-empty text-muted">
        当前筛选无结果，试试清除筛选
      </div>

      <div v-else class="sf-grid">
        <TorrentCard
          v-for="g in cards"
          :key="g.key"
          :group="g"
          :title="title"
          :poster-url="posterUrl"
          @push="startPush"
          @more="openMore"
          @detail="openDetail"
        />
      </div>
    </div>

    <template #footer>
      <el-button v-if="loading" @click="close">取消</el-button>
      <el-button v-else type="primary" @click="close">关闭</el-button>
    </template>
  </el-dialog>

  <TorrentFindMoreDialog v-model:visible="moreVisible" :items="moreItems" @push="startPush" />

  <TorrentPushDialog
    v-model:visible="pushVisible"
    :poster-url="posterUrl"
    :subtitle="pushSubtitle"
    :row="pushRow"
    :downloaders="downloaders"
    :categories="categories"
    :loading="pushLoading"
    :pushed-dl-name="pushedDlName"
    v-model:selected-dl="selectedDl"
    v-model:selected-category="selectedCategory"
    v-model:save-path="savePath"
    v-model:show-advanced="showAdvanced"
    v-model:paused="paused"
    @confirm="confirmPush"
    @goto-downloader-tasks="gotoDownloaderTasks"
    @close="closePush"
  />
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
  grid-template-columns: repeat(auto-fill, minmax(290px, 1fr));
  gap: 12px;
}
</style>
