<script setup lang="ts">
import { VueFlow } from '@vue-flow/core'
import { Background } from '@vue-flow/background'
import { Controls } from '@vue-flow/controls'
import { MiniMap } from '@vue-flow/minimap'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import '@vue-flow/controls/dist/style.css'
import '@vue-flow/minimap/dist/style.css'
import type { WorkflowRun } from '@/api/workflow'
import WorkflowNode from '@/components/workflow/WorkflowNode.vue'
import NodeParamForm from '@/components/workflow/NodeParamForm.vue'
import { useWorkflowEditor } from '@/composables/useWorkflowEditor'
import type { NodeInfo } from '@/api/workflow'

const FLOW_ID = 'wf-editor'
import { onMounted, onUnmounted, ref, computed } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { ElMessageBox } from 'element-plus'

const {
  router, wfId,
  categories,
  name, description, triggerType, timer, eventType, enabled,
  saving, running, selected, selectedEdge,
  loadError, load,
  onDragStart, onDrop, onDragOver,
  save, run, runResult, showResult,
  history, showHistory, openHistory, viewRun,
  deleteSelected, deleteSelectedEdge,
  dirty,
  templates, showTemplates, openTemplates, applyTemplate,
  exportJSON, onImportFile,
} = useWorkflowEditor(FLOW_ID)

// P3: 节点库搜索
const paletteSearch = ref('')
const filteredCategories = computed(() => {
  const q = paletteSearch.value.trim().toLowerCase()
  if (!q) return categories.value
  const m: Record<string, NodeInfo[]> = {}
  for (const [cat, items] of Object.entries(categories.value)) {
    const f = items.filter((a) => a.name.toLowerCase().includes(q) || a.type.toLowerCase().includes(q) || (a.description || "").toLowerCase().includes(q))
    if (f.length) m[cat] = f
  }
  return m
})

// P3: cron 表达式可读预览
const cronDesc = computed(() => {
  if (triggerType.value !== 'timer') return ''
  const parts = timer.value.trim().split(/\s+/)
  if (parts.length !== 5) return '格式应为 5 段：分 时 日 月 周'
  const [min, hour, dom, mon, dow] = parts
  const every = (v) => v === '*'
  if (every(min) && every(hour)) return '每分钟执行'
  if (min.startsWith('*/')) return `每 ${min.slice(2)} 分钟执行`
  if (every(hour) && !min.includes('*')) return `每天每小时的第 ${min} 分钟执行`
  if (hour.startsWith('*/')) return `每 ${hour.slice(2)} 小时的第 ${min || 0} 分钟执行`
  if (every(dom) && every(mon) && every(dow) && !min.includes('*') && !hour.includes('*')) return `每天 ${hour.padStart(2,'0')}:${min.padStart(2,'0')} 执行`
  if (dow !== '*' && !dow.includes('*')) {
    const days = ['周日','周一','周二','周三','周四','周五','周六']
    return `${days[+dow] || dow} ${hour.padStart(2,'0')}:${min.padStart(2,'0')} 执行`
  }
  return `自定义：${timer.value}`
})

onMounted(load)

// P1: 键盘快捷键
function onKeydown(e: KeyboardEvent) {
  // Ctrl+S 保存
  if ((e.ctrlKey || e.metaKey) && e.key === 's') {
    e.preventDefault()
    save()
    return
  }
  // Delete/Backspace 删除选中的节点或边
  if ((e.key === 'Delete' || e.key === 'Backspace') && selected.value) {
    e.preventDefault()
    deleteSelected()
    return
  }
  if ((e.key === 'Delete' || e.key === 'Backspace') && selectedEdge.value) {
    e.preventDefault()
    deleteSelectedEdge()
    return
  }
  // Escape 取消选中
  if (e.key === 'Escape') {
    selected.value = null
    selectedEdge.value = null
  }
}
onMounted(() => window.addEventListener('keydown', onKeydown))
onUnmounted(() => window.removeEventListener('keydown', onKeydown))

// P0: 未保存拦截
onBeforeRouteLeave(async (_to, _from, next) => {
  if (!dirty.value) return next()
  try {
    await ElMessageBox.confirm('有未保存的修改，确定离开吗？', '离开确认', {
      confirmButtonText: '离开',
      cancelButtonText: '继续编辑',
      type: 'warning',
    })
    next()
  } catch {
    next(false)
  }
})
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
        <el-input v-model="paletteSearch" size="small" placeholder="搜索节点…" clearable style="margin-bottom: 8px" />
        <div v-if="loadError" class="load-err">{{ loadError }}</div>
        <div v-else-if="Object.keys(filteredCategories).length" class="cats">
          <div v-for="(items, cat) in filteredCategories" :key="cat" class="cat">
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
        <div v-else-if="!loadError" class="text-muted small" style="padding: 12px; text-align: center">未找到匹配的节点</div>
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
            <div class="text-muted small">5 段式：分 时 日 月 周 · <b>{{ cronDesc }}</b></div>
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
  color: var(--text-muted);
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
  background: var(--surface-muted);
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
  background: var(--surface-muted);
  cursor: pointer;
  user-select: none;
}
.import-btn:hover {
  background: var(--surface-muted);
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
