import { ref, computed, type Ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useVueFlow, type Edge, type Node, type Connection } from '@vue-flow/core'
import { workflowApi, type NodeInfo, type GraphNode, type GraphEdge, type WorkflowRun, type WorkflowTemplate, type ParamField } from '@/api/workflow'

export interface WfNodeData {
  type: string
  name: string
  params: Record<string, any>
  info?: NodeInfo
  branch: boolean
}

export function useWorkflowEditor(flowId: string) {
  const route = useRoute()
  const router = useRouter()
  const wfId = computed(() => (route.query.id ? Number(route.query.id) : 0))

  const { onConnect, addEdges, addNodes, screenToFlowCoordinate, onNodeClick, onPaneClick, onEdgeClick,
    onEdgesChange, applyEdgeChanges,
    setNodes, setEdges, removeNodes, toObject } = useVueFlow(flowId)

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
  const eventConditions = ref<import('@/api/workflow').EventCondition[]>([])
  const enabled = ref(false)
  const saving = ref(false)
  const running = ref(false)

  // dirty 跟踪（未保存修改提示）
  const dirty = ref(false)
  function markDirty() { dirty.value = true }

  const selected = ref<Node | null>(null)
  onNodeClick(({ node }) => {
    selected.value = node
  })
  onPaneClick(() => {
    selected.value = null
    selectedEdge.value = null
  })

  // 连线选中
  const selectedEdge = ref<Edge | null>(null)
  onEdgeClick(({ edge }) => {
    selectedEdge.value = edge
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
        eventConditions.value = [...(wf.event_conditions || [])]
        enabled.value = wf.enabled
        setNodes((wf.nodes ?? []).map(toVFNode))
        setEdges((wf.edges ?? []).map(toVFEdge))
        dirty.value = false
      } catch (e: unknown) {
        ElMessage.error((e as Error).message || '加载工作流失败')
      }
    }
  }

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
    markDirty()
  })

  // 连线变更（删除等）
  onEdgesChange((changes) => {
    applyEdgeChanges(changes)
    if (changes.some((c) => c.type === 'remove')) {
      markDirty()
      selectedEdge.value = null
    }
  })

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
    markDirty()
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
      event_conditions: eventConditions.value,
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
    // P1: 校验各节点必填参数
    const obj0 = toObject()
    const missingNodes: string[] = []
    for (const n of obj0.nodes) {
      const info = (n.data as any).info
      const params = (n.data as any).params || {}
      if (!info?.param_schema) continue
      const nodeName = (n.data as any).name || n.id
      for (const f of info.param_schema as ParamField[]) {
        if (!f.required) continue
        const val = params[f.name]
        if (val === undefined || val === null || val === '' || (Array.isArray(val) && !val.length)) {
          missingNodes.push(`${nodeName} → ${f.label}`)
        }
      }
    }
    if (missingNodes.length) {
      ElMessage.warning(`以下节点必填项为空：${missingNodes.slice(0, 3).join("、")}${missingNodes.length > 3 ? "…" : ""}`)
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
      dirty.value = false
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
    try {
      history.value = await workflowApi.runs(wfId.value, 20)
      showHistory.value = true
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '历史加载失败')
    }
  }
  async function viewRun(rid: number) {
    try {
      runResult.value = await workflowApi.runDetail(rid)
      showResult.value = true
      showHistory.value = false
    } catch (e: unknown) {
      // 失败时不打开 showResult，避免显示旧 runResult
      ElMessage.error((e as Error).message || '运行详情加载失败')
    }
  }

  // 删除选中节点
  function deleteSelected() {
    if (!selected.value) return
    removeNodes([selected.value.id])
    selected.value = null
    markDirty()
  }

  // 删除选中连线
  function deleteSelectedEdge() {
    if (!selectedEdge.value) return
    const id = selectedEdge.value.id
    setEdges((eds) => eds.filter((e) => e.id !== id))
    selectedEdge.value = null
    markDirty()
  }

  // —— 模板 / 导入 / 导出 ——
  const templates = ref<WorkflowTemplate[]>([])
  const showTemplates = ref(false)
  async function openTemplates() {
    try {
      if (!templates.value.length) templates.value = await workflowApi.templates()
      showTemplates.value = true
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '模板加载失败')
    }
  }
  function applyTemplate(t: WorkflowTemplate) {
    name.value = t.name
    description.value = t.description
    triggerType.value = (t.trigger_type as any) || 'manual'
    timer.value = t.timer || '*/30 * * * *'
    eventType.value = t.event_type || ''
    eventConditions.value = [...(t.event_conditions || [])]
    enabled.value = false
    setNodes((t.nodes ?? []).map(toVFNode))
    setEdges((t.edges ?? []).map(toVFEdge))
    showTemplates.value = false
    markDirty()
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
        eventConditions.value = [...(data.event_conditions || [])]
        enabled.value = false
        setNodes((data.nodes || []).map(toVFNode))
        setEdges((data.edges || []).map(toVFEdge))
        markDirty()
        ElMessage.success('已导入')
      } catch {
        ElMessage.error('JSON 解析失败')
      }
    }
    reader.readAsText(file)
    target.value = '' // 允许重复导入同一文件
  }

  return {
    route, router, wfId,
    actions, categories, infoByType,
    name, description, triggerType, timer, eventType, eventConditions, enabled,
    saving, running, selected, dirty,
    loadError, load,
    onDragStart, onDrop, onDragOver,
    save, run, runResult, showResult,
    history, showHistory, openHistory, viewRun,
    deleteSelected, selectedEdge, deleteSelectedEdge,
    templates, showTemplates, openTemplates, applyTemplate,
    exportJSON, onImportFile,
  }
}