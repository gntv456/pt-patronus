import { ref, computed, nextTick, onUnmounted } from 'vue'
import { agentApi, streamAgentChat, type AgentConversation, type AgentMessage } from '@/api/agent'

// 渲染分组类型：把 user/assistant/tool 还原成展示单元。
export interface ToolStep {
  kind: 'tool'
  calls: { name: string; arguments: string; result?: string }[]
  reasoning?: string
}
export interface Bubble {
  kind: 'user' | 'assistant'
  content: string
  reasoning?: string
}
export type Unit = { id: string; data: Bubble | ToolStep }

// AI 助手会话与流式聊天逻辑：会话列表、消息加载、渲染分组、流式发送/停止、危险工具二次确认。
// 抽出为 composable 收敛 Assistant.vue 脚本体积。
// 需传入 scroller 的 ref getter 以实现自动滚动到底；ElMessage / ElMessageBox 依赖 unplugin-auto-import。
export function useAgentChat(getScroller: () => HTMLElement | null) {
  // —— 会话 ——
  const conversations = ref<AgentConversation[]>([])
  const activeId = ref<number | null>(null)
  const messages = ref<AgentMessage[]>([])
  const loadingMsgs = ref(false)

  function scrollToBottom() {
    const el = getScroller()
    if (el) el.scrollTop = el.scrollHeight
  }

  async function loadConversations() {
    try {
      conversations.value = await agentApi.conversations()
      if (!activeId.value && conversations.value.length) await selectConversation(conversations.value[0].id)
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '加载会话失败')
    }
  }
  async function selectConversation(id: number) {
    activeId.value = id
    loadingMsgs.value = true
    try {
      messages.value = await agentApi.messages(id)
      await nextTick()
      scrollToBottom()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '加载消息失败')
    } finally {
      loadingMsgs.value = false
    }
  }
  function newConversation() {
    activeId.value = null
    messages.value = []
    pending.value = null
  }
  async function deleteConversation(id: number) {
    try {
      await ElMessageBox.confirm('删除该会话？', '确认', { type: 'warning' })
    } catch {
      return
    }
    await agentApi.deleteConversation(id)
    if (activeId.value === id) newConversation()
    await loadConversations()
  }

  // —— 渲染分组：把 user/assistant/tool 还原成展示单元 ——
  function safeParse(s: string) {
    try {
      return JSON.parse(s)
    } catch {
      return []
    }
  }
  const units = computed<Unit[]>(() => {
    const out: Unit[] = []
    let curTool: ToolStep | null = null
    for (const m of messages.value) {
      if (m.role === 'user') {
        curTool = null
        out.push({ id: 'u' + m.id, data: { kind: 'user', content: m.content } })
      } else if (m.role === 'assistant') {
        const tcs = m.tool_calls ? safeParse(m.tool_calls) : []
        if (m.content && m.content.trim()) {
          curTool = null
          out.push({ id: 'a' + m.id, data: { kind: 'assistant', content: m.content, reasoning: m.reasoning } })
        } else if (tcs.length) {
          const step: ToolStep = {
            kind: 'tool',
            calls: tcs.map((t: any) => ({ name: t.name, arguments: t.arguments })),
            reasoning: m.reasoning,
          }
          out.push({ id: 't' + m.id, data: step })
          curTool = step
        }
      } else if (m.role === 'tool' && curTool) {
        const c = curTool.calls.find((c) => !c.result)
        if (c) c.result = m.content
      }
    }
    return out
  })

  // —— 流式发送 ——
  const input = ref('')
  const sending = ref(false)
  const pending = ref<Unit | null>(null) // { kind:'assistant', 实时累积 }
  let abortCtrl: AbortController | null = null

  async function send() {
    const text = input.value.trim()
    if (!text || sending.value) return
    input.value = ''
    // 即时显示用户气泡 + 空 assistant 占位
    messages.value = [...messages.value, { id: -1, conversation_id: 0, role: 'user', content: text, created_at: '' }]
    pending.value = { id: 'live', data: { kind: 'assistant', content: '', reasoning: '' } }
    sending.value = true
    abortCtrl = new AbortController()
    let liveContent = ''
    let liveReasoning = ''
    const liveTools: { name: string; arguments: string; result?: string }[] = []
    try {
      const convId = await streamAgentChat(
        text,
        activeId.value,
        (ev) => {
          if (ev.event === 'conversation') activeId.value = ev.data.id
          else if (ev.event === 'token') {
            liveContent += ev.data.content
            updatePending(liveContent, liveReasoning, liveTools)
          } else if (ev.event === 'reasoning') {
            liveReasoning += ev.data.content
            updatePending(liveContent, liveReasoning, liveTools)
          } else if (ev.event === 'tool_call') {
            liveTools.push({ name: ev.data.name, arguments: ev.data.arguments })
            updatePending(liveContent, liveReasoning, liveTools)
          } else if (ev.event === 'tool_result') {
            const t = liveTools.find((x) => x.name === ev.data.name && !x.result)
            if (t) t.result = ev.data.result
            updatePending(liveContent, liveReasoning, liveTools)
          } else if (ev.event === 'confirm_request') handleConfirm(ev.data)
          else if (ev.event === 'error') ElMessage.error(ev.data.message)
          nextTick(scrollToBottom)
        },
        abortCtrl.signal,
      )
      if (convId) activeId.value = convId
      pending.value = null
      if (activeId.value) await selectConversation(activeId.value) // 用 DB 规范化视图替换实时块
      await loadConversations()
    } catch (e: unknown) {
      if ((e as Error).name !== 'AbortError') ElMessage.error((e as Error).message || '发送失败')
    } finally {
      sending.value = false
      abortCtrl = null
    }
  }
  function updatePending(
    content: string,
    reasoning: string,
    tools: { name: string; arguments: string; result?: string }[],
  ) {
    if (!pending.value) return
    const d = pending.value.data as Bubble
    d.content = content
    d.reasoning = reasoning
    // 工具暂以折叠提示嵌在 reasoning 后（DB 规范化后正式展示）
    if (tools.length && !content) d.content = ''
    pending.value = { ...pending.value }
  }
  function stop() {
    abortCtrl?.abort()
  }

  // 危险工具二次确认：收到 confirm_request 时弹窗，用户选择后 POST 决断。
  // streamAgentChat 的 reader.read() 在 await 弹窗期间自然阻塞——后端在 Resolve 前不会下发后续帧，
  // 故无需手动暂停/恢复流消费；确认后同一 SSE 连接继续推送 tool_result。
  async function handleConfirm(d: { id: string; name: string; arguments: string; summary: string }) {
    const args = d.arguments && d.arguments !== '{}' ? `\n参数：${d.arguments}` : ''
    let approved = false
    try {
      await ElMessageBox.confirm(
        `${d.summary || 'AI 想执行一个不可逆操作'}\n工具：${d.name}${args}`,
        '⚠️ 危险操作确认',
        { type: 'warning', confirmButtonText: '允许执行', cancelButtonText: '拒绝', distinguishCancelAndClose: true },
      )
      approved = true
    } catch {
      approved = false // 用户点「拒绝」或关闭弹窗 → 均视为拒绝
    }
    try {
      if (activeId.value) await agentApi.confirmDecision(activeId.value, d.id, approved)
    } catch (e: unknown) {
      ElMessage.error('确认回传失败：' + ((e as Error).message || ''))
    }
  }

  onUnmounted(() => abortCtrl?.abort())

  return {
    conversations,
    activeId,
    messages,
    loadingMsgs,
    units,
    input,
    sending,
    pending,
    loadConversations,
    selectConversation,
    newConversation,
    deleteConversation,
    send,
    stop,
  }
}
