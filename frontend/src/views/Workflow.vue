<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { workflowApi, type Workflow, type WorkflowStats, type WorkflowTemplate } from '@/api/workflow'
import { webhookApi, type Webhook } from '@/api/webhook'
import { copyText } from '@/utils/clipboard'
import { useAuthStore } from '@/stores/auth'
import EmptyState from '@/components/EmptyState.vue'

// Webhook 创建/编辑/删除/重置是 admin 专属（后端 RequireAdmin）；非 admin 隐藏入口。
const auth = useAuthStore()

const { t } = useI18n()
const router = useRouter()
const list = ref<Workflow[]>([])
const loading = ref(false)
const running = ref<number | null>(null)
const stats = ref<WorkflowStats | null>(null)
const templates = ref<WorkflowTemplate[]>([])
const creatingTemplate = ref('')

async function load() {
  loading.value = true
  try {
    const [l, s, t] = await Promise.all([
      workflowApi.list(),
      workflowApi.stats().catch(() => null),
      workflowApi.templates().catch(() => []),
    ])
    list.value = l
    stats.value = s
    templates.value = t
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('common.loadFailed'))
  } finally {
    loading.value = false
  }
}
onMounted(load)

function edit(w: Workflow) {
  router.push({ name: 'workflow-editor', query: { id: w.id } })
}
function create() {
  router.push({ name: 'workflow-editor' })
}

async function createFromTemplate(tpl: WorkflowTemplate) {
  creatingTemplate.value = tpl.key
  try {
    const wf = await workflowApi.create({
      name: tpl.name,
      description: tpl.description,
      trigger_type: tpl.trigger_type,
      timer: tpl.timer,
	  event_type: tpl.event_type,
	  event_conditions: tpl.event_conditions,
      enabled: false,
      nodes: tpl.nodes,
      edges: tpl.edges,
    })
    ElMessage.success(t('workflow.createdFromTemplate'))
    router.push({ name: 'workflow-editor', query: { id: wf.id } })
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('workflow.createFailed'))
  } finally {
    creatingTemplate.value = ''
  }
}

async function run(w: Workflow) {
  running.value = w.id
  try {
    const r = await workflowApi.run(w.id)
    if (r.ok) ElMessage.success(t('workflow.runDone'))
    else ElMessage.warning(r.error || t('workflow.runFailed'))
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('workflow.runFailed'))
  } finally {
    running.value = null
  }
}

async function toggle(w: Workflow) {
  try {
    if (w.enabled) await workflowApi.disable(w.id)
    else await workflowApi.enable(w.id)
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('common.actionFailed'))
  }
}

async function remove(w: Workflow) {
  try {
    await ElMessageBox.confirm(t('workflow.confirmDelete', { name: w.name }), t('common.confirm'), { type: 'warning' })
  } catch {
    return
  }
  try {
    await workflowApi.remove(w.id)
    await load()
    ElMessage.success(t('common.deleted'))
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('common.deleteFailed'))
  }
}

const triggerText = computed<Record<string, string>>(() => ({ manual: t('workflow.trigger.manual'), timer: t('workflow.trigger.timer'), event: t('workflow.trigger.event') }))
const stateText = computed<Record<string, string>>(() => ({ idle: t('workflow.state.idle'), running: t('workflow.state.running'), success: t('workflow.state.success'), failed: t('workflow.state.failed') }))
const stateClass: Record<string, string> = {
  idle: 'st-idle',
  running: 'st-run',
  success: 'st-ok',
  failed: 'st-err',
}
function fmtTime(t?: string) {
  return t ? new Date(t).toLocaleString('zh-CN', { hour12: false }).replace(/\//g, '-') : '—'
}

// —— 进站 Webhook 管理 ——
const showWebhooks = ref(false)
const webhooks = ref<Webhook[]>([])
async function openWebhooks() {
  try {
    webhooks.value = await webhookApi.list()
    showWebhooks.value = true
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('common.loadFailed'))
  }
}
function webhookUrl(w: Webhook) {
  return `${window.location.origin}/api/v1/webhooks/in/${w.token}`
}
async function copyUrl(w: Webhook) {
  await copyText(webhookUrl(w), t('workflow.urlCopied'), webhookUrl(w))
}
async function whDelete(w: Webhook) {
  try {
    await ElMessageBox.confirm(t('workflow.confirmDeleteWh', { name: w.name }), t('common.confirm'), { type: 'warning' })
  } catch {
    return
  }
  try {
    await webhookApi.remove(w.id)
    webhooks.value = await webhookApi.list()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('common.actionFailed'))
  }
}
async function whRotate(w: Webhook) {
  try {
    await ElMessageBox.confirm(t('workflow.confirmRotate'), t('common.confirm'), { type: 'warning' })
  } catch {
    return
  }
  try {
    await webhookApi.rotate(w.id)
    webhooks.value = await webhookApi.list()
    ElMessage.success(t('workflow.rotated'))
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('common.actionFailed'))
  }
}

// webhook 新建表单
const showWhForm = ref(false)
const whForm = ref({ name: '', target_workflow_id: 0, secret: '' })
function openWhForm() {
  whForm.value = { name: '', target_workflow_id: 0, secret: '' }
  showWhForm.value = true
}
async function whSave() {
  if (!whForm.value.name) {
    ElMessage.warning(t('workflow.nameRequired'))
    return
  }
  try {
    await webhookApi.create(whForm.value)
    showWhForm.value = false
    webhooks.value = await webhookApi.list()
    ElMessage.success(t('common.created'))
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('common.actionFailed'))
  }
}
</script>

<template>
  <div class="page">
    <div class="head">
      <div>
        <h2 class="title">{{ t('workflow.title') }}</h2>
        <span class="text-muted hint">{{ t('workflow.subtitle') }}</span>
      </div>
      <el-button type="primary" @click="create">{{ t('workflow.create') }}</el-button>
      <el-button v-if="auth.isAdmin" @click="openWebhooks">{{ t('workflow.webhookEntry') }}</el-button>
    </div>

    <!-- 统计看板 -->
    <div v-if="stats" class="stats">
      <div class="stat glass-hover"><div class="s-num">{{ stats.total }}</div><div class="s-lab">{{ t('workflow.title') }}</div></div>
      <div class="stat glass-hover"><div class="s-num">{{ stats.enabled }}</div><div class="s-lab">{{ t('workflow.enabled') }}</div></div>
      <div class="stat glass-hover"><div class="s-num">{{ stats.runs_total }}</div><div class="s-lab">{{ t('workflow.totalRuns') }}</div></div>
      <div class="stat ok"><div class="s-num">{{ stats.runs_success }}</div><div class="s-lab">{{ t('transfer.st.success') }}</div></div>
      <div class="stat err"><div class="s-num">{{ stats.runs_failed }}</div><div class="s-lab">{{ t('transfer.st.failed') }}</div></div>
      <div class="stat glass-hover"><div class="s-num">{{ Math.round(stats.avg_duration_ms / 1000) }}s</div><div class="s-lab">{{ t('workflow.avgDuration') }}</div></div>
    </div>

    <div v-loading="loading" class="card glass">
      <div v-if="list.length" class="grid">
        <div v-for="w in list" :key="w.id" class="wcard glass-hover">
          <div class="row1">
            <span class="w-name">{{ w.name }}</span>
            <span class="chip" :class="stateClass[w.state]">{{ stateText[w.state] }}</span>
            <span class="chip trig">{{ triggerText[w.trigger_type] }}</span>
          </div>
          <div class="text-muted small">
            {{ w.description || '—' }}
          </div>
          <div class="text-muted small">
            {{ t('workflow.runCount', { n: w.run_count }) }} · {{ t('workflow.lastRun') }} {{ fmtTime(w.last_run_at) }}
          </div>
          <div v-if="w.last_result" class="result small">{{ t('workflow.lastResult') }}{{ w.last_result }}</div>
          <div class="actions">
            <el-button size="small" type="primary" plain @click="edit(w)">{{ t('common.edit') }}</el-button>
            <el-button size="small" type="primary" :loading="running === w.id" @click="run(w)">{{ t('transfer.run') }}</el-button>
            <el-button size="small" @click="toggle(w)">{{ w.enabled ? t('workflow.disable') : t('rss.f.enabled') }}</el-button>
            <el-button size="small" type="danger" plain @click="remove(w)">{{ t('common.delete') }}</el-button>
          </div>
        </div>
      </div>
      <EmptyState
        v-else-if="!loading"
        icon="🔀"
        :title="t('workflow.emptyTitle')"
        :description="t('workflow.subtitleNoDot')"
      >
        <div class="template-actions">
          <el-button type="primary" @click="create">{{ t('workflow.blankWorkflow') }}</el-button>
          <el-button
            v-for="t in templates.slice(0, 3)"
            :key="t.key"
            :loading="creatingTemplate === t.key"
            @click="createFromTemplate(t)"
          >
            {{ t.name }}
          </el-button>
        </div>
      </EmptyState>
    </div>

    <!-- 节点耗时 + 最近失败 -->
    <div v-if="stats && ((stats.node_timing && stats.node_timing.length) || (stats.recent_failures && stats.recent_failures.length))" class="monitor">
      <div v-if="stats.node_timing && stats.node_timing.length" class="card glass">
        <div class="m-title">{{ t('workflow.nodeTiming') }}</div>
        <div v-for="n in stats.node_timing.slice(0, 8)" :key="n.node_type" class="nt-row">
          <span class="nt-name">{{ n.node_name || n.node_type }}</span>
          <el-progress
            :percentage="Math.min(100, Math.round((n.total_ms / Math.max(1, stats.node_timing[0].total_ms)) * 100))"
            :show-text="false"
            :stroke-width="8"
            style="flex: 1; margin: 0 8px"
          />
          <span class="text-muted small">{{ t('workflow.ntStats', { n: n.count, ms: n.avg_ms }) }}</span>
          <span v-if="n.failed" class="small err-t">{{ t('transfer.st.failed') }} {{ n.failed }}</span>
        </div>
      </div>
      <div v-if="stats.recent_failures && stats.recent_failures.length" class="card glass">
        <div class="m-title">{{ t('workflow.recentFailures') }}</div>
        <div v-for="f in stats.recent_failures" :key="f.id" class="rf-row">
          <span class="rf-id">#{{ f.id }}</span>
          <span class="rf-res small">{{ f.result }}</span>
          <span class="text-muted small">{{ new Date(f.started_at).toLocaleString('zh-CN', { hour12: false }) }}</span>
        </div>
      </div>
    </div>

    <!-- Webhook 管理 -->
    <el-dialog v-model="showWebhooks" :title="t('workflow.webhookTitle')" width="680px">
      <div class="wh-tip text-muted small">
        {{ t('workflow.webhookTip1') }}<code>webhook</code>{{ t('workflow.webhookTip2') }}<code>X-Signature</code>{{ t('workflow.webhookTip3') }}
      </div>
      <el-button size="small" type="primary" @click="openWhForm" style="margin: 8px 0">{{ t('workflow.newWebhook') }}</el-button>
      <div v-for="w in webhooks" :key="w.id" class="wh-row">
        <div class="wh-main">
          <span class="wh-name">{{ w.name }}</span>
          <span v-if="w.has_secret" class="chip res">{{ t('workflow.signed') }}</span>
          <span class="text-muted small">{{ t('workflow.triggerCount', { n: w.trigger_count }) }} · {{ fmtTime(w.last_triggered_at) }}</span>
        </div>
        <div class="wh-url">{{ webhookUrl(w) }}</div>
        <div class="wh-actions">
          <el-button size="small" @click="copyUrl(w)">{{ t('workflow.copyUrl') }}</el-button>
          <el-button size="small" @click="whRotate(w)">{{ t('workflow.rotate') }}</el-button>
          <el-button size="small" type="danger" plain @click="whDelete(w)">{{ t('common.delete') }}</el-button>
        </div>
      </div>
      <div v-if="!webhooks.length" class="text-muted" style="padding: 20px; text-align: center">{{ t('workflow.noWebhooks') }}</div>
    </el-dialog>

    <!-- Webhook 新建 -->
    <el-dialog v-model="showWhForm" :title="t('workflow.newWebhook')" width="480px">
      <el-form label-width="110px" size="small">
        <el-form-item :label="t('rss.f.name')"><el-input v-model="whForm.name" :placeholder="t('workflow.f.nameHint')" /></el-form-item>
        <el-form-item :label="t('workflow.f.triggerWorkflow')">
          <el-select v-model="whForm.target_workflow_id" style="width: 100%">
            <el-option :label="t('workflow.f.noDirectTrigger')" :value="0" />
            <el-option v-for="w in list" :key="w.id" :label="w.name" :value="w.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('workflow.f.secret')">
          <el-input v-model="whForm.secret" :placeholder="t('workflow.f.secretHint')" />
          <div class="text-muted small">{{ t('workflow.f.secretNote') }}</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showWhForm = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="whSave">{{ t('workflow.createAction') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>
<style scoped>
.page { display: flex; flex-direction: column; gap: var(--sp-4); }
.head { display: flex; justify-content: space-between; align-items: flex-start; gap: var(--sp-3); }
.title { margin: 4px 0; font-size: var(--fs-title); font-weight: 700; }
.hint { font-size: var(--fs-body); }
.card { padding: 18px; min-height: 160px; }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: var(--sp-3); }
.wcard { display: flex; flex-direction: column; gap: var(--sp-1); padding: 14px; border-radius: var(--radius-sm); background: var(--surface-muted); }
.row1 { display: flex; align-items: center; gap: var(--sp-2); flex-wrap: wrap; }
.w-name { font-weight: 700; font-size: var(--fs-lg); }
.small { font-size: var(--fs-xs); }
.result { color: var(--el-color-primary); word-break: break-all; }
.chip { padding: 2px 10px; border-radius: var(--radius-pill); font-size: var(--fs-xs); font-weight: 600; }
.chip.st-idle { background: rgba(149,165,166,0.2); color: var(--text-muted); }
.chip.st-run { background: rgba(52,152,219,0.2); color: var(--pt-primary-2); }
.chip.st-ok { background: var(--pt-success-bg); color: var(--pt-success); }
.chip.st-err { background: var(--pt-danger-bg); color: var(--pt-danger); }
.chip.trig { background: var(--pt-primary-bg); color: var(--pt-primary); }
.actions { display: flex; gap: var(--sp-2); flex-wrap: wrap; margin-top: 4px; }
.empty { padding: 40px; text-align: center; }
.template-actions {
  display: flex;
  justify-content: center;
  flex-wrap: wrap;
  gap: var(--sp-2);
}
.stats {
  display: grid;
  grid-template-columns: repeat(6, 1fr);
  gap: var(--sp-2);
}
.stat {
  padding: 12px;
  border-radius: var(--radius-sm);
  background: var(--surface-muted);
  text-align: center;
}
.s-num { font-size: var(--fs-title); font-weight: 700; }
.s-lab { font-size: var(--fs-xs); color: var(--text-muted); }
.stat.ok .s-num { color: var(--pt-success); }
.stat.err .s-num { color: var(--pt-danger); }
.monitor {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--sp-3);
}
.m-title { font-weight: 700; margin-bottom: 10px; }
.nt-row { display: flex; align-items: center; gap: var(--sp-1); margin-bottom: 8px; }
.nt-name { width: 90px; font-size: var(--fs-body); }
.err-t { color: var(--pt-danger); margin-left: 6px; }
.rf-row { display: flex; align-items: center; gap: var(--sp-2); margin-bottom: 6px; }
.rf-id { font-weight: 700; color: var(--pt-danger); width: 40px; }
.rf-res { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.wh-tip { margin-bottom: 8px; line-height: 1.6; }
.wh-tip code { background: var(--surface-muted); padding: 1px 5px; border-radius: var(--radius-2xs); }
.wh-row {
  padding: 10px;
  border: 1px solid rgba(0,0,0,0.08);
  border-radius: var(--radius-sm);
  margin-bottom: 8px;
}
.wh-main { display: flex; align-items: center; gap: var(--sp-2); flex-wrap: wrap; }
.wh-name { font-weight: 700; }
.wh-url {
  font-size: var(--fs-2xs);
  color: var(--pt-primary-2);
  word-break: break-all;
  margin: 4px 0;
  font-family: monospace;
}
.wh-actions { display: flex; gap: var(--sp-1); flex-wrap: wrap; }
</style>
