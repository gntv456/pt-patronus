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
  // 侧栏会话清单：后端单页 200 个封顶，被截断时要说出来。
  const convWindow = ref<{ shown: number; total?: number; truncated: boolean }>({
    shown: 0,
    total: undefined,
    truncated: false,
  })
  const activeId = ref<number | null>(null)
  const messages = ref<AgentMessage[]>([])
  const loadingMsgs = ref(false)

  function scrollToBottom() {
    const el = getScroller()
    if (el) el.scrollTop = el.scrollHeight
  }

  async function loadConversations() {
    try {
      const page = await agentApi.conversationsPage()
      conversations.value = page.conversations
      convWindow.value = {
        shown: page.conversations.length,
        total: page.total,
        truncated: page.truncated,
      }
      if (!activeId.value && conversations.value.length) await selectConversation(conversations.value[0].id)
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '加载会话失败')
    }
  }
  // 会话消息请求序号：快速连续切会话时丢弃过期响应，避免旧会话消息串进新会话。
  let msgSeq = 0
  async function selectConversation(id: number) {
    activeId.value = id
    const seq = ++msgSeq
    loadingMsgs.value = true
    try {
      const msgs = await agentApi.messages(id)
      if (seq !== msgSeq || activeId.value !== id) return
      messages.value = msgs
      await nextTick()
      scrollToBottom()
    } catch (e: unknown) {
      if (seq !== msgSeq || activeId.value !== id) return
      ElMessage.error((e as Error).message || '加载消息失败')
    } finally {
      if (seq === msgSeq) loadingMsgs.value = false
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
    try {
      await agentApi.deleteConversation(id)
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '删除会话失败')
      return
    }
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
    let liveError = ''
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
          else if (ev.event === 'error') {
            liveError = ev.data.message || '本轮未完成'
            ElMessage.error(liveError)
          }
          nextTick(scrollToBottom)
        },
        abortCtrl.signal,
      )
      if (convId) activeId.value = convId
    } catch (e: unknown) {
      // 「停止」是用户自己的动作，不弹 toast，但仍要在记录里留一行——
      // 否则实时气泡会永远停在「…」，界面谎报这一轮还在生成。
      if ((e as Error).name === 'AbortError') liveError = '本轮已停止'
      else {
        liveError = (e as Error).message || '发送失败'
        ElMessage.error(liveError)
      }
    } finally {
      sending.value = false
      abortCtrl = null
    }
    pending.value = null
    const conv = activeId.value
    if (conv) await selectConversation(conv) // 用 DB 规范化视图替换实时块
    await loadConversations()
    // 出错那一轮后端不落助手消息，刷新后记录里只剩用户那句：用户看不出这轮跑过、
    // 也不知道为什么没结果，重发还可能再触发一次写工具。把原因钉进会话记录。
    // 放在清单刷新之后：侧栏可能自动切到别的会话，切走了就不该把这条塞进别人的记录。
    if (liveError && activeId.value === conv) {
      messages.value = [
        ...messages.value,
        {
          id: -2,
          conversation_id: conv ?? 0,
          role: 'assistant',
          content: '⚠ ' + liveError,
          created_at: new Date().toISOString(),
        },
      ]
      await nextTick()
      scrollToBottom()
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
    convWindow,
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
