<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { siteApi, type Site, type SiteConnectivityStatus } from '@/api/site'
import { dashboardApi, type SiteStatItem } from '@/api/dashboard'
import { useSiteDetailTabs } from '@/composables/useSiteDetailTabs'

const { t } = useI18n()
const props = defineProps<{ id: string }>()
const route = useRoute()
const router = useRouter()

const siteId = computed(() => {
  const n = Number(props.id || route.params.id)
  return Number.isFinite(n) && n > 0 ? n : 0
})
const tab = ref(String(route.query.tab || 'overview'))
const validTabs = new Set(['overview', 'assessment', 'brushflow', 'reseed', 'freeleech', 'savept'])
if (!validTabs.has(tab.value)) tab.value = 'overview'
const site = ref<Site | null>(null)
const conn = ref<SiteConnectivityStatus | null>(null)
const stat = ref<SiteStatItem | null>(null)
// 读失败置位：区分「站点没数据」与「接口没读到」（失败画成 0 B/'—'/无连接 pill 都是假绿）。
const connFailed = ref(false)
const statFailed = ref(false)
const loading = ref(true)
const loadError = ref('')

// 各 Tab 的懒加载数据与状态抽出为 composable（useSiteDetailTabs）。
const {
  assess,
  assessLoading,
  assessFailed,
  fl,
  flLoading,
  flFailed,
  contract,
  contractCheck,
  contractLoading,
  contractFailed,
  bfTasks,
  bfLoading,
  bfFailed,
  saveptCard,
  saveptLoading,
  saveptFailed,
  reseedHint,
  reseedLoading,
  clearTabState,
  loadTab,
} = useSiteDetailTabs(
  () => siteId.value,
  () => site.value?.name,
)

const statusColor: Record<string, string> = {
  SAFE: 'var(--color-success)',
  PROGRESS: 'var(--color-warning)',
  DANGER: 'var(--color-danger)',
  EXEMPT: 'var(--color-text-muted)',
  UNKNOWN: 'var(--color-text-muted)',
}

async function loadSite() {
  loading.value = true
  loadError.value = ''
  site.value = null
  conn.value = null
  stat.value = null
  connFailed.value = false
  statFailed.value = false
  if (!siteId.value) {
    loadError.value = t('sitedetail.badId')
    loading.value = false
    return
  }
  try {
    const list = await siteApi.list()
    site.value = list.find((s) => s.id === siteId.value) || null
    if (!site.value) {
      loadError.value = t('sitedetail.notFound')
      return
    }
    let statuses: SiteConnectivityStatus[] = []
    let ov: Awaited<ReturnType<typeof dashboardApi.overview>> | null = null
    let cFail = false
    let sFail = false
    await Promise.all([
      siteApi
        .statuses()
        .then((r) => (statuses = r))
        .catch(() => (cFail = true)),
      dashboardApi
        .overview()
        .then((r) => (ov = r))
        .catch(() => (sFail = true)),
    ])
    connFailed.value = cFail
    statFailed.value = sFail
    conn.value = statuses.find((s) => s.site_id === siteId.value) || null
    stat.value =
      (
        ov as Awaited<ReturnType<typeof dashboardApi.overview>> | null
      )?.sites?.find((s) => s.site_id === siteId.value) || null
  } catch (e: unknown) {
    loadError.value = (e as Error).message || t('common.loadFailed')
  } finally {
    loading.value = false
  }
}

function onTab(name: string | number) {
  const n = String(name)
  tab.value = n
  router.replace({
    name: 'site-detail',
    params: { id: String(siteId.value) },
    query: { tab: n },
  })
  void loadTab(n)
}

function backToList() {
  // 返回列表时携带站点与当前 Tab，便于列表侧恢复/高亮（深链可分享、后退可恢复）
  router.push({ name: 'sites', query: { site: String(siteId.value), tab: tab.value } })
}

function goGlobal(name: string) {
  const q: Record<string, string> = { site_id: String(siteId.value) }
  if (site.value?.name) q.site_name = site.value.name
  router.push({ name, query: q })
}

function fmtBytes(n: number) {
  if (!n) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = n
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(i ? 2 : 0)} ${u[i]}`
}

onMounted(async () => {
  await loadSite()
  if (site.value) void loadTab(tab.value)
})

watch(siteId, async () => {
  clearTabState()
  await loadSite()
  if (site.value) void loadTab(tab.value)
})

// 外部改 ?tab= 时同步（浏览器前进/后退或深链）
watch(
  () => String(route.query.tab || 'overview'),
  (v) => {
    const n = validTabs.has(v) ? v : 'overview'
    if (n !== tab.value) {
      tab.value = n
      if (site.value) void loadTab(n)
    }
    // 非法 tab 写回 overview，避免地址栏与 UI 不一致
    if (!validTabs.has(v) && route.query.tab != null) {
      router.replace({
        name: 'site-detail',
        params: { id: String(siteId.value || route.params.id) },
        query: { tab: 'overview' },
      })
    }
  },
)
</script>

<template>
  <div class="page" v-loading="loading" :aria-busy="loading">
    <div class="head">
      <button type="button" class="back" @click="backToList()">
        <SptIcon name="arrow-left" :size="14" aria-hidden="true" /> {{ t('sitedetail.backSites') }}
      </button>
      <div v-if="site" class="title-wrap">
        <SiteFavicon :name="site.name" :base-url="site.base_url" :size="36" />
        <div>
          <h2 class="title">{{ site.name }}</h2>
          <div class="sub">
            <a :href="site.base_url" target="_blank" rel="noopener">{{ site.base_url }}</a>
            <span class="dot">·</span>
            <span :class="['pill', site.is_active ? 'on' : 'off']">{{
              site.is_active ? t('common.enabled') : t('sitedetail.disabled')
            }}</span>
            <template v-if="conn">
              <span class="dot">·</span>
              <span :class="['pill', conn.status === 'connected' ? 'on' : 'off']">
                {{
                  conn.status === 'connected'
                    ? t('sitedetail.online', { level: conn.level || '' })
                    : conn.error || t('sitedetail.offline')
                }}
              </span>
            </template>
            <template v-else-if="connFailed">
              <span class="dot">·</span>
              <span class="pill off">{{ t('common.loadFailed') }}</span>
            </template>
          </div>
        </div>
      </div>
      <div v-if="site" class="head-actions">
        <el-button size="small" @click="goGlobal('assessment')">{{ t('sitedetail.globalAssess') }}</el-button>
        <el-button size="small" type="primary" plain @click="backToList()">{{
          t('sitedetail.backList')
        }}</el-button>
      </div>
    </div>

    <el-alert v-if="loadError" type="error" :title="loadError" show-icon :closable="false" />

    <template v-if="site && !loadError">
      <el-tabs v-model="tab" class="solid-card tabs" @tab-change="onTab">
        <el-tab-pane :label="t('sitedetail.tabOverview')" name="overview">
          <el-alert
            v-if="statFailed"
            type="warning"
            :title="t('common.loadFailed') + '：' + t('sitedetail.tabOverview')"
            show-icon
            :closable="false"
          />
          <div class="grid">
            <div class="card solid-card metric">
              <span class="k">{{ t('sitedetail.level') }}</span
              ><span class="v">{{ stat?.level || conn?.level || '—' }}</span>
            </div>
            <div class="card solid-card metric">
              <span class="k">{{ t('sitedetail.uploaded') }}</span
              ><span class="v">{{ statFailed ? '—' : fmtBytes(stat?.uploaded || 0) }}</span>
            </div>
            <div class="card solid-card metric">
              <span class="k">{{ t('sitedetail.downloaded') }}</span
              ><span class="v">{{ statFailed ? '—' : fmtBytes(stat?.downloaded || 0) }}</span>
            </div>
            <div class="card solid-card metric">
              <span class="k">{{ t('sitedetail.ratio') }}</span
              ><span class="v">{{ stat ? (stat.ratio || 0).toFixed(2) : '—' }}</span>
            </div>
            <div class="card solid-card metric">
              <span class="k">{{ t('sitedetail.seeding') }}</span
              ><span class="v">{{ stat?.seeding ?? '—' }}</span>
            </div>
            <div class="card solid-card metric">
              <span class="k">{{ t('sitedetail.bonus') }}</span
              ><span class="v">{{ stat?.bonus != null ? Math.round(stat.bonus) : '—' }}</span>
            </div>
          </div>
          <p class="hint text-muted">{{ t('sitedetail.drillHint') }}</p>
        </el-tab-pane>

        <el-tab-pane :label="t('sitedetail.tabAssessment')" name="assessment" lazy>
          <div v-loading="assessLoading || contractLoading" class="tab-body">
            <div v-if="assess" class="card solid-card">
              <div class="row-between">
                <h3>{{ t('sitedetail.assess') }}</h3>
                <el-tag :color="statusColor[assess.status]" effect="dark" style="border: 0">{{ assess.status }}</el-tag>
              </div>
              <p class="text-muted">
                {{
                  t('sitedetail.assessProgress', {
                    pct: Number(assess.progress_percent || 0).toFixed(0),
                    days: assess.remaining_days,
                  })
                }}
                <template v-if="assess.assessment_type"> · {{ assess.assessment_type }}</template>
              </p>
              <ul v-if="assess.metrics?.length" class="metrics">
                <li v-for="(m, i) in assess.metrics" :key="i">
                  <b>{{ m.label }}</b
                  >：{{ m.current }}/{{ m.required }} {{ m.unit }}
                  <SptIcon v-if="m.is_completed" name="check" :size="14" class="ok" aria-hidden="true" />
                </li>
              </ul>
              <p v-if="assess.error" class="err">{{ assess.error }}</p>
            </div>
            <div v-else class="card solid-card text-muted">
              {{ assessFailed ? t('common.loadFailed') : t('sitedetail.noAssess') }}
            </div>

            <div class="card solid-card">
              <div class="row-between">
                <h3>{{ t('sitedetail.contract') }}</h3>
                <el-button size="small" text type="primary" @click="goGlobal('contract')">{{
                  t('sitedetail.manage')
                }}</el-button>
              </div>
              <template v-if="contract">
                <p>
                  {{
                    t('sitedetail.contractReq', {
                      ratio: contract.min_ratio || t('sitedetail.unlimited'),
                      seeds: contract.min_seeding || t('sitedetail.unlimited'),
                      gb: contract.min_upload_gb || 0,
                    })
                  }}
                </p>
                <p v-if="contractCheck">
                  {{ t('sitedetail.lastCheck') }}：<b>{{ contractCheck.status }}</b> {{ contractCheck.detail || '' }}
                </p>
              </template>
              <p v-else class="text-muted">
                {{ contractFailed ? t('common.loadFailed') : t('sitedetail.noContract') }}
              </p>
            </div>
          </div>
        </el-tab-pane>

        <el-tab-pane :label="t('sitedetail.tabBrushflow')" name="brushflow" lazy>
          <div v-loading="bfLoading" class="tab-body">
            <div class="row-between">
              <h3>{{ t('sitedetail.bfTasks') }}</h3>
              <el-button size="small" type="primary" @click="goGlobal('brushflow')">{{
                t('sitedetail.globalBf')
              }}</el-button>
            </div>
            <div v-if="!bfTasks.length" class="card solid-card text-muted">
              {{ bfFailed ? t('common.loadFailed') : t('sitedetail.noBf') }}
            </div>
            <div v-for="task in bfTasks" :key="task.id" class="card solid-card">
              <div class="row-between">
                <b>{{ task.name }}</b>
                <el-tag size="small" :type="task.enabled ? 'success' : 'info'">{{
                  task.enabled ? t('common.enabled') : t('sitedetail.disabled')
                }}</el-tag>
              </div>
              <p class="text-muted small">
                {{
                  t('sitedetail.bfMeta', {
                    added: task.total_added,
                    deleted: task.total_deleted,
                    seeds: task.max_seeds,
                  })
                }}
              </p>
            </div>
          </div>
        </el-tab-pane>

        <el-tab-pane :label="t('sitedetail.tabReseed')" name="reseed" lazy>
          <div v-loading="reseedLoading" class="card solid-card tab-body">
            <p>{{ reseedHint || t('common.loading') }}</p>
            <el-button type="primary" @click="goGlobal('reseed')">{{ t('sitedetail.openReseed') }}</el-button>
          </div>
        </el-tab-pane>

        <el-tab-pane :label="t('sitedetail.tabFreeleech')" name="freeleech" lazy>
          <div v-loading="flLoading" class="tab-body">
            <div class="row-between">
              <h3>{{ t('sitedetail.flStatus') }}</h3>
              <el-button size="small" @click="goGlobal('freeleech')">{{ t('sitedetail.globalFl') }}</el-button>
            </div>
            <div v-if="!fl?.notices?.length" class="card solid-card text-muted">
              {{ flFailed ? t('common.loadFailed') : (fl?.error || t('sitedetail.noFl')) }}
            </div>
            <div v-for="(n, i) in fl?.notices || []" :key="i" class="card solid-card">
              <div class="row-between">
                <b>{{ n.title || n.status }}</b>
                <el-tag size="small">{{ n.status }}</el-tag>
              </div>
              <p class="text-muted">{{ n.content }}</p>
              <a v-if="n.link" :href="n.link" target="_blank" rel="noopener">{{ t('sitedetail.viewSource') }}</a>
            </div>
          </div>
        </el-tab-pane>

        <el-tab-pane :label="t('sitedetail.tabSavept')" name="savept" lazy>
          <div v-loading="saveptLoading" class="tab-body">
            <div class="row-between">
              <h3>{{ t('sitedetail.saveptRoom') }}</h3>
              <el-button size="small" @click="goGlobal('savept')">{{ t('sitedetail.globalSavept') }}</el-button>
            </div>
            <div v-if="saveptCard" class="card solid-card">
              <p>
                <b>{{ saveptCard.name }}</b> · {{ saveptCard.status_text || saveptCard.status }}
              </p>
              <p class="text-muted">{{ saveptCard.duration_text || saveptCard.year }}</p>
            </div>
            <div v-else class="card solid-card text-muted">
              {{ saveptFailed ? t('common.loadFailed') : t('sitedetail.noSavept') }}
            </div>
          </div>
        </el-tab-pane>
      </el-tabs>
    </template>
  </div>
</template>

<style scoped src="./SiteDetail.css"></style>
