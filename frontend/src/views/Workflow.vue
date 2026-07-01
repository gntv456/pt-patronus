<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { workflowApi, type Workflow, type WorkflowStats } from '@/api/workflow'
import { webhookApi, type Webhook } from '@/api/webhook'
import { copyText } from '@/utils/clipboard'
import { useAuthStore } from '@/stores/auth'
import EmptyState from '@/components/EmptyState.vue'

// Webhook 创建/编辑/删除/重置是 admin 专属（后端 RequireAdmin）；非 admin 隐藏入口。
const auth = useAuthStore()

const router = useRouter()
const list = ref<Workflow[]>([])
const loading = ref(false)
const running = ref<number | null>(null)
const stats = ref<WorkflowStats | null>(null)

async function load() {
  loading.value = true
  try {
    const [l, s] = await Promise.all([workflowApi.list(), workflowApi.stats().catch(() => null)])
    list.value = l
    stats.value = s
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载失败')
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

async function run(w: Workflow) {
  running.value = w.id
  try {
    const r = await workflowApi.run(w.id)
    if (r.ok) ElMessage.success('执行完成')
    else ElMessage.warning(r.error || '执行失败')
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '执行失败')
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
    ElMessage.error((e as Error).message || '操作失败')
  }
}

async function remove(w: Workflow) {
  try {
    await ElMessageBox.confirm(`确定删除工作流「${w.name}」？`, '确认', { type: 'warning' })
  } catch {
    return
  }
  try {
    await workflowApi.remove(w.id)
    await load()
    ElMessage.success('已删除')
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '删除失败')
  }
}

const triggerText: Record<string, string> = { manual: '手动', timer: '定时', event: '事件' }
const stateText: Record<string, string> = { idle: '空闲', running: '执行中', success: '成功', failed: '失败' }
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
    ElMessage.error((e as Error).message || '加载失败')
  }
}
function webhookUrl(w: Webhook) {
  return `${window.location.origin}/api/v1/webhooks/in/${w.token}`
}
async function copyUrl(w: Webhook) {
  await copyText(webhookUrl(w), '已复制接收 URL', webhookUrl(w))
}
async function whDelete(w: Webhook) {
  try {
    await ElMessageBox.confirm(`删除 webhook「${w.name}」？旧 URL 立即失效`, '确认', { type: 'warning' })
  } catch {
    return
  }
  await webhookApi.remove(w.id)
  webhooks.value = await webhookApi.list()
}
async function whRotate(w: Webhook) {
  try {
    await ElMessageBox.confirm('重生成 token？旧 URL 立即失效', '确认', { type: 'warning' })
  } catch {
    return
  }
  await webhookApi.rotate(w.id)
  webhooks.value = await webhookApi.list()
  ElMessage.success('已重生成，请复制新 URL')
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
    ElMessage.warning('名称必填')
    return
  }
  await webhookApi.create(whForm.value)
  showWhForm.value = false
  webhooks.value = await webhookApi.list()
  ElMessage.success('已创建')
}
</script>

<template>
  <div class="page">
    <div class="head">
      <div>
        <h2 class="title">工作流</h2>
        <span class="text-muted hint">可视化节点编排：触发 → 搜索/RSS → 过滤 → 下载 → 通知，支持条件分支。参考 MoviePilot 工作流。</span>
      </div>
      <el-button type="primary" @click="create">+ 新建工作流</el-button>
      <el-button v-if="auth.isAdmin" @click="openWebhooks">Webhook 入口</el-button>
    </div>

    <!-- 统计看板 -->
    <div v-if="stats" class="stats">
      <div class="stat glass-hover"><div class="s-num">{{ stats.total }}</div><div class="s-lab">工作流</div></div>
      <div class="stat glass-hover"><div class="s-num">{{ stats.enabled }}</div><div class="s-lab">已启用</div></div>
      <div class="stat glass-hover"><div class="s-num">{{ stats.runs_total }}</div><div class="s-lab">总运行</div></div>
      <div class="stat ok"><div class="s-num">{{ stats.runs_success }}</div><div class="s-lab">成功</div></div>
      <div class="stat err"><div class="s-num">{{ stats.runs_failed }}</div><div class="s-lab">失败</div></div>
      <div class="stat glass-hover"><div class="s-num">{{ Math.round(stats.avg_duration_ms / 1000) }}s</div><div class="s-lab">平均耗时</div></div>
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
            运行 {{ w.run_count }} 次 · 上次 {{ fmtTime(w.last_run_at) }}
          </div>
          <div v-if="w.last_result" class="result small">上次结果：{{ w.last_result }}</div>
          <div class="actions">
            <el-button size="small" type="primary" plain @click="edit(w)">编辑</el-button>
            <el-button size="small" type="primary" :loading="running === w.id" @click="run(w)">运行</el-button>
            <el-button size="small" @click="toggle(w)">{{ w.enabled ? '禁用' : '启用' }}</el-button>
            <el-button size="small" type="danger" plain @click="remove(w)">删除</el-button>
          </div>
        </div>
      </div>
      <EmptyState
        v-else-if="!loading"
        icon="🔀"
        title="暂无工作流"
        description="可视化节点编排：触发 → 搜索/RSS → 过滤 → 下载 → 通知，支持条件分支"
      >
        <el-button type="primary" @click="create">+ 新建工作流</el-button>
      </EmptyState>
    </div>

    <!-- 节点耗时 + 最近失败 -->
    <div v-if="stats && ((stats.node_timing && stats.node_timing.length) || (stats.recent_failures && stats.recent_failures.length))" class="monitor">
      <div v-if="stats.node_timing && stats.node_timing.length" class="card glass">
        <div class="m-title">节点耗时排行</div>
        <div v-for="n in stats.node_timing.slice(0, 8)" :key="n.node_type" class="nt-row">
          <span class="nt-name">{{ n.node_name || n.node_type }}</span>
          <el-progress
            :percentage="Math.min(100, Math.round((n.total_ms / Math.max(1, stats.node_timing[0].total_ms)) * 100))"
            :show-text="false"
            :stroke-width="8"
            style="flex: 1; margin: 0 8px"
          />
          <span class="text-muted small">{{ n.count }} 次 · 均 {{ n.avg_ms }}ms</span>
          <span v-if="n.failed" class="small err-t">失败 {{ n.failed }}</span>
        </div>
      </div>
      <div v-if="stats.recent_failures && stats.recent_failures.length" class="card glass">
        <div class="m-title">最近失败</div>
        <div v-for="f in stats.recent_failures" :key="f.id" class="rf-row">
          <span class="rf-id">#{{ f.id }}</span>
          <span class="rf-res small">{{ f.result }}</span>
          <span class="text-muted small">{{ new Date(f.started_at).toLocaleString('zh-CN', { hour12: false }) }}</span>
        </div>
      </div>
    </div>

    <!-- Webhook 管理 -->
    <el-dialog v-model="showWebhooks" title="进站 Webhook" width="680px">
      <div class="wh-tip text-muted small">
        外部系统 POST 到「接收 URL」即可触发。可选拖目标工作流（payload 注入变量），或建 event 型工作流监听
        <code>webhook</code> 事件。设了签名密钥时需带 <code>X-Signature</code> 头（HMAC-SHA256）。
      </div>
      <el-button size="small" type="primary" @click="openWhForm" style="margin: 8px 0">+ 新建 Webhook</el-button>
      <div v-for="w in webhooks" :key="w.id" class="wh-row">
        <div class="wh-main">
          <span class="wh-name">{{ w.name }}</span>
          <span v-if="w.has_secret" class="chip res">已签名</span>
          <span class="text-muted small">触发 {{ w.trigger_count }} 次 · {{ fmtTime(w.last_triggered_at) }}</span>
        </div>
        <div class="wh-url">{{ webhookUrl(w) }}</div>
        <div class="wh-actions">
          <el-button size="small" @click="copyUrl(w)">复制 URL</el-button>
          <el-button size="small" @click="whRotate(w)">重生成</el-button>
          <el-button size="small" type="danger" plain @click="whDelete(w)">删除</el-button>
        </div>
      </div>
      <div v-if="!webhooks.length" class="text-muted" style="padding: 20px; text-align: center">暂无 Webhook</div>
    </el-dialog>

    <!-- Webhook 新建 -->
    <el-dialog v-model="showWhForm" title="新建 Webhook" width="480px">
      <el-form label-width="110px" size="small">
        <el-form-item label="名称"><el-input v-model="whForm.name" placeholder="如：HomeAssistant" /></el-form-item>
        <el-form-item label="触发工作流">
          <el-select v-model="whForm.target_workflow_id" style="width: 100%">
            <el-option label="不直接触发（仅记录/发事件）" :value="0" />
            <el-option v-for="w in list" :key="w.id" :label="w.name" :value="w.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="签名密钥">
          <el-input v-model="whForm.secret" placeholder="留空=不校验签名" />
          <div class="text-muted small">设置后请求需带 X-Signature=HMAC-SHA256(body,密钥)</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showWhForm = false">取消</el-button>
        <el-button type="primary" @click="whSave">创建</el-button>
      </template>
    </el-dialog>
  </div>
</template>
<style scoped>
.page { display: flex; flex-direction: column; gap: 16px; }
.head { display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; }
.title { margin: 4px 0; font-size: 22px; font-weight: 700; }
.hint { font-size: 13px; }
.card { padding: 18px; min-height: 160px; }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 14px; }
.wcard { display: flex; flex-direction: column; gap: 6px; padding: 14px; border-radius: 12px; background: var(--surface-muted); }
.row1 { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.w-name { font-weight: 700; font-size: 16px; }
.small { font-size: 12px; }
.result { color: var(--el-color-primary); word-break: break-all; }
.chip { padding: 2px 10px; border-radius: 999px; font-size: 12px; font-weight: 600; }
.chip.st-idle { background: rgba(149,165,166,0.2); color: #7f8c8d; }
.chip.st-run { background: rgba(52,152,219,0.2); color: #2980b9; }
.chip.st-ok { background: rgba(46,204,113,0.2); color: #27ae60; }
.chip.st-err { background: rgba(231,76,60,0.2); color: #c0392b; }
.chip.trig { background: rgba(108,92,231,0.18); color: #6c5ce7; }
.actions { display: flex; gap: 8px; flex-wrap: wrap; margin-top: 4px; }
.empty { padding: 40px; text-align: center; }
.stats {
  display: grid;
  grid-template-columns: repeat(6, 1fr);
  gap: 10px;
}
.stat {
  padding: 12px;
  border-radius: 12px;
  background: var(--surface-muted);
  text-align: center;
}
.s-num { font-size: 22px; font-weight: 700; }
.s-lab { font-size: 12px; color: var(--text-muted); }
.stat.ok .s-num { color: #27ae60; }
.stat.err .s-num { color: #c0392b; }
.monitor {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
}
.m-title { font-weight: 700; margin-bottom: 10px; }
.nt-row { display: flex; align-items: center; gap: 6px; margin-bottom: 8px; }
.nt-name { width: 90px; font-size: 13px; }
.err-t { color: #c0392b; margin-left: 6px; }
.rf-row { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
.rf-id { font-weight: 700; color: #c0392b; width: 40px; }
.rf-res { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.wh-tip { margin-bottom: 8px; line-height: 1.6; }
.wh-tip code { background: var(--surface-muted); padding: 1px 5px; border-radius: 4px; }
.wh-row {
  padding: 10px;
  border: 1px solid rgba(0,0,0,0.08);
  border-radius: 10px;
  margin-bottom: 8px;
}
.wh-main { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.wh-name { font-weight: 700; }
.wh-url {
  font-size: 11px;
  color: #4285f4;
  word-break: break-all;
  margin: 4px 0;
  font-family: monospace;
}
.wh-actions { display: flex; gap: 6px; flex-wrap: wrap; }
</style>
