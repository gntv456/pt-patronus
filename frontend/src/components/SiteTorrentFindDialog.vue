<script setup lang="ts">
/**
 * MoviePilot-style "find on sites" dialog.
 * Card grid + "更多来源" grouping, progressive results while parent streams.
 */
import { useI18n } from 'vue-i18n'
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

const { t } = useI18n()
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
  siteApi
    .list()
    .then((list) => {
      sites.value = list
    })
    .catch(() => {})
})

const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => {
    emit('update:modelValue', v)
    if (!v) emit('close')
  },
})

const resultsComputed = computed(() => props.results)
const {
  flatRows,
  filter,
  sort,
  options,
  filtered,
  hasActive,
  activeTags,
  removeFilter,
  clearAll,
  hitSites,
  hitTotal,
  failedSites,
  showFailed,
  cards,
  moreVisible,
  moreItems,
  openMore,
  openDetail,
} = useTorrentFindCards(resultsComputed, siteMap)

const {
  visible: pushVisible,
  row: pushRow,
  downloaders,
  selectedDl,
  categories,
  selectedCategory,
  savePath,
  showAdvanced,
  paused,
  loading: pushLoading,
  pushedDlName,
  subtitle: pushSubtitle,
  start: startPush,
  confirm: confirmPush,
  gotoDownloaderTasks,
  close: closePush,
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
    :title="t('stfd.title', { name: title })"
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
            {{ t('stfd.searching') }}
            <template v-if="results.length">
              {{ t('stfd.returned', { n: results.length }) }}
              <template v-if="hitTotal"> · {{ t('stfd.hits', { n: hitTotal }) }}</template>
            </template>
          </span>
        </template>
        <template v-else>
          {{ t('stfd.hitSummary', { sites: hitSites, n: hitTotal }) }}
          <template v-if="hasActive"> · {{ t('stfd.filteredN', { n: filtered.length }) }}</template>
          <span v-if="results.length" class="text-muted">{{
            t('stfd.queried', { q: results.length, shown: cards.length })
          }}</span>
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
          <span>⚠ {{ t('trt.failedSites', { n: failedSites.length }) }}</span>
          <span class="text-muted">{{ showFailed ? t('trt.collapse') : t('trt.expand') }}</span>
        </button>
        <ul v-if="showFailed" class="sf-fail-list">
          <li v-for="f in failedSites" :key="f.site_id">
            <strong>{{ f.site_name }}</strong>
            <span class="text-muted">{{ f.error }}</span>
          </li>
        </ul>
      </div>

      <div v-if="!loading && !flatRows.length" class="sf-empty text-muted">
        {{ t('stfd.noResults') }}
      </div>
      <div v-else-if="hasActive && !cards.length" class="sf-empty text-muted">
        {{ t('stfd.noFilteredResults') }}
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
      <el-button v-if="loading" @click="close">{{ t('common.cancel') }}</el-button>
      <el-button v-else type="primary" @click="close">{{ t('common.close') }}</el-button>
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
  gap: var(--sp-2);
  margin-bottom: 12px;
  font-size: var(--fs-body);
  color: var(--text-strong);
}
.sf-spinner {
  width: 14px;
  height: 14px;
  border: 2px solid var(--pt-primary-bg);
  border-top-color: var(--pt-primary, #6c5ce7);
  border-radius: 50%;
  animation: sf-spin 0.7s linear infinite;
  flex: none;
}
@keyframes sf-spin {
  to {
    transform: rotate(360deg);
  }
}
.sf-fail {
  margin-bottom: 12px;
  padding: 8px 12px;
  border-radius: var(--radius-sm);
  background: var(--pt-danger-bg);
  border: 1px solid var(--pt-danger-bg);
}
.sf-fail-toggle {
  display: flex;
  width: 100%;
  justify-content: space-between;
  gap: var(--sp-2);
  background: transparent;
  border: 0;
  cursor: pointer;
  font-size: var(--fs-body);
  color: var(--text-strong);
  padding: 0;
}
.sf-fail-list {
  margin: 8px 0 0;
  padding-left: 18px;
  font-size: var(--fs-xs);
}
.sf-fail-list li {
  margin: 2px 0;
  display: flex;
  gap: var(--sp-2);
  flex-wrap: wrap;
}
.sf-empty {
  padding: 40px 8px;
  text-align: center;
}
.sf-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(290px, 1fr));
  gap: var(--sp-3);
}
</style>
