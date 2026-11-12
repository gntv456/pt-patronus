// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { http } from './http'

// 工作流：可视化节点图引擎。

export interface Position {
  x: number
  y: number
}

export interface GraphNode {
  id: string
  type: string // 节点类型键 → /workflows/actions 目录
  name: string
  params: Record<string, any>
  position: Position
}

export interface GraphEdge {
  id: string
  source: string
  target: string
  source_port?: string // 条件分支输出端口标签（true/false）
  target_port?: string
}

export interface EventCondition {
  field: string
  operator: 'eq' | 'ne' | 'contains' | 'regex'
  value: string
}

export interface Workflow {
  id: number
  name: string
  description: string
  trigger_type: 'manual' | 'timer' | 'event'
  timer: string
  event_type: string
  event_conditions: EventCondition[]
  enabled: boolean
  state: 'idle' | 'running' | 'success' | 'failed'
  run_count: number
  last_run_at?: string
  last_result: string
  nodes: GraphNode[]
  edges: GraphEdge[]
  created_at: string
  updated_at: string
}

export interface ParamOption {
  label: string
  value: any
}

export interface ParamField {
  name: string
  label: string
  type: 'text' | 'number' | 'select' | 'textarea' | 'switch' | 'kv' | 'site_select' | 'downloader_select'
  options?: ParamOption[]
  required?: boolean
  default?: any
  help?: string
}

export interface NodeInfo {
  type: string
  name: string
  category: string
  description: string
  icon: string
  default_params: Record<string, any>
  param_schema: ParamField[]
}

export interface LogEntry {
  node_id: string
  node_type: string
  node_name: string
  status: 'success' | 'failed' | 'skipped'
  message: string
  duration_ms: number
  at: string
}

export interface WorkflowRun {
  id: number
  workflow_id: number
  state: 'running' | 'success' | 'failed'
  trigger: 'manual' | 'timer' | 'event'
  started_at: string
  finished_at?: string
  duration_ms: number
  node_executions: LogEntry[]
  result: string
  created_at: string
}

export interface WorkflowInput {
  name: string
  description?: string
  trigger_type?: string
  timer?: string
  event_type?: string
  event_conditions?: EventCondition[]
  enabled?: boolean
  nodes: GraphNode[]
  edges: GraphEdge[]
}

// 工作流模板（GET /workflows/templates）。
export interface WorkflowTemplate {
  key: string
  name: string
  description: string
  category: string
  trigger_type: string
  timer: string
  event_type?: string
  event_conditions?: EventCondition[]
  nodes: GraphNode[]
  edges: GraphEdge[]
}

// 运行统计（GET /workflows/stats）。
export interface NodeTiming {
  node_type: string
  node_name: string
  count: number
  total_ms: number
  avg_ms: number
  failed: number
}
export interface WorkflowStats {
  total: number
  enabled: number
  runs_total: number
  runs_success: number
  runs_failed: number
  avg_duration_ms: number
  recent_failures: WorkflowRun[]
  node_timing: NodeTiming[]
}

export const workflowApi = {
  list: () => http.get<{ workflows: Workflow[] }>('/workflows').then((r) => r.data.workflows ?? []),
  get: (id: number) => http.get<Workflow>(`/workflows/${id}`).then((r) => r.data),
  actions: () => http.get<{ actions: NodeInfo[] }>('/workflows/actions').then((r) => r.data.actions ?? []),
  templates: () =>
    http.get<{ templates: WorkflowTemplate[] }>('/workflows/templates').then((r) => r.data.templates ?? []),
  stats: () => http.get<WorkflowStats>('/workflows/stats').then((r) => r.data),
  create: (data: WorkflowInput) => http.post<Workflow>('/workflows', data).then((r) => r.data),
  update: (id: number, data: WorkflowInput) => http.put<Workflow>(`/workflows/${id}`, data).then((r) => r.data),
  remove: (id: number) => http.delete(`/workflows/${id}`).then((r) => r.data),
  run: (id: number) =>
    http.post<{ ok: boolean; run?: WorkflowRun; error?: string }>(`/workflows/${id}/run`).then((r) => r.data),
  enable: (id: number) => http.post(`/workflows/${id}/enable`).then((r) => r.data),
  disable: (id: number) => http.post(`/workflows/${id}/disable`).then((r) => r.data),
  reset: (id: number) => http.post(`/workflows/${id}/reset`).then((r) => r.data),
  runs: (workflowId: number, limit = 20) =>
    http
      .get<{ runs: WorkflowRun[]; total?: number; limit?: number; truncated?: boolean }>('/workflow-runs', {
        params: { workflow_id: workflowId, limit },
      })
      .then((r) => ({
        runs: r.data.runs ?? [],
        total: r.data.total,
        limit: r.data.limit ?? limit,
        // 缺字段按「可能被截断」处理：不把一页说成整个运行历史。
        truncated: r.data.truncated ?? true,
      })),
  runDetail: (rid: number) => http.get<WorkflowRun>(`/workflow-runs/${rid}`).then((r) => r.data),
}
