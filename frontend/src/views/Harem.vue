<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { haremApi, type HaremResult, type HaremOverview, type PrescriptionItem } from '@/api/harem'
import { copyText } from '@/utils/clipboard'
import EmptyState from '@/components/EmptyState.vue'
import SiteFilterDialog from '@/components/SiteFilterDialog.vue'
import SptIcon from '@/components/SptIcon.vue'

const { t } = useI18n()
const data = ref<HaremResult | null>(null)
const loading = ref(false)
const detail = ref<PrescriptionItem | null>(null)
const exportText = ref('')
const exportVisible = ref(false)
// 批量选择（表格批量操作栏）
const selectedRows = ref<PrescriptionItem[]>([])

const EMPTY_OVERVIEW: HaremOverview = {
  total_sites: 0,
  total_invitees: 0,
  total_perm_invites: 0,
  total_temp_invites: 0,
  total_low_ratio: 0,
  total_banned: 0,
  total_no_data: 0,
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
      (o) => {
        if (data.value) data.value.overview = o
      },
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

async function copySelected() {
  const text = selectedRows.value
    .map((p) => `${p.site_name}\t${p.permanent_count}\t${p.temporary_count}`)
    .join('\n')
  await copyText(text, t('harem.copied'))
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

    <div v-if="overview" class="summary solid-card">
      <div class="tile">
        <span class="num">{{ overview.total_sites }}</span
        ><span class="lbl">{{ t('freeleech.tile.sites') }}</span>
      </div>
      <div class="tile">
        <span class="num">{{ overview.total_invitees }}</span
        ><span class="lbl">{{ t('harem.members') }}</span>
      </div>
      <div class="tile">
        <span class="num">{{ overview.total_perm_invites }}</span
        ><span class="lbl">{{ t('harem.permInvites') }}</span>
      </div>
      <div class="tile">
        <span class="num">{{ overview.total_temp_invites }}</span
        ><span class="lbl">{{ t('harem.tempInvites') }}</span>
      </div>
      <div class="tile">
        <span class="num" style="color: var(--pt-warn)">{{ overview.total_low_ratio }}</span
        ><span class="lbl">{{ t('harem.lowRatio') }}</span>
      </div>
      <div class="tile">
        <span class="num" style="color: var(--pt-danger)">{{ overview.total_banned }}</span
        ><span class="lbl">{{ t('harem.banned') }}</span>
      </div>
      <div class="tile">
        <span class="num" style="color: var(--text-muted)">{{ overview.total_no_data }}</span
        ><span class="lbl">{{ t('mediaserver.noData') }}</span>
      </div>
    </div>

    <div class="card solid-card">
      <div v-if="selectedRows.length" class="batch-bar">
        <span>已选 {{ selectedRows.length }} 条</span>
        <el-button size="small" @click="copySelected">{{ t('harem.copy') }}</el-button>
        <el-button size="small" text @click="selectedRows = []">{{ t('common.cancel') }}</el-button>
      </div>
      <el-table
        v-if="filteredPrescriptions.length"
        :data="filteredPrescriptions"
        style="width: 100%"
        max-height="480"
        @selection-change="(rows: PrescriptionItem[]) => (selectedRows = rows)"
      >
        <el-table-column type="selection" width="46" />
        <el-table-column :label="t('contract.f.site')" width="120">
          <template #default="{ row }"
            ><span class="site-tag">{{ row.site_name }}</span></template
          >
        </el-table-column>
        <el-table-column prop="permanent_count" :label="t('harem.perm')" width="80" sortable align="right" />
        <el-table-column prop="temporary_count" :label="t('harem.temp')" width="70" sortable align="right">
          <template #default="{ row }">
            <span :style="{ color: row.temporary_count ? 'var(--pt-warn-fg)' : 'inherit' }">{{ row.temporary_count }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="can_buy" :label="t('medalwall.status.available')" width="90" sortable align="right" />
        <el-table-column :label="t('harem.permission')" width="70" align="center">
          <template #default="{ row }">
            <span :style="{ color: row.can_invite ? 'var(--pt-success-fg)' : 'var(--pt-danger-fg)' }"><SptIcon :name="row.can_invite ? 'check' : 'x'" :size="14" aria-hidden="true" /></span>
          </template>
        </el-table-column>
        <el-table-column :label="t('harem.count')" width="90" align="right">
          <template #default="{ row }">
            {{ row.harem_count
            }}<span v-if="row.dead_count" style="color: var(--pt-danger)"> ({{ row.dead_count }})</span>
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
            <el-button size="small" :disabled="!row.invitees?.length" @click="detail = row as PrescriptionItem">{{
              t('harem.detail')
            }}</el-button>
          </template>
        </el-table-column>
      </el-table>
      <EmptyState
        v-else-if="!loading"
        :title="t('announcements.emptyTitle')"
        :description="t('harem.emptyDesc')"
      />
      <el-skeleton v-else :rows="5" animated />
    </div>

    <el-dialog v-model="exportVisible" :title="t('harem.rxTitle')" width="560">
      <el-input v-model="exportText" type="textarea" :rows="12" readonly />
      <template #footer>
        <el-button @click="exportVisible = false">{{ t('common.close') }}</el-button>
        <el-button type="primary" @click="copyExport">{{ t('harem.copy') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog
      :model-value="!!detail"
      @update:model-value="
        (v: boolean) => {
          if (!v) detail = null
        }
      "
      :title="detail ? detail.site_name + ' ' + t('harem.members') : ''"
      width="640"
    >
      <el-table
        v-if="detail"
        :data="detail.invitees || []"
        style="width: 100%"
        max-height="420"
      >
        <el-table-column :label="t('mediaserver.col.user')" min-width="120">
          <template #default="{ row }">
            <span :style="{ color: row.status === 'disabled' ? 'var(--pt-danger-fg)' : 'inherit' }">{{ row.username }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="status" :label="t('reseed.col.status')" width="90">
          <template #default="{ row }">
            <span class="pt-chip" :class="row.status === 'disabled' ? 'pt-chip--danger' : row.status === 'pending' ? 'pt-chip--warn' : 'pt-chip--success'">{{ row.status }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('contract.uploadShort')" width="80" align="right"
          ><template #default="{ row }">{{
            row.uploaded ? (row.uploaded / 1073741824).toFixed(1) + 'G' : '—'
          }}</template></el-table-column
        >
        <el-table-column :label="t('harem.down')" width="80" align="right"
          ><template #default="{ row }">{{
            row.downloaded ? (row.downloaded / 1073741824).toFixed(1) + 'G' : '—'
          }}</template></el-table-column
        >
        <el-table-column prop="ratio" :label="t('harem.ratio')" width="80" align="right" />
      </el-table>
      <template #footer
        ><el-button @click="detail = null">{{ t('common.close') }}</el-button></template
      >
    </el-dialog>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: var(--sp-4);
}
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.head > div {
  display: flex;
  gap: var(--sp-2);
}
.title {
  margin: 4px 0;
  font-size: var(--fs-title);
  font-weight: 700;
}
.summary {
  display: flex;
  gap: var(--sp-1);
  padding: 12px;
  flex-wrap: wrap;
}
.tile {
  display: flex;
  flex-direction: column;
  align-items: center;
  flex: 1;
  min-width: 76px;
}
.tile .num {
  font-size: var(--fs-title);
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}
.tile .lbl {
  font-size: var(--fs-2xs);
  color: var(--text-muted);
  margin-top: 2px;
}
.card {
  padding: 18px;
  min-height: 200px;
  overflow-x: auto;
}
.batch-bar {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: 8px 12px;
  margin-bottom: 8px;
  background: var(--color-primary-soft);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-control);
  font-size: var(--fs-xs);
}
.site-tag {
  display: inline-block;
  padding: 2px 10px;
  border-radius: var(--radius-pill);
  background: var(--pt-primary-bg);
  color: var(--pt-primary);
  font-size: var(--fs-xs);
}
.err {
  color: var(--pt-danger);
  font-size: var(--fs-body);
}
.empty {
  padding: 40px;
  text-align: center;
}
.table-scroll {
  overflow-x: auto;
}
:deep(.el-table__header-wrapper th.el-table__cell) {
  height: var(--table-row-height);
  background: var(--color-surface-2);
  color: var(--color-text);
  font-weight: 600;
}
:deep(.el-table__body-wrapper td.el-table__cell) {
  height: var(--table-row-height);
}
:deep(.num) {
  font-variant-numeric: tabular-nums;
}
</style>
