<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { workflowApi, type Workflow, type WorkflowStats, type WorkflowTemplate } from '@/api/workflow'
import { webhookApi, type Webhook } from '@/api/webhook'
import { copyText } from '@/utils/clipboard'
import { useAuthStore } from '@/stores/auth'
import EmptyState from '@/components/EmptyState.vue'
import { useFeedback } from '@/composables/useFeedback'

// Webhook 创建/编辑/删除/重置是 admin 专属（后端 RequireAdmin）；非 admin 隐藏入口。
const auth = useAuthStore()
const feedback = useFeedback()

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
  const ok = await feedback.confirm({
    title: t('common.delete'),
    detail: t('workflow.confirmDelete', { name: w.name }),
    danger: true,
  })
  if (!ok) return
  try {
    await workflowApi.remove(w.id)
    await load()
    feedback.toast(t('common.deleted'))
  } catch (e: unknown) {
    feedback.error((e as Error).message || t('common.deleteFailed'))
  }
}

const triggerText = computed<Record<string, string>>(() => ({
  manual: t('workflow.trigger.manual'),
  timer: t('workflow.trigger.timer'),
  event: t('workflow.trigger.event'),
}))
const stateText = computed<Record<string, string>>(() => ({
  idle: t('workflow.state.idle'),
  running: t('workflow.state.running'),
  success: t('workflow.state.success'),
  failed: t('workflow.state.failed'),
}))
const stateClass: Record<string, string> = {
  idle: 'pt-chip--muted',
  running: 'pt-chip--info',
  success: 'pt-chip--success',
  failed: 'pt-chip--danger',
}
function fmtTime(t?: string) {
  return t ? new Date(t).toLocaleString('zh-CN', { hour12: false }).replace(/\//g, '-') : '—'
}

// —— 进站 Webhook 管理 ——
const showWebhooks = ref(false)
const webhooks = ref<Webhook[]>([])
// token 不再随 List 回传（后端脱敏）：仅创建/rotate 时一次性下发，本地暂存供拼 URL/复制。
const webhookTokens = ref<Record<number, string>>({})
function webhookUrl(w: Webhook) {
  const tk = w.token || webhookTokens.value[w.id]
  return tk ? `${window.location.origin}/api/v1/webhooks/in/${tk}` : `${window.location.origin}/api/v1/webhooks/in/<${t('workflow.rotateToReveal')}>`
}
async function openWebhooks() {
  try {
    webhooks.value = await webhookApi.list()
    showWebhooks.value = true
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('common.loadFailed'))
  }
}
async function copyUrl(w: Webhook) {
  await copyText(webhookUrl(w), t('workflow.urlCopied'), webhookUrl(w))
}
async function whDelete(w: Webhook) {
  const ok = await feedback.confirm({
    title: t('common.delete'),
    detail: t('workflow.confirmDeleteWh', { name: w.name }),
    danger: true,
  })
  if (!ok) return
  try {
    await webhookApi.remove(w.id)
    webhooks.value = await webhookApi.list()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('common.actionFailed'))
  }
}
async function whRotate(w: Webhook) {
  const ok = await feedback.confirm({
    title: t('workflow.rotate'),
    detail: t('workflow.confirmRotate'),
    danger: true,
  })
  if (!ok) return
  try {
    const r = await webhookApi.rotate(w.id)
    webhookTokens.value[w.id] = r.token // rotate 响应一次性下发新 token
    webhooks.value = await webhookApi.list()
    ElMessage.success(t('workflow.rotated'))
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('common.actionFailed'))
  }
}

// webhook 新建表单
const showWhForm = ref(false)
const whError = ref('')
const whForm = ref({ name: '', target_workflow_id: 0, secret: '' })
function openWhForm() {
  whForm.value = { name: '', target_workflow_id: 0, secret: '' }
  whError.value = ''
  showWhForm.value = true
}
async function whSave() {
  if (!whForm.value.name.trim()) {
    whError.value = t('workflow.nameRequired')
    return
  }
  whError.value = ''
  try {
    const created = await webhookApi.create(whForm.value)
    if (created.token) webhookTokens.value[created.id] = created.token
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
      <div class="stat solid-card">
        <div class="s-num">{{ stats.total }}</div>
        <div class="s-lab">{{ t('workflow.title') }}</div>
      </div>
      <div class="stat solid-card">
        <div class="s-num">{{ stats.enabled }}</div>
        <div class="s-lab">{{ t('workflow.enabled') }}</div>
      </div>
      <div class="stat solid-card">
        <div class="s-num">{{ stats.runs_total }}</div>
        <div class="s-lab">{{ t('workflow.totalRuns') }}</div>
      </div>
      <div class="stat ok">
        <div class="s-num">{{ stats.runs_success }}</div>
        <div class="s-lab">{{ t('transfer.st.success') }}</div>
      </div>
      <div class="stat err">
        <div class="s-num">{{ stats.runs_failed }}</div>
        <div class="s-lab">{{ t('transfer.st.failed') }}</div>
      </div>
      <div class="stat solid-card">
        <div class="s-num">{{ Math.round(stats.avg_duration_ms / 1000) }}s</div>
        <div class="s-lab">{{ t('workflow.avgDuration') }}</div>
      </div>
    </div>

    <div v-loading="loading" class="card solid-card">
      <el-skeleton v-if="loading && !list.length" :rows="4" animated />
      <div v-else-if="list.length" class="grid">
        <div v-for="w in list" :key="w.id" class="wcard solid-card">
          <div class="row1">
            <span class="w-name">{{ w.name }}</span>
            <span class="pt-chip" :class="stateClass[w.state]">{{ stateText[w.state] }}</span>
            <span class="pt-chip pt-chip--info">{{ triggerText[w.trigger_type] }}</span>
          </div>
          <div class="text-muted small">
            {{ w.description || '—' }}
          </div>
          <div class="text-muted small">
            {{ t('workflow.runCount', { n: w.run_count }) }} · {{ t('workflow.lastRun') }} {{ fmtTime(w.last_run_at) }}
          </div>
          <div v-if="w.last_result" class="result small">{{ t('workflow.lastResult') }}{{ w.last_result }}</div>
          <div class="actions">
            <el-button size="small" @click="edit(w)">{{ t('common.edit') }}</el-button>
            <el-button size="small" type="primary" :loading="running === w.id" @click="run(w)">{{
              t('transfer.run')
            }}</el-button>
            <el-button size="small" @click="toggle(w)">{{
              w.enabled ? t('workflow.disable') : t('rss.f.enabled')
            }}</el-button>
            <el-button size="small" type="danger" plain @click="remove(w)">{{ t('common.delete') }}</el-button>
          </div>
        </div>
      </div>
      <EmptyState
        v-else-if="!loading"
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
    <div
      v-if="
        stats &&
        ((stats.node_timing && stats.node_timing.length) || (stats.recent_failures && stats.recent_failures.length))
      "
      class="monitor"
    >
      <div v-if="stats.node_timing && stats.node_timing.length" class="card solid-card">
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
      <div v-if="stats.recent_failures && stats.recent_failures.length" class="card solid-card">
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
        {{ t('workflow.webhookTip1') }}<code>webhook</code>{{ t('workflow.webhookTip2') }}<code>X-Signature</code
        >{{ t('workflow.webhookTip3') }}
      </div>
      <el-button size="small" type="primary" @click="openWhForm" style="margin: 8px 0">{{
        t('workflow.newWebhook')
      }}</el-button>
      <div v-for="w in webhooks" :key="w.id" class="wh-row">
        <div class="wh-main">
          <span class="wh-name">{{ w.name }}</span>
          <span v-if="w.has_secret" class="pt-chip pt-chip--success">{{ t('workflow.signed') }}</span>
          <span class="text-muted small"
            >{{ t('workflow.triggerCount', { n: w.trigger_count }) }} · {{ fmtTime(w.last_triggered_at) }}</span
          >
        </div>
        <div class="wh-url">{{ webhookUrl(w) }}</div>
        <div class="wh-actions">
          <el-button size="small" @click="copyUrl(w)">{{ t('workflow.copyUrl') }}</el-button>
          <el-button size="small" @click="whRotate(w)">{{ t('workflow.rotate') }}</el-button>
          <el-button size="small" type="danger" plain @click="whDelete(w)">{{ t('common.delete') }}</el-button>
        </div>
      </div>
      <div v-if="!webhooks.length" class="text-muted" style="padding: 20px; text-align: center">
        {{ t('workflow.noWebhooks') }}
      </div>
    </el-dialog>

    <!-- Webhook 新建 -->
    <el-dialog v-model="showWhForm" :title="t('workflow.newWebhook')" width="480px">
      <el-form label-width="110px" size="small">
        <el-form-item :label="t('rss.f.name')" required :error="whError">
          <el-input v-model="whForm.name" :placeholder="t('workflow.f.nameHint')" aria-describedby="wh-name-error" @input="whError = ''" />
          <div v-if="whError" id="wh-name-error" role="alert" class="field-error">{{ whError }}</div>
        </el-form-item>
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
<style scoped src="./Workflow.css"></style>
