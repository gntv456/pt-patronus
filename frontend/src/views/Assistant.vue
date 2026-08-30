<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { agentApi, type AgentConfig, type ToolCatalogEntry, type AgentStats } from '@/api/agent'
import { useAgentChat, type Unit } from '@/composables/useAgentChat'
import EmptyState from '@/components/EmptyState.vue'

const { t } = useI18n()
const scroller = ref<HTMLElement | null>(null)
// 会话与流式聊天逻辑抽出为 composable（useAgentChat）。
const {
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
} = useAgentChat(() => scroller.value)

function onKey(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    send()
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
    { title: t('assistant.tools.destructive'), cls: 'danger', items: destructive },
    { title: t('assistant.tools.write'), cls: '', items: write },
    { title: t('assistant.tools.read'), cls: '', items: read },
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
  return {
    enabled: false,
    provider: 'openai',
    base_url: 'https://api.openai.com/v1',
    model: 'gpt-4o-mini',
    temperature: 0.3,
    max_tokens: 2048,
    enable_thinking: true,
    max_iterations: 12,
    system_prompt: '',
    enabled_tools: [],
    api_key_configured: false,
  }
}
async function openConfig() {
  try {
    cfg.value = await agentApi.getConfig()
  } catch {
    cfg.value = emptyCfg()
  }
  apiKeyInput.value = ''
  try {
    allTools.value = await agentApi.tools()
  } catch {
    allTools.value = []
  }
  try {
    stats.value = await agentApi.stats()
  } catch {
    stats.value = null
  }
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
    ElMessage.success(t('assistant.configSaved'))
    showConfig.value = false
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('common.saveFailed'))
  } finally {
    savingCfg.value = false
  }
}

onMounted(loadConversations)
</script>

<template>
  <div class="page">
    <div class="layout">
      <!-- 会话侧栏 -->
      <aside class="glass sidebar">
        <div class="side-head">
          <span>{{ t('assistant.conversations') }}</span>
          <div class="side-actions">
            <el-button size="small" type="primary" @click="newConversation">{{ t('assistant.newChat') }}</el-button>
            <el-button size="small" @click="openConfig">⚙</el-button>
          </div>
        </div>
        <div class="conv-list">
          <div
            v-a11y-button
            v-for="c in conversations"
            :key="c.id"
            class="conv-item"
            :class="{ active: c.id === activeId }"
            @click="selectConversation(c.id)"
          >
            <span class="conv-title">{{ c.title || t('assistant.untitledChat') }}</span>
            <el-button class="conv-del" size="small" link @click.stop="deleteConversation(c.id)">🗑</el-button>
          </div>
          <EmptyState v-if="!conversations.length" icon="message" :title="t('assistant.noConversations')" />
        </div>
      </aside>

      <!-- 对话区 -->
      <section class="glass chat">
        <div ref="scroller" class="msg-list">
          <template v-if="!units.length && !pending">
            <EmptyState icon="message" :title="t('assistant.chatTitle')" :sub="t('assistant.chatSub')" />
          </template>
          <template v-for="u in units" :key="u.id">
            <!-- 用户 -->
            <div v-if="u.data.kind === 'user'" class="row user">
              <div class="bubble user-bubble">{{ u.data.content }}</div>
            </div>
            <!-- 助手 -->
            <div v-else-if="u.data.kind === 'assistant'" class="row assistant">
              <div class="bubble assistant-bubble">
                <details v-if="u.data.reasoning" class="reasoning">
                  <summary>🧠 {{ t('assistant.reasoning') }}</summary>
                  <pre>{{ u.data.reasoning }}</pre>
                </details>
                <div class="content">{{ u.data.content }}</div>
              </div>
            </div>
            <!-- 工具步骤 -->
            <div v-else class="row assistant">
              <div class="tool-step">
                <details>
                  <summary>🔧 {{ t('assistant.toolCalls', { n: (u.data as any).calls.length }) }}</summary>
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
              <details v-if="(pending.data as any).reasoning" class="reasoning">
                <summary>🧠 {{ t('assistant.reasoning') }}</summary>
                <pre>{{ (pending.data as any).reasoning }}</pre>
              </details>
              <div class="content">{{ (pending.data as any).content || '…' }}</div>
            </div>
          </div>
        </div>
        <div class="composer">
          <textarea
            v-model="input"
            class="input"
            rows="2"
            :placeholder="t('assistant.inputPlaceholder')"
            @keydown="onKey"
            :disabled="sending"
          />
          <el-button v-if="!sending" type="primary" @click="send" :disabled="!input.trim()">{{
            t('assistant.send')
          }}</el-button>
          <el-button v-else type="danger" @click="stop">{{ t('assistant.stop') }}</el-button>
        </div>
      </section>
    </div>

    <!-- 配置 -->
    <el-dialog v-model="showConfig" :title="t('assistant.configTitle')" width="560px">
      <el-form :model="cfg" label-width="120px" size="small">
        <el-form-item v-if="stats" :label="t('assistant.stats.usage')">
          <div class="stats-grid">
            <div class="stat">
              <span class="stat-num">{{ stats.total_requests }}</span
              ><span class="stat-label">{{ t('assistant.stats.requests') }}</span>
            </div>
            <div class="stat">
              <span class="stat-num">{{ stats.total_input_tokens + stats.total_output_tokens }}</span
              ><span class="stat-label">Token</span>
            </div>
            <div class="stat">
              <span class="stat-num">${{ stats.total_cost_usd.toFixed(4) }}</span
              ><span class="stat-label">{{ t('assistant.stats.cost') }}</span>
            </div>
            <div class="stat">
              <span class="stat-num">{{ stats.avg_latency_ms }}ms</span
              ><span class="stat-label">{{ t('assistant.stats.avgLatency') }}</span>
            </div>
            <div class="stat">
              <span class="stat-num">{{ stats.error_count }}</span
              ><span class="stat-label">{{ t('assistant.stats.errors') }}</span>
            </div>
            <div class="stat">
              <span class="stat-num">{{ stats.rate_limit_count }}</span
              ><span class="stat-label">{{ t('assistant.stats.rateLimited') }}</span>
            </div>
            <div class="stat">
              <span class="stat-num">{{ stats.tool_calls }}</span
              ><span class="stat-label">{{ t('assistant.stats.toolCalls') }}</span>
            </div>
            <div class="stat">
              <span class="stat-num">{{ toolSuccessRate }}%</span
              ><span class="stat-label">{{ t('assistant.stats.toolSuccess') }}</span>
            </div>
          </div>
        </el-form-item>
        <el-form-item :label="t('rss.f.enabled')"><el-switch v-model="cfg.enabled" /></el-form-item>
        <el-form-item label="Provider">
          <el-select v-model="cfg.provider" style="width: 100%">
            <el-option :label="t('assistant.providerOpenAI')" value="openai" />
            <el-option :label="t('assistant.providerAnthropic')" value="anthropic" />
          </el-select>
        </el-form-item>
        <el-form-item label="API Key">
          <el-input
            v-model="apiKeyInput"
            type="password"
            show-password
            :placeholder="cfg.api_key_configured ? t('assistant.apiKeySet') : t('assistant.apiKeyPrompt')"
          />
        </el-form-item>
        <el-form-item label="Base URL"
          ><el-input v-model="cfg.base_url" placeholder="https://api.openai.com/v1"
        /></el-form-item>
        <el-form-item :label="t('assistant.model')"
          ><el-input v-model="cfg.model" placeholder="gpt-4o-mini / deepseek-chat / claude-sonnet-5"
        /></el-form-item>
        <el-form-item :label="t('assistant.deepThinking')"
          ><el-switch v-model="cfg.enable_thinking" /><span class="hint">{{
            t('assistant.deepThinkingHint')
          }}</span></el-form-item
        >
        <el-form-item label="Temperature"
          ><el-input-number v-model="cfg.temperature" :min="0" :max="2" :step="0.1"
        /></el-form-item>
        <el-form-item label="Max Tokens"
          ><el-input-number v-model="cfg.max_tokens" :min="256" :step="256"
        /></el-form-item>
        <el-form-item :label="t('assistant.maxIterations')"
          ><el-input-number v-model="cfg.max_iterations" :min="1" :max="30"
        /></el-form-item>
        <el-form-item :label="t('assistant.systemPrompt')"
          ><el-input v-model="cfg.system_prompt" type="textarea" :rows="4"
        /></el-form-item>
        <el-form-item :label="t('assistant.toolPerms')">
          <div class="tool-perms">
            <el-switch :model-value="toolAllowAll" @update:model-value="(v: any) => onToggleAllowAll(v as boolean)" />
            <span class="hint">{{ t('assistant.allToolsHint') }}</span>
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
                  <span class="tool-desc">{{
                    t.risk === 'destructive' ? t.risk_note || t.description : t.description
                  }}</span>
                </el-checkbox>
              </div>
            </div>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showConfig = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="savingCfg" @click="saveConfig">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
/* 填满 .content 可用高度（随 topbar / GlobalNoticeBar 显隐 / 移动端自适应），
   不能写死 calc(100vh - Npx)：通知栏 v-if 条件渲染会改变下方可用高度，
   固定值会导致输入框被顶到视口外。box-sizing 让 padding 留在高度内。 */
.page {
  padding: 12px;
  height: 100%;
  box-sizing: border-box;
}
.layout {
  display: flex;
  gap: var(--sp-3);
  height: 100%;
}
.sidebar {
  width: 240px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  padding: 10px;
  border-radius: var(--radius-md);
}
.side-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-weight: 700;
  margin-bottom: 8px;
}
.side-actions {
  display: flex;
  gap: var(--sp-1);
}
.conv-list {
  flex: 1;
  overflow-y: auto;
}
.conv-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 10px;
  border-radius: var(--radius-xs);
  cursor: pointer;
  font-size: var(--fs-body);
  transition:
    background var(--dur-fast) var(--ease-out-smooth),
    color var(--dur-fast) var(--ease-out-smooth),
    border-color var(--dur-fast) var(--ease-out-smooth);
}
.conv-item:hover {
  background: var(--surface-hover);
}
.conv-item.active {
  background: var(--glass-active, var(--pt-primary-bg));
}
.conv-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.chat {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 12px;
  border-radius: var(--radius-md);
  min-width: 0;
}
.msg-list {
  flex: 1;
  overflow-y: auto;
  padding: 4px;
}
.row {
  display: flex;
  margin-bottom: 12px;
}
.row.user {
  justify-content: flex-end;
}
.row.assistant {
  justify-content: flex-start;
}
.bubble {
  max-width: 78%;
  padding: 10px 14px;
  border-radius: var(--radius-md);
  font-size: var(--fs-md);
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
}
.user-bubble {
  background: linear-gradient(135deg, #6c5ce7, #4285f4);
  color: #fff;
}
.assistant-bubble {
  background: var(--surface-1);
  border: 1px solid var(--surface-border);
}
.reasoning {
  font-size: var(--fs-xs);
  opacity: 0.75;
  margin-bottom: 6px;
}
.reasoning summary {
  cursor: pointer;
}
.reasoning pre {
  white-space: pre-wrap;
  margin: 6px 0 0;
  max-height: 200px;
  overflow-y: auto;
}
.content:empty {
  display: none;
}
.tool-step {
  max-width: 78%;
}
.tool-step details {
  background: var(--glass-bubble, rgba(127, 127, 127, 0.1));
  border-radius: var(--radius-sm);
  padding: 6px 10px;
  font-size: var(--fs-xs);
}
.tool-step summary {
  cursor: pointer;
  opacity: 0.85;
}
.tool-item {
  border-top: 1px solid var(--divider);
  padding: 6px 0;
}
.tool-name {
  font-weight: 600;
  color: var(--pt-primary-2);
}
.tool-args,
.tool-result {
  white-space: pre-wrap;
  margin: 4px 0;
  font-size: var(--fs-2xs);
  max-height: 160px;
  overflow-y: auto;
}
.composer {
  display: flex;
  gap: var(--sp-2);
  align-items: flex-end;
  margin-top: 8px;
}
.input {
  flex: 1;
  resize: none;
  padding: 10px;
  border-radius: var(--radius-sm);
  background: var(--glass-input, rgba(127, 127, 127, 0.1));
  border: 1px solid rgba(127, 127, 127, 0.25);
  color: inherit;
  font-family: inherit;
  font-size: var(--fs-md);
}
.input:focus {
  outline: none;
  border-color: var(--pt-primary);
}
.hint {
  margin-left: 10px;
  font-size: var(--fs-xs);
  opacity: 0.6;
}
.tool-perms {
  width: 100%;
}
.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--sp-1);
  width: 100%;
}
.stat {
  display: flex;
  flex-direction: column;
  align-items: center;
  background: var(--glass-bubble, rgba(127, 127, 127, 0.1));
  border-radius: var(--radius-xs);
  padding: 6px 2px;
}
.stat-num {
  font-size: var(--fs-md);
  font-weight: 700;
}
.stat-label {
  font-size: var(--fs-2xs);
  opacity: 0.6;
}
.tool-groups {
  margin-top: 8px;
  max-height: 220px;
  overflow-y: auto;
  border: 1px solid var(--divider);
  border-radius: var(--radius-xs);
  padding: 4px 8px;
}
.tool-group {
  padding: 4px 0;
}
.tool-group + .tool-group {
  border-top: 1px solid var(--divider);
  margin-top: 4px;
}
.tool-group-title {
  font-size: var(--fs-xs);
  font-weight: 600;
  opacity: 0.85;
  margin: 4px 0;
}
.tool-group-title.danger {
  color: var(--pt-danger);
}
.tool-groups :deep(.el-checkbox) {
  display: flex;
  align-items: flex-start;
  margin: 2px 0;
  height: auto;
}
.tool-label {
  font-weight: 600;
  font-size: var(--fs-xs);
}
.tool-desc {
  display: block;
  font-size: var(--fs-2xs);
  opacity: 0.55;
  margin-left: 24px;
  line-height: 1.3;
}

/* 聊天气泡：水润入场 */
.row {
  animation: pt-fade-in-up var(--dur-base) var(--ease-out-smooth) both;
}
.bubble {
  transition: transform var(--dur-fast) var(--ease-out-smooth);
}
.input {
  transition:
    border-color var(--dur-base) var(--ease-out-smooth),
    box-shadow var(--dur-base) var(--ease-out-smooth);
}
.input:focus {
  box-shadow: 0 0 0 3px var(--pt-primary-bg);
}
</style>
