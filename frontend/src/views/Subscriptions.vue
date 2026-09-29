<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import SubscriptionCard from '@/components/subscribe/SubscriptionCard.vue'
import SubscriptionGlobalDialog from '@/components/subscribe/SubscriptionGlobalDialog.vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { subscribeApi, type Subscription, type GlobalDefaults } from '@/api/subscribe'
import { siteApi } from '@/api/site'
import { useSubscriptionToggles } from '@/composables/useSubscriptionToggles'
import { useSubscriptionList } from '@/composables/useSubscriptionList'
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
  // 批量退订是破坏性操作：ElMessageBox 统一确认（替代原生 confirm——视觉与全站割裂，
  // 且阻塞主线程）。type:warning 与其余 40+ 处确认形态一致。
  if (op === 'delete') {
    try {
      await ElMessageBox.confirm(
        t('subscriptions.batch.confirmDelete', { n: selectedIds.value.length }),
        t('common.confirm'),
        { type: 'warning', confirmButtonText: t('common.delete'), cancelButtonText: t('common.cancel') }
      )
    } catch {
      return
    }
  }
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
        <SubscriptionGlobalDialog v-model:open="globalOpen" :form="globalForm" :saving="savingGlobal" :all-sites="sites" @save="saveGlobal" />

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
            <SubscriptionCard
              v-for="s in list"
              :key="s.id"
              :s="s"
              :batch-mode="batchMode"
              :selected="selectedIds.includes(s.id)"
              :checking="checking"
              :global-defaults="globalDefaults"
              :sites="sites"
              :promo-options="PROMO_OPTIONS"
              :eff-rating-guard="effRatingGuard"
              :eff-best-version="effBestVersion"
              :eff-official-only="effOfficialOnly"
              :status-text="statusText"
              :fmt-time="fmtTime"
              :promo-label="promoLabel"
              @toggle-select="toggleSelect"
              @set-resolution="setResolution"
              @set-rating-guard="setRatingGuard"
              @reset-rating-guard="resetRatingGuard"
              @set-best-version="setBestVersion"
              @reset-best-version="resetBestVersion"
              @set-official-only="setOfficialOnly"
              @reset-official-only="resetOfficialOnly"
              @set-promo-types="setPromoTypes"
              @set-site-ids="setSiteIds"
              @check-now="checkNow"
              @toggle-status="toggleStatus"
              @remove="remove"
            />
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
