<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { brushflowApi, type BrushFlowTask, type BrushFlowSeed } from '@/api/brushflow'
import { siteApi } from '@/api/site'
import { downloaderApi } from '@/api/downloader'
import { filterApi } from '@/api/filter'
import GlassIcon from '@/components/GlassIcon.vue'
import EmptyState from '@/components/EmptyState.vue'
import { ElMessageBox, ElMessage } from 'element-plus'

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
    ElMessage.error(e?.userMessage || e?.message || '加载失败')
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
    ElMessage.warning('名称、站点、下载器必填')
    return
  }
  try {
    if (editing.value) {
      await brushflowApi.update(editing.value.id, form.value)
      ElMessage.success('已更新')
    } else {
      await brushflowApi.create(form.value)
      ElMessage.success('已创建')
    }
    showForm.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(e?.userMessage || e?.message || '保存失败')
  }
}

async function remove(t: BrushFlowTask) {
  await ElMessageBox.confirm(`确定删除刷流任务「${t.name}」？`, '提示', { type: 'warning' })
  await brushflowApi.remove(t.id)
  ElMessage.success('已删除')
  await load()
}

async function toggle(t: BrushFlowTask, val: boolean) {
  await brushflowApi.update(t.id, { ...t, enabled: val })
  t.enabled = val
}

async function runAction(t: BrushFlowTask, kind: 'select' | 'sweep') {
  try {
    const fn = kind === 'select' ? brushflowApi.select : brushflowApi.sweep
    await fn(t.id)
    ElMessage.success(kind === 'select' ? '选种完成' : '扫删完成')
    await load()
  } catch (e: any) {
    ElMessage.error(e?.userMessage || e?.message || '执行失败')
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
      <h2 class="title">刷流</h2>
      <el-button type="primary" @click="openCreate">+ 新建任务</el-button>
    </div>
    <p class="sub">按规则自动选种做种，达到删除规则（分享率 / 保种时长 / 上传量）后自动删种，循环刷上传量与分享率。</p>

    <el-alert
      v-if="focusSiteId"
      type="info"
      :closable="false"
      show-icon
      style="margin-bottom: 12px"
      :title="`仅显示站点「${focusSiteName}」的刷流任务（${displayTasks.length}）`"
    />

    <div v-loading="loading" class="grid">
      <div v-for="t in displayTasks" :key="t.id" class="glass glass-hover card">
        <div class="card-head">
          <GlassIcon name="brushflow" :size="34" from="#f7971e" to="#ff6a00" />
          <div class="card-title">
            <div class="name">{{ t.name }}</div>
            <div class="meta">{{ siteName(t.site_id) }} → {{ dlName(t.downloader_id) }}</div>
          </div>
          <el-switch :model-value="t.enabled" @update:model-value="(v: any) => toggle(t, !!v)" />
        </div>

        <div class="stats">
          <div class="stat"><span class="sv">{{ t.total_added }}</span><span class="sl">加入</span></div>
          <div class="stat"><span class="sv">{{ t.total_deleted }}</span><span class="sl">删除</span></div>
          <div class="stat"><span class="sv">{{ fmtBytes(t.total_upload) }}</span><span class="sl">上传</span></div>
        </div>

        <div v-if="t.last_error" class="err">⚠ {{ t.last_error }}</div>

        <div class="actions">
          <el-button size="small" @click="runAction(t, 'select')">选种</el-button>
          <el-button size="small" @click="runAction(t, 'sweep')">扫删</el-button>
          <el-button size="small" @click="openSeeds(t)">历史</el-button>
          <el-button size="small" @click="openEdit(t)">编辑</el-button>
          <el-button size="small" type="danger" plain @click="remove(t)">删除</el-button>
        </div>
      </div>
      <EmptyState
        v-if="!displayTasks.length && !loading"
        icon="🚀"
        :title="focusSiteId ? '该站点暂无刷流任务' : '暂无刷流任务'"
        :description="focusSiteId ? '可为该站新建刷流任务' : '新建任务开始自动刷上传量与分享率'"
      >
        <el-button type="primary" @click="openCreate">新建任务</el-button>
      </EmptyState>
    </div>

    <el-dialog v-model="showForm" :title="editing ? '编辑任务' : '新建任务'" width="640px" top="6vh">
      <el-form :model="form" label-width="110px" label-position="right">
        <el-form-item label="名称"><el-input v-model="form.name" placeholder="如：HDHome 刷流" /></el-form-item>
        <el-form-item label="站点">
          <el-select v-model="form.site_id">
            <el-option v-for="s in sites" :key="s.id" :label="s.name" :value="s.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="下载器">
          <el-select v-model="form.downloader_id">
            <el-option v-for="d in downloaders" :key="d.id" :label="d.name" :value="d.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="启用"><el-switch v-model="form.enabled" /></el-form-item>
        <el-form-item label="关键字"><el-input v-model="form.keyword" placeholder="留空=综合" /></el-form-item>
        <el-form-item label="分类">
          <el-select v-model="form.category" clearable>
            <el-option label="综合" value="" />
            <el-option label="电影" value="movie" />
            <el-option label="电视剧" value="tv" />
            <el-option label="音乐" value="music" />
          </el-select>
        </el-form-item>
        <el-form-item label="大小(MB)">
          <div class="range">
            <el-input-number v-model="form.min_size_mb" :min="0" controls-position="right" />
            <span>~</span>
            <el-input-number v-model="form.max_size_mb" :min="0" controls-position="right" />
            <span class="hint">0=不限</span>
          </div>
        </el-form-item>
        <el-form-item label="做种人数">
          <div class="range">
            <el-input-number v-model="form.min_seeders" :min="0" controls-position="right" />
            <span>~</span>
            <el-input-number v-model="form.max_seeders" :min="0" controls-position="right" />
          </div>
        </el-form-item>
        <el-form-item label="仅免费"><el-switch v-model="form.free_only" /></el-form-item>
        <el-form-item label="过滤规则组">
          <el-select v-model="filterGroupArray" multiple collapse-tags collapse-tags-tooltip placeholder="留空=不过滤" style="width: 100%">
            <el-option v-for="g in filterGroups" :key="g" :label="g" :value="g" />
          </el-select>
        </el-form-item>
        <el-divider content-position="left">删除规则（全 0 / 空表达式 = 永不因规则删）</el-divider>
        <el-form-item label="组合逻辑">
          <el-radio-group v-model="form.delete_logic">
            <el-radio-button label="or">任一命中</el-radio-button>
            <el-radio-button label="and">全部命中</el-radio-button>
          </el-radio-group>
          <span class="hint">有「阶梯表达式」时以表达式为准</span>
        </el-form-item>
        <el-form-item label="分享率 ≥"><el-input-number v-model="form.delete_ratio" :min="0" :step="0.1" controls-position="right" /></el-form-item>
        <el-form-item label="保种(小时) ≥"><el-input-number v-model="form.delete_seed_hours" :min="0" :step="0.5" controls-position="right" /></el-form-item>
        <el-form-item label="上传(MB) ≥"><el-input-number v-model="form.delete_upload_mb" :min="0" controls-position="right" /></el-form-item>
        <el-form-item label="最低上传(KB/s)">
          <el-input-number v-model="form.delete_min_up_rate_kb" :min="0" controls-position="right" />
          <span class="hint">仅对已完成做种；上传速度低于此值则删</span>
        </el-form-item>
        <el-form-item label="不活跃(小时)">
          <el-input-number v-model="form.delete_inactive_hours" :min="0" :step="0.5" controls-position="right" />
          <span class="hint">最后活跃超过此时长则删</span>
        </el-form-item>
        <el-form-item label="阶梯表达式">
          <el-input v-model="form.delete_rule_expr" type="textarea" :rows="2" placeholder="例：swarm_seeders>50 || (ratio>=1 && seed_hours>=24)" />
          <span class="hint">变量：ratio / seed_hours / upload_mb / up_rate_kb / inactive_hours；支持 &amp;&amp; || () &gt;= &lt;= &gt; &lt; ==</span>
        </el-form-item>
        <el-form-item label="总体积上限(GB)">
          <el-input-number v-model="form.max_total_size_gb" :min="0" :step="1" controls-position="right" />
          <span class="hint">超限按最慢上传淘汰；0=不限</span>
        </el-form-item>
        <el-form-item label="swarm做种上限">
          <el-input-number v-model="form.delete_max_swarm_seeders" :min="0" controls-position="right" />
          <span class="hint">NumComplete 超过则删，0=不限</span>
        </el-form-item>
        <el-form-item label="swarm下载下限">
          <el-input-number v-model="form.delete_min_swarm_leechers" :min="0" controls-position="right" />
          <span class="hint">做种中 NumIncomplete 低于则删，0=不限</span>
        </el-form-item>
        <el-form-item label="删种删文件"><el-switch v-model="form.delete_files" /></el-form-item>
        <el-form-item label="原生停种">
          <el-switch v-model="form.use_share_limit" />
          <span class="hint">开启：下载器达标即暂停（不必等扫删），扫删仍会清理删种</span>
        </el-form-item>
        <el-divider content-position="left">并发与路径</el-divider>
        <el-form-item label="最大做种数"><el-input-number v-model="form.max_seeds" :min="1" controls-position="right" /></el-form-item>
        <el-form-item label="保存路径"><el-input v-model="form.save_path" placeholder="留空=下载器默认" /></el-form-item>
        <el-form-item label="下载器分类"><el-input v-model="form.category_dl" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showForm = false">取消</el-button>
        <el-button type="primary" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="showSeeds" :title="`种子历史 · ${seedsTask}`" width="640px" top="8vh">
      <div v-loading="seedsLoading">
        <el-table :data="seeds" size="small" max-height="420" empty-text="暂无记录">
          <el-table-column prop="title" label="标题" show-overflow-tooltip />
          <el-table-column label="大小" width="90">
            <template #default="{ row }">{{ fmtBytes(row.size) }}</template>
          </el-table-column>
          <el-table-column label="加入时间" width="160">
            <template #default="{ row }">{{ new Date(row.added_at).toLocaleString() }}</template>
          </el-table-column>
        </el-table>
      </div>
    </el-dialog>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: 14px; }
.head { display: flex; align-items: center; justify-content: space-between; }
.title { margin: 0; font-size: 22px; font-weight: 700; }
.sub { margin: -8px 0 0; font-size: var(--fs-sm); color: var(--text-muted); }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: 14px; }
.card { padding: 16px; display: flex; flex-direction: column; gap: 12px; }
.card-head { display: flex; align-items: center; gap: 12px; }
.card-title { flex: 1; min-width: 0; }
.name { font-weight: 600; font-size: var(--fs-body); }
.meta { font-size: var(--fs-xs); color: var(--text-muted); }
.stats { display: flex; gap: 8px; }
.stat { flex: 1; background: rgba(127, 127, 127, 0.12); border-radius: 10px; padding: 8px; text-align: center; }
.sv { display: block; font-weight: 700; font-size: var(--fs-body); }
.sl { font-size: var(--fs-xs); color: var(--text-muted); }
.err { font-size: var(--fs-xs); color: #ef4444; }
.actions { display: flex; flex-wrap: wrap; gap: 6px; }
.range { display: flex; align-items: center; gap: 8px; }
.hint { font-size: var(--fs-xs); color: var(--text-muted); }
</style>
