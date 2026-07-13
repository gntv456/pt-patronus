import { http } from './http'

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

// SSE 事件（与后端 writeSSE 对齐）。
export type AgentEvent =
  | { event: 'conversation'; data: { id: number } }
  | { event: 'token'; data: { content: string } }
  | { event: 'reasoning'; data: { content: string } }
  | { event: 'tool_call'; data: { id: string; name: string; arguments: string } }
  | { event: 'tool_result'; data: { id: string; name: string; result: string } }
  | { event: 'done'; data: Record<string, never> }
  | { event: 'error'; data: { message: string } }

export const agentApi = {
  conversations: () =>
    http.get<{ conversations: AgentConversation[] }>('/agent/conversations').then((r) => r.data.conversations ?? []),
  messages: (id: number) =>
    http.get<{ messages: AgentMessage[] }>(`/agent/conversations/${id}/messages`).then((r) => r.data.messages ?? []),
  deleteConversation: (id: number) => http.delete(`/agent/conversations/${id}`).then((r) => r.data),
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
    const txt = await resp.text().catch(() => '')
    throw new Error(`HTTP ${resp.status}: ${txt.slice(0, 200)}`)
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
    let idx: number
    while ((idx = buf.indexOf('\n\n')) >= 0) {
      const frame = buf.slice(0, idx)
      buf = buf.slice(idx + 2)
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
  for (const line of frame.split('\n')) {
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
