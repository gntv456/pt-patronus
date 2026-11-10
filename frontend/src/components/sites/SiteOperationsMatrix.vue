<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useWs } from '@/composables/useWs'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import type { Site, SiteConnectivityStatus } from '@/api/site'
import type { SiteStatItem } from '@/api/dashboard'
import { dashboardApi } from '@/api/dashboard'
import SiteMatrixTable from './SiteMatrixTable.vue'
import SiteMatrixHistoryDialog from './SiteMatrixHistoryDialog.vue'
import { useSiteMatrixData, type MatrixFilter, type MatrixLayout } from '@/composables/useSiteMatrixData'
import { useInactiveSiteVisit } from '@/composables/useInactiveSiteVisit'
import { alertBox } from '@/utils/alertBox'

const props = withDefaults(
  defineProps<{
    sites: Site[]
    connectivity: Record<number, SiteConnectivityStatus>
    connectivityLoading?: boolean
    cookieAlerts?: readonly string[]
    loadingSites?: boolean
    testingSiteId?: number | null
    checkiningSiteId?: number | null
    canDelete?: boolean
  }>(),
  {
    connectivityLoading: false,
    cookieAlerts: () => [],
    loadingSites: false,
    testingSiteId: null,
    checkiningSiteId: null,
    canDelete: false,
  },
)

const emit = defineEmits<{
  create: []
  edit: [site: Site]
  remove: [site: Site]
  batchRemove: [sites: Site[]]
  test: [site: Site]
  checkin: [site: Site]
  detail: [site: Site]
}>()

const route = useRoute()
const panelRef = ref<HTMLElement | null>(null)
const panelWidth = ref(0)
const matrixLayout = computed<MatrixLayout>(() => {
  if (panelWidth.value >= 1450) return 'wide'
  if (panelWidth.value >= 1050) return 'balanced'
  return 'compact'
})
let resizeObserver: ResizeObserver | null = null

const { t } = useI18n()
const {
  siteQuery,
  matrixFilter,
  sortKey,
  sortDir,
  currentPage,
  pageSize,
  historyVisible,
  historyLoading,
  historySiteName,
  history,
  selectedSites,
  rows,
  siteBaseUrl,
  siteConfig,
  allRows,
  inactiveSiteCount,
  siteHasIssue,
  onSortChange,
  matrixRows,
  pagedRows,
  pageRange,
  reload,
  syncAll,
  syncOne,
  viewHistory,
  matrixRowClassName,
  handleSelectionChange,
  syncing,
  loadError,
  refreshing,
} = useSiteMatrixData(props)

// W4：全站同步进度改吃 WS task-progress 实时帧（后端逐站推送）；未连接 WS 时回退
// 到本地静态总量提示。顶栏 TaskIndicator 与此处同源。
const { taskProgress } = useWs()
const liveSyncText = computed(() => {
  const p = taskProgress.value['site-sync']
  if (!p) return ''
  return `（${p.done}/${p.total}）`
})

const { visitInactiveSites } = useInactiveSiteVisit(
  computed(() =>
    allRows.value
      .map((row) => ({ row, site: siteConfig(row.site_id) }))
      .filter(({ row, site }) => {
        if (!site?.is_active || !/^https?:\/\//i.test(site.base_url)) return false
        const lastAccess = new Date(row.last_access_at).getTime()
        return Number.isFinite(lastAccess) && lastAccess > 0 && lastAccess < Date.now() - 30 * 24 * 60 * 60 * 1000
      }),
  ),
  // 每访问完一个站点就重新抓取其统计（「最近访问」由站点侧记录，需同步才能取回新值），
  // 否则矩阵里该站的最后访问时间要等用户手动刷新才会更新。
  ({ row, site }) => {
    if (!site) return
    void refreshSiteStat(site.id, row.site_name)
  },
)

function showConnectivityReason(row: SiteStatItem) {
  const status = props.connectivity[row.site_id]
  if (!status || status.status === 'connected') return
  const title =
    status.status === 'disabled'
      ? t('som.disabledTitle', { name: row.site_name })
      : t('som.offlineTitle', { name: row.site_name })
  const site = siteConfig(row.site_id)
  const detail = status.error || t('som.noErrDetail')
  const url = site?.base_url ? t('som.siteUrl', { url: site.base_url }) : ''
  alertBox(detail + url, title, {
    type: status.status === 'disabled' ? 'warning' : 'error',
    confirmButtonText: t('som.gotIt'),
  })
}

function openDetail(row: SiteStatItem) {
  const site = siteConfig(row.site_id)
  if (!site) {
    ElMessage.warning(t('som.noSiteCfg'))
    return
  }
  emit('detail', site)
}

function handleRowCommand(command: string, row: SiteStatItem) {
  if (command === 'refresh') {
    void syncOne(row)
    return
  }
  if (command === 'history') {
    void viewHistory(row)
    return
  }
  const site = siteConfig(row.site_id)
  if (!site) {
    ElMessage.warning(t('som.noSiteCfg'))
    return
  }
  if (command === 'detail') emit('detail', site)
  else if (command === 'checkin') emit('checkin', site)
  else if (command === 'test') emit('test', site)
  else if (command === 'edit') emit('edit', site)
  else if (command === 'remove') emit('remove', site)
}

function routeFilter(value: unknown): MatrixFilter {
  return ['issues', 'connected', 'disabled'].includes(String(value)) ? (String(value) as MatrixFilter) : 'all'
}

// 静默刷新单个站点统计：只更新 rows，不弹成功提示（访问流程中逐站调用，提示会刷屏）。
async function refreshSiteStat(siteId: number, siteName: string) {
  refreshing.value.add(siteId)
  try {
    rows.value = (await dashboardApi.syncOne(siteId)).sites ?? []
  } catch {
    // 访问后的统计回捞失败不打断流程，用户仍可手动「刷新统计」。
  } finally {
    refreshing.value.delete(siteId)
  }
}

watch(
  () => route.query.matrix,
  (value) => {
    matrixFilter.value = routeFilter(value)
  },
  { immediate: true },
)
watch([siteQuery, matrixFilter, sortKey, sortDir, pageSize], () => {
  currentPage.value = 1
})
watch(
  () => matrixRows.value.length,
  (length) => {
    currentPage.value = Math.min(currentPage.value, Math.max(1, Math.ceil(length / pageSize.value)))
  },
)

onMounted(() => {
  void reload()
  const panel = panelRef.value
  if (!panel) return
  panelWidth.value = panel.clientWidth
  resizeObserver = new ResizeObserver(([entry]) => {
    panelWidth.value = Math.round(entry.contentRect.width)
  })
  resizeObserver.observe(panel)
})
onBeforeUnmount(() => {
  resizeObserver?.disconnect()
})

defineExpose({ reload, inactiveSiteCount, visitInactiveSites })
</script>

<template>
  <section ref="panelRef" class="operations-matrix glass">
    <div class="matrix-head">
      <div>
        <h3>{{ t('som.title') }}</h3>
        <p>{{ t('som.rangeInfo', { shown: pageRange, matched: matrixRows.length, total: allRows.length }) }}</p>
      </div>
      <div class="matrix-tools">
        <el-input v-model="siteQuery" clearable :placeholder="t('som.searchPh')" class="matrix-search" />
        <div class="matrix-filter" :aria-label="t('som.filterAria')">
          <button type="button" :class="{ active: matrixFilter === 'all' }" @click="matrixFilter = 'all'">
            {{ t('sh.all') }}
          </button>
          <button type="button" :class="{ active: matrixFilter === 'issues' }" @click="matrixFilter = 'issues'">
            {{ t('som.issues') }}
          </button>
          <button type="button" :class="{ active: matrixFilter === 'connected' }" @click="matrixFilter = 'connected'">
            {{ t('som.connected') }}
          </button>
          <button type="button" :class="{ active: matrixFilter === 'disabled' }" @click="matrixFilter = 'disabled'">
            {{ t('som.disabled') }}
          </button>
        </div>
        <el-button size="small" :loading="syncing" @click="syncAll">
          {{ t('som.syncAll') }}<span v-if="liveSyncText" class="sync-progress">{{ liveSyncText }}</span>
        </el-button>
        <el-button
          v-if="canDelete"
          size="small"
          type="danger"
          plain
          :disabled="!selectedSites.length"
          @click="emit('batchRemove', selectedSites)"
        >
          {{ t('som.batchDelete') }}{{ selectedSites.length ? `(${selectedSites.length})` : '' }}
        </el-button>
      </div>
    </div>

    <div v-if="loadError" class="matrix-error">
      <span>{{ loadError }}</span>
      <button type="button" @click="reload">{{ t('common.retry') }}</button>
    </div>

    <SiteMatrixTable
      :data="pagedRows"
      :layout="matrixLayout"
      :connectivity="connectivity"
      :connectivity-loading="connectivityLoading"
      :cookie-alerts="cookieAlerts"
      :can-delete="canDelete"
      :refreshing="refreshing"
      :testing-site-id="testingSiteId"
      :checkining-site-id="checkiningSiteId"
      :site-base-url="siteBaseUrl"
      :site-config="siteConfig"
      :row-class-name="matrixRowClassName"
      @sort-change="onSortChange"
      @selection-change="handleSelectionChange"
      @detail="openDetail"
      @history="viewHistory"
      @command="handleRowCommand"
      @reason="showConnectivityReason"
      @create="emit('create')"
    />

    <div v-if="matrixRows.length > 8" class="matrix-pagination">
      <el-pagination
        v-model:current-page="currentPage"
        v-model:page-size="pageSize"
        :page-sizes="[8, 15, 30]"
        :total="matrixRows.length"
        layout="total, sizes, prev, pager, next"
        background
      />
    </div>

    <SiteMatrixHistoryDialog
      :visible="historyVisible"
      :site-name="historySiteName"
      :loading="historyLoading"
      :history="history"
      @update:visible="historyVisible = $event"
    />
  </section>
</template>

<style scoped src="./siteOperationsMatrix.css"></style>
