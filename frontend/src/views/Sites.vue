<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { ref, computed, onMounted } from 'vue'
import { siteApi, type Site, type SiteDef, type DefinitionDiagnostic } from '@/api/site'
import SiteDefinitionDiagnostics from '@/components/sites/SiteDefinitionDiagnostics.vue'
import { fmtTime } from '@/utils/format'
import { useAuthStore } from '@/stores/auth'
import { useWs } from '@/composables/useWs'
import { useSiteConnectivity } from '@/composables/useSiteConnectivity'
import SiteOperationsMatrix from '@/components/sites/SiteOperationsMatrix.vue'
import { useSitesForm, defUrls } from '@/composables/useSitesForm'
import { useSitesActions } from '@/composables/useSitesActions'

// 非 admin 的 VIP 用户可读可用，但删除站点是 admin 专属（后端 RequireAdmin 守卫）。
const auth = useAuthStore()
const router = useRouter()
// WS 推送的 cookie 失效站点名集合，用于在站点表上直接标红（而非只在仪表盘卡片提示）。
const { cookieAlerts } = useWs()

const sites = ref<Site[]>([])
const defs = ref<SiteDef[]>([])
const defDiags = ref<DefinitionDiagnostic[]>([])
const defDiagsFailed = ref(false)
const defDiagsErrText = ref('')
const loading = ref(false)
// 站点连通状态：进页面瞬时显示上次结果 + 后台 SSE 逐站刷新（composable 封装缓存/节流/中断）。
const { connectivity, refreshing, loadCached, refresh: refreshConnectivity } = useSiteConnectivity()
const operationsMatrixRef = ref<InstanceType<typeof SiteOperationsMatrix> | null>(null)
const staleVisitLoading = ref(false)
const staleVisitProgress = ref('')
const inactiveSiteCount = computed(() => operationsMatrixRef.value?.inactiveSiteCount ?? 0)

async function matrixReload() {
  return operationsMatrixRef.value?.reload() ?? Promise.resolve()
}
async function reload() {
  await load()
}

const {
  dialogVisible,
  editingId,
  isEdit,
  dialogTitle,
  submitting,
  form,
  needsApiKey,
  needsPasskey,
  apiKeyPlaceholder,
  baseUrlPlaceholder,
  baseUrlOptions,
  filteredDefs,
  filterDefinitionOptions,
  onDefChange,
  openCreate,
  openEdit,
  submit,
} = useSitesForm(defs, matrixReload, reload)

const {
  testing,
  checkining,
  checkinAllLoading,
  recordsDrawer,
  records,
  recordsWindow,
  recordsLoading,
  testSite,
  checkinSite,
  checkinAll,
  openRecords,
  removeSite,
  batchRemoveSites,
} = useSitesActions(sites, connectivity, reload, matrixReload)

async function load() {
  loading.value = true
  try {
    // 站点列表为主数据（失败即报）；definitions 仅建站下拉用，降级为空。
    const [nextSites, nextDefs] = await Promise.all([siteApi.list(), siteApi.definitions().catch(() => [])])
    // 诊断单独读、读失败要标出来：静默退回空数组＝把「有 N 条规则没生效」这件事再丢一次。
    const nextDiags = await siteApi
      .definitionDiagnostics()
      .then((d) => {
        defDiagsFailed.value = false
        defDiagsErrText.value = ''
        return d
      })
      .catch((e) => {
        defDiagsFailed.value = true
        defDiagsErrText.value = String((e as Error)?.message ?? e)
        return [] as DefinitionDiagnostic[]
      })
    sites.value = nextSites
    defs.value = nextDefs
    defDiags.value = nextDiags
  } catch (e) {
    ElMessage.error(t('sites.loadFailed'))
  } finally {
    loading.value = false
  }
  // 进页面瞬时填充上次检测结果，随后后台 SSE 逐站刷新（带 2 分钟节流，频繁进出不反复探测）。
  void loadCached().then(() => refreshConnectivity())
}

function openDetail(s: Site) {
  router.push({ name: 'site-detail', params: { id: String(s.id) } })
}

async function visitStaleSites() {
  const matrix = operationsMatrixRef.value
  if (!matrix || staleVisitLoading.value) return
  staleVisitLoading.value = true
  staleVisitProgress.value = ''
  try {
    await matrix.visitInactiveSites((current, total) => {
      staleVisitProgress.value = `${current}/${total}`
    })
  } finally {
    staleVisitLoading.value = false
    staleVisitProgress.value = ''
  }
}

const { t } = useI18n()
onMounted(load)
</script>

<template>
  <div class="sites">
    <SiteDefinitionDiagnostics :diags="defDiags" :failed="defDiagsFailed" :err-text="defDiagsErrText" />
    <div class="head">
      <h2 class="page-title">{{ t('sites.title') }}</h2>
      <div class="head-actions">
        <el-button @click="openRecords()">{{ t('sites.records') }}</el-button>
        <el-tooltip :content="t('sites.checkinAllHint')" placement="bottom" :disabled="!checkinAllLoading">
          <el-button :loading="checkinAllLoading" @click="checkinAll">{{ t('sites.checkinAll') }}</el-button>
        </el-tooltip>
        <el-button :loading="refreshing" :title="t('sites.refreshStatusTitle')" @click="refreshConnectivity(true)">{{
          t('sites.refreshStatus')
        }}</el-button>
        <el-button
          :loading="staleVisitLoading"
          :disabled="!inactiveSiteCount || staleVisitLoading"
          :title="t('sites.staleVisitTitle')"
          @click="visitStaleSites"
        >
          {{
            staleVisitLoading && staleVisitProgress
              ? t('sites.visiting', { n: staleVisitProgress })
              : t('sites.visitStale', { count: inactiveSiteCount ? ` (${inactiveSiteCount})` : '' })
          }}
        </el-button>
        <el-button type="primary" @click="openCreate">{{ t('sites.addSite') }}</el-button>
      </div>
    </div>

    <SiteOperationsMatrix
      ref="operationsMatrixRef"
      :sites="sites"
      :connectivity="connectivity"
      :connectivity-loading="false"
      :cookie-alerts="cookieAlerts"
      :loading-sites="loading"
      :testing-site-id="testing"
      :checkining-site-id="checkining"
      :can-delete="auth.isAdmin"
      @create="openCreate"
      @edit="openEdit"
      @detail="openDetail"
      @remove="removeSite"
      @batch-remove="batchRemoveSites"
      @test="testSite"
      @checkin="checkinSite"
    />

    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="480">
      <el-form label-width="80">
        <el-form-item :label="t('sites.name')">
          <el-input v-model="form.name" :placeholder="t('sites.namePlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('sites.definition')">
          <el-select
            v-model="form.definition"
            :placeholder="t('sites.defPlaceholder')"
            style="width: 100%"
            filterable
            :filter-method="filterDefinitionOptions"
            @change="onDefChange"
          >
            <el-option
              v-for="d in filteredDefs"
              :key="d.id"
              :label="`${d.name}（${d.schema || t('sites.genericSchema')}）${d.aka?.length ? ' · ' + d.aka.join(' / ') : ''}${d.dead ? ' · ' + t('sites.deadSite') : ''}`"
              :value="d.id"
            >
              <span>{{ d.name }}（{{ d.schema || t('sites.genericSchema') }}）</span>
              <span v-if="defUrls(d).length" class="def-url">{{ defUrls(d).join(' / ') }}</span>
              <el-tag v-if="d.aka?.length" size="small" type="primary" effect="plain" style="margin-left: 8px">
                {{ d.aka.join(' / ') }}
              </el-tag>
              <el-tag v-if="d.dead" size="small" type="info" effect="plain" style="margin-left: 8px">
                {{ t('sites.deadSite') }}
              </el-tag>
            </el-option>
          </el-select>
        </el-form-item>
        <el-form-item :label="t('sites.address')">
          <el-select
            v-if="baseUrlOptions.length > 1"
            v-model="form.base_url"
            filterable
            allow-create
            default-first-option
            :placeholder="baseUrlPlaceholder"
            style="width: 100%"
          >
            <el-option v-for="u in baseUrlOptions" :key="u" :label="u" :value="u" />
          </el-select>
          <el-input v-else v-model="form.base_url" :placeholder="baseUrlPlaceholder" />
        </el-form-item>
        <!-- API 型站点（M-Team/YemaPT/FluxTorrent 等）走 token 鉴权，无 cookie/账密概念 -->
        <el-form-item v-if="needsApiKey" label="API Key">
          <el-input v-model="form.api_key" type="password" show-password :placeholder="apiKeyPlaceholder" />
        </el-form-item>
        <!-- FluxTorrent：passkey 用于下载直链，留空则首次用户信息抓取时自动获取回写 -->
        <el-form-item v-if="needsPasskey" label="Passkey">
          <el-input
            v-model="form.passkey"
            type="password"
            show-password
            :placeholder="isEdit ? t('sites.keepIfEdit') : '留空则自动从站点获取'"
          />
        </el-form-item>
        <el-form-item v-if="!needsApiKey" label="Cookie">
          <el-input
            v-model="form.cookie"
            type="textarea"
            :rows="3"
            :placeholder="isEdit ? t('sites.keepIfEdit') : t('sites.cookiePlaceholder')"
          />
        </el-form-item>
        <el-form-item v-if="!needsApiKey" :label="t('sites.account')">
          <el-input v-model="form.username" :placeholder="t('sites.accountPlaceholder')" />
        </el-form-item>
        <el-form-item v-if="!needsApiKey" :label="t('sites.password')">
          <el-input
            v-model="form.password"
            type="password"
            show-password
            :placeholder="isEdit ? t('sites.keepIfEdit') : t('sites.passwordPlaceholder')"
          />
        </el-form-item>
        <el-form-item :label="t('sites.enabled')">
          <el-switch v-model="form.is_active" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="submitting" @click="submit">{{
          isEdit ? t('common.save') : t('common.add')
        }}</el-button>
      </template>
    </el-dialog>

    <el-drawer v-model="recordsDrawer" :title="t('sites.records')" size="600">
      <div v-loading="recordsLoading">
        <div v-if="recordsWindow.truncated" class="text-muted" style="margin-bottom: 8px">
          {{
            recordsWindow.total == null
              ? t('sites.recordsTruncatedUnknown', { n: recordsWindow.shown })
              : t('sites.recordsTruncated', { n: recordsWindow.shown, total: recordsWindow.total })
          }}
        </div>
        <el-table :data="records" style="width: 100%" max-height="100%">
          <el-table-column :label="t('sites.rec.time')" width="150">
            <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column prop="site_name" :label="t('sites.rec.site')" min-width="100" />
          <el-table-column :label="t('sites.rec.result')" min-width="160">
            <template #default="{ row }">
              <span v-if="row.success" class="pill on">
                {{ row.bonus > 0 ? t('sites.rec.bonus', { n: row.bonus }) : row.message || t('sites.rec.success') }}
              </span>
              <span v-else class="pill off">{{ row.error || row.message || t('sites.rec.failed') }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('sites.rec.source')" width="80">
            <template #default="{ row }">{{
              row.source === 'cron' ? t('sites.rec.auto') : t('sites.rec.manual')
            }}</template>
          </el-table-column>
        </el-table>
        <div v-if="!records.length && !recordsLoading" class="empty text-muted">{{ t('sites.rec.empty') }}</div>
      </div>
    </el-drawer>
  </div>
</template>

<style scoped>
.sites {
  display: flex;
  flex-direction: column;
  gap: var(--sp-4);
}
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--sp-3);
}
.head-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: var(--sp-2);
}
.page-title {
  margin: 4px 0;
  font-size: var(--fs-title);
  font-weight: 700;
}
.pill {
  display: inline-block;
  padding: 2px 10px;
  border-radius: var(--radius-pill);
  font-size: var(--fs-xs);
}
.pill.on {
  background: var(--pt-success-bg);
  color: var(--pt-success);
}
.pill.off {
  background: var(--pt-danger-bg);
  color: var(--pt-danger);
}
.empty {
  padding: 40px;
  text-align: center;
}
.def-url {
  margin-left: 8px;
  color: var(--pt-text-muted);
  font-size: var(--fs-xs);
}
:deep(.el-table),
:deep(.el-table tr),
:deep(.el-table td),
:deep(.el-table th) {
  background-color: transparent !important;
}
:deep(.el-table) {
  --el-table-border-color: var(--divider);
}
</style>
