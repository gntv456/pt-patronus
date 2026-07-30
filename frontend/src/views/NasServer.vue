<script setup lang="ts">
import { ref, computed, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { useRouter } from 'vue-router'
import DeviceCard from '@/components/monitor/DeviceCard.vue'
import DeviceForm from '@/components/monitor/DeviceForm.vue'
import DeviceDetail from '@/components/monitor/DeviceDetail.vue'
import AlertList from '@/components/monitor/AlertList.vue'
import RtspPlayer from '@/components/monitor/RtspPlayer.vue'
import AgentManager from '@/components/monitor/AgentManager.vue'
import GlassIcon from '@/components/GlassIcon.vue'
import {
  monitorApi,
  commandRunApi,
  agentApi,
  credentialApi,
  opsConfigApi,
  parseMetrics,
  type MonitorTarget,
  type Metrics,
  type MonitorAlert,
  type TargetInput,
  type CommandRun,
  type CommandRunTarget,
  type OpsCredential,
  type OpsCredentialInput,
  type OpsConfig,
  type OpsConfigInput,
  type OpsConfigHistory,
} from '@/api/monitor'
import { ElMessage, ElMessageBox } from 'element-plus'
import { isSSHAuthError, showSSHAuthFix } from '@/utils/ssh-fix'
import { showAgentSecret } from '@/utils/agent-secret'
import { useAuthStore } from '@/stores/auth'
import { authApi } from '@/api/auth'
import {
  type ExecStatus,
  type ExecLog,
  type CommandRisk,
  highRiskRules,
  mediumRiskRules,
  quickCommands,
  tabs,
  shQuote,
  basenameOf,
  nowTime,
  runTimeText,
  runDateText,
  runStatusText,
  runStatusType,
  runSourceText,
  logStatus,
  commandRisk,
  credentialTypeText,
  configFormatText,
  isSSHHost,
  targetAddr,
  targetState,
  execLogsFromRun,
} from './nasServerHelpers'
import { useNasCredentials } from './useNasCredentials'
import { useNasOpsConfigs } from './useNasOpsConfigs'
import { useCommandExec } from './useCommandExec'

// Agent 创建/删除/重置密钥是 admin 专属（后端 RequireAdmin）；非 admin 隐藏入口。
const auth = useAuthStore()
const router = useRouter()

type DeployForm = {
  targetIds: number[]
  repoUrl: string
  branch: string
  projectDir: string
  buildCmd: string
  serviceName: string
  backupDir: string
  grayFirst: boolean
}
type MonitorTab = 'host' | 'nas' | 'camera' | 'http' | 'tcp' | 'process' | 'script'
type Workspace = 'asset' | 'execute' | 'monitor' | 'deploy'

// NasServer — 设备监控主视图：设备与监控项共用同一套卡片组件。
const targets = ref<MonitorTarget[]>([])
const latestMap = ref<Record<number, Metrics | null>>({})
const alerts = ref<MonitorAlert[]>([])
const activeTab = ref<MonitorTab>('host')
const activeWorkspace = ref<Workspace>('asset')

const {
  credentials,
  credentialVisible,
  credentialFormVisible,
  editingCredential,
  credentialForm,
  openCredentialCenter,
  openCredentialAdd,
  openCredentialEdit,
  credentialUsage,
  saveCredential,
  removeCredential,
} = useNasCredentials(targets, loadAll)

const {
  opsConfigs,
  configVisible,
  configFormVisible,
  editingConfig,
  configHistory,
  configHistoryTarget,
  configForm,
  openConfigCenter,
  openConfigAdd,
  openConfigEdit,
  saveConfig,
  removeConfig,
  showConfigHistory,
} = useNasOpsConfigs()

const formVisible = ref(false)
const editing = ref<MonitorTarget | null>(null)
const formDefaultType = ref('host')
const detailVisible = ref(false)
const detailTarget = ref<MonitorTarget | null>(null)
const previewVisible = ref(false)
const previewTarget = ref<MonitorTarget | null>(null)
const alertVisible = ref(false)
const agentVisible = ref(false)
const helpVisible = ref(false)
const terminalVisible = ref(false)
const terminalTarget = ref<MonitorTarget | null>(null)
const terminalOutput = ref('')
const terminalInput = ref('')
const terminalConnected = ref(false)
const terminalConnecting = ref(false)
const terminalOutputEl = ref<HTMLElement | null>(null)
let terminalSocket: WebSocket | null = null
const deployVisible = ref(false)
const deployConfigId = ref<number | null>(null)
const deployForm = ref<DeployForm>({
  targetIds: [],
  repoUrl: '',
  branch: 'main',
  projectDir: '/opt/app',
  buildCmd: '',
  serviceName: '',
  backupDir: '/opt/backups',
  grayFirst: true,
})
const execPanel = ref<HTMLElement | null>(null)

const filtered = computed(() => targets.value.filter((t) => t.type === activeTab.value))
const unreadCount = computed(() => alerts.value.filter((a) => !a.read).length)
const hostCount = computed(() => targets.value.filter((t) => t.type === 'host').length)
const monitorCount = computed(() => targets.value.filter((t) => ['http', 'tcp', 'process', 'script'].includes(t.type)).length)
const configCount = computed(() => opsConfigs.value.length)
const deployProfiles = computed(() => opsConfigs.value.filter((c) => c.key.startsWith('deploy.') && c.format === 'json'))
const activeAlertCount = computed(() => alerts.value.filter((a) => !a.resolved_at).length)
const sshTargets = computed(() => targets.value.filter(isSSHHost))
const {
  scriptFileInput,
  execTargetIds,
  execCommand,
  execTimeout,
  execRunning,
  execLogs,
  commandHistory,
  commandRuns,
  selectedCommandRun,
  selectedExecTargets,
  loadCommandHistory,
  selectAllExecTargets,
  selectOnlineExecTargets,
  clearExecTargets,
  applyCommand,
  pickScriptFile,
  importScriptFile,
  showCommandRun,
  runBatchCommand,
} = useCommandExec(sshTargets, focusExecPanel)
const deployRuns = computed(() => commandRuns.value.filter((run) => run.source === 'deploy'))
const assetTabs = computed(() => tabs.filter((t) => ['host', 'nas', 'camera'].includes(t.name)))
const monitorTabs = computed(() => tabs.filter((t) => ['http', 'tcp', 'process', 'script'].includes(t.name)))
const visibleTabs = computed(() => (activeWorkspace.value === 'monitor' ? monitorTabs.value : assetTabs.value))
const configValuePlaceholder = computed(() =>
  configForm.value.format === 'text' ? '任意文本配置' : '{\n  "key": "value"\n}',
)
const deploySteps = computed(() => [
  { title: '备份当前目录', desc: deployForm.value.backupDir || '/opt/backups' },
  { title: '拉取代码', desc: `${deployForm.value.branch || 'main'} · ${deployForm.value.repoUrl || '未填写仓库'}` },
  { title: '执行构建', desc: deployForm.value.buildCmd.trim() || '跳过构建' },
  { title: '重启服务', desc: deployForm.value.serviceName.trim() || '未配置服务名' },
  { title: '完成记录', desc: deployForm.value.grayFirst ? '灰度：先执行第一台' : '全部服务器执行' },
])
let timer: number | undefined

// 区分首屏与后台轮询：首屏失败显示 error 态 + 重试；后台轮询失败仍静默。
const error = ref('')
const firstLoad = ref(true)

async function loadAll() {
  try {
    const [ts, al] = await Promise.all([
      monitorApi.list(),
      monitorApi.alerts(undefined, true).catch(() => []),
    ])
    targets.value = ts
    alerts.value = al
    if (auth.isAdmin) {
      const [credList, cfgList, runList] = await Promise.all([
        credentialApi.list().catch(() => credentials.value),
        opsConfigApi.list().catch(() => opsConfigs.value),
        commandRunApi.list(20).catch(() => commandRuns.value),
      ])
      credentials.value = credList
      opsConfigs.value = cfgList
      commandRuns.value = runList
    }
    latestMap.value = {}
    // 每个 target 取最新一条快照作为卡片实时指标。
    // 后端 ListSnapshots 返回「最近 N 条升序」——末项才是最新；limit=1 时 [0] 也是最新。
    // 仍取 last，避免以后 limit 调大时又踩到最旧条。
    await Promise.all(
      ts.map(async (t) => {
        const snaps = await monitorApi.metrics(t.id, undefined, 1).catch(() => [])
        latestMap.value[t.id] = snaps.length ? parseMetrics(snaps[snaps.length - 1]) : null
      }),
    )
    error.value = ''
  } catch (e) {
    if (firstLoad.value) {
      error.value = (e as Error).message || '加载失败'
    }
    /* 后台轮询失败静默，下一轮自动重试 */
  } finally {
    firstLoad.value = false
  }
}

function setWorkspace(workspace: Workspace) {
  activeWorkspace.value = workspace
  if (workspace === 'asset' && !['host', 'nas', 'camera'].includes(activeTab.value)) activeTab.value = 'host'
  if (workspace === 'monitor' && !['http', 'tcp', 'process', 'script'].includes(activeTab.value)) activeTab.value = 'http'
}

function openAddAsset() {
  setWorkspace('asset')
  editing.value = null
  formDefaultType.value = ['host', 'nas', 'camera'].includes(activeTab.value) ? activeTab.value : 'host'
  formVisible.value = true
}

function openAddMonitorItem(type: MonitorTab = 'http') {
  setWorkspace('monitor')
  activeTab.value = ['http', 'tcp', 'process', 'script'].includes(type) ? type : 'http'
  editing.value = null
  formDefaultType.value = activeTab.value
  formVisible.value = true
}

function openAdd() {
  if (activeWorkspace.value === 'monitor') openAddMonitorItem(activeTab.value)
  else openAddAsset()
}
function openEdit(t: MonitorTarget) {
  editing.value = t
  formVisible.value = true
}
async function onSubmit(input: TargetInput) {
  try {
    if (!editing.value && input.protocol === 'agent') {
      // Agent 设备：走 Agent 创建（建 target + 生成 secret），弹安装命令
      const r = await agentApi.create(input.name, input.type === 'nas' ? 'nas' : 'host')
      showAgentSecret(r.agent, r.plain_secret)
    } else if (editing.value) {
      await monitorApi.update(editing.value.id, input)
      ElMessage.success('已更新')
    } else {
      await monitorApi.create(input)
      ElMessage.success('已添加，开始采集')
    }
    await loadAll()
  } catch (e) {
    ElMessage.error('保存失败：' + (e as Error).message)
  }
}
async function onProbe(t: MonitorTarget) {
  try {
    const r = await monitorApi.probe(t.id)
    if (r.online) ElMessage.success(`${t.name} 在线`)
    else if (isSSHAuthError(r.error)) showSSHAuthFix()
    else ElMessage.warning(`${t.name} 离线${r.error ? '：' + r.error : ''}`)
    await loadAll()
  } catch {
    ElMessage.error('探活失败')
  }
}
async function onRemove(t: MonitorTarget) {
  try {
    await ElMessageBox.confirm(`确定删除「${t.name}」？关联的指标与告警将一并删除。`, '删除设备', {
      type: 'warning',
    })
    await monitorApi.remove(t.id)
    ElMessage.success('已删除')
    await loadAll()
  } catch {
    /* 用户取消 */
  }
}
function onDetail(t: MonitorTarget) {
  detailTarget.value = t
  detailVisible.value = true
}
function onPreview(t: MonitorTarget) {
  previewTarget.value = t
  previewVisible.value = true
}
async function retry() {
  // 重试视为首屏，失败仍显示 error 态
  firstLoad.value = true
  await loadAll()
}
async function onReadAlert(a: MonitorAlert) {
  await monitorApi.readAlert(a.id)
  await loadAll()
}

async function onAckAlert(a: MonitorAlert) {
  await monitorApi.ackAlert(a.id)
  ElMessage.success('告警已确认')
  await loadAll()
}

async function onSilenceAlert(a: MonitorAlert) {
  await monitorApi.silenceAlert(a.id, 30)
  ElMessage.success('已静默 30 分钟')
  await loadAll()
}

function terminalURL(id: number, ticket: string) {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  return `${proto}://${location.host}/api/v1/monitors/${id}/terminal?ticket=${encodeURIComponent(ticket)}&cols=120&rows=32`
}

function appendTerminal(text: string) {
  terminalOutput.value = (terminalOutput.value + text).slice(-80000)
  nextTick(() => {
    if (terminalOutputEl.value) terminalOutputEl.value.scrollTop = terminalOutputEl.value.scrollHeight
  })
}

function closeTerminalSocket() {
  terminalSocket?.close()
  terminalSocket = null
  terminalConnected.value = false
  terminalConnecting.value = false
}

async function openTerminal(t: MonitorTarget) {
  if (!auth.isAdmin) {
    ElMessage.warning('只有管理员可以打开终端')
    return
  }
  if (!isSSHHost(t)) {
    ElMessage.warning('目前仅 SSH 主机支持在线终端')
    return
  }
  terminalTarget.value = t
  terminalVisible.value = true
  terminalOutput.value = ''
  terminalInput.value = ''
  await connectTerminal()
}

async function connectTerminal() {
  const t = terminalTarget.value
  if (!t) return
  closeTerminalSocket()
  terminalConnecting.value = true
  appendTerminal(`[${nowTime()}] 正在连接 ${t.name} (${targetAddr(t)})...\r\n`)
  try {
    const ticket = (await authApi.wsTicket()).ticket
    const ws = new WebSocket(terminalURL(t.id, ticket))
    terminalSocket = ws
    ws.onopen = () => {
      terminalConnected.value = true
      terminalConnecting.value = false
      appendTerminal(`[${nowTime()}] 已连接，输入命令后回车执行。\r\n\r\n`)
    }
    ws.onmessage = (ev) => appendTerminal(String(ev.data || ''))
    ws.onerror = () => {
      appendTerminal(`\r\n[${nowTime()}] 终端连接错误\r\n`)
    }
    ws.onclose = () => {
      terminalConnected.value = false
      terminalConnecting.value = false
      appendTerminal(`\r\n[${nowTime()}] 连接已关闭\r\n`)
    }
  } catch (e: unknown) {
    terminalConnecting.value = false
    appendTerminal(`[${nowTime()}] 连接失败：${(e as Error).message || '未知错误'}\r\n`)
  }
}

function sendTerminalLine() {
  if (!terminalSocket || terminalSocket.readyState !== WebSocket.OPEN) {
    ElMessage.warning('终端未连接')
    return
  }
  terminalSocket.send(`${terminalInput.value}\r`)
  terminalInput.value = ''
}

function sendTerminalCtrlC() {
  if (terminalSocket?.readyState === WebSocket.OPEN) terminalSocket.send('\x03')
}

function focusExecPanel() {
  setWorkspace('execute')
  nextTick(() => execPanel.value?.scrollIntoView({ behavior: 'smooth', block: 'start' }))
}

function openMonitorTab() {
  setWorkspace('monitor')
  activeTab.value = 'http'
  nextTick(() => document.querySelector('.np-tabs')?.scrollIntoView({ behavior: 'smooth', block: 'start' }))
}

function go(name: string) {
  router.push({ name })
}

function openDeployPanel() {
  setWorkspace('deploy')
  deployForm.value.targetIds = selectedExecTargets.value.length
    ? selectedExecTargets.value.map((t) => t.id)
    : sshTargets.value.filter((t) => t.last_online !== false).slice(0, 1).map((t) => t.id)
  deployVisible.value = true
}

function loadDeployProfile(id: number | null) {
  const profile = deployProfiles.value.find((p) => p.id === id)
  if (!profile) return
  try {
    const parsed = JSON.parse(profile.value) as Partial<DeployForm>
    deployForm.value = {
      ...deployForm.value,
      ...parsed,
      targetIds: Array.isArray(parsed.targetIds) ? parsed.targetIds : deployForm.value.targetIds,
      grayFirst: parsed.grayFirst !== false,
    }
  } catch {
    ElMessage.error('部署模板 JSON 格式错误')
  }
}

async function saveDeployProfile() {
  if (!deployForm.value.repoUrl.trim() || !deployForm.value.projectDir.trim()) {
    ElMessage.warning('请先填写 Git 仓库和项目目录')
    return
  }
  const keyBase = basenameOf(deployForm.value.projectDir).replace(/[^\w.-]+/g, '-').toLowerCase() || 'app'
  const input: OpsConfigInput = {
    key: `deploy.${keyBase}`,
    name: `${keyBase} 发布流程`,
    env: deployForm.value.branch || 'main',
    format: 'json',
    value: JSON.stringify(deployForm.value, null, 2),
    tags: '部署,发布',
    note: 'NAS 运维控制台发布部署模板',
  }
  try {
    if (deployConfigId.value) await opsConfigApi.update(deployConfigId.value, input)
    else {
      const created = await opsConfigApi.create(input)
      deployConfigId.value = created.id
    }
    opsConfigs.value = await opsConfigApi.list()
    ElMessage.success('发布模板已保存')
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '保存发布模板失败')
  }
}

function deployTargets() {
  const ids = deployForm.value.grayFirst ? deployForm.value.targetIds.slice(0, 1) : deployForm.value.targetIds
  return ids.filter((id) => sshTargets.value.some((t) => t.id === id))
}

function buildDeployCommand() {
  const f = deployForm.value
  const app = shQuote(f.projectDir)
  const backup = shQuote(f.backupDir)
  const appName = shQuote(basenameOf(f.projectDir))
  const branch = shQuote(f.branch || 'main')
  const repo = shQuote(f.repoUrl)
  const build = f.buildCmd.trim()
  const service = f.serviceName.trim()
  return [
    'set -e',
    `APP_DIR=${app}`,
    `BACKUP_DIR=${backup}`,
    `APP_NAME=${appName}`,
    'TS=$(date +%Y%m%d%H%M%S)',
    'mkdir -p "$BACKUP_DIR"',
    'if [ -d "$APP_DIR" ]; then tar -czf "$BACKUP_DIR/$APP_NAME-$TS.tgz" -C "$(dirname "$APP_DIR")" "$APP_NAME"; fi',
    `if [ ! -d "$APP_DIR/.git" ]; then mkdir -p "$(dirname "$APP_DIR")"; git clone --branch ${branch} ${repo} "$APP_DIR"; else cd "$APP_DIR"; git fetch --all; git checkout ${branch}; git pull --ff-only origin ${branch}; fi`,
    'cd "$APP_DIR"',
    build ? build : 'echo "跳过构建"',
    service ? `sudo -n systemctl restart ${shQuote(service)}` : 'echo "未配置服务名，跳过重启"',
    'echo "部署完成：$APP_DIR"',
  ].join('\n')
}

function buildRollbackCommand() {
  const f = deployForm.value
  const app = shQuote(f.projectDir)
  const backup = shQuote(f.backupDir)
  const appName = shQuote(basenameOf(f.projectDir))
  const service = f.serviceName.trim()
  return [
    'set -e',
    `APP_DIR=${app}`,
    `BACKUP_DIR=${backup}`,
    `APP_NAME=${appName}`,
    'LATEST=$(ls -1t "$BACKUP_DIR"/"$APP_NAME"-*.tgz 2>/dev/null | head -n1)',
    'if [ -z "$LATEST" ]; then echo "没有可回滚的备份"; exit 1; fi',
    service ? `sudo -n systemctl stop ${shQuote(service)} || true` : 'echo "未配置服务名，跳过停止服务"',
    'rm -rf "$APP_DIR"',
    'mkdir -p "$(dirname "$APP_DIR")"',
    'tar -xzf "$LATEST" -C "$(dirname "$APP_DIR")"',
    service ? `sudo -n systemctl start ${shQuote(service)}` : 'echo "未配置服务名，跳过启动服务"',
    'echo "已回滚到：$LATEST"',
  ].join('\n')
}

async function runDeployCommand(kind: 'deploy' | 'rollback') {
  if (!deployForm.value.targetIds.length) {
    ElMessage.warning('请选择部署服务器')
    return
  }
  if (!deployForm.value.projectDir.trim()) {
    ElMessage.warning('请填写项目目录')
    return
  }
  if (kind === 'deploy' && !deployForm.value.repoUrl.trim()) {
    ElMessage.warning('请填写 Git 仓库地址')
    return
  }
  const ids = deployTargets()
  if (!ids.length) {
    ElMessage.warning('没有可部署的 SSH 主机')
    return
  }
  execTargetIds.value = ids
  execCommand.value = kind === 'deploy' ? buildDeployCommand() : buildRollbackCommand()
  if (execTimeout.value < 300) execTimeout.value = 300
  deployVisible.value = false
  focusExecPanel()
  await nextTick()
  await runBatchCommand('deploy', kind === 'deploy' ? '发布部署' : '回滚上一版')
}

onMounted(() => {
  loadCommandHistory()
  loadAll()
  timer = window.setInterval(loadAll, 15000)
})
onBeforeUnmount(() => {
  if (timer) window.clearInterval(timer)
  closeTerminalSocket()
})

</script>

<template>
  <div class="nas-page">
    <header class="np-head glass-bar">
      <div class="np-title">
        <GlassIcon name="server" from="#0f2027" to="#2c5364" :size="32" />
        <div>
          <h2>NAS 运维控制台</h2>
          <span>主机、文件、发布、监控、告警</span>
        </div>
      </div>
      <div class="np-actions">
        <el-button @click="helpVisible = true">使用说明</el-button>
        <el-button :type="unreadCount ? 'danger' : 'default'" @click="alertVisible = true">
          告警<span v-if="unreadCount" class="badge">{{ unreadCount }}</span>
        </el-button>
        <el-button v-if="auth.isAdmin" @click="openCredentialCenter">凭据</el-button>
        <el-button v-if="auth.isAdmin" @click="openConfigCenter">配置</el-button>
        <el-button v-if="auth.isAdmin" @click="agentVisible = true">Agent</el-button>
        <el-button @click="openAddMonitorItem()">+ 添加监控项</el-button>
        <el-button type="primary" @click="openAddAsset">+ 添加设备</el-button>
      </div>
    </header>

    <div v-if="error" class="empty glass error-state">
      <div class="err-text">⚠ {{ error }}</div>
      <el-button size="small" @click="retry">重试</el-button>
    </div>

    <section v-if="!error" class="ops-grid">
      <button
        class="ops-tile glass glass-hover"
        :class="{ active: activeWorkspace === 'asset' }"
        type="button"
        @click="setWorkspace('asset')"
      >
        <GlassIcon name="server" from="#0f2027" to="#2c5364" :size="24" />
        <strong>{{ hostCount }}</strong>
        <span>资产管理</span>
      </button>
      <button
        class="ops-tile glass glass-hover"
        :class="{ active: activeWorkspace === 'execute' }"
        type="button"
        @click="focusExecPanel"
      >
        <GlassIcon name="server" from="#34495e" to="#16a085" :size="24" />
        <strong>终端</strong>
        <span>在线执行</span>
      </button>
      <button
        class="ops-tile glass glass-hover"
        :class="{ active: activeWorkspace === 'execute' }"
        type="button"
        @click="focusExecPanel"
      >
        <GlassIcon name="workflow" from="#11998e" to="#38ef7d" :size="24" />
        <strong>{{ sshTargets.length }}</strong>
        <span>批量执行</span>
      </button>
      <button class="ops-tile glass glass-hover" type="button" @click="go('files')">
        <GlassIcon name="files" from="#4285f4" to="#6c5ce7" :size="24" />
        <strong>文件</strong>
        <span>上传下载</span>
      </button>
      <button
        class="ops-tile glass glass-hover"
        :class="{ active: activeWorkspace === 'deploy' }"
        type="button"
        @click="openDeployPanel"
      >
        <GlassIcon name="workflow" from="#f7971e" to="#ffd200" :size="24" />
        <strong>发布</strong>
        <span>流程 / 回滚</span>
      </button>
      <button class="ops-tile glass glass-hover" type="button" @click="go('workflow')">
        <GlassIcon name="history" from="#00b894" to="#0984e3" :size="24" />
        <strong>任务</strong>
        <span>Cron 计划</span>
      </button>
      <button class="ops-tile glass glass-hover" type="button" @click="openCredentialCenter">
        <GlassIcon name="settings" from="#8e44ad" to="#3498db" :size="24" />
        <strong>{{ credentials.length }}</strong>
        <span>凭据中心</span>
      </button>
      <button class="ops-tile glass glass-hover" type="button" @click="openConfigCenter">
        <GlassIcon name="files" from="#4285f4" to="#16a085" :size="24" />
        <strong>{{ configCount }}</strong>
        <span>配置中心</span>
      </button>
      <button
        class="ops-tile glass glass-hover"
        :class="{ active: activeWorkspace === 'monitor' }"
        type="button"
        @click="openMonitorTab"
      >
        <GlassIcon name="dashboard" from="#6c5ce7" to="#4285f4" :size="24" />
        <strong>{{ monitorCount }}</strong>
        <span>监控中心</span>
      </button>
      <button class="ops-tile glass glass-hover" type="button" @click="go('settings')">
        <GlassIcon name="alert" from="#e74c3c" to="#c0392b" :size="24" />
        <strong>{{ activeAlertCount }}</strong>
        <span>报警中心</span>
      </button>
    </section>

    <section v-if="!error && activeWorkspace === 'execute'" ref="execPanel" class="exec-panel glass">
      <div class="exec-head">
        <div>
          <h3>批量执行</h3>
          <span>
            {{ selectedExecTargets.length }}/{{ sshTargets.length }} 台 SSH 主机
            <template v-if="selectedCommandRun"> · 当前记录 #{{ selectedCommandRun.id }}</template>
          </span>
        </div>
        <div class="exec-head-actions">
          <el-button size="small" @click="selectOnlineExecTargets">选择在线</el-button>
          <el-button size="small" @click="selectAllExecTargets">全选</el-button>
          <el-button size="small" @click="clearExecTargets">清空</el-button>
        </div>
      </div>
      <div class="exec-layout">
        <div class="exec-targets">
          <div v-if="sshTargets.length" class="target-list">
            <el-checkbox-group v-model="execTargetIds">
              <el-checkbox v-for="t in sshTargets" :key="t.id" :value="t.id" class="target-check">
                <span class="target-main">
                  <b>{{ t.name }}</b>
                  <small>{{ targetAddr(t) }}</small>
                </span>
                <span class="target-state" :class="{ online: t.last_online === true, offline: t.last_online === false }">
                  {{ targetState(t) }}
                </span>
                <button class="target-terminal" type="button" @click.stop.prevent="openTerminal(t)">终端</button>
              </el-checkbox>
            </el-checkbox-group>
          </div>
          <div v-else class="exec-empty">
            暂无 SSH 主机
            <el-button size="small" type="primary" @click="openAdd">添加服务器</el-button>
          </div>
        </div>
        <div class="exec-command">
          <div class="quick-row">
            <button v-for="q in quickCommands" :key="q.label" type="button" @click="applyCommand(q.command)">
              {{ q.label }}
            </button>
            <button type="button" @click="pickScriptFile">导入脚本</button>
            <input ref="scriptFileInput" class="hidden-file" type="file" accept=".sh,.bash,.txt,.cmd,.ps1" @change="importScriptFile" />
          </div>
          <el-input
            v-model="execCommand"
            type="textarea"
            :rows="5"
            resize="none"
            placeholder="输入要执行的命令"
          />
          <div v-if="commandHistory.length" class="history-row">
            <button v-for="cmd in commandHistory" :key="cmd" type="button" @click="applyCommand(cmd)">
              {{ cmd }}
            </button>
          </div>
          <div class="exec-run-row">
            <el-input-number v-model="execTimeout" :min="5" :max="600" :step="5" size="small" />
            <span>秒超时</span>
            <el-button type="primary" :loading="execRunning" :disabled="!auth.isAdmin" @click="runBatchCommand()">
              一键执行
            </el-button>
          </div>
        </div>
      </div>
      <div v-if="commandRuns.length" class="run-history">
        <div class="section-title">执行历史</div>
        <div class="run-history-list">
          <button v-for="run in commandRuns" :key="run.id" type="button" class="run-card" @click="showCommandRun(run)">
            <span>
              <b>#{{ run.id }} · {{ runSourceText(run.source) }}</b>
              <small>{{ runDateText(run.started_at) }} · {{ run.success_count }}/{{ run.total_count }} 成功</small>
            </span>
            <el-tag size="small" :type="runStatusType(run.status)">{{ runStatusText(run.status) }}</el-tag>
          </button>
        </div>
      </div>
      <div v-if="execLogs.length" class="exec-log-list">
        <div v-for="log in execLogs" :key="log.id" class="exec-log" :class="`is-${log.status}`">
          <div class="log-top">
            <div>
              <b>{{ log.targetName }}</b>
              <span>{{ log.time }}</span>
              <em v-if="log.durationMs">{{ (log.durationMs / 1000).toFixed(1) }}s</em>
            </div>
            <el-tag size="small" :type="log.status === 'success' ? 'success' : log.status === 'running' ? 'info' : 'danger'">
              {{ log.status === 'running' ? '运行中' : log.status === 'success' ? '成功' : '失败' }}
            </el-tag>
          </div>
          <code>{{ log.command }}</code>
          <pre v-if="log.stdout" class="cmd-output">{{ log.stdout }}</pre>
          <pre v-if="log.stderr" class="cmd-output err">{{ log.stderr }}</pre>
          <p v-if="log.error" class="cmd-error">{{ log.error }}</p>
        </div>
      </div>
    </section>

    <el-tabs v-if="!error && (activeWorkspace === 'asset' || activeWorkspace === 'monitor')" v-model="activeTab" class="np-tabs">
      <el-tab-pane v-for="tab in visibleTabs" :key="tab.name" :name="tab.name">
        <template #label>
          <span class="tab-label">
            <GlassIcon :name="tab.icon" :from="tab.color[0]" :to="tab.color[1]" :size="22" />
            {{ tab.label }}
            <span class="tab-count">{{ targets.filter((t) => t.type === tab.name).length }}</span>
          </span>
        </template>
        <div v-if="filtered.length" class="card-grid">
          <DeviceCard
            v-for="t in filtered"
            :key="t.id"
            :target="t"
            :metrics="latestMap[t.id]"
            @edit="openEdit"
            @detail="onDetail"
            @probe="onProbe"
            @remove="onRemove"
            @preview="onPreview"
            @terminal="openTerminal"
          />
        </div>
        <div v-else class="empty-state glass">
          <GlassIcon :name="tab.icon" :from="tab.color[0]" :to="tab.color[1]" :size="48" />
          <p>暂无{{ tab.label }}{{ activeWorkspace === 'monitor' ? '监控项' : '设备' }}</p>
          <el-button type="primary" @click="activeWorkspace === 'monitor' ? openAddMonitorItem(tab.name) : openAddAsset()">
            {{ activeWorkspace === 'monitor' ? '添加监控项' : '添加第一台' }}
          </el-button>
        </div>
      </el-tab-pane>
    </el-tabs>

    <section v-if="!error && activeWorkspace === 'deploy'" class="deploy-workspace glass">
      <div class="exec-head">
        <div>
          <h3>发布部署</h3>
          <span>模板、备份、灰度、回滚和执行审计统一管理</span>
        </div>
        <div class="exec-head-actions">
          <el-button @click="openConfigCenter">模板配置</el-button>
          <el-button type="primary" @click="openDeployPanel">配置发布流程</el-button>
        </div>
      </div>
      <div class="deploy-step-list">
        <div v-for="(step, index) in deploySteps" :key="step.title" class="deploy-step">
          <b>{{ index + 1 }}</b>
          <span>
            <strong>{{ step.title }}</strong>
            <small>{{ step.desc }}</small>
          </span>
        </div>
      </div>
      <div v-if="deployRuns.length" class="run-history deploy-history">
        <div class="section-title">发布历史</div>
        <div class="run-history-list">
          <button v-for="run in deployRuns" :key="run.id" type="button" class="run-card" @click="showCommandRun(run)">
            <span>
              <b>#{{ run.id }} · {{ run.note || '发布部署' }}</b>
              <small>{{ runDateText(run.started_at) }} · {{ run.success_count }}/{{ run.total_count }} 成功</small>
            </span>
            <el-tag size="small" :type="runStatusType(run.status)">{{ runStatusText(run.status) }}</el-tag>
          </button>
        </div>
      </div>
      <div v-else class="exec-empty">
        暂无发布记录
        <el-button size="small" type="primary" @click="openDeployPanel">开始第一次发布</el-button>
      </div>
    </section>

    <DeviceForm
      v-model:visible="formVisible"
      :target="editing"
      :default-type="formDefaultType"
      :credentials="credentials"
      @submit="onSubmit"
    />
    <AgentManager v-model:visible="agentVisible" @changed="loadAll" />
    <DeviceDetail v-model:visible="detailVisible" :target="detailTarget" />

    <el-drawer v-model="deployVisible" title="发布部署" size="560px">
      <div class="deploy-panel">
        <div class="deploy-note">
          部署会先备份当前目录，再拉取 Git 仓库、执行构建命令并重启服务。开启灰度时只先部署选择列表里的第一台。
        </div>
        <el-form label-width="92px">
          <el-form-item label="模板">
            <div class="deploy-profile-row">
              <el-select
                v-model="deployConfigId"
                clearable
                placeholder="选择已保存的发布流程"
                style="width: 100%"
                @change="loadDeployProfile"
              >
                <el-option
                  v-for="p in deployProfiles"
                  :key="p.id"
                  :label="`${p.name} · ${p.env || 'default'}`"
                  :value="p.id"
                />
              </el-select>
              <el-button @click="saveDeployProfile">保存模板</el-button>
            </div>
          </el-form-item>
          <el-form-item label="服务器" required>
            <el-select v-model="deployForm.targetIds" multiple collapse-tags collapse-tags-tooltip style="width: 100%">
              <el-option
                v-for="t in sshTargets"
                :key="t.id"
                :label="`${t.name} · ${targetAddr(t)}`"
                :value="t.id"
              />
            </el-select>
          </el-form-item>
          <el-form-item label="Git 仓库" required>
            <el-input v-model="deployForm.repoUrl" placeholder="https://github.com/user/repo.git" />
          </el-form-item>
          <el-form-item label="分支">
            <el-input v-model="deployForm.branch" placeholder="main" />
          </el-form-item>
          <el-form-item label="项目目录" required>
            <el-input v-model="deployForm.projectDir" placeholder="/opt/app" />
          </el-form-item>
          <el-form-item label="构建命令">
            <el-input v-model="deployForm.buildCmd" type="textarea" :rows="3" resize="vertical" placeholder="npm install && npm run build" />
          </el-form-item>
          <el-form-item label="服务名">
            <el-input v-model="deployForm.serviceName" placeholder="ptpatronus.service，可留空" />
          </el-form-item>
          <el-form-item label="备份目录">
            <el-input v-model="deployForm.backupDir" placeholder="/opt/backups" />
          </el-form-item>
          <el-form-item label="灰度">
            <el-switch v-model="deployForm.grayFirst" active-text="先跑一台" inactive-text="全部执行" />
          </el-form-item>
        </el-form>
        <div class="deploy-step-list compact">
          <div v-for="(step, index) in deploySteps" :key="step.title" class="deploy-step">
            <b>{{ index + 1 }}</b>
            <span>
              <strong>{{ step.title }}</strong>
              <small>{{ step.desc }}</small>
            </span>
          </div>
        </div>
        <div class="deploy-preview">
          <div class="deploy-preview-title">将执行的部署脚本</div>
          <pre>{{ buildDeployCommand() }}</pre>
        </div>
        <div class="deploy-actions">
          <el-button @click="deployVisible = false">取消</el-button>
          <el-button type="warning" plain @click="runDeployCommand('rollback')">回滚上一版</el-button>
          <el-button type="primary" @click="runDeployCommand('deploy')">开始部署</el-button>
        </div>
      </div>
    </el-drawer>

    <el-drawer
      v-model="terminalVisible"
      :title="terminalTarget ? `${terminalTarget.name} · 在线终端` : '在线终端'"
      size="min(860px, 96vw)"
      @closed="closeTerminalSocket"
    >
      <div class="terminal-wrap">
        <div class="terminal-head">
          <span :class="{ on: terminalConnected }">{{ terminalConnected ? '已连接' : terminalConnecting ? '连接中' : '未连接' }}</span>
          <div>
            <el-button size="small" :loading="terminalConnecting" @click="connectTerminal">重连</el-button>
            <el-button size="small" type="warning" plain @click="sendTerminalCtrlC">Ctrl+C</el-button>
          </div>
        </div>
        <pre ref="terminalOutputEl" class="terminal-screen">{{ terminalOutput }}</pre>
        <div class="terminal-input">
          <el-input
            v-model="terminalInput"
            placeholder="输入命令，回车发送"
            :disabled="!terminalConnected"
            @keydown.enter.prevent="sendTerminalLine"
          />
          <el-button type="primary" :disabled="!terminalConnected" @click="sendTerminalLine">发送</el-button>
        </div>
      </div>
    </el-drawer>

    <el-drawer v-model="credentialVisible" title="凭据中心" size="520px">
      <div class="cred-toolbar">
        <span>统一管理 SSH/NAS 凭据，设备绑定后改一次全局生效。</span>
        <el-button type="primary" size="small" @click="openCredentialAdd">新增凭据</el-button>
      </div>
      <div v-if="credentials.length" class="cred-list">
        <div v-for="c in credentials" :key="c.id" class="cred-card">
          <div class="cred-main">
            <div>
              <b>{{ c.name }}</b>
              <span>{{ credentialTypeText(c.type) }} · {{ c.username || '无用户名' }}</span>
            </div>
            <el-tag size="small" type="info">{{ credentialUsage(c.id) }} 台设备</el-tag>
          </div>
          <div class="cred-flags">
            <span v-if="c.has_password">密码</span>
            <span v-if="c.has_private_key">私钥</span>
            <span v-if="c.tags">{{ c.tags }}</span>
          </div>
          <p v-if="c.note">{{ c.note }}</p>
          <div class="cred-actions">
            <el-button size="small" @click="openCredentialEdit(c)">编辑</el-button>
            <el-button size="small" type="danger" plain @click="removeCredential(c)">删除</el-button>
          </div>
        </div>
      </div>
      <div v-else class="cred-empty">
        暂无统一凭据
        <el-button type="primary" size="small" @click="openCredentialAdd">新增第一条</el-button>
      </div>
    </el-drawer>

    <el-drawer v-model="configVisible" title="配置中心" size="640px">
      <div class="cred-toolbar">
        <span>统一管理 KV、文本、JSON 配置，保存时自动生成历史版本。</span>
        <el-button type="primary" size="small" @click="openConfigAdd">新增配置</el-button>
      </div>
      <div v-if="opsConfigs.length" class="cred-list">
        <div v-for="c in opsConfigs" :key="c.id" class="cred-card config-card">
          <div class="cred-main">
            <div>
              <b>{{ c.name }}</b>
              <span>{{ c.key }} · {{ c.env || 'default' }}</span>
            </div>
            <el-tag size="small" type="info">v{{ c.version }} · {{ configFormatText(c.format) }}</el-tag>
          </div>
          <pre class="config-preview">{{ c.value || '空配置' }}</pre>
          <div class="cred-flags">
            <span v-if="c.tags">{{ c.tags }}</span>
            <span>{{ new Date(c.updated_at).toLocaleString('zh-CN', { hour12: false }) }}</span>
          </div>
          <p v-if="c.note">{{ c.note }}</p>
          <div class="cred-actions">
            <el-button size="small" @click="openConfigEdit(c)">编辑</el-button>
            <el-button size="small" @click="showConfigHistory(c)">历史</el-button>
            <el-button size="small" type="danger" plain @click="removeConfig(c)">删除</el-button>
          </div>
        </div>
      </div>
      <div v-else class="cred-empty">
        暂无配置
        <el-button type="primary" size="small" @click="openConfigAdd">新增第一条</el-button>
      </div>

      <section v-if="configHistoryTarget" class="config-history">
        <div class="section-title">历史版本 · {{ configHistoryTarget.name }}</div>
        <div v-if="configHistory.length" class="history-list">
          <div v-for="h in configHistory" :key="h.id" class="history-item">
            <div class="history-top">
              <b>v{{ h.version }}</b>
              <span>{{ new Date(h.created_at).toLocaleString('zh-CN', { hour12: false }) }}</span>
            </div>
            <pre>{{ h.value || '空配置' }}</pre>
          </div>
        </div>
        <div v-else class="cred-empty">暂无历史记录</div>
      </section>
    </el-drawer>

    <el-dialog
      v-model="credentialFormVisible"
      :title="editingCredential ? '编辑凭据' : '新增凭据'"
      width="520px"
      destroy-on-close
    >
      <el-form label-width="82px">
        <el-form-item label="名称" required>
          <el-input v-model="credentialForm.name" placeholder="如：生产 root / NAS 管理员" />
        </el-form-item>
        <el-form-item label="类型">
          <el-select v-model="credentialForm.type" style="width: 100%">
            <el-option label="SSH" value="ssh" />
            <el-option label="NAS" value="nas" />
            <el-option label="通用" value="generic" />
          </el-select>
        </el-form-item>
        <el-form-item label="用户名">
          <el-input v-model="credentialForm.username" placeholder="root / admin" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input
            v-model="credentialForm.password"
            type="password"
            show-password
            :placeholder="editingCredential ? '留空不改' : '密码 / Token / API Key'"
          />
        </el-form-item>
        <el-form-item label="私钥">
          <el-input
            v-model="credentialForm.private_key"
            type="textarea"
            :rows="6"
            resize="vertical"
            :placeholder="editingCredential?.has_private_key ? '已配置私钥，留空不改' : 'SSH 私钥，可留空'"
          />
        </el-form-item>
        <el-form-item label="标签">
          <el-input v-model="credentialForm.tags" placeholder="生产环境, root" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="credentialForm.note" type="textarea" :rows="3" resize="vertical" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="credentialFormVisible = false">取消</el-button>
        <el-button type="primary" @click="saveCredential">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="configFormVisible"
      :title="editingConfig ? '编辑配置' : '新增配置'"
      width="560px"
      destroy-on-close
    >
      <el-form label-width="82px">
        <el-form-item label="Key" required>
          <el-input v-model="configForm.key" placeholder="app.deploy.prod" />
        </el-form-item>
        <el-form-item label="名称" required>
          <el-input v-model="configForm.name" placeholder="生产发布配置" />
        </el-form-item>
        <el-form-item label="环境">
          <el-input v-model="configForm.env" placeholder="prod / test / dev" />
        </el-form-item>
        <el-form-item label="格式">
          <el-radio-group v-model="configForm.format">
            <el-radio-button label="kv">KV</el-radio-button>
            <el-radio-button label="text">文本</el-radio-button>
            <el-radio-button label="json">JSON</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="内容" required>
          <el-input
            v-model="configForm.value"
            type="textarea"
            :rows="9"
            resize="vertical"
            :placeholder="configValuePlaceholder"
          />
        </el-form-item>
        <el-form-item label="标签">
          <el-input v-model="configForm.tags" placeholder="生产, 发布" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="configForm.note" type="textarea" :rows="3" resize="vertical" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="configFormVisible = false">取消</el-button>
        <el-button type="primary" @click="saveConfig">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="helpVisible" title="使用说明" width="620px" class="help">
      <h4>选哪种监控方式？</h4>
      <p><b>服务器</b></p>
      <ul>
        <li><b>本机</b>：监控 PTPatronus 所在机器，无需填地址</li>
        <li><b>SSH 远程</b>：监控其它 Linux，填 IP + 账号 + 密码</li>
        <li><b>Agent 上报</b>：被监控机装 agent 主动上报 —— <b>推荐</b>，免 SSH 认证烦恼</li>
      </ul>
      <p><b>NAS</b>：国产 NAS 推荐 Agent 上报；群晖 / TrueNAS 也可用厂商 API，支持 SNMP 的设备可用通用 SNMP</p>
      <p><b>摄像头</b>：ONVIF / RTSP</p>

      <h4>SSH 连不上？</h4>
      <p>报 <code>no supported methods remain</code> = root 被禁密码登录（OpenSSH 默认安全策略）。</p>
      <ul>
        <li>点错误旁的「如何修复」→ 复制一条命令到服务器执行（放开 root 密码）</li>
        <li>或改用 <b>Agent 上报</b>，完全绕开 SSH</li>
      </ul>

      <h4>装 Agent（最省事）</h4>
      <ol>
        <li>点「+ 添加设备」→ 选择「服务器」或「NAS」→ 采集方式「Agent 上报」→ 填名 → 保存</li>
        <li>复制弹出的一条命令</li>
        <li>SSH / 宝塔进被监控机（Linux，root），粘贴执行（自动下载 + 装 systemd + 开机自启）</li>
        <li>回本页，设备自动上线并上报指标</li>
      </ol>

      <h4>其它</h4>
      <ul>
        <li>网站 / 端口 / 进程 / 脚本监控：点「+ 添加设备」后选择对应类型</li>
        <li>采集间隔 / 超时、告警阈值（CPU / 内存 / 磁盘 / 温度 / 离线）：编辑设备调整</li>
        <li>SSH 私钥认证：编辑 SSH 设备粘贴私钥（高级，适合禁密码的服务器）</li>
      </ul>
    </el-dialog>

    <el-dialog
      v-model="previewVisible"
      :title="previewTarget?.name + ' · 实时画面'"
      width="640px"
      destroy-on-close
    >
      <RtspPlayer v-if="previewTarget" :target="previewTarget" />
    </el-dialog>

    <el-drawer v-model="alertVisible" title="告警记录" size="420px">
      <AlertList :alerts="alerts" @read="onReadAlert" @ack="onAckAlert" @silence="onSilenceAlert" />
    </el-drawer>
  </div>
</template>

<style scoped>
.nas-page {
  padding: 4px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.np-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 18px;
}
.np-title {
  display: flex;
  align-items: center;
  gap: 12px;
}
.np-title h2 {
  margin: 0;
  font-size: 18px;
}
.np-title span {
  display: block;
  margin-top: 2px;
  font-size: 12px;
  color: var(--text-muted);
}
.np-actions {
  display: flex;
  gap: 8px;
  align-items: center;
}
.badge {
  display: inline-block;
  margin-left: 4px;
  min-width: 18px;
  padding: 0 5px;
  border-radius: 999px;
  background: #fff;
  color: #e74c3c;
  font-size: 11px;
  font-weight: 700;
}
.np-tabs {
  padding: 0 12px;
}
.tab-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.tab-count {
  font-size: 12px;
  color: var(--text-muted);
  background: var(--glass-bg);
  padding: 1px 7px;
  border-radius: 999px;
}
.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
  padding-top: 12px;
}
.empty-state {
  padding: 48px;
  text-align: center;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
}
.empty-state p {
  color: var(--text-muted);
  margin: 4px 0;
}
.error-state {
  padding: 40px;
  text-align: center;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
}
.err-text {
  color: var(--pt-danger, #e74c3c);
  font-size: 14px;
}
.ops-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(118px, 1fr));
  gap: 12px;
}
.ops-tile {
  min-height: 92px;
  border: 0;
  color: var(--text-strong);
  cursor: pointer;
  display: grid;
  align-content: center;
  justify-items: center;
  gap: 5px;
  padding: 12px 8px;
}
.ops-tile strong {
  font-size: 18px;
  line-height: 1;
}
.ops-tile span {
  font-size: 12px;
  color: var(--text-muted);
}
.ops-tile.active {
  outline: 2px solid rgba(66, 133, 244, 0.38);
  background: rgba(66, 133, 244, 0.12);
}
.exec-panel {
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.exec-head {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: flex-start;
}
.exec-head h3 {
  margin: 0;
  font-size: 16px;
  color: var(--text-strong);
}
.exec-head span {
  display: block;
  margin-top: 3px;
  font-size: 12px;
  color: var(--text-muted);
}
.exec-head-actions {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  justify-content: flex-end;
}
.exec-layout {
  display: grid;
  grid-template-columns: minmax(260px, 0.85fr) minmax(360px, 1.15fr);
  gap: 14px;
}
.exec-targets,
.exec-command {
  min-width: 0;
}
.target-list {
  max-height: 238px;
  overflow: auto;
  padding-right: 4px;
}
.target-list :deep(.el-checkbox-group) {
  display: grid;
  gap: 8px;
}
.target-check {
  margin-right: 0;
  min-height: 48px;
  padding: 8px 10px;
  border: 1px solid var(--glass-border);
  border-radius: 8px;
  background: rgba(148, 163, 184, 0.08);
}
.target-check :deep(.el-checkbox__label) {
  min-width: 0;
  flex: 1;
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto;
  gap: 10px;
  align-items: center;
}
.target-main {
  min-width: 0;
  display: grid;
  gap: 2px;
}
.target-main b,
.target-main small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.target-main small {
  color: var(--text-muted);
  font-size: 11px;
}
.target-state {
  font-size: 12px;
  color: var(--text-muted);
}
.target-state.online {
  color: var(--pt-success);
}
.target-state.offline {
  color: var(--pt-danger);
}
.target-terminal {
  border: 0;
  background: rgba(17, 153, 142, 0.12);
  color: #11998e;
  cursor: pointer;
  border-radius: 7px;
  padding: 4px 8px;
  font-size: 12px;
}
.exec-empty {
  min-height: 168px;
  display: grid;
  place-items: center;
  gap: 10px;
  color: var(--text-muted);
  border: 1px dashed var(--glass-border);
  border-radius: 8px;
}
.quick-row,
.history-row,
.exec-run-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.quick-row {
  margin-bottom: 8px;
}
.quick-row button,
.history-row button {
  max-width: 100%;
  border: 1px solid var(--glass-border);
  border-radius: 8px;
  background: rgba(66, 133, 244, 0.08);
  color: var(--text-strong);
  cursor: pointer;
  padding: 5px 9px;
  font-size: 12px;
}
.history-row {
  margin-top: 8px;
}
.history-row button {
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  background: rgba(148, 163, 184, 0.08);
  color: var(--text-muted);
}
.hidden-file {
  display: none;
}
.exec-run-row {
  margin-top: 10px;
  justify-content: flex-end;
}
.exec-run-row span {
  font-size: 12px;
  color: var(--text-muted);
}
.exec-log-list {
  display: grid;
  gap: 10px;
  max-height: 420px;
  overflow: auto;
}
.run-history {
  display: grid;
  gap: 9px;
}
.run-history-list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 8px;
}
.run-card {
  min-width: 0;
  border: 1px solid var(--glass-border);
  border-radius: 8px;
  background: rgba(148, 163, 184, 0.08);
  color: var(--text-strong);
  cursor: pointer;
  padding: 9px 10px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
  text-align: left;
}
.run-card span {
  min-width: 0;
  display: grid;
  gap: 3px;
}
.run-card b,
.run-card small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.run-card small {
  color: var(--text-muted);
  font-size: 11px;
}
.exec-log {
  padding: 10px 12px;
  border-radius: 8px;
  border: 1px solid var(--glass-border);
  background: rgba(148, 163, 184, 0.08);
}
.exec-log.is-success {
  border-left: 3px solid var(--pt-success);
}
.exec-log.is-error {
  border-left: 3px solid var(--pt-danger);
}
.exec-log.is-running {
  border-left: 3px solid var(--pt-primary-2);
}
.log-top {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  align-items: center;
  margin-bottom: 6px;
}
.log-top div {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.log-top b {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.log-top span,
.log-top em {
  flex: 0 0 auto;
  font-size: 12px;
  color: var(--text-muted);
  font-style: normal;
}
.exec-log code {
  display: block;
  margin-bottom: 6px;
  font-size: 12px;
  color: var(--text-muted);
  word-break: break-all;
}
.cmd-output {
  margin: 6px 0 0;
  padding: 9px;
  max-height: 220px;
  overflow: auto;
  border-radius: 8px;
  background: rgba(15, 23, 42, 0.86);
  color: #e5e7eb;
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
}
.cmd-output.err {
  color: #fecaca;
}
.cmd-error {
  margin: 6px 0 0;
  color: var(--pt-danger);
  font-size: 12.5px;
  word-break: break-word;
}
.terminal-wrap {
  height: calc(100vh - 128px);
  min-height: 480px;
  display: grid;
  grid-template-rows: auto minmax(0, 1fr) auto;
  gap: 10px;
}
.terminal-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}
.terminal-head span {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--text-muted);
  font-size: 12px;
}
.terminal-head span::before {
  content: '';
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #9ca3af;
}
.terminal-head span.on {
  color: var(--pt-success);
}
.terminal-head span.on::before {
  background: var(--pt-success);
  box-shadow: 0 0 6px var(--pt-success);
}
.terminal-screen {
  margin: 0;
  padding: 12px;
  overflow: auto;
  border-radius: 8px;
  background: #0b1020;
  color: #d1fae5;
  font-size: 12.5px;
  line-height: 1.55;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}
.terminal-input {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 8px;
}
.deploy-panel {
  display: grid;
  gap: 12px;
}
.deploy-workspace {
  padding: 16px;
  display: grid;
  gap: 14px;
}
.deploy-step-list {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(176px, 1fr));
  gap: 8px;
}
.deploy-step-list.compact {
  grid-template-columns: 1fr;
}
.deploy-step {
  min-width: 0;
  display: flex;
  gap: 9px;
  align-items: flex-start;
  padding: 10px;
  border: 1px solid var(--glass-border);
  border-radius: 8px;
  background: rgba(148, 163, 184, 0.08);
}
.deploy-step b {
  width: 22px;
  height: 22px;
  flex: none;
  border-radius: 999px;
  display: grid;
  place-items: center;
  background: rgba(66, 133, 244, 0.16);
  color: var(--pt-primary-2);
  font-size: 12px;
}
.deploy-step span {
  min-width: 0;
  display: grid;
  gap: 2px;
}
.deploy-step strong,
.deploy-step small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.deploy-step strong {
  color: var(--text-strong);
  font-size: 13px;
}
.deploy-step small {
  color: var(--text-muted);
  font-size: 12px;
}
.deploy-history {
  margin-top: 2px;
}
.deploy-note {
  padding: 10px 12px;
  border-radius: 8px;
  background: rgba(66, 133, 244, 0.1);
  color: var(--text-muted);
  font-size: 12.5px;
  line-height: 1.55;
}
.deploy-preview {
  border: 1px solid var(--glass-border);
  border-radius: 8px;
  overflow: hidden;
}
.deploy-preview-title {
  padding: 8px 10px;
  color: var(--text-muted);
  font-size: 12px;
  background: rgba(148, 163, 184, 0.08);
}
.deploy-preview pre {
  margin: 0;
  padding: 10px;
  max-height: 260px;
  overflow: auto;
  background: #0b1020;
  color: #d1fae5;
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
}
.deploy-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  flex-wrap: wrap;
}
.deploy-profile-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 8px;
  width: 100%;
}
.cred-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}
.cred-toolbar span {
  min-width: 0;
  color: var(--text-muted);
  font-size: 12.5px;
  line-height: 1.5;
}
.cred-list {
  display: grid;
  gap: 10px;
}
.cred-card {
  padding: 12px;
  border: 1px solid var(--glass-border);
  border-radius: 8px;
  background: rgba(148, 163, 184, 0.08);
}
.cred-main {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 10px;
}
.cred-main div {
  min-width: 0;
  display: grid;
  gap: 3px;
}
.cred-main b,
.cred-main span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.cred-main span {
  color: var(--text-muted);
  font-size: 12px;
}
.cred-flags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 9px;
}
.cred-flags span {
  padding: 2px 7px;
  border-radius: 999px;
  background: rgba(66, 133, 244, 0.12);
  color: var(--pt-primary-2);
  font-size: 12px;
}
.cred-card p {
  margin: 8px 0 0;
  color: var(--text-muted);
  font-size: 12.5px;
  line-height: 1.5;
  word-break: break-word;
}
.cred-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 10px;
}
.cred-empty {
  min-height: 180px;
  display: grid;
  place-items: center;
  gap: 10px;
  color: var(--text-muted);
  border: 1px dashed var(--glass-border);
  border-radius: 8px;
}
.config-preview,
.history-item pre {
  max-height: 150px;
  overflow: auto;
  margin: 10px 0 0;
  padding: 9px 10px;
  border-radius: 8px;
  background: rgba(15, 23, 42, 0.06);
  color: var(--text-strong);
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.config-history {
  margin-top: 16px;
}
.section-title {
  margin: 0 0 10px;
  font-weight: 700;
  color: var(--text-strong);
}
.history-list {
  display: grid;
  gap: 10px;
}
.history-item {
  padding: 10px;
  border: 1px solid var(--glass-border);
  border-radius: 8px;
}
.history-top {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  color: var(--text-muted);
  font-size: 12px;
}
.history-top b {
  color: var(--text-strong);
}
.help h4 {
  margin: 14px 0 6px;
  font-size: 14px;
  color: var(--text-strong);
}
.help h4:first-child {
  margin-top: 0;
}
.help p,
.help ul,
.help ol {
  margin: 4px 0;
  font-size: 13px;
  line-height: 1.7;
}
.help ul,
.help ol {
  padding-left: 20px;
}
.help code {
  background: var(--glass-bg);
  padding: 1px 5px;
  border-radius: 4px;
  font-size: 12px;
}

@media (max-width: 760px) {
  .np-head,
  .exec-head {
    flex-direction: column;
  }
  .np-actions,
  .exec-head-actions {
    width: 100%;
    justify-content: flex-start;
    flex-wrap: wrap;
  }
  .ops-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 8px;
  }
  .ops-tile {
    min-height: 74px;
    padding: 9px 6px;
  }
  .exec-layout {
    grid-template-columns: 1fr;
  }
  .exec-panel {
    padding: 12px;
  }
  .run-history-list,
  .deploy-step-list {
    grid-template-columns: 1fr;
  }
  .target-check :deep(.el-checkbox__label) {
    grid-template-columns: minmax(0, 1fr) auto;
  }
  .target-terminal {
    grid-column: 1 / -1;
    justify-self: start;
  }
  .terminal-wrap {
    height: calc(100vh - 118px);
    min-height: 420px;
  }
  .deploy-profile-row {
    grid-template-columns: 1fr;
  }
  .cred-toolbar {
    flex-direction: column;
    align-items: stretch;
  }
  .log-top {
    align-items: flex-start;
  }
  .log-top div {
    display: grid;
    gap: 2px;
  }
}

@media (max-width: 480px) {
  .ops-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
