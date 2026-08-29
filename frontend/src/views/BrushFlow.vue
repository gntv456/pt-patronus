<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
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
    const [ts, ss, ds] = await Promise.all([brushflowApi.list(), siteApi.list(), downloaderApi.list()])
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
    name: '', site_id: 0, downloader_id: 0, enabled: true,
    keyword: '', category: '', min_size_mb: 0, max_size_mb: 0,
    min_seeders: 0, max_seeders: 0, free_only: false, paused: false,
    delete_ratio: 0, delete_seed_hours: 0, delete_upload_mb: 0, delete_files: false, use_share_limit: false, delete_min_up_rate_kb: 0, delete_inactive_hours: 0, delete_logic: 'or', delete_rule_expr: '', max_total_size_gb: 0, delete_max_swarm_seeders: 0, delete_min_swarm_leechers: 0,
    max_seeds: 10, save_path: '', category_dl: '', filter_groups: '',
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
  get: () => (form.value.filter_groups || '').split('|').map((s) => s.trim()).filter(Boolean),
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
  await ElMessageBox.confirm(t('brushflow.confirmDelete', { name: task.name }), t('messages.tip'), { type: 'warning' })
  await brushflowApi.remove(task.id)
  ElMessage.success(t('common.deleted'))
  await load()
}

async function toggle(t: BrushFlowTask, val: boolean) {
  await brushflowApi.update(t.id, { ...t, enabled: val })
  t.enabled = val
}

async function runAction(task: BrushFlowTask, kind: 'select' | 'sweep') {
  try {
    const fn = kind === 'select' ? brushflowApi.select : brushflowApi.sweep
    await fn(task.id)
    ElMessage.success(kind === 'select' ? t('brushflow.selectDone') : t('brushflow.sweepDone'))
    await load()
  } catch (e: any) {
    ElMessage.error(e?.userMessage || e?.message || t('common.actionFailed'))
  }
}

const showSeeds = ref(false)
const seeds = ref<BrushFlowSeed[]>([])
const seedsLoading = ref(false)
const seedsTask = ref('')

async function openSeeds(t: BrushFlowTask) {
  seedsTask.value = t.name
  showSeeds.value = true
  seedsLoading.value = true
  try {
    seeds.value = await brushflowApi.seeds(t.id)
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
  filterApi.groups().then((gs) => {
    filterGroups.value = gs.map((g) => g.name)
  }).catch(() => [])
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

    <div v-loading="loading" class="grid">
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
          <div class="stat"><span class="sv">{{ task.total_added }}</span><span class="sl">{{ t('brushflow.statAdded') }}</span></div>
          <div class="stat"><span class="sv">{{ task.total_deleted }}</span><span class="sl">{{ t('common.delete') }}</span></div>
          <div class="stat"><span class="sv">{{ fmtBytes(task.total_upload) }}</span><span class="sl">{{ t('contract.uploadShort') }}</span></div>
        </div>

        <div v-if="task.last_error" class="err">⚠ {{ task.last_error }}</div>

        <div class="actions">
          <el-button size="small" @click="runAction(task, 'select')">{{ t('brushflow.select') }}</el-button>
          <el-button size="small" @click="runAction(task, 'sweep')">{{ t('brushflow.sweep') }}</el-button>
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

    <el-dialog v-model="showForm" :title="editing ? t('brushflow.editTitle') : t('brushflow.newTitle')" width="640px" top="6vh">
      <el-form :model="form" label-width="110px" label-position="right">
        <el-form-item :label="t('rss.f.name')"><el-input v-model="form.name" :placeholder="t('brushflow.f.nameHint')" /></el-form-item>
        <el-form-item :label="t('contract.f.site')">
          <el-select v-model="form.site_id">
            <el-option v-for="s in sites" :key="s.id" :label="s.name" :value="s.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('downloaders.title')">
          <el-select v-model="form.downloader_id">
            <el-option v-for="d in downloaders" :key="d.id" :label="d.name" :value="d.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('rss.f.enabled')"><el-switch v-model="form.enabled" /></el-form-item>
        <el-form-item :label="t('brushflow.f.keyword')"><el-input v-model="form.keyword" :placeholder="t('brushflow.f.kwHint')" /></el-form-item>
        <el-form-item :label="t('rss.f.category')">
          <el-select v-model="form.category" clearable>
            <el-option :label="t('brushflow.cat.all')" value="" />
            <el-option :label="t('search.catMovie')" value="movie" />
            <el-option :label="t('search.catTv')" value="tv" />
            <el-option :label="t('search.catMusic')" value="music" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('brushflow.f.sizeMB')">
          <div class="range">
            <el-input-number v-model="form.min_size_mb" :min="0" controls-position="right" />
            <span>~</span>
            <el-input-number v-model="form.max_size_mb" :min="0" controls-position="right" />
            <span class="hint">{{ t('brushflow.f.zeroAny') }}</span>
          </div>
        </el-form-item>
        <el-form-item :label="t('brushflow.f.seeders')">
          <div class="range">
            <el-input-number v-model="form.min_seeders" :min="0" controls-position="right" />
            <span>~</span>
            <el-input-number v-model="form.max_seeders" :min="0" controls-position="right" />
          </div>
        </el-form-item>
        <el-form-item :label="t('brushflow.f.freeOnly')"><el-switch v-model="form.free_only" /></el-form-item>
        <el-form-item :label="t('rss.f.filterGroups')">
          <el-select v-model="filterGroupArray" multiple collapse-tags collapse-tags-tooltip :placeholder="t('rss.f.filterGroupsHint')" style="width: 100%">
            <el-option v-for="g in filterGroups" :key="g" :label="g" :value="g" />
          </el-select>
        </el-form-item>
        <el-divider content-position="left">{{ t('brushflow.f.delRules') }}</el-divider>
        <el-form-item :label="t('brushflow.f.logic')">
          <el-radio-group v-model="form.delete_logic">
            <el-radio-button label="or">{{ t('brushflow.f.logicOr') }}</el-radio-button>
            <el-radio-button label="and">{{ t('brushflow.f.logicAnd') }}</el-radio-button>
          </el-radio-group>
          <span class="hint">{{ t('brushflow.f.exprWins') }}</span>
        </el-form-item>
        <el-form-item :label="t('brushflow.f.delRatio')"><el-input-number v-model="form.delete_ratio" :min="0" :step="0.1" controls-position="right" /></el-form-item>
        <el-form-item :label="t('brushflow.f.delSeedHours')"><el-input-number v-model="form.delete_seed_hours" :min="0" :step="0.5" controls-position="right" /></el-form-item>
        <el-form-item :label="t('brushflow.f.delUpload')"><el-input-number v-model="form.delete_upload_mb" :min="0" controls-position="right" /></el-form-item>
        <el-form-item :label="t('brushflow.f.delMinRate')">
          <el-input-number v-model="form.delete_min_up_rate_kb" :min="0" controls-position="right" />
          <span class="hint">{{ t('brushflow.f.delMinRateHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('brushflow.f.delInactive')">
          <el-input-number v-model="form.delete_inactive_hours" :min="0" :step="0.5" controls-position="right" />
          <span class="hint">{{ t('brushflow.f.delInactiveHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('brushflow.f.expr')">
          <el-input v-model="form.delete_rule_expr" type="textarea" :rows="2" :placeholder="t('brushflow.f.exprPlaceholder')" />
          <span class="hint">{{ t('brushflow.f.exprHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('brushflow.f.maxTotalSize')">
          <el-input-number v-model="form.max_total_size_gb" :min="0" :step="1" controls-position="right" />
          <span class="hint">{{ t('brushflow.f.maxTotalSizeHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('brushflow.f.maxSwarmSeeders')">
          <el-input-number v-model="form.delete_max_swarm_seeders" :min="0" controls-position="right" />
          <span class="hint">{{ t('brushflow.f.maxSwarmSeedersHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('brushflow.f.minSwarmLeechers')">
          <el-input-number v-model="form.delete_min_swarm_leechers" :min="0" controls-position="right" />
          <span class="hint">{{ t('brushflow.f.minSwarmLeechersHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('brushflow.f.delFiles')"><el-switch v-model="form.delete_files" /></el-form-item>
        <el-form-item :label="t('brushflow.f.nativeStop')">
          <el-switch v-model="form.use_share_limit" />
          <span class="hint">{{ t('brushflow.f.nativeStopHint') }}</span>
        </el-form-item>
        <el-divider content-position="left">{{ t('brushflow.f.concurrency') }}</el-divider>
        <el-form-item :label="t('brushflow.f.maxSeeds')"><el-input-number v-model="form.max_seeds" :min="1" controls-position="right" /></el-form-item>
        <el-form-item :label="t('rss.f.savePath')"><el-input v-model="form.save_path" :placeholder="t('brushflow.f.savePathHint')" /></el-form-item>
        <el-form-item :label="t('brushflow.f.dlCategory')"><el-input v-model="form.category_dl" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showForm = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="save">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="showSeeds" :title="t('brushflow.seedsTitle', { name: seedsTask })" width="640px" top="8vh">
      <div v-loading="seedsLoading">
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

<style scoped>
.page { display: flex; flex-direction: column; gap: var(--sp-3); }
.head { display: flex; align-items: center; justify-content: space-between; }
.title { margin: 0; font-size: var(--fs-title); font-weight: 700; }
.sub { margin: -8px 0 0; font-size: var(--fs-sm); color: var(--text-muted); }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: var(--sp-3); }
.card { padding: 16px; display: flex; flex-direction: column; gap: var(--sp-3); }
.card-head { display: flex; align-items: center; gap: var(--sp-3); }
.card-title { flex: 1; min-width: 0; }
.name { font-weight: 600; font-size: var(--fs-body); }
.meta { font-size: var(--fs-xs); color: var(--text-muted); }
.stats { display: flex; gap: var(--sp-2); }
.stat { flex: 1; background: var(--surface-muted); border-radius: var(--radius-sm); padding: 8px; text-align: center; }
.sv { display: block; font-weight: 700; font-size: var(--fs-body); }
.sl { font-size: var(--fs-xs); color: var(--text-muted); }
.err { font-size: var(--fs-xs); color: var(--pt-danger); }
.actions { display: flex; flex-wrap: wrap; gap: var(--sp-1); }
.range { display: flex; align-items: center; gap: var(--sp-2); }
.hint { font-size: var(--fs-xs); color: var(--text-muted); }

/* 刷流卡 staggered 入场 + hover */
.grid > * { animation: pt-fade-in-up var(--dur-slow) var(--ease-out-smooth) both; }
.grid > *:nth-child(1) { animation-delay: 0ms; }
.grid > *:nth-child(2) { animation-delay: 50ms; }
.grid > *:nth-child(3) { animation-delay: 100ms; }
.grid > *:nth-child(4) { animation-delay: 150ms; }
.grid > *:nth-child(5) { animation-delay: 200ms; }
.grid > *:nth-child(n+6) { animation-delay: 250ms; }
.card { transition: transform var(--dur-base) var(--ease-out-spring), box-shadow var(--dur-base) var(--ease-out-smooth); }
.card:hover { transform: translateY(-2px); box-shadow: var(--shadow-md); }
</style>
