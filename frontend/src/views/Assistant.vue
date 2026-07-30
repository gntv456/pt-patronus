<script setup lang="ts">
import { ref, computed, nextTick, onMounted, onUnmounted } from 'vue'
import { agentApi, streamAgentChat, type AgentConversation, type AgentMessage, type AgentConfig, type ToolCatalogEntry, type AgentStats } from '@/api/agent'
import EmptyState from '@/components/EmptyState.vue'

// —— 会话 ——
const conversations = ref<AgentConversation[]>([])
const activeId = ref<number | null>(null)
const messages = ref<AgentMessage[]>([])
const loadingMsgs = ref(false)

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
interface ToolStep { kind: 'tool'; calls: { name: string; arguments: string; result?: string }[]; reasoning?: string }
interface Bubble { kind: 'user' | 'assistant'; content: string; reasoning?: string }
type Unit = { id: string; data: Bubble | ToolStep }
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
        const step: ToolStep = { kind: 'tool', calls: tcs.map((t: any) => ({ name: t.name, arguments: t.arguments })), reasoning: m.reasoning }
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
    const convId = await streamAgentChat(text, activeId.value, (ev) => {
      if (ev.event === 'conversation') activeId.value = ev.data.id
      else if (ev.event === 'token') { liveContent += ev.data.content; updatePending(liveContent, liveReasoning, liveTools) }
      else if (ev.event === 'reasoning') { liveReasoning += ev.data.content; updatePending(liveContent, liveReasoning, liveTools) }
      else if (ev.event === 'tool_call') { liveTools.push({ name: ev.data.name, arguments: ev.data.arguments }); updatePending(liveContent, liveReasoning, liveTools) }
      else if (ev.event === 'tool_result') { const t = liveTools.find((x) => x.name === ev.data.name && !x.result); if (t) t.result = ev.data.result; updatePending(liveContent, liveReasoning, liveTools) }
      else if (ev.event === 'confirm_request') handleConfirm(ev.data)
      else if (ev.event === 'error') ElMessage.error(ev.data.message)
      nextTick(scrollToBottom)
    }, abortCtrl.signal)
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
function updatePending(content: string, reasoning: string, tools: { name: string; arguments: string; result?: string }[]) {
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

// —— 配置 ——
const showConfig = ref(false)
const cfg = ref<AgentConfig>(emptyCfg())
const apiKeyInput = ref('')
const savingCfg = ref(false)
// 用量统计（配置弹窗展示；本次运行以来累计，重启清零）。
const stats = ref<AgentStats | null>(null)
const toolSuccessRate = computed(() => {
  if (!stats.value || stats.value.tool_calls === 0) return '—'
  return Math.round((1 - stats.value.tool_errors / stats.value.tool_calls) * 100)
})
// 工具白名单：toolAllowAll=true → enabled_tools=[]（全开，新增工具自动可用）；false → 仅勾选的可用。
const allTools = ref<ToolCatalogEntry[]>([])
const toolAllowAll = ref(true)
const enabledSet = ref<Set<string>>(new Set())
const toolSections = computed(() => {
  const destructive: ToolCatalogEntry[] = []
  const write: ToolCatalogEntry[] = []
  const read: ToolCatalogEntry[] = []
  for (const t of allTools.value) {
    if (t.risk === 'destructive') destructive.push(t)
    else if (t.risk === 'read') read.push(t)
    else write.push(t)
  }
  return [
    { title: '⚠️ 危险操作（执行前仍需二次确认）', cls: 'danger', items: destructive },
    { title: '写入（变更可逆状态）', cls: '', items: write },
    { title: '只读查询', cls: '', items: read },
  ].filter((s) => s.items.length > 0)
})
function toggleTool(name: string, checked: boolean) {
  const s = new Set(enabledSet.value)
  if (checked) s.add(name)
  else s.delete(name)
  enabledSet.value = s
}
function onToggleAllowAll(v: boolean) {
  toolAllowAll.value = v
  // 从「全开」切到「自选」时，默认全部勾上，让用户从全启用开始按需收窄。
  if (!v && enabledSet.value.size === 0) enabledSet.value = new Set(allTools.value.map((t) => t.name))
}
function emptyCfg(): AgentConfig {
  return { enabled: false, provider: 'openai', base_url: 'https://api.openai.com/v1', model: 'gpt-4o-mini', temperature: 0.3, max_tokens: 2048, enable_thinking: true, max_iterations: 12, system_prompt: '', enabled_tools: [], api_key_configured: false }
}
async function openConfig() {
  try { cfg.value = await agentApi.getConfig() } catch { cfg.value = emptyCfg() }
  apiKeyInput.value = ''
  try { allTools.value = await agentApi.tools() } catch { allTools.value = [] }
  try { stats.value = await agentApi.stats() } catch { stats.value = null }
  // enabled_tools 为空 = 全开；否则按显式清单勾选。
  if (cfg.value.enabled_tools.length === 0) {
    toolAllowAll.value = true
    enabledSet.value = new Set(allTools.value.map((t) => t.name))
  } else {
    toolAllowAll.value = false
    enabledSet.value = new Set(cfg.value.enabled_tools)
  }
  showConfig.value = true
}
async function saveConfig() {
  savingCfg.value = true
  try {
    const payload: any = { ...cfg.value, enabled_tools: toolAllowAll.value ? [] : [...enabledSet.value] }
    if (apiKeyInput.value.trim()) payload.api_key = apiKeyInput.value.trim()
    await agentApi.setConfig(payload)
    ElMessage.success('配置已保存')
    showConfig.value = false
  } catch (e: unknown) { ElMessage.error((e as Error).message || '保存失败') } finally { savingCfg.value = false }
}

// —— 工具 ——
function safeParse(s: string) { try { return JSON.parse(s) } catch { return [] } }
const scroller = ref<HTMLElement | null>(null)
function scrollToBottom() { if (scroller.value) scroller.value.scrollTop = scroller.value.scrollHeight }
function onKey(e: KeyboardEvent) { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send() } }

onMounted(loadConversations)
onUnmounted(() => abortCtrl?.abort())
</script>

<template>
  <div class="page">
    <div class="layout">
      <!-- 会话侧栏 -->
      <aside class="glass sidebar">
        <div class="side-head">
          <span>会话</span>
          <div class="side-actions">
            <el-button size="small" type="primary" @click="newConversation">✨ 新对话</el-button>
            <el-button size="small" @click="openConfig">⚙</el-button>
          </div>
        </div>
        <div class="conv-list">
          <div v-for="c in conversations" :key="c.id" class="conv-item" :class="{ active: c.id === activeId }" @click="selectConversation(c.id)">
            <span class="conv-title">{{ c.title || '(新对话)' }}</span>
            <el-button class="conv-del" size="small" link @click.stop="deleteConversation(c.id)">🗑</el-button>
          </div>
          <EmptyState v-if="!conversations.length" icon="message" title="暂无会话" />
        </div>
      </aside>

      <!-- 对话区 -->
      <section class="glass chat">
        <div ref="scroller" class="msg-list">
          <template v-if="!units.length && !pending">
            <EmptyState icon="message" title="和 PTPatronus 助手对话" sub="问站点统计、搜影视、订阅、整理下载…自然语言即可" />
          </template>
          <template v-for="u in units" :key="u.id">
            <!-- 用户 -->
            <div v-if="u.data.kind === 'user'" class="row user"><div class="bubble user-bubble">{{ u.data.content }}</div></div>
            <!-- 助手 -->
            <div v-else-if="u.data.kind === 'assistant'" class="row assistant">
              <div class="bubble assistant-bubble">
                <details v-if="u.data.reasoning" class="reasoning"><summary>🧠 思考过程</summary><pre>{{ u.data.reasoning }}</pre></details>
                <div class="content">{{ u.data.content }}</div>
              </div>
            </div>
            <!-- 工具步骤 -->
            <div v-else class="row assistant">
              <div class="tool-step">
                <details>
                  <summary>🔧 调用 {{ (u.data as any).calls.length }} 个工具</summary>
                  <div v-for="(t, i) in (u.data as any).calls" :key="i" class="tool-item">
                    <div class="tool-name">{{ t.name }}</div>
                    <pre class="tool-args">{{ t.arguments }}</pre>
                    <pre v-if="t.result" class="tool-result">{{ t.result }}</pre>
                  </div>
                </details>
              </div>
            </div>
          </template>
          <!-- 实时 -->
          <div v-if="pending" class="row assistant">
            <div class="bubble assistant-bubble">
              <details v-if="(pending.data as any).reasoning" class="reasoning"><summary>🧠 思考过程</summary><pre>{{ (pending.data as any).reasoning }}</pre></details>
              <div class="content">{{ (pending.data as any).content || '…' }}</div>
            </div>
          </div>
        </div>
        <div class="composer">
          <textarea v-model="input" class="input" rows="2" placeholder="输入指令…（Enter 发送，Shift+Enter 换行）" @keydown="onKey" :disabled="sending" />
          <el-button v-if="!sending" type="primary" @click="send" :disabled="!input.trim()">发送</el-button>
          <el-button v-else type="danger" @click="stop">停止</el-button>
        </div>
      </section>
    </div>

    <!-- 配置 -->
    <el-dialog v-model="showConfig" title="AI 助手设置" width="560px">
      <el-form :model="cfg" label-width="120px" size="small">
        <el-form-item v-if="stats" label="用量统计">
          <div class="stats-grid">
            <div class="stat"><span class="stat-num">{{ stats.total_requests }}</span><span class="stat-label">请求</span></div>
            <div class="stat"><span class="stat-num">{{ stats.total_input_tokens + stats.total_output_tokens }}</span><span class="stat-label">Token</span></div>
            <div class="stat"><span class="stat-num">${{ stats.total_cost_usd.toFixed(4) }}</span><span class="stat-label">费用</span></div>
            <div class="stat"><span class="stat-num">{{ stats.avg_latency_ms }}ms</span><span class="stat-label">均延迟</span></div>
            <div class="stat"><span class="stat-num">{{ stats.error_count }}</span><span class="stat-label">错误</span></div>
            <div class="stat"><span class="stat-num">{{ stats.rate_limit_count }}</span><span class="stat-label">限流</span></div>
            <div class="stat"><span class="stat-num">{{ stats.tool_calls }}</span><span class="stat-label">工具调用</span></div>
            <div class="stat"><span class="stat-num">{{ toolSuccessRate }}%</span><span class="stat-label">工具成功率</span></div>
          </div>
        </el-form-item>
        <el-form-item label="启用"><el-switch v-model="cfg.enabled" /></el-form-item>
        <el-form-item label="Provider">
          <el-select v-model="cfg.provider" style="width:100%">
            <el-option label="OpenAI 兼容（OpenAI/DeepSeek/Qwen/Ollama/中转）" value="openai" />
            <el-option label="Anthropic Claude（原生）" value="anthropic" />
          </el-select>
        </el-form-item>
        <el-form-item label="API Key">
          <el-input v-model="apiKeyInput" type="password" show-password :placeholder="cfg.api_key_configured ? '已配置（不修改请留空）' : '请输入 API key'" />
        </el-form-item>
        <el-form-item label="Base URL"><el-input v-model="cfg.base_url" placeholder="https://api.openai.com/v1" /></el-form-item>
        <el-form-item label="模型"><el-input v-model="cfg.model" placeholder="gpt-4o-mini / deepseek-chat / claude-sonnet-5" /></el-form-item>
        <el-form-item label="深度思考"><el-switch v-model="cfg.enable_thinking" /><span class="hint">推理模型显示思考过程</span></el-form-item>
        <el-form-item label="Temperature"><el-input-number v-model="cfg.temperature" :min="0" :max="2" :step="0.1" /></el-form-item>
        <el-form-item label="Max Tokens"><el-input-number v-model="cfg.max_tokens" :min="256" :step="256" /></el-form-item>
        <el-form-item label="最大思考轮数"><el-input-number v-model="cfg.max_iterations" :min="1" :max="30" /></el-form-item>
        <el-form-item label="系统提示词"><el-input v-model="cfg.system_prompt" type="textarea" :rows="4" /></el-form-item>
        <el-form-item label="工具权限">
          <div class="tool-perms">
            <el-switch :model-value="toolAllowAll" @update:model-value="(v) => onToggleAllowAll(v as boolean)" />
            <span class="hint">启用全部工具（不限制；新增工具自动可用）</span>
            <div v-if="!toolAllowAll" class="tool-groups">
              <div v-for="(sec, i) in toolSections" :key="i" class="tool-group">
                <div class="tool-group-title" :class="sec.cls">{{ sec.title }}</div>
                <el-checkbox
                  v-for="t in sec.items"
                  :key="t.name"
                  :model-value="enabledSet.has(t.name)"
                  @update:model-value="(v) => toggleTool(t.name, v as boolean)"
                >
                  <span class="tool-label">{{ t.name }}</span>
                  <span class="tool-desc">{{ t.risk === 'destructive' ? (t.risk_note || t.description) : t.description }}</span>
                </el-checkbox>
              </div>
            </div>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showConfig = false">取消</el-button>
        <el-button type="primary" :loading="savingCfg" @click="saveConfig">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
/* 填满 .content 可用高度（随 topbar / GlobalNoticeBar 显隐 / 移动端自适应），
   不能写死 calc(100vh - Npx)：通知栏 v-if 条件渲染会改变下方可用高度，
   固定值会导致输入框被顶到视口外。box-sizing 让 padding 留在高度内。 */
.page { padding: 12px; height: 100%; box-sizing: border-box; }
.layout { display: flex; gap: 12px; height: 100%; }
.sidebar { width: 240px; flex-shrink: 0; display: flex; flex-direction: column; padding: 10px; border-radius: 14px; }
.side-head { display: flex; justify-content: space-between; align-items: center; font-weight: 700; margin-bottom: 8px; }
.side-actions { display: flex; gap: 6px; }
.conv-list { flex: 1; overflow-y: auto; }
.conv-item { display: flex; align-items: center; justify-content: space-between; padding: 8px 10px; border-radius: 8px; cursor: pointer; font-size: 13px; }
.conv-item:hover { background: var(--glass-hover, rgba(127,127,127,0.12)); }
.conv-item.active { background: var(--glass-active, rgba(108,92,231,0.2)); }
.conv-title { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.chat { flex: 1; display: flex; flex-direction: column; padding: 12px; border-radius: 14px; min-width: 0; }
.msg-list { flex: 1; overflow-y: auto; padding: 4px; }
.row { display: flex; margin-bottom: 12px; }
.row.user { justify-content: flex-end; }
.row.assistant { justify-content: flex-start; }
.bubble { max-width: 78%; padding: 10px 14px; border-radius: 14px; font-size: 14px; line-height: 1.6; white-space: pre-wrap; word-break: break-word; }
.user-bubble { background: linear-gradient(135deg, #6c5ce7, #4285f4); color: #fff; }
.assistant-bubble { background: var(--glass-bubble, rgba(127,127,127,0.14)); }
.reasoning { font-size: 12px; opacity: 0.75; margin-bottom: 6px; }
.reasoning summary { cursor: pointer; }
.reasoning pre { white-space: pre-wrap; margin: 6px 0 0; max-height: 200px; overflow-y: auto; }
.content:empty { display: none; }
.tool-step { max-width: 78%; }
.tool-step details { background: var(--glass-bubble, rgba(127,127,127,0.1)); border-radius: 10px; padding: 6px 10px; font-size: 12px; }
.tool-step summary { cursor: pointer; opacity: 0.85; }
.tool-item { border-top: 1px solid rgba(127,127,127,0.2); padding: 6px 0; }
.tool-name { font-weight: 600; color: #4285f4; }
.tool-args, .tool-result { white-space: pre-wrap; margin: 4px 0; font-size: 11px; max-height: 160px; overflow-y: auto; }
.composer { display: flex; gap: 8px; align-items: flex-end; margin-top: 8px; }
.input { flex: 1; resize: none; padding: 10px; border-radius: 10px; background: var(--glass-input, rgba(127,127,127,0.1)); border: 1px solid rgba(127,127,127,0.25); color: inherit; font-family: inherit; font-size: 14px; }
.input:focus { outline: none; border-color: #6c5ce7; }
.hint { margin-left: 10px; font-size: 12px; opacity: 0.6; }
.tool-perms { width: 100%; }
.stats-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 6px; width: 100%; }
.stat { display: flex; flex-direction: column; align-items: center; background: var(--glass-bubble, rgba(127,127,127,0.1)); border-radius: 8px; padding: 6px 2px; }
.stat-num { font-size: 14px; font-weight: 700; }
.stat-label { font-size: 11px; opacity: 0.6; }
.tool-groups { margin-top: 8px; max-height: 220px; overflow-y: auto; border: 1px solid rgba(127,127,127,0.2); border-radius: 8px; padding: 4px 8px; }
.tool-group { padding: 4px 0; }
.tool-group + .tool-group { border-top: 1px solid rgba(127,127,127,0.15); margin-top: 4px; }
.tool-group-title { font-size: 12px; font-weight: 600; opacity: 0.85; margin: 4px 0; }
.tool-group-title.danger { color: #e8553a; }
.tool-groups :deep(.el-checkbox) { display: flex; align-items: flex-start; margin: 2px 0; height: auto; }
.tool-label { font-weight: 600; font-size: 12px; }
.tool-desc { display: block; font-size: 11px; opacity: 0.55; margin-left: 24px; line-height: 1.3; }
</style>
