<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { contractApi, type ContractRule, type ContractCheckResult } from '@/api/contract'
import { siteApi } from '@/api/site'
import GlassIcon from '@/components/GlassIcon.vue'
import EmptyState from '@/components/EmptyState.vue'
import { ElMessageBox, ElMessage } from 'element-plus'

const rules = ref<ContractRule[]>([])
const sites = ref<{ id: number; name: string }[]>([])
const loading = ref(false)
const results = ref<Record<number, ContractCheckResult>>({})

async function load() {
  loading.value = true
  try {
    const [rs, ss] = await Promise.all([contractApi.list(), siteApi.list()])
    rules.value = rs
    sites.value = ss.map((s) => ({ id: s.id, name: s.name }))
  } catch (e: any) {
    ElMessage.error(e?.userMessage || e?.message || '加载失败')
  } finally {
    loading.value = false
  }
}

const showForm = ref(false)
const editing = ref<ContractRule | null>(null)
const form = ref<Partial<ContractRule>>(defaultForm())

function defaultForm(): Partial<ContractRule> {
  return {
    site_id: 0, enabled: true, min_ratio: 0, min_seeding: 0, min_upload_gb: 0,
    official_only: false, min_seeding_size_gb: 0, period: '',
  }
}

function openCreate() {
  editing.value = null
  form.value = defaultForm()
  if (sites.value.length) form.value.site_id = sites.value[0].id
  showForm.value = true
}

function openEdit(r: ContractRule) {
  editing.value = r
  form.value = { ...r }
  showForm.value = true
}

async function save() {
  if (!form.value.site_id) {
    ElMessage.warning('请选择站点')
    return
  }
  try {
    await contractApi.upsert(form.value)
    ElMessage.success('已保存')
    showForm.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(e?.userMessage || e?.message || '保存失败')
  }
}

async function remove(r: ContractRule) {
  await ElMessageBox.confirm(`删除「${siteName(r.site_id)}」的保种契约？`, '提示', { type: 'warning' })
  await contractApi.remove(r.id)
  ElMessage.success('已删除')
  await load()
}

async function toggle(r: ContractRule, val: boolean) {
  await contractApi.upsert({ ...r, enabled: val })
  r.enabled = val
}

async function check(r: ContractRule) {
  try {
    const res = await contractApi.check(r.id)
    results.value[r.id] = res
    await load()
  } catch (e: any) {
    ElMessage.error(e?.userMessage || e?.message || '检查失败')
  }
}

function siteName(id: number) {
  return sites.value.find((s) => s.id === id)?.name || `#${id}`
}
function statusType(s: string): 'success' | 'warning' | 'danger' | 'info' {
  return ({ ok: 'success', breach: 'warning', error: 'danger' } as const)[s as 'ok'] || 'info'
}
function statusLabel(s: string) {
  return ({ ok: '达标', breach: '未达标', error: '出错' } as const)[s as 'ok'] || '未检查'
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="head">
      <h2 class="title">保种契约</h2>
      <el-button type="primary" @click="openCreate">+ 新建契约</el-button>
    </div>
    <p class="sub">为站点设分享率 / 做种数 / 上传量下限，每日 9 点自动检查，低于下限告警通知（防分享率过低被封）。</p>

    <div v-loading="loading" class="grid">
      <div v-for="r in rules" :key="r.id" class="glass glass-hover card">
        <div class="card-head">
          <GlassIcon name="contract" :size="34" from="#6c5ce7" to="#4285f4" />
          <div class="card-title">
            <div class="name">{{ siteName(r.site_id) }}</div>
            <div class="meta">
              分享率≥{{ r.min_ratio || '-' }} · 做种≥{{ r.min_seeding || '-' }} · 上传≥{{ r.min_upload_gb || '-' }}GiB<template
                v-if="r.official_only"> · 仅官种</template><template v-if="r.min_seeding_size_gb"> · 体积≥{{
                r.min_seeding_size_gb }}GiB</template><template v-if="r.period"> · {{ r.period === 'weekly' ? '每周' : '每月' }}</template>
            </div>
          </div>
          <el-switch :model-value="r.enabled" @update:model-value="(v: any) => toggle(r, !!v)" />
        </div>
        <div class="status-row">
          <el-tag :type="statusType(r.last_status)" size="small">{{ statusLabel(r.last_status) }}</el-tag>
          <span v-if="r.last_detail" class="detail">{{ r.last_detail }}</span>
          <span v-if="r.last_checked_at" class="time">{{ new Date(r.last_checked_at).toLocaleString() }}</span>
        </div>
        <div v-if="results[r.id]" class="check-result">
          实时：分享率 {{ results[r.id].ratio.toFixed(2) }} · 做种 {{ results[r.id].seeding }}<template
            v-if="results[r.id].seeding_official">（官种 {{ results[r.id].seeding_official }}）</template> · 上传
          {{ results[r.id].upload_gb.toFixed(1) }}GiB · 体积 {{ results[r.id].seeding_size_gb.toFixed(1) }}GiB<template
            v-if="results[r.id].unsupported"> · ⚠ {{ results[r.id].unsupported }}</template>
        </div>
        <div class="actions">
          <el-button size="small" @click="check(r)">检查</el-button>
          <el-button size="small" @click="openEdit(r)">编辑</el-button>
          <el-button size="small" type="danger" plain @click="remove(r)">删除</el-button>
        </div>
      </div>
      <EmptyState
        v-if="!rules.length && !loading"
        icon="🔖"
        title="暂无契约"
        description="新建契约，为站点设保种下限"
      >
        <el-button type="primary" @click="openCreate">新建契约</el-button>
      </EmptyState>
    </div>

    <el-dialog v-model="showForm" :title="editing ? '编辑契约' : '新建契约'" width="480px">
      <el-form :model="form" label-width="110px">
        <el-form-item label="站点">
          <el-select v-model="form.site_id" :disabled="!!editing">
            <el-option v-for="s in sites" :key="s.id" :label="s.name" :value="s.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="启用"><el-switch v-model="form.enabled" /></el-form-item>
        <el-form-item label="分享率 ≥">
          <el-input-number v-model="form.min_ratio" :min="0" :step="0.1" controls-position="right" />
        </el-form-item>
        <el-form-item label="做种数 ≥"><el-input-number v-model="form.min_seeding" :min="0" controls-position="right" /></el-form-item>
        <el-form-item label="上传(GiB) ≥"><el-input-number v-model="form.min_upload_gb" :min="0" controls-position="right" /></el-form-item>
        <el-divider content-position="left"><span class="text-muted">扩展维度（官种 / 体积 / 周期 / 生效时间）</span></el-divider>
        <el-form-item label="仅官种">
          <el-switch v-model="form.official_only" />
          <span class="hint text-muted" style="margin-left: 8px">开启后做种数/体积只统计官方种子</span>
        </el-form-item>
        <el-form-item label="做种体积(GiB) ≥">
          <el-input-number v-model="form.min_seeding_size_gb" :min="0" controls-position="right" />
        </el-form-item>
        <el-form-item label="考核周期">
          <el-select v-model="form.period" style="width: 130px">
            <el-option label="每日" value="" />
            <el-option label="每周" value="weekly" />
            <el-option label="每月" value="monthly" />
          </el-select>
          <span class="hint text-muted" style="margin-left: 8px">告警按周期去重</span>
        </el-form-item>
        <el-form-item label="生效时间">
          <el-date-picker
            v-model="form.start_time"
            type="datetime"
            placeholder="留空=立即生效"
            value-format="YYYY-MM-DDTHH:mm:ssZ"
            style="width: 220px"
          />
        </el-form-item>
        <p class="hint text-muted">设 0 表示该项不检查。</p>
      </el-form>
      <template #footer>
        <el-button @click="showForm = false">取消</el-button>
        <el-button type="primary" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: 14px; }
.head { display: flex; align-items: center; justify-content: space-between; }
.title { margin: 0; font-size: 22px; font-weight: 700; }
.sub { margin: -8px 0 0; font-size: var(--fs-sm); color: var(--text-muted); }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: 14px; }
.card { padding: 16px; display: flex; flex-direction: column; gap: 10px; }
.card-head { display: flex; align-items: center; gap: 12px; }
.card-title { flex: 1; min-width: 0; }
.name { font-weight: 600; font-size: var(--fs-body); }
.meta { font-size: var(--fs-xs); color: var(--text-muted); }
.status-row { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; font-size: var(--fs-xs); }
.detail { color: #ef4444; }
.time { color: var(--text-muted); margin-left: auto; }
.check-result { font-size: var(--fs-xs); color: var(--text-muted); background: rgba(127, 127, 127, 0.12); border-radius: 8px; padding: 6px 8px; }
.actions { display: flex; gap: 6px; }
.hint { margin: 0; }
</style>
