// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { http } from './http'
import { streamErr } from './sse'

// 对话会话（对齐后端 models.AgentConversation）。
export interface AgentConversation {
  id: number
  title: string
  created_at: string
  updated_at: string
}

// 单条消息（含思考 + 工具调用）。
export interface AgentMessage {
  id: number
  conversation_id: number
  role: 'user' | 'assistant' | 'tool'
  content: string
  reasoning?: string
  tool_calls?: string // JSON: [{id,name,arguments}]
  tool_call_id?: string
  name?: string
  created_at: string
}

// provider 配置（脱敏：api_key 仅 _configured）。
export interface AgentConfig {
  enabled: boolean
  provider: 'openai' | 'anthropic'
  base_url: string
  model: string
  temperature: number
  max_tokens: number
  enable_thinking: boolean
  max_iterations: number
  system_prompt: string
  enabled_tools: string[]
  api_key_configured: boolean
}

// 工具目录条目（含 read/write/destructive 风险等级，供白名单 UI 分组）。
export interface ToolCatalogEntry {
  name: string
  description: string
  risk: 'read' | 'write' | 'destructive'
  risk_note: string
}

// 用量统计快照（对齐后端 aiagent.StatsSnapshot）。
export interface AgentUsageRecord {
  conversation_id: number
  provider: string
  model: string
  input_tokens: number
  output_tokens: number
  cost_usd: number
  duration_ms: number
  status: string
  tool_calls: number
  tool_errors: number
  timestamp: string
}
export interface AgentStats {
  total_requests: number
  total_input_tokens: number
  total_output_tokens: number
  total_cost_usd: number
  ok_count: number
  error_count: number
  rate_limit_count: number
  cancelled_count: number
  max_iter_count: number
  avg_latency_ms: number
  tool_calls: number
  tool_errors: number
  recent: AgentUsageRecord[]
}

// SSE 事件（与后端 writeSSE 对齐）。
export type AgentEvent =
  | { event: 'conversation'; data: { id: number } }
  | { event: 'token'; data: { content: string } }
  | { event: 'reasoning'; data: { content: string } }
  | { event: 'tool_call'; data: { id: string; name: string; arguments: string } }
  | { event: 'tool_result'; data: { id: string; name: string; result: string } }
  | {
      event: 'confirm_request'
      data: { id: string; name: string; arguments: string; token: string; risk: string; summary: string }
    }
  | { event: 'done'; data: Record<string, never> }
  | { event: 'error'; data: { message: string } }

export const agentApi = {
  conversationsPage: () =>
    http
      .get<{ conversations: AgentConversation[]; total?: number; limit?: number; truncated?: boolean }>(
        '/agent/conversations',
      )
      .then((r) => ({
        conversations: r.data.conversations ?? [],
        total: r.data.total,
        limit: r.data.limit ?? 0,
        // 缺字段按「可能被截断」处理：单页 200 个会话封顶，别把一页说成建过的全部会话。
        truncated: r.data.truncated ?? true,
      })),
  conversations: () => agentApi.conversationsPage().then((p) => p.conversations),
  messages: (id: number) =>
    http.get<{ messages: AgentMessage[] }>(`/agent/conversations/${id}/messages`).then((r) => r.data.messages ?? []),
  deleteConversation: (id: number) => http.delete(`/agent/conversations/${id}`).then((r) => r.data),
  // 工具目录（含风险等级），供白名单 UI 分组勾选。
  tools: () => http.get<{ tools: ToolCatalogEntry[] }>('/agent/tools').then((r) => r.data.tools ?? []),
  // 用量统计快照（请求数/Token/费用/延迟/错误/工具成功率）。
  stats: () => http.get<AgentStats>('/agent/stats').then((r) => r.data),
  // 危险工具二次确认决断：convId 走 URL，tool_call_id（= confirm_request.data.id）走 body。
  confirmDecision: (convId: number, toolCallId: string, approved: boolean) =>
    http.post(`/agent/conversations/${convId}/confirm`, { tool_call_id: toolCallId, approved }).then((r) => r.data),
  getConfig: () => http.get<AgentConfig>('/agent/config').then((r) => r.data),
  setConfig: (cfg: Partial<AgentConfig> & { api_key?: string }) => http.put('/agent/config', cfg).then((r) => r.data),
}

// chat 走原生 fetch + ReadableStream（axios 不支持流式；需 POST + Authorization header）。
// onEvent 收每条 SSE 事件；signal 用于「停止」中断。返回会话 id。
export async function streamAgentChat(
  message: string,
  conversationId: number | null,
  onEvent: (e: AgentEvent) => void,
  signal?: AbortSignal,
): Promise<number | null> {
  const token = localStorage.getItem('token') || ''
  const resp = await fetch('/api/v1/agent/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: token ? `Bearer ${token}` : '' },
    body: JSON.stringify({ conversation_id: conversationId ?? 0, message }),
    signal,
  })
  if (!resp.ok || !resp.body) {
    throw new Error(await streamErr(resp))
  }
  const reader = resp.body.getReader()
  const dec = new TextDecoder()
  let buf = ''
  let convId: number | null = null
  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buf += dec.decode(value, { stream: true })
    // SSE 帧以空行分隔
    let boundary: RegExpMatchArray | null
    while ((boundary = buf.match(/\r?\n\r?\n/)) && boundary.index !== undefined) {
      const frame = buf.slice(0, boundary.index)
      buf = buf.slice(boundary.index + boundary[0].length)
      const ev = parseSSEFrame(frame)
      if (ev) {
        if (ev.event === 'conversation') convId = ev.data.id
        onEvent(ev)
      }
    }
  }
  return convId
}

function parseSSEFrame(frame: string): AgentEvent | null {
  let event = 'message'
  const dataLines: string[] = []
  for (const line of frame.split(/\r?\n/)) {
    if (line.startsWith('event:')) event = line.slice(6).trim()
    else if (line.startsWith('data:')) dataLines.push(line.slice(5).trim())
  }
  const raw = dataLines.join('\n')
  if (!raw) return null
  try {
    return { event, data: JSON.parse(raw) } as AgentEvent
  } catch {
    return null
  }
}
