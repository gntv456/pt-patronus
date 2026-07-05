<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Bell, Delete, Document, Download, Refresh, Setting, SwitchButton, Tickets, VideoPlay } from '@element-plus/icons-vue'
import { pluginApi, type PluginActionSpec, type PluginConfigField, type PluginInfo, type PluginLog, type PluginMarketIndex } from '@/api/plugin'

const plugins = ref<PluginInfo[]>([])
const logs = ref<PluginLog[]>([])
const loading = ref(false)
const logsLoading = ref(false)
const filter = ref<'all' | 'enabled' | 'available'>('all')
const logPlugin = ref('')
const activeTab = ref('installed')
const marketSource = ref('')
const market = ref<PluginMarketIndex | null>(null)
const marketLoading = ref(false)
const installing = ref('')

const configVisible = ref(false)
const configTarget = ref<PluginInfo | null>(null)
const configForm = ref<Record<string, any>>({})
const savingConfig = ref(false)

const actionVisible = ref(false)
const actionTarget = ref<PluginInfo | null>(null)
const actionSpec = ref<PluginActionSpec | null>(null)
const actionInputText = ref('{}')
const actionOutput = ref('')
const actionRunning = ref(false)

const filteredPlugins = computed(() => {
  if (filter.value === 'enabled') return plugins.value.filter((p) => p.enabled)
  if (filter.value === 'available') return plugins.value.filter((p) => !p.enabled)
  return plugins.value
})

const enabledCount = computed(() => plugins.value.filter((p) => p.enabled).length)

async function load() {
  loading.value = true
  try {
    plugins.value = await pluginApi.list()
    await loadLogs()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载插件失败')
  } finally {
    loading.value = false
  }
}

async function loadLogs() {
  logsLoading.value = true
  try {
    logs.value = await pluginApi.logs(logPlugin.value, 120)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载插件日志失败')
  } finally {
    logsLoading.value = false
  }
}

onMounted(load)

async function loadMarket() {
  marketLoading.value = true
  try {
    market.value = await pluginApi.market(marketSource.value)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载插件市场失败')
  } finally {
    marketLoading.value = false
  }
}

async function installMarketPlugin(id: string) {
  installing.value = id
  try {
    const res = await pluginApi.install(marketSource.value, id)
    ElMessage.success(res.restart_needed ? '安装完成，重启后端后生效' : '安装完成，已加载生效')
    await loadMarket()
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '安装失败')
  } finally {
    installing.value = ''
  }
}

async function reloadPlugin(p: PluginInfo) {
  try {
    await pluginApi.reload(p.id)
    ElMessage.success('插件已重载')
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '重载失败')
  }
}

async function uninstallPlugin(p: PluginInfo) {
  try {
    await ElMessageBox.confirm(`确定卸载「${p.name}」？插件目录会被删除。`, '卸载插件', { type: 'warning' })
  } catch {
    return
  }
  try {
    await pluginApi.uninstall(p.id)
    ElMessage.success('插件已卸载')
    await load()
    if (market.value) await loadMarket()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '卸载失败')
  }
}

async function togglePlugin(p: PluginInfo) {
  try {
    if (p.enabled) {
      await ElMessageBox.confirm(`确定停用「${p.name}」？`, '停用插件', { type: 'warning' })
      await pluginApi.disable(p.id)
    } else {
      await pluginApi.enable(p.id)
    }
    ElMessage.success(p.enabled ? '已停用' : '已启用')
    await load()
  } catch (e: unknown) {
    if ((e as Error).message) ElMessage.error((e as Error).message)
  }
}

function openConfig(p: PluginInfo) {
  configTarget.value = p
  configForm.value = { ...defaultsFromSchema(p.config_schema), ...(p.config || {}) }
  configVisible.value = true
}

async function saveConfig() {
  if (!configTarget.value) return
  savingConfig.value = true
  try {
    await pluginApi.saveConfig(configTarget.value.id, configForm.value)
    configVisible.value = false
    ElMessage.success('配置已保存')
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '保存失败')
  } finally {
    savingConfig.value = false
  }
}

function openAction(p: PluginInfo, a: PluginActionSpec) {
  actionTarget.value = p
  actionSpec.value = a
  actionInputText.value = JSON.stringify(defaultsFromSchema(a.input_schema || []), null, 2)
  actionOutput.value = ''
  actionVisible.value = true
}

async function runAction() {
  if (!actionTarget.value || !actionSpec.value) return
  let input: Record<string, any>
  try {
    input = JSON.parse(actionInputText.value || '{}')
  } catch {
    ElMessage.warning('动作输入不是合法 JSON')
    return
  }
  actionRunning.value = true
  try {
    const res = await pluginApi.action(actionTarget.value.id, actionSpec.value.name, input)
    actionOutput.value = JSON.stringify(res.output || {}, null, 2)
    ElMessage.success('动作执行完成')
    await loadLogs()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '动作执行失败')
  } finally {
    actionRunning.value = false
  }
}

async function publishTestEvent() {
  try {
    await pluginApi.publishEvent('plugin.test', {
      source: 'frontend',
      message: '插件测试事件',
      at: new Date().toISOString(),
    })
    ElMessage.success('测试事件已发布')
    setTimeout(loadLogs, 300)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '发布失败')
  }
}

function defaultsFromSchema(schema: PluginConfigField[]) {
  const out: Record<string, any> = {}
  for (const f of schema) {
    if (f.default !== undefined) out[f.key] = f.default
    else if (f.type === 'switch') out[f.key] = false
    else if (f.type === 'number') out[f.key] = 0
    else out[f.key] = ''
  }
  return out
}

function fmtTime(t?: string) {
  return t ? new Date(t).toLocaleString('zh-CN', { hour12: false }).replace(/\//g, '-') : '-'
}

function jsonText(v?: Record<string, any>) {
  if (!v || !Object.keys(v).length) return ''
  return JSON.stringify(v)
}
</script>

<template>
  <div class="page">
    <div class="head">
      <div>
        <h2 class="title">插件</h2>
        <span class="text-muted hint">内置插件、事件订阅、配置和动作执行</span>
      </div>
      <div class="head-actions">
        <el-radio-group v-model="filter" size="small">
          <el-radio-button label="all">全部 {{ plugins.length }}</el-radio-button>
          <el-radio-button label="enabled">已启用 {{ enabledCount }}</el-radio-button>
          <el-radio-button label="available">可启用</el-radio-button>
        </el-radio-group>
        <el-button :icon="Bell" @click="publishTestEvent">测试事件</el-button>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
      </div>
    </div>

    <el-tabs v-model="activeTab" class="plugin-tabs">
      <el-tab-pane label="已安装" name="installed" />
      <el-tab-pane label="插件市场" name="market" />
    </el-tabs>

    <div v-if="activeTab === 'installed'" v-loading="loading" class="plugin-grid">
      <div v-for="p in filteredPlugins" :key="p.id" class="plugin-card glass-hover">
        <div class="plugin-top">
          <GlassIcon name="plugin" from="#0ea5e9" to="#22c55e" :size="40" />
          <div class="plugin-title">
            <div class="name-row">
              <span class="plugin-name">{{ p.name }}</span>
              <el-tag size="small" :type="p.enabled ? 'success' : 'info'">{{ p.enabled ? '运行中' : '未启用' }}</el-tag>
              <el-tag v-if="p.built_in" size="small">内置</el-tag>
            </div>
            <div class="meta">{{ p.id }} · v{{ p.version }} · {{ p.author }}</div>
          </div>
        </div>

        <div class="desc">{{ p.description }}</div>

        <div class="chips">
          <el-tag v-for="perm in p.permissions" :key="perm" size="small" effect="plain">{{ perm }}</el-tag>
        </div>

        <div class="detail-row">
          <span>事件</span>
          <span>{{ p.events.map((e) => e.type).join(', ') || '-' }}</span>
        </div>
        <div class="detail-row">
          <span>上次事件</span>
          <span>{{ fmtTime(p.last_event_at) }}</span>
        </div>
        <div v-if="p.last_error" class="error-line">{{ p.last_error }}</div>

        <div class="actions">
          <el-button :icon="SwitchButton" :type="p.enabled ? 'warning' : 'primary'" @click="togglePlugin(p)">
            {{ p.enabled ? '停用' : '启用' }}
          </el-button>
          <el-button :icon="Setting" @click="openConfig(p)">配置</el-button>
          <el-button v-if="!p.built_in" :icon="Refresh" @click="reloadPlugin(p)">重载</el-button>
          <el-button v-if="!p.built_in" :icon="Delete" type="danger" plain @click="uninstallPlugin(p)">卸载</el-button>
          <el-dropdown v-if="p.actions?.length" trigger="click">
            <el-button :icon="VideoPlay">动作</el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item v-for="a in p.actions" :key="a.name" @click="openAction(p, a)">
                  {{ a.label }}
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </div>
    </div>

    <div v-if="activeTab === 'installed'" class="logs glass">
      <div class="logs-head">
        <div class="logs-title"><el-icon><Document /></el-icon> 插件日志</div>
        <div class="logs-tools">
          <el-select v-model="logPlugin" clearable placeholder="全部插件" size="small" style="width: 180px" @change="loadLogs">
            <el-option v-for="p in plugins" :key="p.id" :label="p.name" :value="p.id" />
          </el-select>
          <el-button :icon="Refresh" size="small" :loading="logsLoading" @click="loadLogs">刷新</el-button>
        </div>
      </div>
      <el-table v-loading="logsLoading" :data="logs" size="small" class="log-table" empty-text="暂无日志">
        <el-table-column label="时间" width="170">
          <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column prop="plugin_id" label="插件" min-width="140" />
        <el-table-column prop="level" label="级别" width="90" />
        <el-table-column prop="event" label="事件/动作" min-width="130" />
        <el-table-column prop="message" label="消息" min-width="180" />
        <el-table-column label="数据" min-width="220">
          <template #default="{ row }">
            <span class="mono">{{ jsonText(row.data) }}</span>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <div v-if="activeTab === 'market'" class="market glass">
      <div class="market-tools">
        <el-input
          v-model="marketSource"
          clearable
          placeholder="市场索引地址；留空读取 data/plugin-market.json"
        />
        <el-button :icon="Refresh" :loading="marketLoading" @click="loadMarket">刷新市场</el-button>
      </div>
      <el-table v-loading="marketLoading" :data="market?.plugins || []" empty-text="暂无市场数据">
        <el-table-column label="插件" min-width="220">
          <template #default="{ row }">
            <div class="market-name">{{ row.name }}</div>
            <div class="meta">{{ row.id }} · v{{ row.version }} · {{ row.runtime }}</div>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="说明" min-width="260" />
        <el-table-column label="权限" min-width="220">
          <template #default="{ row }">
            <div class="chips">
              <el-tag v-for="perm in row.permissions || []" :key="perm" size="small" effect="plain">{{ perm }}</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="校验" min-width="180">
          <template #default="{ row }">
            <span class="mono">{{ row.sha256 ? row.sha256.slice(0, 16) + '...' : '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="130" fixed="right">
          <template #default="{ row }">
            <el-button
              type="primary"
              :icon="Download"
              :disabled="row.installed"
              :loading="installing === row.id"
              @click="installMarketPlugin(row.id)"
            >
              {{ row.installed ? '已安装' : '安装' }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog v-model="configVisible" :title="`配置：${configTarget?.name || ''}`" width="560px">
      <el-form label-width="110px" label-position="left">
        <el-form-item v-for="f in configTarget?.config_schema || []" :key="f.key" :label="f.label" :required="f.required">
          <el-switch v-if="f.type === 'switch'" v-model="configForm[f.key]" />
          <el-select v-else-if="f.type === 'select'" v-model="configForm[f.key]" style="width: 100%">
            <el-option v-for="opt in f.options || []" :key="String(opt.value)" :label="opt.label" :value="opt.value" />
          </el-select>
          <el-input-number v-else-if="f.type === 'number'" v-model="configForm[f.key]" style="width: 100%" />
          <el-input v-else-if="f.type === 'textarea'" v-model="configForm[f.key]" type="textarea" :rows="4" />
          <el-input v-else v-model="configForm[f.key]" />
          <div v-if="f.help" class="field-help">{{ f.help }}</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="configVisible = false">取消</el-button>
        <el-button type="primary" :icon="Tickets" :loading="savingConfig" @click="saveConfig">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="actionVisible" :title="`动作：${actionSpec?.label || ''}`" width="640px">
      <div class="action-desc">{{ actionSpec?.description }}</div>
      <el-input v-model="actionInputText" type="textarea" :rows="8" class="mono-input" />
      <div v-if="actionOutput" class="action-output">
        <div class="output-title">输出</div>
        <pre>{{ actionOutput }}</pre>
      </div>
      <template #footer>
        <el-button @click="actionVisible = false">关闭</el-button>
        <el-button type="primary" :icon="VideoPlay" :loading="actionRunning" @click="runAction">执行</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.title {
  margin: 0;
  font-size: 24px;
}
.hint {
  display: inline-block;
  margin-top: 4px;
}
.head-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}
.plugin-tabs {
  margin-bottom: -6px;
}
.plugin-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 14px;
}
.plugin-card {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 16px;
  border-radius: var(--radius-md);
}
.plugin-top,
.name-row,
.actions,
.logs-head,
.logs-tools {
  display: flex;
  align-items: center;
  gap: 10px;
}
.plugin-title {
  min-width: 0;
}
.plugin-name {
  font-size: 18px;
  font-weight: 700;
}
.meta,
.desc,
.field-help,
.action-desc {
  color: var(--text-muted);
  font-size: var(--fs-sm);
}
.desc {
  min-height: 42px;
  line-height: 1.5;
}
.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.detail-row {
  display: grid;
  grid-template-columns: 72px 1fr;
  gap: 10px;
  font-size: var(--fs-sm);
}
.detail-row span:first-child {
  color: var(--text-muted);
}
.detail-row span:last-child {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.error-line {
  color: var(--pt-danger);
  font-size: var(--fs-sm);
  word-break: break-word;
}
.actions {
  margin-top: auto;
  flex-wrap: wrap;
}
.logs {
  padding: 14px;
}
.market {
  padding: 14px;
}
.market-tools {
  display: grid;
  grid-template-columns: minmax(260px, 1fr) auto;
  gap: 10px;
  margin-bottom: 12px;
}
.market-name {
  font-weight: 700;
}
.logs-head {
  justify-content: space-between;
  margin-bottom: 10px;
}
.logs-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-weight: 700;
}
.log-table {
  width: 100%;
}
.mono,
.mono-input :deep(textarea),
.action-output pre {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}
.mono {
  font-size: 12px;
  word-break: break-all;
}
.field-help {
  width: 100%;
  margin-top: 4px;
  line-height: 1.4;
}
.action-output {
  margin-top: 12px;
}
.output-title {
  margin-bottom: 6px;
  font-weight: 700;
}
.action-output pre {
  max-height: 240px;
  overflow: auto;
  padding: 10px;
  border-radius: var(--radius-sm);
  background: rgba(15, 23, 42, 0.08);
  white-space: pre-wrap;
}
@media (max-width: 720px) {
  .head {
    align-items: stretch;
    flex-direction: column;
  }
  .head-actions {
    justify-content: flex-start;
  }
  .plugin-grid {
    grid-template-columns: 1fr;
  }
  .logs-head {
    align-items: flex-start;
    flex-direction: column;
  }
  .market-tools {
    grid-template-columns: 1fr;
  }
}
</style>
