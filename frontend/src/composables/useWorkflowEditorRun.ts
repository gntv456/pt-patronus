// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// useWorkflowEditor 的保存/运行/输入构建（从主文件拆出；行为不变）。
import { ElMessage } from 'element-plus'
import { i18n } from '@/i18n'
import type { GraphNode, GraphEdge, ParamField } from '@/api/workflow'


const t = (k: string) => i18n.global.t(k)

export function useWorkflowEditorRun(opts: Record<string, any>) {
  const { name, description, triggerType, timer, eventType, eventConditions, enabled,
    saving, running, dirty, edges, router, wfId, markDirty, load,
    toObject, showResult, workflowApi } = opts
  const wfApi = workflowApi ?? opts.wfApi
  const runResult = opts.runResult
function buildInput() {
    const obj = toObject()
    const nodes: GraphNode[] = obj.nodes.map((n: any) => ({
      id: n.id,
      type: (n.data as any).type,
      name: (n.data as any).name,
      params: (n.data as any).params || {},
      position: n.position,
    }))
    const edges: GraphEdge[] = obj.edges.map((e: any) => ({
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
      ElMessage.warning(t('opuseworkfloweditorrun.k1'))
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
      ElMessage.warning(
        `以下节点必填项为空：${missingNodes.slice(0, 3).join('、')}${missingNodes.length > 3 ? '…' : ''}`,
      )
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
      ElMessage.success(t('common.saved'))
      dirty.value = false
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('common.saveFailed'))
    } finally {
      saving.value = false
    }
  }
async function run() {
    if (!wfId.value) {
      ElMessage.warning(t('opuseworkfloweditorrun.k2'))
      return
    }
    running.value = true
    try {
      const r = await workflowApi.run(wfId.value)
      if (r.ok && r.run) {
        runResult.value = r.run
        showResult.value = true
        ElMessage.success(t('opuseworkfloweditorrun.k3'))
      } else {
        ElMessage.error(r.error || t('common.execFailed'))
      }
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('common.execFailed'))
    } finally {
      running.value = false
    }
  }

  return { buildInput, save, run }
}
