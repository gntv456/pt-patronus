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
import NasOpsGrid from '@/components/nas/NasOpsGrid.vue'
import NasExecPanel from '@/components/nas/NasExecPanel.vue'
import NasDeployWorkspace from '@/components/nas/NasDeployWorkspace.vue'
import NasDeployDrawer from '@/components/nas/NasDeployDrawer.vue'
import NasTerminalDrawer from '@/components/nas/NasTerminalDrawer.vue'
import NasCredentialDrawer from '@/components/nas/NasCredentialDrawer.vue'
import NasConfigDrawer from '@/components/nas/NasConfigDrawer.vue'
import NasHelpDialog from '@/components/nas/NasHelpDialog.vue'
import {
  monitorApi, commandRunApi, credentialApi, opsConfigApi, parseMetrics,
  type MonitorTarget, type Metrics, type MonitorAlert, type CommandRun,
} from '@/api/monitor'
import { useAuthStore } from '@/stores/auth'
import { tabs, isSSHHost } from './nasServerHelpers'
import { useNasCredentials } from './useNasCredentials'
import { useNasOpsConfigs } from './useNasOpsConfigs'
import { useCommandExec } from './useCommandExec'
import { useDeploy } from './useDeploy'
import { useDeviceCrud } from './useDeviceCrud'
import { useTerminal } from './useTerminal'

const auth = useAuthStore()
const router = useRouter()
type MonitorTab = 'host' | 'nas' | 'camera' | 'http' | 'tcp' | 'process' | 'script'
type Workspace = 'asset' | 'execute' | 'monitor' | 'deploy'

const targets = ref<MonitorTarget[]>([])
const latestMap = ref<Record<number, Metrics | null>>({})
const alerts = ref<MonitorAlert[]>([])
const activeTab = ref<MonitorTab>('host')
const activeWorkspace = ref<Workspace>('asset')

const { credentials, credentialVisible, credentialFormVisible, editingCredential, credentialForm,
  openCredentialCenter, openCredentialAdd, openCredentialEdit, credentialUsage, saveCredential, removeCredential,
} = useNasCredentials(targets, loadAll)

const { opsConfigs, configVisible, configFormVisible, editingConfig, configHistory, configHistoryTarget, configForm,
  openConfigCenter, openConfigAdd, openConfigEdit, saveConfig, removeConfig, showConfigHistory,
} = useNasOpsConfigs()

const helpVisible = ref(false)
const agentVisible = ref(false)

const filtered = computed(() => targets.value.filter((t) => t.type === activeTab.value))
const unreadCount = computed(() => alerts.value.filter((a) => !a.read).length)
const hostCount = computed(() => targets.value.filter((t) => t.type === 'host').length)
const monitorCount = computed(() => targets.value.filter((t) => ['http', 'tcp', 'process', 'script'].includes(t.type)).length)
const configCount = computed(() => opsConfigs.value.length)
const activeAlertCount = computed(() => alerts.value.filter((a) => !a.resolved_at).length)
const sshTargets = computed(() => targets.value.filter(isSSHHost))

const { scriptFileInput, execTargetIds, execCommand, execTimeout, execRunning, execLogs, commandHistory,
  commandRuns, selectedCommandRun, selectedExecTargets, loadCommandHistory,
  selectAllExecTargets, selectOnlineExecTargets, clearExecTargets, applyCommand,
  pickScriptFile, importScriptFile, showCommandRun, runBatchCommand,
} = useCommandExec(sshTargets, focusExecPanel)

const { deployVisible, deployConfigId, deployForm, deployProfiles, deployRuns, deploySteps,
  openDeployPanel, loadDeployProfile, saveDeployProfile, runDeployCommand, buildDeployCommand, buildRollbackCommand,
} = useDeploy({ sshTargets, selectedExecTargets, opsConfigs, commandRuns, execTargetIds, execCommand, execTimeout,
  runBatchCommand, focusExecPanel, enterDeployWorkspace: () => setWorkspace('deploy') })

const assetTabs = computed(() => tabs.filter((t) => ['host', 'nas', 'camera'].includes(t.name)))
const monitorTabs = computed(() => tabs.filter((t) => ['http', 'tcp', 'process', 'script'].includes(t.name)))
const visibleTabs = computed(() => (activeWorkspace.value === 'monitor' ? monitorTabs.value : assetTabs.value))
let timer: number | undefined
const error = ref('')
const firstLoad = ref(true)

async function loadAll() {
  try {
    const [ts, al] = await Promise.all([monitorApi.list(), monitorApi.alerts(undefined, true).catch(() => [])])
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
    // 每个 target 取最新一条快照作为卡片实时指标（末项才是最新）。
    await Promise.all(ts.map(async (t) => {
      const snaps = await monitorApi.metrics(t.id, undefined, 1).catch(() => [])
      latestMap.value[t.id] = snaps.length ? parseMetrics(snaps[snaps.length - 1]) : null
    }))
    error.value = ''
  } catch (e) {
    if (firstLoad.value) error.value = (e as Error).message || '加载失败'
  } finally {
    firstLoad.value = false
  }
}

function setWorkspace(workspace: Workspace) {
  activeWorkspace.value = workspace
  if (workspace === 'asset' && !['host', 'nas', 'camera'].includes(activeTab.value)) activeTab.value = 'host'
  if (workspace === 'monitor' && !['http', 'tcp', 'process', 'script'].includes(activeTab.value)) activeTab.value = 'http'
}

async function retry() { firstLoad.value = true; await loadAll() }

const { formVisible, editing, formDefaultType, detailVisible, detailTarget, previewVisible, previewTarget, alertVisible,
  openAddAsset, openAddMonitorItem, openAdd, openEdit, onSubmit, onProbe, onRemove, onDetail, onPreview,
  onReadAlert, onAckAlert, onSilenceAlert,
} = useDeviceCrud(auth, { activeWorkspace, activeTab, credentials, reload: loadAll, setWorkspace })

const { terminalVisible, terminalTarget, terminalOutput, terminalInput, terminalConnected, terminalConnecting,
  openTerminal, closeTerminalSocket, connectTerminal, sendTerminalLine, sendTerminalCtrlC,
} = useTerminal(auth)

function focusExecPanel() {
  setWorkspace('execute')
  nextTick(() => document.querySelector('.exec-panel')?.scrollIntoView({ behavior: 'smooth', block: 'start' }))
}
function openMonitorTab() {
  setWorkspace('monitor')
  activeTab.value = 'http'
  nextTick(() => document.querySelector('.np-tabs')?.scrollIntoView({ behavior: 'smooth', block: 'start' }))
}
function go(name: string) { router.push({ name }) }

onMounted(() => { loadCommandHistory(); loadAll(); timer = window.setInterval(loadAll, 15000) })
onBeforeUnmount(() => { if (timer) window.clearInterval(timer); closeTerminalSocket() })
</script>

<template>
  <div class="nas-page">
    <header class="np-head glass-bar">
      <div class="np-title">
        <GlassIcon name="server" from="#0f2027" to="#2c5364" :size="32" />
        <div><h2>NAS 运维控制台</h2><span>主机、文件、发布、监控、告警</span></div>
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

    <NasOpsGrid v-if="!error" :active-workspace="activeWorkspace" :host-count="hostCount" :monitor-count="monitorCount"
      :ssh-target-count="sshTargets.length" :credential-count="credentials.length" :config-count="configCount"
      :active-alert-count="activeAlertCount"
      @workspace="setWorkspace" @execute="focusExecPanel" @go="go" @deploy="openDeployPanel"
      @credentials="openCredentialCenter" @configs="openConfigCenter" @monitor="openMonitorTab" />

    <NasExecPanel v-if="!error && activeWorkspace === 'execute'" :exec-target-ids="execTargetIds"
      :ssh-targets="sshTargets" :selected-exec-targets="selectedExecTargets" :selected-command-run="selectedCommandRun"
      :exec-command="execCommand" :exec-timeout="execTimeout" :exec-running="execRunning" :command-history="commandHistory"
      :command-runs="commandRuns" :exec-logs="execLogs" :is-admin="auth.isAdmin"
      @update:exec-target-ids="execTargetIds = $event" @update:exec-command="execCommand = $event"
      @update:exec-timeout="execTimeout = $event" @select-online="selectOnlineExecTargets" @select-all="selectAllExecTargets"
      @clear-targets="clearExecTargets" @apply-command="applyCommand" @pick-script-file="pickScriptFile"
      @import-script-file="importScriptFile" @run-batch="runBatchCommand()" @show-command-run="showCommandRun"
      @terminal="openTerminal" />

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
          <DeviceCard v-for="t in filtered" :key="t.id" :target="t" :metrics="latestMap[t.id]"
            @edit="openEdit" @detail="onDetail" @probe="onProbe" @remove="onRemove" @preview="onPreview" @terminal="openTerminal" />
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

    <NasDeployWorkspace v-if="!error && activeWorkspace === 'deploy'" :steps="deploySteps" :runs="deployRuns"
      @configs="openConfigCenter" @open-panel="openDeployPanel" @show-run="showCommandRun" />

    <DeviceForm v-model:visible="formVisible" :target="editing" :default-type="formDefaultType" :credentials="credentials" @submit="onSubmit" />
    <AgentManager v-model:visible="agentVisible" @changed="loadAll" />
    <DeviceDetail v-model:visible="detailVisible" :target="detailTarget" />

    <NasDeployDrawer :visible="deployVisible" :config-id="deployConfigId" :form="deployForm" :profiles="deployProfiles"
      :runs="deployRuns" :steps="deploySteps" :ssh-targets="sshTargets" :deploy-command="buildDeployCommand()"
      @update:visible="deployVisible = $event" @update:config-id="deployConfigId = $event"
      @load-profile="loadDeployProfile" @save-profile="saveDeployProfile" @run="runDeployCommand" />

    <NasTerminalDrawer :visible="terminalVisible" :target="terminalTarget" :output="terminalOutput" :input="terminalInput"
      :connected="terminalConnected" :connecting="terminalConnecting"
      @update:visible="terminalVisible = $event" @update:input="terminalInput = $event" @closed="closeTerminalSocket"
      @connect="connectTerminal" @send-line="sendTerminalLine" @ctrl-c="sendTerminalCtrlC" />

    <NasCredentialDrawer :drawer-visible="credentialVisible" :credentials="credentials" :form-visible="credentialFormVisible"
      :editing="editingCredential" :form="credentialForm" :credential-usage="credentialUsage"
      @update:drawer-visible="credentialVisible = $event" @update:form-visible="credentialFormVisible = $event"
      @add="openCredentialAdd" @edit="openCredentialEdit" @remove="removeCredential" @save="saveCredential" />

    <NasConfigDrawer :drawer-visible="configVisible" :configs="opsConfigs" :history="configHistory" :history-target="configHistoryTarget"
      :form-visible="configFormVisible" :editing="editingConfig" :form="configForm"
      @update:drawer-visible="configVisible = $event" @update:form-visible="configFormVisible = $event"
      @add="openConfigAdd" @edit="openConfigEdit" @remove="removeConfig" @show-history="showConfigHistory" @save="saveConfig" />

    <NasHelpDialog v-model:visible="helpVisible" />

    <el-dialog v-model="previewVisible" :title="previewTarget?.name + ' · 实时画面'" width="640px" destroy-on-close>
      <RtspPlayer v-if="previewTarget" :target="previewTarget" />
    </el-dialog>

    <el-drawer v-model="alertVisible" title="告警记录" size="420px">
      <AlertList :alerts="alerts" @read="onReadAlert" @ack="onAckAlert" @silence="onSilenceAlert" />
    </el-drawer>
  </div>
</template>

<style scoped src="./NasServer.css"></style>
