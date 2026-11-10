<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import BrushFlowFormDialog from '@/components/brushflow/BrushFlowFormDialog.vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { brushflowApi, type BrushFlowTask, type BrushFlowSeed } from '@/api/brushflow'
import { siteApi } from '@/api/site'
import { downloaderApi } from '@/api/downloader'
import { filterApi } from '@/api/filter'
import GlassIcon from '@/components/GlassIcon.vue'
import EmptyState from '@/components/EmptyState.vue'
import { ElMessageBox, ElMessage } from 'element-plus'

const { t } = useI18n()
const route = useRoute()
const tasks = ref<BrushFlowTask[]>([])
/** 站点详情深链：?site_id= 仅显示该站任务 */
const focusSiteId = computed(() => {
  const n = Number(route.query.site_id)
  return Number.isFinite(n) && n > 0 ? n : 0
})
const displayTasks = computed(() =>
  focusSiteId.value ? tasks.value.filter((t) => t.site_id === focusSiteId.value) : tasks.value,
)
const focusSiteName = computed(() => {
  if (!focusSiteId.value) return ''
  const fromList = sites.value.find((s) => s.id === focusSiteId.value)?.name
  const q = route.query.site_name
  return fromList || (typeof q === 'string' ? q : '') || `#${focusSiteId.value}`
})
const sites = ref<{ id: number; name: string }[]>([])
const downloaders = ref<{ id: number; name: string }[]>([])
const filterGroups = ref<string[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    // 任务列表为主数据（失败即报）；站点/下载器仅表单下拉用，逐项降级为空。
    const [ts, ss, ds] = await Promise.all([
      brushflowApi.list(),
      siteApi.list().catch(() => []),
      downloaderApi.list().catch(() => []),
    ])
    tasks.value = ts
    sites.value = ss.map((s) => ({ id: s.id, name: s.name }))
    downloaders.value = ds.map((d) => ({ id: d.id, name: d.name }))
  } catch (e: any) {
    ElMessage.error(e?.userMessage || e?.message || t('common.loadFailed'))
  } finally {
    loading.value = false
  }
}

const showForm = ref(false)
const editing = ref<BrushFlowTask | null>(null)
const form = ref<Partial<BrushFlowTask>>(defaultForm())

function defaultForm(): Partial<BrushFlowTask> {
  return {
    name: '',
    site_id: 0,
    downloader_id: 0,
    enabled: true,
    keyword: '',
    category: '',
    min_size_mb: 0,
    max_size_mb: 0,
    min_seeders: 0,
    max_seeders: 0,
    free_only: false,
    paused: false,
    delete_ratio: 0,
    delete_seed_hours: 0,
    delete_upload_mb: 0,
    delete_files: false,
    use_share_limit: false,
    delete_min_up_rate_kb: 0,
    delete_inactive_hours: 0,
    delete_logic: 'or',
    delete_rule_expr: '',
    max_total_size_gb: 0,
    delete_max_swarm_seeders: 0,
    delete_min_swarm_leechers: 0,
    max_seeds: 10,
    save_path: '',
    category_dl: '',
    filter_groups: '',
  }
}

function openCreate() {
  editing.value = null
  form.value = defaultForm()
  if (focusSiteId.value) form.value.site_id = focusSiteId.value
  else if (sites.value.length) form.value.site_id = sites.value[0].id
  if (downloaders.value.length) form.value.downloader_id = downloaders.value[0].id
  showForm.value = true
}

function openEdit(t: BrushFlowTask) {
  editing.value = t
  // 旧任务缺省新字段时回落 defaultForm，避免 el-radio 空值
  form.value = {
    ...defaultForm(),
    ...t,
    delete_logic: t.delete_logic || 'or',
    delete_rule_expr: t.delete_rule_expr || '',
  }
  showForm.value = true
}

// 过滤规则组：表单以 | 分隔字符串提交，UI 用多选拆分回填。
const filterGroupArray = computed<string[]>({
  get: () =>
    (form.value.filter_groups || '')
      .split('|')
      .map((s) => s.trim())
      .filter(Boolean),
  set: (v: string[]) => {
    form.value.filter_groups = v.join('|')
  },
})

async function save() {
  if (!form.value.name || !form.value.site_id || !form.value.downloader_id) {
    ElMessage.warning(t('brushflow.nameSiteDlRequired'))
    return
  }
  try {
    if (editing.value) {
      await brushflowApi.update(editing.value.id, form.value)
      ElMessage.success(t('common.updated'))
    } else {
      await brushflowApi.create(form.value)
      ElMessage.success(t('common.created'))
    }
    showForm.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(e?.userMessage || e?.message || t('common.saveFailed'))
  }
}

async function remove(task: BrushFlowTask) {
  try {
    await ElMessageBox.confirm(t('brushflow.confirmDelete', { name: task.name }), t('messages.tip'), {
      type: 'warning',
    })
  } catch {
    return
  }
  try {
    await brushflowApi.remove(task.id)
    ElMessage.success(t('common.deleted'))
    await load()
  } catch (e: any) {
    ElMessage.error(e?.userMessage || e?.message || t('common.deleteFailed'))
  }
}

async function toggle(task: BrushFlowTask, val: boolean) {
  try {
    await brushflowApi.update(task.id, { ...task, enabled: val })
    task.enabled = val
  } catch (e: any) {
    ElMessage.error(e?.userMessage || e?.message || t('common.saveFailed'))
  }
}

const running = ref<string | null>(null)
async function runAction(task: BrushFlowTask, kind: 'select' | 'sweep') {
  const key = `${task.id}:${kind}`
  if (running.value) return
  running.value = key
  try {
    const fn = kind === 'select' ? brushflowApi.select : brushflowApi.sweep
    await fn(task.id)
    ElMessage.success(kind === 'select' ? t('brushflow.selectDone') : t('brushflow.sweepDone'))
    await load()
  } catch (e: any) {
    ElMessage.error(e?.userMessage || e?.message || t('common.actionFailed'))
  } finally {
    running.value = null
  }
}

const showSeeds = ref(false)
const seeds = ref<BrushFlowSeed[]>([])
// 种子历史单页 500 条封顶：被截断时说出来，否则更早加入的种子在界面上凭空消失。
const seedWindow = ref<{ shown: number; total?: number; truncated: boolean }>({
  shown: 0,
  total: undefined,
  truncated: false,
})
const seedsLoading = ref(false)
const seedsTask = ref('')

async function openSeeds(task: BrushFlowTask) {
  seedsTask.value = task.name
  showSeeds.value = true
  seedsLoading.value = true
  try {
    const page = await brushflowApi.seeds(task.id)
    seeds.value = page.seeds
    seedWindow.value = { shown: page.seeds.length, total: page.total, truncated: page.truncated }
  } catch (e: any) {
    seeds.value = []
    seedWindow.value = { shown: 0, total: undefined, truncated: false }
    ElMessage.error(e?.userMessage || e?.message || t('common.loadFailed'))
  } finally {
    seedsLoading.value = false
  }
}

function siteName(id: number) {
  return sites.value.find((s) => s.id === id)?.name || `#${id}`
}
function dlName(id: number) {
  return downloaders.value.find((d) => d.id === id)?.name || `#${id}`
}
function fmtBytes(n: number) {
  if (!n) return '0'
  const u = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = n
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(1)} ${u[i]}`
}

onMounted(() => {
  load()
  filterApi
    .groups()
    .then((gs) => {
      filterGroups.value = gs.map((g) => g.name)
    })
    .catch(() => [])
})
</script>

<template>
  <div class="page">
    <div class="head">
      <h2 class="title">{{ t('brushflow.title') }}</h2>
      <el-button type="primary" @click="openCreate">{{ t('brushflow.create') }}</el-button>
    </div>
    <p class="sub">{{ t('brushflow.subtitle') }}</p>

    <el-alert
      v-if="focusSiteId"
      type="info"
      :closable="false"
      show-icon
      style="margin-bottom: 12px"
      :title="t('brushflow.focusTitle', { site: focusSiteName, n: displayTasks.length })"
    />

    <!-- 首屏骨架（W3）：任务卡结构已知（图标+标题+三列统计），骨架屏比转圈更
         传达「内容即将就位」；glass.css 的 .pt-skeleton 此前零使用。 -->
    <div v-if="loading && !displayTasks.length" class="grid" aria-busy="true">
      <div v-for="i in 4" :key="'sk' + i" class="glass card sk-card">
        <div class="card-head">
          <div class="pt-skeleton sk-icon"></div>
          <div class="card-title">
            <div class="pt-skeleton sk-line w60"></div>
            <div class="pt-skeleton sk-line w40"></div>
          </div>
          <div class="pt-skeleton sk-switch"></div>
        </div>
        <div class="stats">
          <div v-for="j in 3" :key="j" class="stat">
            <div class="pt-skeleton sk-num"></div>
            <div class="pt-skeleton sk-line w40"></div>
          </div>
        </div>
      </div>
    </div>
    <div v-else v-loading="loading" class="grid">
      <div v-for="task in displayTasks" :key="task.id" class="glass glass-hover card">
        <div class="card-head">
          <GlassIcon name="brushflow" :size="34" from="#f7971e" to="#ff6a00" />
          <div class="card-title">
            <div class="name">{{ task.name }}</div>
            <div class="meta">{{ siteName(task.site_id) }} → {{ dlName(task.downloader_id) }}</div>
          </div>
          <el-switch :model-value="task.enabled" @update:model-value="(v: any) => toggle(task, !!v)" />
        </div>

        <div class="stats">
          <div class="stat">
            <span class="sv">{{ task.total_added }}</span
            ><span class="sl">{{ t('brushflow.statAdded') }}</span>
          </div>
          <div class="stat">
            <span class="sv">{{ task.total_deleted }}</span
            ><span class="sl">{{ t('common.delete') }}</span>
          </div>
          <div class="stat">
            <span class="sv">{{ fmtBytes(task.total_upload) }}</span
            ><span class="sl">{{ t('contract.uploadShort') }}</span>
          </div>
        </div>

        <div v-if="task.last_error" class="err">⚠ {{ task.last_error }}</div>

        <div class="actions">
          <el-button size="small" :loading="running === `${task.id}:select`" @click="runAction(task, 'select')">{{
            t('brushflow.select')
          }}</el-button>
          <el-button size="small" :loading="running === `${task.id}:sweep`" @click="runAction(task, 'sweep')">{{
            t('brushflow.sweep')
          }}</el-button>
          <el-button size="small" @click="openSeeds(task)">{{ t('brushflow.history') }}</el-button>
          <el-button size="small" @click="openEdit(task)">{{ t('common.edit') }}</el-button>
          <el-button size="small" type="danger" plain @click="remove(task)">{{ t('common.delete') }}</el-button>
        </div>
      </div>
      <EmptyState
        v-if="!displayTasks.length && !loading"
        icon="🚀"
        :title="focusSiteId ? t('brushflow.emptySiteTitle') : t('brushflow.emptyTitle')"
        :description="focusSiteId ? t('brushflow.emptySiteDesc') : t('brushflow.emptyDesc')"
      >
        <el-button type="primary" @click="openCreate">{{ t('brushflow.newRule') }}</el-button>
      </EmptyState>
    </div>

    <BrushFlowFormDialog
      v-model:visible="showForm"
      :form="form"
      :editing="!!editing"
      :sites="sites"
      :downloaders="downloaders"
      :filter-groups="filterGroups"
      :filter-group-array="filterGroupArray"
      @save="save"
      @update:filter-group-array="(v: any) => (filterGroupArray = v)"
    />

    <el-dialog v-model="showSeeds" :title="t('brushflow.seedsTitle', { name: seedsTask })" width="640px" top="8vh">
      <div v-loading="seedsLoading">
        <div v-if="seedWindow.truncated" class="seed-note">
          {{
            seedWindow.total == null
              ? t('brushflow.seedsTruncatedUnknown', { n: seedWindow.shown })
              : t('brushflow.seedsTruncated', { n: seedWindow.shown, total: seedWindow.total })
          }}
        </div>
        <el-table :data="seeds" size="small" max-height="420" :empty-text="t('brushflow.noRecords')">
          <el-table-column prop="title" :label="t('rss.col.title')" show-overflow-tooltip />
          <el-table-column :label="t('rss.col.size')" width="90">
            <template #default="{ row }">{{ fmtBytes(row.size) }}</template>
          </el-table-column>
          <el-table-column :label="t('brushflow.col.addedAt')" width="160">
            <template #default="{ row }">{{ new Date(row.added_at).toLocaleString() }}</template>
          </el-table-column>
        </el-table>
      </div>
    </el-dialog>
  </div>
</template>

<style scoped src="./BrushFlow.css"></style>
