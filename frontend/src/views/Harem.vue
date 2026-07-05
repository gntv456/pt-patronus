<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { haremApi, type HaremResult, type PrescriptionItem } from '@/api/harem'
import { copyText } from '@/utils/clipboard'
import EmptyState from '@/components/EmptyState.vue'
import SiteFilterDialog from '@/components/SiteFilterDialog.vue'

const data = ref<HaremResult | null>(null)
const loading = ref(false)
const detail = ref<PrescriptionItem | null>(null)
const exportText = ref('')
const exportVisible = ref(false)

async function load(refresh = false) {
  loading.value = true
  try {
    data.value = await haremApi.all(refresh)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载后宫失败')
  } finally {
    loading.value = false
  }
}

async function doExport() {
  try {
    const r = await haremApi.export()
    exportText.value = r.text
    exportVisible.value = true
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '导出失败')
  }
}

async function copyExport() {
  await copyText(exportText.value, '已复制药单')
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
</script>

<template>
  <div class="page" v-loading="loading">
    <div class="head">
      <h2 class="title">后宫</h2>
      <div class="head-tools">
        <SiteFilterDialog v-model="selectedSiteIds" :sites="sites" title="选择后宫站点" />
        <el-button @click="doExport">导出药单</el-button>
        <el-button :loading="loading" @click="load(true)">刷新</el-button>
      </div>
    </div>

    <div v-if="overview" class="summary glass">
      <div class="tile"><span class="num">{{ overview.total_sites }}</span><span class="lbl">站点</span></div>
      <div class="tile"><span class="num">{{ overview.total_invitees }}</span><span class="lbl">后宫成员</span></div>
      <div class="tile"><span class="num">{{ overview.total_perm_invites }}</span><span class="lbl">永久邀请</span></div>
      <div class="tile"><span class="num">{{ overview.total_temp_invites }}</span><span class="lbl">临时邀请</span></div>
      <div class="tile"><span class="num" style="color:#f39c12">{{ overview.total_low_ratio }}</span><span class="lbl">低分享率</span></div>
      <div class="tile"><span class="num" style="color:#f56c6c">{{ overview.total_banned }}</span><span class="lbl">已禁用</span></div>
      <div class="tile"><span class="num" style="color:#95a5a6">{{ overview.total_no_data }}</span><span class="lbl">无数据</span></div>
    </div>

    <div class="card glass">
      <el-table v-if="filteredPrescriptions.length" :data="filteredPrescriptions" style="width:100%" :row-style="{ background: 'transparent' }">
        <el-table-column label="站点" width="120">
          <template #default="{ row }"><span class="site-tag">{{ row.site_name }}</span></template>
        </el-table-column>
        <el-table-column prop="permanent_count" label="永久" width="80" sortable align="right" />
        <el-table-column prop="temporary_count" label="临时" width="70" sortable align="right">
          <template #default="{ row }">
            <span :style="{ color: row.temporary_count ? '#f39c12' : 'inherit' }">{{ row.temporary_count }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="can_buy" label="可购买" width="90" sortable align="right" />
        <el-table-column label="权限" width="70" align="center">
          <template #default="{ row }">
            <span :style="{ color: row.can_invite ? '#2ecc71' : '#f56c6c' }">{{ row.can_invite ? '✓' : '✗' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="后宫数" width="90" align="right">
          <template #default="{ row }">
            {{ row.harem_count }}<span v-if="row.dead_count" style="color:#f56c6c"> ({{ row.dead_count }})</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" min-width="140">
          <template #default="{ row }">
            <span v-if="row.error" class="err">{{ row.error }}</span>
            <span v-else-if="!row.invitees?.length" class="text-muted">无被邀请人</span>
            <span v-else class="text-muted">点击查看花名册</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="80" fixed="right">
          <template #default="{ row }">
            <el-button size="small" :disabled="!row.invitees?.length" @click="detail = row as PrescriptionItem">详情</el-button>
          </template>
        </el-table-column>
      </el-table>
      <EmptyState
        v-else-if="!loading"
        icon="💗"
        title="暂无数据，请先添加并登录站点"
        description="添加并启用 PT 站点后，这里会聚合展示各站后宫成员与可购买邀请名额"
      />
    </div>

    <el-dialog v-model="exportVisible" title="药单" width="560">
      <el-input v-model="exportText" type="textarea" :rows="12" readonly />
      <template #footer>
        <el-button @click="exportVisible = false">关闭</el-button>
        <el-button type="primary" @click="copyExport">复制</el-button>
      </template>
    </el-dialog>

    <el-dialog :model-value="!!detail" @update:model-value="(v: boolean) => { if (!v) detail = null }" :title="detail ? detail.site_name + ' 后宫成员' : ''" width="640">
      <el-table v-if="detail" :data="detail.invitees || []" style="width:100%" :row-style="{ background: 'transparent' }">
        <el-table-column label="用户" min-width="120">
          <template #default="{ row }">
            <span :style="{ color: row.status === 'disabled' ? '#f56c6c' : 'inherit' }">{{ row.username }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="90">
          <template #default="{ row }">
            <span class="chip" :style="{ background: row.status === 'disabled' ? '#f56c6c22' : row.status === 'pending' ? '#f39c1222' : '#2ecc7122', color: row.status === 'disabled' ? '#f56c6c' : row.status === 'pending' ? '#f39c12' : '#2ecc71' }">{{ row.status }}</span>
          </template>
        </el-table-column>
        <el-table-column label="上传" width="80" align="right"><template #default="{ row }">{{ row.uploaded ? (row.uploaded/1073741824).toFixed(1)+'G' : '—' }}</template></el-table-column>
        <el-table-column label="下载" width="80" align="right"><template #default="{ row }">{{ row.downloaded ? (row.downloaded/1073741824).toFixed(1)+'G' : '—' }}</template></el-table-column>
        <el-table-column prop="ratio" label="分享率" width="80" align="right" />
      </el-table>
      <template #footer><el-button @click="detail = null">关闭</el-button></template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: 16px; }
.head { display: flex; align-items: center; justify-content: space-between; }
.head > div { display: flex; gap: 8px; }
.title { margin: 4px 0; font-size: 22px; font-weight: 700; }
.summary { display: flex; gap: 6px; padding: 12px; flex-wrap: wrap; }
.tile { display: flex; flex-direction: column; align-items: center; flex: 1; min-width: 76px; }
.tile .num { font-size: 22px; font-weight: 700; font-variant-numeric: tabular-nums; }
.tile .lbl { font-size: 11px; color: var(--text-muted); margin-top: 2px; }
.card { padding: 18px; min-height: 200px; }
.site-tag { display: inline-block; padding: 2px 10px; border-radius: 999px; background: rgba(108,92,231,0.16); color: var(--pt-primary,#6c5ce7); font-size: 12px; }
.err { color: var(--pt-danger); font-size: 13px; }
.chip { padding: 1px 8px; border-radius: 999px; font-size: 11px; }
.empty { padding: 40px; text-align: center; }
:deep(.el-table), :deep(.el-table tr), :deep(.el-table td), :deep(.el-table th) { background-color: transparent !important; }
</style>
