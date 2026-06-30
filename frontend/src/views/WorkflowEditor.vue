<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { VueFlow, useVueFlow, type Edge, type Node, type Connection } from '@vue-flow/core'
import { Background } from '@vue-flow/background'
import { Controls } from '@vue-flow/controls'
import { MiniMap } from '@vue-flow/minimap'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import '@vue-flow/controls/dist/style.css'
import '@vue-flow/minimap/dist/style.css'
import { workflowApi, type NodeInfo, type GraphNode, type GraphEdge, type WorkflowRun, type WorkflowTemplate } from '@/api/workflow'
import WorkflowNode from '@/components/workflow/WorkflowNode.vue'
import NodeParamForm from '@/components/workflow/NodeParamForm.vue'

const route = useRoute()
const router = useRouter()
const wfId = computed(() => (route.query.id ? Number(route.query.id) : 0))
const FLOW_ID = 'wf-editor'

const { onConnect, addEdges, addNodes, screenToFlowCoordinate, onNodeClick, onPaneClick,
  setNodes, setEdges, removeNodes, toObject } = useVueFlow(FLOW_ID)

const actions = ref<NodeInfo[]>([])
const infoByType = computed(() => {
  const m: Record<string, NodeInfo> = {}
  for (const a of actions.value) m[a.type] = a
  return m
})
const categories = computed(() => {
  const m: Record<string, NodeInfo[]> = {}
  for (const a of actions.value) (m[a.category] ||= []).push(a)
  return m
})

// 工作流元信息
const name = ref('')
const description = ref('')
const triggerType = ref<'manual' | 'timer' | 'event'>('manual')
const timer = ref('*/30 * * * *')
const eventType = ref('')
const enabled = ref(false)
const saving = ref(false)
const running = ref(false)

const selected = ref<Node | null>(null)
onNodeClick(({ node }) => {
  selected.value = node
})
onPaneClick(() => {
  selected.value = null
})

// 加载目录 + （编辑模式）已有工作流
const loadError = ref('')
async function load() {
  try {
    actions.value = await workflowApi.actions()
    if (!actions.value.length) {
      loadError.value = '节点目录为空：后端可能未重启（缺少 /workflows 路由）'
      ElMessage.error(loadError.value)
      return
    }
  } catch (e: unknown) {
    loadError.value = (e as Error).message || '节点目录加载失败（请确认后端已重启并包含工作流路由）'
    ElMessage.error(loadError.value)
    return
  }
  if (wfId.value) {
    try {
      const wf = await workflowApi.get(wfId.value)
      name.value = wf.name
      description.value = wf.description
      triggerType.value = wf.trigger_type
      timer.value = wf.timer || '*/30 * * * *'
      eventType.value = wf.event_type
      enabled.value = wf.enabled
      setNodes(wf.nodes.map(toVFNode))
      setEdges(wf.edges.map(toVFEdge))
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '加载工作流失败')
    }
  }
}
onMounted(load)

function toVFNode(g: GraphNode): Node {
  const info = infoByType.value[g.type]
  return {
    id: g.id,
    type: 'wf',
    position: g.position || { x: 0, y: 0 },
    data: { type: g.type, name: g.name || info?.name || g.type, params: { ...(g.params || {}) }, info, branch: g.type === 'condition' },
  }
}
function toVFEdge(g: GraphEdge): Edge {
  return {
    id: g.id,
    source: g.source,
    target: g.target,
    sourceHandle: g.source_port || undefined,
    targetHandle: g.target_port || undefined,
    animated: true,
  }
}

// 连线
onConnect((params: Connection) => {
  addEdges([params])
})

// 节点拖拽位置变更随 VF 内部状态自动维护（无需额外处理）

// 调色板拖拽 → 画布
function onDragStart(e: DragEvent, info: NodeInfo) {
  if (!e.dataTransfer) return
  e.dataTransfer.setData('application/wf', info.type)
  e.dataTransfer.effectAllowed = 'move'
}
function onDrop(e: DragEvent) {
  const type = e.dataTransfer?.getData('application/wf')
  if (!type) return
  const info = infoByType.value[type]
  if (!info) return
  const position = screenToFlowCoordinate({ x: e.clientX, y: e.clientY })
  const id = `${type}-${Date.now()}`
  addNodes([
    {
      id,
      type: 'wf',
      position,
      data: { type, name: info.name, params: { ...(info.default_params || {}) }, info, branch: type === 'condition' },
    },
  ])
}
function onDragOver(e: DragEvent) {
  e.preventDefault()
  if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
}

// 保存
function buildInput() {
  const obj = toObject()
  const nodes: GraphNode[] = obj.nodes.map((n) => ({
    id: n.id,
    type: (n.data as any).type,
    name: (n.data as any).name,
    params: (n.data as any).params || {},
    position: n.position,
  }))
  const edges: GraphEdge[] = obj.edges.map((e) => ({
    id: e.id,
    source: e.source,
    target: e.target,
    source_port: e.sourceHandle || '',
    target_port: e.targetHandle || '',
  }))
  return {
    name: name.value,
    description: description.value,
    trigger_type: triggerType.value,
    timer: timer.value,
    event_type: eventType.value,
    enabled: enabled.value,
    nodes,
    edges,
  }
}

async function save() {
  if (!name.value.trim()) {
    ElMessage.warning('请填写工作流名称')
    return
  }
  saving.value = true
  try {
    const input = buildInput()
    if (wfId.value) {
      await workflowApi.update(wfId.value, input)
    } else {
      const created = await workflowApi.create(input)
      router.replace({ name: 'workflow-editor', query: { id: created.id } })
    }
    ElMessage.success('已保存')
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '保存失败')
  } finally {
    saving.value = false
  }
}

// 运行
const runResult = ref<WorkflowRun | null>(null)
const showResult = ref(false)
async function run() {
  if (!wfId.value) {
    ElMessage.warning('请先保存工作流')
    return
  }
  running.value = true
  try {
    const r = await workflowApi.run(wfId.value)
    if (r.ok && r.run) {
      runResult.value = r.run
      showResult.value = true
      ElMessage.success('执行完成')
    } else {
      ElMessage.error(r.error || '执行失败')
    }
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '执行失败')
  } finally {
    running.value = false
  }
}

// 历史
const history = ref<WorkflowRun[]>([])
const showHistory = ref(false)
async function openHistory() {
  if (!wfId.value) return
  history.value = await workflowApi.runs(wfId.value, 20)
  showHistory.value = true
}
async function viewRun(rid: number) {
  runResult.value = await workflowApi.runDetail(rid)
  showResult.value = true
  showHistory.value = false
}

// 删除选中节点
function deleteSelected() {
  if (!selected.value) return
  removeNodes([selected.value.id])
  selected.value = null
}

// —— 模板 / 导入 / 导出 ——
const templates = ref<WorkflowTemplate[]>([])
const showTemplates = ref(false)
async function openTemplates() {
  if (!templates.value.length) templates.value = await workflowApi.templates()
  showTemplates.value = true
}
function applyTemplate(t: WorkflowTemplate) {
  name.value = t.name
  description.value = t.description
  triggerType.value = (t.trigger_type as any) || 'manual'
  timer.value = t.timer || '*/30 * * * *'
  enabled.value = false
  setNodes(t.nodes.map(toVFNode))
  setEdges(t.edges.map(toVFEdge))
  showTemplates.value = false
  ElMessage.success('已载入模板，编辑后保存')
}
function exportJSON() {
  const input = buildInput()
  const blob = new Blob([JSON.stringify(input, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `${input.name || 'workflow'}.json`
  a.click()
  URL.revokeObjectURL(url)
}
function onImportFile(ev: Event) {
  const target = ev.target as HTMLInputElement
  const file = target.files?.[0]
  if (!file) return
  const reader = new FileReader()
  reader.onload = () => {
    try {
      const data = JSON.parse(String(reader.result))
      name.value = data.name || ''
      description.value = data.description || ''
      triggerType.value = (data.trigger_type as any) || 'manual'
      timer.value = data.timer || '*/30 * * * *'
      eventType.value = data.event_type || ''
      enabled.value = false
      setNodes((data.nodes || []).map(toVFNode))
      setEdges((data.edges || []).map(toVFEdge))
      ElMessage.success('已导入')
    } catch {
      ElMessage.error('JSON 解析失败')
    }
  }
  reader.readAsText(file)
  target.value = '' // 允许重复导入同一文件
}
</script>

<template>
  <div class="editor">
    <!-- 顶栏 -->
    <div class="toolbar glass-bar">
      <el-button size="small" @click="router.push({ name: 'workflow' })">← 返回</el-button>
      <span class="t-title">{{ wfId ? '编辑工作流' : '新建工作流' }}</span>
      <div class="spacer"></div>
      <el-button size="small" @click="openTemplates">📋 模板</el-button>
      <el-button size="small" @click="exportJSON">⬆ 导出</el-button>
      <label class="import-btn">
        ⬇ 导入
        <input type="file" accept="application/json" hidden @change="onImportFile" />
      </label>
      <el-button size="small" :loading="running" type="primary" @click="run">▶ 运行</el-button>
      <el-button v-if="wfId" size="small" @click="openHistory">历史</el-button>
      <el-button size="small" type="success" :loading="saving" @click="save">💾 保存</el-button>
    </div>

    <div class="body">
      <!-- 左：节点库 -->
      <aside class="palette glass">
        <div class="p-title">节点库（拖入画布）</div>
        <div v-if="loadError" class="load-err">{{ loadError }}</div>
        <div v-else class="cats">
          <div v-for="(items, cat) in categories" :key="cat" class="cat">
            <div class="cat-name">{{ cat }}</div>
            <div
              v-for="a in items"
              :key="a.type"
              class="cat-item"
              draggable="true"
              @dragstart="onDragStart($event, a)"
              :title="a.description"
            >
              {{ a.name }}
            </div>
          </div>
        </div>
      </aside>

      <!-- 中：画布 -->
      <div class="canvas-wrap glass" @drop="onDrop" @dragover="onDragOver">
        <VueFlow :id="FLOW_ID" :default-edge-options="{ animated: true }" fit-view-on-init>
          <Background pattern-color="#aaa" :gap="16" />
          <Controls />
          <MiniMap />
          <template #node-wf="props">
            <WorkflowNode v-bind="props" />
          </template>
        </VueFlow>
      </div>

      <!-- 右：配置面板 -->
      <aside class="config glass">
        <div class="p-title">工作流设置</div>
        <div class="wf-form">
          <div class="field">
            <label>名称</label>
            <el-input v-model="name" size="small" placeholder="如：自动追剧" />
          </div>
          <div class="field">
            <label>说明</label>
            <el-input v-model="description" size="small" />
          </div>
          <div class="field">
            <label>触发方式</label>
            <el-select v-model="triggerType" size="small" style="width: 100%">
              <el-option label="手动" value="manual" />
              <el-option label="定时 (cron)" value="timer" />
              <el-option label="事件" value="event" />
            </el-select>
          </div>
          <div v-if="triggerType === 'timer'" class="field">
            <label>Cron 表达式</label>
            <el-input v-model="timer" size="small" placeholder="*/30 * * * *" />
            <div class="text-muted small">5 段式：分 时 日 月 周</div>
          </div>
          <div v-if="triggerType === 'event'" class="field">
            <label>事件类型</label>
            <el-input v-model="eventType" size="small" placeholder="如 download.completed" />
          </div>
          <div class="field row">
            <el-switch v-model="enabled" />
            <span class="small">启用（注册触发器）</span>
          </div>
        </div>

        <div class="divider"></div>

        <div v-if="selected" class="node-cfg">
          <div class="p-title">节点配置</div>
          <div class="field">
            <label>节点名称</label>
            <el-input v-model="(selected.data as any).name" size="small" />
          </div>
          <NodeParamForm
            v-if="(selected.data as any).info"
            :schema="(selected.data as any).info.param_schema"
            :params="(selected.data as any).params"
          />
          <el-button size="small" type="danger" plain style="margin-top: 12px" @click="deleteSelected">删除节点</el-button>
        </div>
        <div v-else class="text-muted small pad">选中画布上的节点以配置参数。</div>
      </aside>
    </div>

    <!-- 运行结果 / 历史详情 -->
    <el-dialog v-model="showResult" title="执行结果" width="640px">
      <div v-if="runResult">
        <div class="text-muted small" style="margin-bottom: 8px">
          状态：{{ runResult.state }} · 耗时 {{ runResult.duration_ms }}ms · {{ runResult.result }}
        </div>
        <el-timeline>
          <el-timeline-item
            v-for="(log, i) in runResult.node_executions || []"
            :key="i"
            :type="log.status === 'success' ? 'success' : log.status === 'failed' ? 'danger' : 'info'"
            :timestamp="log.at"
          >
            <strong>{{ log.node_name }}</strong>
            <span class="text-muted small"> · {{ log.duration_ms }}ms</span>
            <div v-if="log.message" class="small">{{ log.message }}</div>
          </el-timeline-item>
        </el-timeline>
      </div>
    </el-dialog>

    <!-- 历史 -->
    <el-dialog v-model="showHistory" title="运行历史" width="560px">
      <el-table :data="history" size="small" @row-click="(r: WorkflowRun) => viewRun(r.id)">
        <el-table-column prop="id" label="#" width="60" />
        <el-table-column prop="state" label="状态" width="90" />
        <el-table-column prop="trigger" label="触发" width="90" />
        <el-table-column label="时间">
          <template #default="{ row }">{{ new Date(row.started_at).toLocaleString('zh-CN', { hour12: false }) }}</template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <!-- 模板 -->
    <el-dialog v-model="showTemplates" title="从模板创建" width="560px">
      <div class="tpl-list">
        <div v-for="t in templates" :key="t.key" class="tpl-item" @click="applyTemplate(t)">
          <div class="tpl-name">{{ t.name }} <span class="tpl-cat">{{ t.category }}</span></div>
          <div class="text-muted small">{{ t.description }}</div>
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<style scoped>
.editor {
  display: flex;
  flex-direction: column;
  gap: 12px;
  height: calc(100vh - 32px);
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 14px;
}
.t-title {
  font-weight: 700;
  font-size: 16px;
}
.spacer {
  flex: 1;
}
.body {
  flex: 1;
  display: flex;
  gap: 12px;
  min-height: 0;
}
.palette {
  width: 190px;
  padding: 12px;
  overflow-y: auto;
}
.p-title {
  font-weight: 700;
  font-size: 14px;
  margin-bottom: 10px;
}
.cats {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.cat-name {
  font-size: 12px;
  font-weight: 700;
  color: #888;
  margin-bottom: 4px;
}
.cat-item {
  padding: 7px 10px;
  border-radius: 8px;
  background: rgba(108, 92, 231, 0.1);
  font-size: 13px;
  margin-bottom: 4px;
  cursor: grab;
  user-select: none;
}
.cat-item:hover {
  background: rgba(108, 92, 231, 0.2);
}
.canvas-wrap {
  flex: 1;
  min-width: 0;
  position: relative;
}
.config {
  width: 300px;
  padding: 12px;
  overflow-y: auto;
}
.wf-form,
.node-cfg {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.field {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.field.row {
  flex-direction: row;
  align-items: center;
  gap: 8px;
}
.field label {
  font-size: 12px;
  font-weight: 600;
}
.small {
  font-size: 12px;
}
.divider {
  height: 1px;
  background: rgba(0, 0, 0, 0.1);
  margin: 12px 0;
}
.pad {
  padding: 8px 0;
}
.load-err {
  font-size: 12px;
  color: #c0392b;
  padding: 8px;
  background: rgba(231, 76, 60, 0.08);
  border-radius: 8px;
}
.import-btn {
  display: inline-flex;
  align-items: center;
  padding: 0 12px;
  height: 24px;
  font-size: 12px;
  border-radius: var(--el-border-radius-base);
  background: rgba(0, 0, 0, 0.05);
  cursor: pointer;
  user-select: none;
}
.import-btn:hover {
  background: rgba(0, 0, 0, 0.1);
}
.tpl-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.tpl-item {
  padding: 12px;
  border-radius: 10px;
  border: 1px solid rgba(0, 0, 0, 0.1);
  cursor: pointer;
}
.tpl-item:hover {
  background: rgba(108, 92, 231, 0.08);
  border-color: #6c5ce7;
}
.tpl-name {
  font-weight: 700;
}
.tpl-cat {
  font-size: 11px;
  padding: 1px 8px;
  border-radius: 999px;
  background: rgba(66, 133, 244, 0.15);
  color: #4285f4;
  margin-left: 6px;
}
</style>
