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
const { t } = useI18n()
import { onMounted, onUnmounted, ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { onBeforeRouteLeave } from 'vue-router'
import { ElMessageBox } from 'element-plus'

const {
  router,
  wfId,
  categories,
  name,
  description,
  triggerType,
  timer,
  eventType,
  enabled,
  saving,
  running,
  selected,
  selectedEdge,
  loadError,
  load,
  onDragStart,
  onDrop,
  onDragOver,
  save,
  run,
  runResult,
  showResult,
  history,
  showHistory,
  openHistory,
  viewRun,
  deleteSelected,
  deleteSelectedEdge,
  dirty,
  templates,
  showTemplates,
  openTemplates,
  applyTemplate,
  exportJSON,
  onImportFile,
} = useWorkflowEditor(FLOW_ID)

// P3: 节点库搜索
const paletteSearch = ref('')
const filteredCategories = computed(() => {
  const q = paletteSearch.value.trim().toLowerCase()
  if (!q) return categories.value
  const m: Record<string, NodeInfo[]> = {}
  for (const [cat, items] of Object.entries(categories.value)) {
    const f = items.filter(
      (a) =>
        a.name.toLowerCase().includes(q) ||
        a.type.toLowerCase().includes(q) ||
        (a.description || '').toLowerCase().includes(q),
    )
    if (f.length) m[cat] = f
  }
  return m
})

// P3: cron 表达式可读预览
const cronDesc = computed(() => {
  if (triggerType.value !== 'timer') return ''
  const parts = timer.value.trim().split(/\s+/)
  if (parts.length !== 5) return t('wfe.cronBad')
  const [min, hour, dom, mon, dow] = parts
  const every = (v: string) => v === '*'
  if (every(min) && every(hour)) return t('wfe.cronEveryMin')
  if (min.startsWith('*/')) return t('wfe.cronEveryNMin', { n: min.slice(2) })
  if (every(hour) && !min.includes('*')) return t('wfe.cronHourlyAt', { min })
  if (hour.startsWith('*/')) return t('wfe.cronEveryNHourAt', { h: hour.slice(2), min: min || 0 })
  if (every(dom) && every(mon) && every(dow) && !min.includes('*') && !hour.includes('*'))
    return t('wfe.cronDailyAt', { time: `${hour.padStart(2, '0')}:${min.padStart(2, '0')}` })
  if (dow !== '*' && !dow.includes('*')) {
    const time = `${hour.padStart(2, '0')}:${min.padStart(2, '0')}`
    return `${t(`wfe.dow.${+dow >= 0 && +dow <= 6 ? +dow : 'n'}`)} ${time} ${t('wfe.cronAtSuffix')}`
  }
  return t('wfe.cronCustom', { expr: timer.value })
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
    await ElMessageBox.confirm(t('wfe.dirtyLeaveMsg'), t('wfe.dirtyLeaveTitle'), {
      confirmButtonText: t('wfe.leave'),
      cancelButtonText: t('wfe.keepEditing'),
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
      <el-button size="small" @click="router.push({ name: 'workflow' })">← {{ t('wfe.back') }}</el-button>
      <span class="t-title">{{ wfId ? t('wfe.editWf') : t('wfe.newWf') }}</span>
      <div class="spacer"></div>
      <el-button size="small" @click="openTemplates">📋 {{ t('wfe.templates') }}</el-button>
      <el-button size="small" @click="exportJSON">⬆ {{ t('wfe.export') }}</el-button>
      <label class="import-btn">
        ⬇ {{ t('wfe.import') }}
        <input type="file" accept="application/json" hidden @change="onImportFile" />
      </label>
      <el-button size="small" :loading="running" type="primary" @click="run">▶ {{ t('wfe.run') }}</el-button>
      <el-button v-if="wfId" size="small" @click="openHistory">{{ t('wfe.history') }}</el-button>
      <el-button size="small" type="success" :loading="saving" @click="save">💾 {{ t('common.save') }}</el-button>
    </div>

    <div class="body">
      <!-- 左：节点库 -->
      <aside class="palette glass">
        <div class="p-title">{{ t('wfe.palette') }}</div>
        <el-input
          v-model="paletteSearch"
          size="small"
          :placeholder="t('wfe.searchNode')"
          clearable
          style="margin-bottom: 8px"
        />
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
        <div v-else-if="!loadError" class="text-muted small" style="padding: 12px; text-align: center">
          {{ t('wfe.noNodeMatch') }}
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
        <div class="p-title">{{ t('wfe.wfSettings') }}</div>
        <div class="wf-form">
          <div class="field">
            <label>{{ t('common.name') }}</label>
            <el-input v-model="name" size="small" :placeholder="t('wfe.namePh')" />
          </div>
          <div class="field">
            <label>{{ t('wfe.desc') }}</label>
            <el-input v-model="description" size="small" />
          </div>
          <div class="field">
            <label>{{ t('wfe.trigger') }}</label>
            <el-select v-model="triggerType" size="small" style="width: 100%">
              <el-option :label="t('wfe.trigManual')" value="manual" />
              <el-option :label="t('wfe.trigTimer')" value="timer" />
              <el-option :label="t('wfe.trigEvent')" value="event" />
            </el-select>
          </div>
          <div v-if="triggerType === 'timer'" class="field">
            <label>{{ t('wfe.cronExpr') }}</label>
            <el-input v-model="timer" size="small" placeholder="*/30 * * * *" />
            <div class="text-muted small">
              {{ t('wfe.cron5') }} · <b>{{ cronDesc }}</b>
            </div>
          </div>
          <div v-if="triggerType === 'event'" class="field">
            <label>{{ t('wfe.eventType') }}</label>
            <el-input v-model="eventType" size="small" :placeholder="t('wfe.eventPh')" />
          </div>
          <div class="field row">
            <el-switch v-model="enabled" />
            <span class="small">{{ t('wfe.enableReg') }}</span>
          </div>
        </div>

        <div class="divider"></div>

        <div v-if="selected" class="node-cfg">
          <div class="p-title">{{ t('wfe.nodeCfg') }}</div>
          <div class="field">
            <label>{{ t('wfe.nodeName') }}</label>
            <el-input v-model="(selected.data as any).name" size="small" />
          </div>
          <NodeParamForm
            v-if="(selected.data as any).info"
            :schema="(selected.data as any).info.param_schema"
            v-model:params="(selected.data as any).params"
          />
          <el-button size="small" type="danger" plain style="margin-top: 12px" @click="deleteSelected">{{
            t('wfe.deleteNode')
          }}</el-button>
        </div>
        <div v-else class="text-muted small pad">{{ t('wfe.selectNodeHint') }}</div>
      </aside>
    </div>

    <!-- 运行结果 / 历史详情 -->
    <el-dialog v-model="showResult" :title="t('wfe.resultTitle')" width="640px">
      <div v-if="runResult">
        <div class="text-muted small" style="margin-bottom: 8px">
          {{ t('wfe.resultMeta', { state: runResult.state, ms: runResult.duration_ms, result: runResult.result }) }}
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
    <el-dialog v-model="showHistory" :title="t('wfe.historyTitle')" width="560px">
      <el-table :data="history" size="small" @row-click="(r: WorkflowRun) => viewRun(r.id)">
        <el-table-column prop="id" label="#" width="60" />
        <el-table-column prop="state" :label="t('cloud.status')" width="90" />
        <el-table-column prop="trigger" :label="t('wfe.triggerCol')" width="90" />
        <el-table-column :label="t('wfe.time')">
          <template #default="{ row }">{{
            new Date(row.started_at).toLocaleString('zh-CN', { hour12: false })
          }}</template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <!-- 模板 -->
    <el-dialog v-model="showTemplates" :title="t('wfe.tplTitle')" width="560px">
      <div class="tpl-list">
        <div v-a11y-button v-for="t in templates" :key="t.key" class="tpl-item" @click="applyTemplate(t)">
          <div class="tpl-name">
            {{ t.name }} <span class="tpl-cat">{{ t.category }}</span>
          </div>
          <div class="text-muted small">{{ t.description }}</div>
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<style scoped src="./WorkflowEditor.css"></style>
