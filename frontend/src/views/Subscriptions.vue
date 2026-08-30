<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { subscribeApi, type Subscription, type GlobalDefaults } from '@/api/subscribe'
import { siteApi } from '@/api/site'
import { useSubscriptionToggles } from '@/composables/useSubscriptionToggles'
import { useSubscriptionList } from '@/composables/useSubscriptionList'
import MediaPoster from '@/components/MediaPoster.vue'
import EmptyState from '@/components/EmptyState.vue'
import SiteFilterDialog from '@/components/SiteFilterDialog.vue'
import RssView from '@/views/Rss.vue'

// 订阅列表：展示订阅状态，支持手动检查 / 暂停-恢复 / 改分辨率 / 删除。
const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const mainTab = ref(String(route.query.tab || 'media'))
// 站点多选（参与订阅搜索）选项：活跃站点 {id,name}，复用 SiteFilterDialog。
const sites = ref<{ id: number; name: string }[]>([])

// 全局订阅默认设置：未单独设置某项的订阅回退到此（运行时回退）。
const globalDefaults = ref<GlobalDefaults>({
  resolution: '',
  promo_types: [],
  site_ids: [],
  enable_rating_guard: false,
  best_version: false,
  official_only: false,
})
const globalOpen = ref(false)
const globalForm = ref<GlobalDefaults>({
  resolution: '',
  promo_types: [],
  site_ids: [],
  enable_rating_guard: false,
  best_version: false,
  official_only: false,
})
const savingGlobal = ref(false)

// 列表加载与逐条操作抽出为 composable（useSubscriptionList）。
const { list, loading, checking, load, checkNow, toggleStatus, setResolution, setPromoTypes, setSiteIds, remove } =
  useSubscriptionList()

async function loadGlobalDefaults() {
  try {
    const g = await subscribeApi.getGlobalDefaults()
    // 后端零值切片历史会回 null（go-nil-slice-json-null），此处归一 [] 兜底，避免模板读 .length 崩溃。
    globalDefaults.value = { ...g, promo_types: g.promo_types ?? [], site_ids: g.site_ids ?? [] }
  } catch {
    // 加载失败不阻塞订阅展示，「跟随全局」提示退化为不显示。
  }
}

// 归一全局设置切片字段（PUT 响应/表单兜底共用；null → [] 防 .length 崩溃）。
function normalizeGlobal(g: GlobalDefaults): GlobalDefaults {
  return { ...g, promo_types: g.promo_types ?? [], site_ids: g.site_ids ?? [] }
}

function openGlobal() {
  const g = globalDefaults.value
  globalForm.value = { ...g, promo_types: [...(g.promo_types ?? [])], site_ids: [...(g.site_ids ?? [])] }
  globalOpen.value = true
}

async function saveGlobal() {
  savingGlobal.value = true
  try {
    globalDefaults.value = normalizeGlobal(await subscribeApi.updateGlobalDefaults(globalForm.value))
    globalOpen.value = false
    ElMessage.success(t('subscriptions.globalSaved'))
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('common.saveFailed'))
  } finally {
    savingGlobal.value = false
  }
}

// 三态开关簇（评分保护/洗版/仅官种）抽出为 composable：订阅自定义优先，null 回退全局默认。
const {
  effRatingGuard,
  effBestVersion,
  effOfficialOnly,
  setRatingGuard,
  resetRatingGuard,
  setBestVersion,
  resetBestVersion,
  setOfficialOnly,
  resetOfficialOnly,
} = useSubscriptionToggles(globalDefaults)
function promoLabel(v: string) {
  return PROMO_OPTIONS.find((o) => o.value === v)?.label || v
}

async function loadSites() {
  try {
    const all = await siteApi.list()
    sites.value = all.filter((s) => s.is_active).map((s) => ({ id: s.id, name: s.name }))
  } catch {
    // 站点列表加载失败不阻塞订阅展示，选择器显示空即可。
  }
}

// 促销类型选项（与后端 site.PromoType 对齐；空=不限，任何促销都接受）。
const PROMO_OPTIONS = [
  { value: 'free', label: t('subscriptions.promo.free') },
  { value: '2xfree', label: t('subscriptions.promo.2xfree') },
  { value: '2x', label: t('subscriptions.promo.2x') },
  { value: 'discount', label: t('subscriptions.promo.discount') },
  { value: '2xdiscount', label: t('subscriptions.promo.2xdiscount') },
]

function statusText(s: string) {
  return (
    (
      {
        subscribed: t('subscriptions.status.subscribed'),
        completed: t('subscriptions.status.completed'),
        paused: t('subscriptions.status.paused'),
      } as Record<string, string>
    )[s] || s
  )
}
function fmtTime(t?: string) {
  return t ? new Date(t).toLocaleString('zh-CN', { hour12: false }).replace(/\//g, '-') : '—'
}

// ---- 批量操作（对标 MSaber：卡片勾选 → 批量编辑 / 重新搜索 / 取消订阅） ----
const batchMode = ref(false)
const selectedIds = ref<number[]>([])
const batchRunning = ref(false)
// 批量编辑表单：开关开启的字段才应用到所选订阅，未开启保持原值。
const batchForm = ref({
  resolutionOn: false,
  resolution: '',
  statusOn: false,
  status: 'subscribed',
  ratingGuardOn: false,
  ratingGuard: false,
  bestVersionOn: false,
  bestVersion: false,
})
const allSelected = () => list.value.length > 0 && selectedIds.value.length === list.value.length
function toggleSelectAll() {
  selectedIds.value = allSelected() ? [] : list.value.map((s) => s.id)
}
function toggleSelect(id: number) {
  const i = selectedIds.value.indexOf(id)
  if (i >= 0) selectedIds.value.splice(i, 1)
  else selectedIds.value.push(id)
}
function enterBatch() {
  batchMode.value = true
  selectedIds.value = []
}
function exitBatch() {
  batchMode.value = false
  selectedIds.value = []
}
async function runBatch(op: 'update' | 'check' | 'delete') {
  if (!selectedIds.value.length) {
    ElMessage.warning(t('subscriptions.batch.selectFirst'))
    return
  }
  if (op === 'delete' && !confirm(t('subscriptions.batch.confirmDelete', { n: selectedIds.value.length }))) return
  let patch: Record<string, unknown> | undefined
  if (op === 'update') {
    patch = {}
    if (batchForm.value.resolutionOn) patch.resolution = batchForm.value.resolution
    if (batchForm.value.statusOn) patch.status = batchForm.value.status
    if (batchForm.value.ratingGuardOn) patch.enable_rating_guard = batchForm.value.ratingGuard
    if (batchForm.value.bestVersionOn) patch.best_version = batchForm.value.bestVersion
    if (!Object.keys(patch).length) {
      ElMessage.warning(t('subscriptions.batch.noFieldSelected'))
      return
    }
  }
  batchRunning.value = true
  try {
    const res = await subscribeApi.batch(selectedIds.value, op, patch as never)
    ElMessage.success(
      t('subscriptions.batch.done', {
        ok: res.succeeded,
        total: res.total,
        failPart: res.failed
          ? t('subscriptions.batch.failPart', { n: res.failed, err: res.failures[0]?.error || '' })
          : '',
      }),
    )
    if (op === 'delete') exitBatch()
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('subscriptions.batch.failed'))
  } finally {
    batchRunning.value = false
  }
}

watch(
  () => route.query.tab,
  (v) => {
    mainTab.value = String(v || 'media')
  },
)
function onMainTab(name: string | number) {
  const n = String(name)
  mainTab.value = n
  router.replace({ name: 'subscriptions', query: n === 'media' ? {} : { tab: n } })
}

onMounted(() => {
  load()
  loadSites()
  loadGlobalDefaults()
})
</script>

<template>
  <div class="page">
    <div class="head">
      <div class="head-left">
        <h2 class="title">{{ t('subscriptions.title') }}</h2>
        <span class="text-muted hint"
          >{{ t('subscriptions.subtitle')
          }}<router-link to="/search">{{ t('subscriptions.subtitleSearch') }}</router-link
          >{{ t('subscriptions.subtitleTail') }}</span
        >
      </div>
      <div style="display: flex; gap: 8px">
        <el-button v-if="!batchMode && list.length" class="global-btn" @click="enterBatch">{{
          t('subscriptions.batch.button')
        }}</el-button>
        <el-button class="global-btn" @click="openGlobal">{{ t('subscriptions.globalSettings') }}</el-button>
      </div>
    </div>

    <el-tabs v-model="mainTab" class="page-tabs glass" @tab-change="onMainTab">
      <el-tab-pane :label="t('subscriptions.tabMedia')" name="media">
        <el-dialog
          v-model="globalOpen"
          :title="t('subscriptions.globalTitle')"
          width="540px"
          append-to-body
          top="6vh"
          class="global-subscribe-dialog"
        >
          <div class="text-muted small" style="margin-bottom: 12px">
            {{ t('subscriptions.globalDesc') }}
          </div>
          <div class="gd-row">
            <span class="small gd-label">{{ t('subscriptions.resolution') }}</span>
            <el-input
              v-model="globalForm.resolution"
              size="small"
              style="width: 160px"
              :placeholder="t('subscriptions.unlimited')"
            />
            <span class="small text-muted">{{ t('subscriptions.globalResHint') }}</span>
          </div>
          <div class="gd-row">
            <span class="small gd-label">{{ t('subscriptions.ratingGuard') }}</span>
            <el-switch v-model="globalForm.enable_rating_guard" size="small" />
            <span class="small text-muted">{{ t('subscriptions.globalRatingHint') }}</span>
          </div>
          <div class="gd-row">
            <span class="small gd-label">{{ t('subscriptions.bestVersion') }}</span>
            <el-switch v-model="globalForm.best_version" size="small" />
            <span class="small text-muted">{{ t('subscriptions.globalBestHint') }}</span>
          </div>
          <div class="gd-row">
            <span class="small gd-label">{{ t('subscriptions.officialOnly') }}</span>
            <el-switch v-model="globalForm.official_only" size="small" />
            <span class="small text-muted">{{ t('subscriptions.globalOfficialHint') }}</span>
          </div>
          <div class="gd-row">
            <span class="small gd-label">{{ t('subscriptions.promoTypes') }}</span>
            <el-select
              v-model="globalForm.promo_types"
              multiple
              collapse-tags
              collapse-tags-tooltip
              size="small"
              style="width: 240px"
              :placeholder="t('subscriptions.unlimited')"
            >
              <el-option v-for="o in PROMO_OPTIONS" :key="o.value" :label="o.label" :value="o.value" />
            </el-select>
            <span class="small text-muted">{{ t('subscriptions.emptyUnlimited') }}</span>
          </div>
          <div class="gd-row">
            <span class="small gd-label">{{ t('subscriptions.sites') }}</span>
            <SiteFilterDialog
              :model-value="globalForm.site_ids"
              :sites="sites"
              :title="t('subscriptions.globalSitesTitle')"
              @update:model-value="(ids: number[]) => (globalForm.site_ids = ids)"
            >
              <span class="small">{{
                globalForm.site_ids.length
                  ? t('subscriptions.nSites', { n: globalForm.site_ids.length })
                  : t('subscriptions.allSites')
              }}</span>
            </SiteFilterDialog>
            <span class="small text-muted">{{ t('subscriptions.sitesEmptyHint') }}</span>
          </div>
          <template #footer>
            <el-button @click="globalOpen = false">{{ t('common.cancel') }}</el-button>
            <el-button type="primary" :loading="savingGlobal" @click="saveGlobal">{{ t('common.save') }}</el-button>
          </template>
        </el-dialog>

        <div v-loading="loading" class="card glass">
          <!-- 批量操作条：进入后卡片显示勾选框；编辑用字段开关控制只改所选字段 -->
          <div v-if="batchMode" class="batch-bar">
            <el-checkbox :model-value="allSelected()" @change="toggleSelectAll">{{
              t('subscriptions.batch.selectAll')
            }}</el-checkbox>
            <span class="small text-muted">{{ t('subscriptions.batch.selected', { n: selectedIds.length }) }}</span>
            <el-button size="small" @click="exitBatch">{{ t('subscriptions.batch.exit') }}</el-button>
            <el-button size="small" type="primary" :loading="batchRunning" @click="runBatch('update')">{{
              t('subscriptions.batch.apply')
            }}</el-button>
            <el-button size="small" :loading="batchRunning" @click="runBatch('check')">{{
              t('subscriptions.batch.recheck')
            }}</el-button>
            <el-button size="small" type="danger" plain :loading="batchRunning" @click="runBatch('delete')">{{
              t('subscriptions.batch.unsubscribe')
            }}</el-button>
            <div class="batch-edit gd-row" style="width: 100%; margin-top: 6px">
              <el-checkbox v-model="batchForm.resolutionOn">{{ t('subscriptions.resolution') }}</el-checkbox>
              <el-input
                v-model="batchForm.resolution"
                size="small"
                style="width: 110px"
                :placeholder="t('subscriptions.resPlaceholder')"
                :disabled="!batchForm.resolutionOn"
              />
              <el-checkbox v-model="batchForm.statusOn">{{ t('subscriptions.batch.status') }}</el-checkbox>
              <el-select v-model="batchForm.status" size="small" style="width: 100px" :disabled="!batchForm.statusOn">
                <el-option :label="t('subscriptions.status.subscribed')" value="subscribed" />
                <el-option :label="t('subscriptions.status.paused')" value="paused" />
                <el-option :label="t('subscriptions.status.completed')" value="completed" />
              </el-select>
              <el-checkbox v-model="batchForm.ratingGuardOn">{{ t('subscriptions.ratingGuard') }}</el-checkbox>
              <el-switch v-model="batchForm.ratingGuard" size="small" :disabled="!batchForm.ratingGuardOn" />
              <el-checkbox v-model="batchForm.bestVersionOn">{{ t('subscriptions.bestVersion') }}</el-checkbox>
              <el-switch v-model="batchForm.bestVersion" size="small" :disabled="!batchForm.bestVersionOn" />
              <span class="small text-muted">{{ t('subscriptions.batch.fieldHint') }}</span>
            </div>
          </div>
          <div v-if="list.length" class="subs">
            <div
              v-for="s in list"
              :key="s.id"
              class="sub glass-hover"
              :class="{ selected: batchMode && selectedIds.includes(s.id) }"
            >
              <div class="poster">
                <el-checkbox
                  v-if="batchMode"
                  :model-value="selectedIds.includes(s.id)"
                  class="batch-check"
                  @change="toggleSelect(s.id)"
                />
                <MediaPoster :url="s.poster_url" :kind="s.type" />
              </div>
              <div class="info">
                <div class="row1">
                  <span class="s-title">{{ s.title }}</span>
                  <span v-if="s.year" class="text-muted">({{ s.year }})</span>
                  <span class="chip" :class="s.status">{{ statusText(s.status) }}</span>
                  <span v-if="s.resolution" class="chip res">{{ s.resolution }}</span>
                </div>
                <div class="text-muted small">
                  {{ t('subscriptions.lastChecked', { time: fmtTime(s.last_checked_at) }) }} ·
                  {{ t('subscriptions.typeLabel') }}
                  {{ s.type === 'tv' ? t('subscriptions.type.tv') : t('subscriptions.type.movie') }} ·
                  {{ t('subscriptions.sourceLabel') }} {{ s.source }}
                </div>
                <div class="res-row">
                  <span class="small">{{ t('subscriptions.resFilter') }}</span>
                  <el-input
                    v-model="s.resolution"
                    size="small"
                    style="width: 120px"
                    :placeholder="t('subscriptions.unlimited')"
                    @change="setResolution(s)"
                  />
                  <span v-if="!s.resolution && globalDefaults.resolution" class="small text-muted">{{
                    t('subscriptions.followGlobal', { v: globalDefaults.resolution })
                  }}</span>
                  <span v-else class="small text-muted">{{ t('subscriptions.resHint') }}</span>
                </div>
                <div class="res-row">
                  <span class="small">{{ t('subscriptions.ratingGuardColon') }}</span>
                  <el-switch
                    :model-value="effRatingGuard(s)"
                    :disabled="s.enable_rating_guard === null"
                    size="small"
                    @update:model-value="(v: any) => setRatingGuard(s, !!v)"
                  />
                  <span v-if="s.enable_rating_guard === null" class="small text-muted">{{
                    t('subscriptions.followGlobal', {
                      v: globalDefaults.enable_rating_guard ? t('common.on') : t('common.off'),
                    })
                  }}</span>
                  <template v-else>
                    <el-button link type="primary" class="small" @click="resetRatingGuard(s)">{{
                      t('subscriptions.resetGlobal')
                    }}</el-button>
                    <span class="small text-muted">{{ t('subscriptions.ratingThresholdHint') }}</span>
                  </template>
                </div>
                <div class="res-row">
                  <span class="small">{{ t('subscriptions.bestVersionColon') }}</span>
                  <el-switch
                    :model-value="effBestVersion(s)"
                    :disabled="s.best_version === null"
                    size="small"
                    @update:model-value="(v: any) => setBestVersion(s, !!v)"
                  />
                  <span v-if="s.best_version === null" class="small text-muted">{{
                    t('subscriptions.followGlobal', {
                      v: globalDefaults.best_version ? t('common.on') : t('common.off'),
                    })
                  }}</span>
                  <template v-else>
                    <el-button link type="primary" class="small" @click="resetBestVersion(s)">{{
                      t('subscriptions.resetGlobal')
                    }}</el-button>
                    <span class="small text-muted">{{ t('subscriptions.bestHint') }}</span>
                  </template>
                  <span v-if="effBestVersion(s) && s.type === 'movie' && s.current_priority" class="chip res">
                    Pri {{ s.current_priority }}
                  </span>
                </div>
                <div class="res-row">
                  <span class="small">{{ t('subscriptions.officialOnlyColon') }}</span>
                  <el-switch
                    :model-value="effOfficialOnly(s)"
                    :disabled="s.official_only === null"
                    size="small"
                    @update:model-value="(v: any) => setOfficialOnly(s, !!v)"
                  />
                  <span v-if="s.official_only === null" class="small text-muted">{{
                    t('subscriptions.followGlobal', {
                      v: globalDefaults.official_only ? t('common.on') : t('common.off'),
                    })
                  }}</span>
                  <template v-else>
                    <el-button link type="primary" class="small" @click="resetOfficialOnly(s)">{{
                      t('subscriptions.resetGlobal')
                    }}</el-button>
                    <span class="small text-muted">{{ t('subscriptions.officialHint') }}</span>
                  </template>
                </div>
                <div class="res-row">
                  <span class="small">{{ t('subscriptions.promoTypesColon') }}</span>
                  <el-select
                    :model-value="s.promo_types"
                    multiple
                    collapse-tags
                    collapse-tags-tooltip
                    size="small"
                    style="width: 240px"
                    :placeholder="t('subscriptions.unlimited')"
                    @change="(v: any) => setPromoTypes(s, v as string[])"
                  >
                    <el-option v-for="o in PROMO_OPTIONS" :key="o.value" :label="o.label" :value="o.value" />
                  </el-select>
                  <span v-if="!s.promo_types.length && globalDefaults.promo_types.length" class="small text-muted">{{
                    t('subscriptions.followGlobal', { v: globalDefaults.promo_types.map(promoLabel).join('、') })
                  }}</span>
                  <span v-else class="small text-muted">{{ t('subscriptions.promoEmptyHint') }}</span>
                </div>
                <div class="res-row">
                  <span class="small">{{ t('subscriptions.sitesColon') }}</span>
                  <SiteFilterDialog
                    :model-value="s.site_ids"
                    :sites="sites"
                    :title="t('subscriptions.sitesDialogTitle')"
                    @update:model-value="(ids: number[]) => setSiteIds(s, ids)"
                  >
                    <span class="small">{{
                      s.site_ids.length
                        ? t('subscriptions.nSites', { n: s.site_ids.length })
                        : globalDefaults.site_ids.length
                          ? t('subscriptions.followGlobalNSites', { n: globalDefaults.site_ids.length })
                          : t('subscriptions.allSites')
                    }}</span>
                  </SiteFilterDialog>
                  <span v-if="!s.site_ids.length && globalDefaults.site_ids.length" class="small text-muted">{{
                    t('subscriptions.sitesFollowGlobalHint')
                  }}</span>
                  <span v-else class="small text-muted">{{ t('subscriptions.sitesEmptyHint') }}</span>
                </div>
                <div class="actions">
                  <el-button size="small" type="primary" :loading="checking === s.id" @click="checkNow(s)">{{
                    t('subscriptions.checkNow')
                  }}</el-button>
                  <el-button size="small" @click="toggleStatus(s)">{{
                    s.status === 'paused' ? t('subscriptions.resume') : t('subscriptions.pause')
                  }}</el-button>
                  <el-button size="small" type="danger" plain @click="remove(s)">{{ t('common.delete') }}</el-button>
                </div>
              </div>
            </div>
          </div>
          <EmptyState
            v-else-if="!loading"
            icon="🔖"
            :title="t('subscriptions.emptyTitle')"
            :description="t('subscriptions.emptyDesc')"
          >
            <router-link to="/search">
              <el-button type="primary">{{ t('subscriptions.goSearch') }}</el-button>
            </router-link>
          </EmptyState>
        </div>
      </el-tab-pane>
      <el-tab-pane label="RSS" name="rss" lazy>
        <div class="rss-embed">
          <RssView />
        </div>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<style scoped src="./subscriptions.css"></style>
