<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { haremApi, type HaremResult, type HaremOverview, type PrescriptionItem } from '@/api/harem'
import { copyText } from '@/utils/clipboard'
import EmptyState from '@/components/EmptyState.vue'
import SiteFilterDialog from '@/components/SiteFilterDialog.vue'

const { t } = useI18n()
const data = ref<HaremResult | null>(null)
const loading = ref(false)
const detail = ref<PrescriptionItem | null>(null)
const exportText = ref('')
const exportVisible = ref(false)

const EMPTY_OVERVIEW: HaremOverview = {
  total_sites: 0, total_invitees: 0, total_perm_invites: 0, total_temp_invites: 0,
  total_low_ratio: 0, total_banned: 0, total_no_data: 0,
}
let abortCtrl: AbortController | null = null
async function load(refresh = false) {
  abortCtrl?.abort()
  const ctrl = new AbortController()
  abortCtrl = ctrl
  loading.value = true
  data.value = { overview: { ...EMPTY_OVERVIEW }, prescriptions: [] }
  try {
    await haremApi.allStream(
      refresh,
      (item) => data.value?.prescriptions.push(item),
      (o) => { if (data.value) data.value.overview = o },
      ctrl.signal,
    )
  } catch (e: unknown) {
    if ((e as Error).name !== 'AbortError') ElMessage.error((e as Error).message || t('harem.loadFailed'))
  } finally {
    if (abortCtrl === ctrl) {
      loading.value = false
      abortCtrl = null
    }
  }
}

async function doExport() {
  try {
    const r = await haremApi.export()
    exportText.value = r.text
    exportVisible.value = true
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('harem.exportFailed'))
  }
}

async function copyExport() {
  await copyText(exportText.value, t('harem.copied'))
}

const overview = computed(() => data.value?.overview)

// 站点筛选：空=全部，非空=仅选中站点（多选，纯前端筛选 prescriptions）。
const selectedSiteIds = ref<number[]>([])
const sites = computed(() => {
  const m = new Map<number, string>()
  for (const p of data.value?.prescriptions ?? []) if (!m.has(p.site_id)) m.set(p.site_id, p.site_name)
  return [...m].map(([id, name]) => ({ id, name }))
})
const filteredPrescriptions = computed(() => {
  const list = data.value?.prescriptions ?? []
  return selectedSiteIds.value.length ? list.filter((p) => selectedSiteIds.value.includes(p.site_id)) : list
})

onMounted(() => load())
onUnmounted(() => abortCtrl?.abort())
</script>

<template>
  <div class="page">
    <div class="head">
      <h2 class="title">{{ t('harem.title') }}</h2>
      <div class="head-tools">
        <SiteFilterDialog v-model="selectedSiteIds" :sites="sites" :title="t('harem.siteFilterTitle')" />
        <el-button @click="doExport">{{ t('harem.exportRx') }}</el-button>
        <el-button :loading="loading" @click="load(true)">{{ t('common.refresh') }}</el-button>
      </div>
    </div>

    <div v-if="overview" class="summary glass">
      <div class="tile"><span class="num">{{ overview.total_sites }}</span><span class="lbl">{{ t('freeleech.tile.sites') }}</span></div>
      <div class="tile"><span class="num">{{ overview.total_invitees }}</span><span class="lbl">{{ t('harem.members') }}</span></div>
      <div class="tile"><span class="num">{{ overview.total_perm_invites }}</span><span class="lbl">{{ t('harem.permInvites') }}</span></div>
      <div class="tile"><span class="num">{{ overview.total_temp_invites }}</span><span class="lbl">{{ t('harem.tempInvites') }}</span></div>
      <div class="tile"><span class="num" style="color:var(--pt-warn)">{{ overview.total_low_ratio }}</span><span class="lbl">{{ t('harem.lowRatio') }}</span></div>
      <div class="tile"><span class="num" style="color:var(--pt-danger)">{{ overview.total_banned }}</span><span class="lbl">{{ t('harem.banned') }}</span></div>
      <div class="tile"><span class="num" style="color:var(--text-muted)">{{ overview.total_no_data }}</span><span class="lbl">{{ t('mediaserver.noData') }}</span></div>
    </div>

    <div class="card glass">
      <el-table v-if="filteredPrescriptions.length" :data="filteredPrescriptions" style="width:100%" :row-style="{ background: 'transparent' }">
        <el-table-column :label="t('contract.f.site')" width="120">
          <template #default="{ row }"><span class="site-tag">{{ row.site_name }}</span></template>
        </el-table-column>
        <el-table-column prop="permanent_count" :label="t('harem.perm')" width="80" sortable align="right" />
        <el-table-column prop="temporary_count" :label="t('harem.temp')" width="70" sortable align="right">
          <template #default="{ row }">
            <span :style="{ color: row.temporary_count ? '#f39c12' : 'inherit' }">{{ row.temporary_count }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="can_buy" :label="t('medalwall.status.available')" width="90" sortable align="right" />
        <el-table-column :label="t('harem.permission')" width="70" align="center">
          <template #default="{ row }">
            <span :style="{ color: row.can_invite ? '#2ecc71' : '#f56c6c' }">{{ row.can_invite ? '✓' : '✗' }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('harem.count')" width="90" align="right">
          <template #default="{ row }">
            {{ row.harem_count }}<span v-if="row.dead_count" style="color:var(--pt-danger)"> ({{ row.dead_count }})</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('reseed.col.status')" min-width="140">
          <template #default="{ row }">
            <span v-if="row.error" class="err">{{ row.error }}</span>
            <span v-else-if="!row.invitees?.length" class="text-muted">{{ t('harem.noInvitees') }}</span>
            <span v-else class="text-muted">{{ t('harem.viewRoster') }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('reseed.col.actions')" width="80" fixed="right">
          <template #default="{ row }">
            <el-button size="small" :disabled="!row.invitees?.length" @click="detail = row as PrescriptionItem">{{ t('harem.detail') }}</el-button>
          </template>
        </el-table-column>
      </el-table>
      <EmptyState
        v-else-if="!loading"
        icon="💗"
        :title="t('announcements.emptyTitle')"
        :description="t('harem.emptyDesc')"
      />
      <p v-else style="text-align:center;color:var(--text-muted);padding:40px 0">{{ t('common.loading') }}</p>
    </div>

    <el-dialog v-model="exportVisible" :title="t('harem.rxTitle')" width="560">
      <el-input v-model="exportText" type="textarea" :rows="12" readonly />
      <template #footer>
        <el-button @click="exportVisible = false">{{ t('common.close') }}</el-button>
        <el-button type="primary" @click="copyExport">{{ t('harem.copy') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog :model-value="!!detail" @update:model-value="(v: boolean) => { if (!v) detail = null }" :title="detail ? detail.site_name + ' ' + t('harem.members') : ''" width="640">
      <el-table v-if="detail" :data="detail.invitees || []" style="width:100%" :row-style="{ background: 'transparent' }">
        <el-table-column :label="t('mediaserver.col.user')" min-width="120">
          <template #default="{ row }">
            <span :style="{ color: row.status === 'disabled' ? '#f56c6c' : 'inherit' }">{{ row.username }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="status" :label="t('reseed.col.status')" width="90">
          <template #default="{ row }">
            <span class="chip" :style="{ background: row.status === 'disabled' ? '#f56c6c22' : row.status === 'pending' ? '#f39c1222' : '#2ecc7122', color: row.status === 'disabled' ? '#f56c6c' : row.status === 'pending' ? '#f39c12' : '#2ecc71' }">{{ row.status }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('contract.uploadShort')" width="80" align="right"><template #default="{ row }">{{ row.uploaded ? (row.uploaded/1073741824).toFixed(1)+'G' : '—' }}</template></el-table-column>
        <el-table-column :label="t('harem.down')" width="80" align="right"><template #default="{ row }">{{ row.downloaded ? (row.downloaded/1073741824).toFixed(1)+'G' : '—' }}</template></el-table-column>
        <el-table-column prop="ratio" :label="t('harem.ratio')" width="80" align="right" />
      </el-table>
      <template #footer><el-button @click="detail = null">{{ t('common.close') }}</el-button></template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: var(--sp-4); }
.head { display: flex; align-items: center; justify-content: space-between; }
.head > div { display: flex; gap: var(--sp-2); }
.title { margin: 4px 0; font-size: var(--fs-title); font-weight: 700; }
.summary { display: flex; gap: var(--sp-1); padding: 12px; flex-wrap: wrap; }
.tile { display: flex; flex-direction: column; align-items: center; flex: 1; min-width: 76px; }
.tile .num { font-size: var(--fs-title); font-weight: 700; font-variant-numeric: tabular-nums; }
.tile .lbl { font-size: var(--fs-2xs); color: var(--text-muted); margin-top: 2px; }
.card { padding: 18px; min-height: 200px; }
.site-tag { display: inline-block; padding: 2px 10px; border-radius: var(--radius-pill); background: var(--pt-primary-bg); color: var(--pt-primary,#6c5ce7); font-size: var(--fs-xs); }
.err { color: var(--pt-danger); font-size: var(--fs-body); }
.chip { padding: 1px 8px; border-radius: var(--radius-pill); font-size: var(--fs-2xs); }
.empty { padding: 40px; text-align: center; }
:deep(.el-table), :deep(.el-table tr), :deep(.el-table td), :deep(.el-table th) { background-color: transparent !important; }
</style>
