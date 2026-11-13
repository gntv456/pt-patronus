<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, computed, nextTick, watch } from 'vue'
import SptIcon from '@/components/SptIcon.vue'
import { useQueryState } from '@/composables/useUrlState'
import { useFeedback } from '@/composables/useFeedback'
import { useRouter } from 'vue-router'
import DeviceCard from '@/components/monitor/DeviceCard.vue'
import DeviceForm from '@/components/monitor/DeviceForm.vue'
import DeviceDetail from '@/components/monitor/DeviceDetail.vue'
import RtspPlayer from '@/components/monitor/RtspPlayer.vue'
import AgentManager from '@/components/monitor/AgentManager.vue'
import NasOpsGrid from '@/components/nas/NasOpsGrid.vue'
import NasExecPanel from '@/components/nas/NasExecPanel.vue'
import NasDeployWorkspace from '@/components/nas/NasDeployWorkspace.vue'
import NasDeployDrawer from '@/components/nas/NasDeployDrawer.vue'
import NasTerminalDrawer from '@/components/nas/NasTerminalDrawer.vue'
import NasCredentialDrawer from '@/components/nas/NasCredentialDrawer.vue'
import NasConfigDrawer from '@/components/nas/NasConfigDrawer.vue'
import NasHelpDialog from '@/components/nas/NasHelpDialog.vue'
import NasAlertDrawer from '@/components/nas/NasAlertDrawer.vue'
import {
  monitorApi,
  commandRunApi,
  credentialApi,
  opsConfigApi,
  parseMetrics,
  type MonitorTarget,
  type Metrics,
  type MonitorAlert,
  type CommandRun,
} from '@/api/monitor'
import { useAuthStore } from '@/stores/auth'
import { tabs, isSSHHost, type MonitorTab, type Workspace } from './nasServerHelpers'
import { useNasDerived } from './useNasDerived'
import { useNasCredentials } from './useNasCredentials'
import { useNasOpsConfigs } from './useNasOpsConfigs'
import { useCommandExec } from './useCommandExec'
import { useDeploy } from './useDeploy'
import { useDeviceCrud } from './useDeviceCrud'
import { useNasLoader } from './useNasLoader'
import { useNasPolling } from './useNasPolling'
import { useNasNav } from './useNasNav'
// 在线终端抽屉已自管理 WebSocket（useTerminal 不再需要）

const { t } = useI18n()
const auth = useAuthStore()
const router = useRouter()

const targets = ref<MonitorTarget[]>([])
const latestMap = ref<Record<number, Metrics | null>>({})
const alerts = ref<MonitorAlert[]>([])
const activeTab = ref<MonitorTab>('host')
const activeWorkspace = ref<Workspace>('asset')

// 工作区 / Tab 写回 URL query：15s 轮询与刷新后不丢状态（skill: deep-linking）。
const wsQuery = useQueryState('ws', 'asset')
const tabQuery = useQueryState('tab', 'host')
const VALID_WS: Workspace[] = ['asset', 'execute', 'monitor', 'deploy']
const VALID_TABS = tabs.map((x) => x.name) as MonitorTab[]
if (VALID_WS.includes(wsQuery.value as Workspace)) activeWorkspace.value = wsQuery.value as Workspace
if (VALID_TABS.includes(tabQuery.value as MonitorTab)) activeTab.value = tabQuery.value as MonitorTab
watch(activeWorkspace, (v) => {
  if (wsQuery.value !== v) wsQuery.value = v
})
watch(activeTab, (v) => {
  if (tabQuery.value !== v) tabQuery.value = v
})
watch(
  () => wsQuery.value,
  (v) => {
    if (VALID_WS.includes(v as Workspace) && v !== activeWorkspace.value) activeWorkspace.value = v as Workspace
  },
)
watch(
  () => tabQuery.value,
  (v) => {
    if (VALID_TABS.includes(v as MonitorTab) && v !== activeTab.value) activeTab.value = v as MonitorTab
  },
)

// 卡片列表多选 + 批量删除（把执行面板已有的「全选」推广到资产/监控卡片）。
const selectedIds = ref<number[]>([])
const feedback = useFeedback()
function toggleSelect(id: number) {
  const i = selectedIds.value.indexOf(id)
  if (i >= 0) selectedIds.value.splice(i, 1)
  else selectedIds.value.push(id)
}
function toggleSelectAll() {
  const list = filtered.value
  selectedIds.value = selectedIds.value.length === list.length ? [] : list.map((x) => x.id)
}
async function batchRemove() {
  const ids = [...selectedIds.value]
  if (!ids.length) return
  const ok = await feedback.confirm({
    title: t('common.remove'),
    detail: t('nas.batchRemoveConfirm', { n: ids.length }),
    danger: true,
  })
  if (!ok) return
  try {
    await Promise.all(ids.map((id) => monitorApi.remove(id)))
    selectedIds.value = []
    feedback.toast(t('common.deleted'))
    await loadAll()
  } catch (e) {
    feedback.error((e as Error).message)
    await loadAll()
  }
}

function go(name: string) {
  router.push({ name })
}

// 工作区切换/终端/导航拆出 useNasNav
const { terminalVisible, terminalTarget, openTerminal, setWorkspace, focusExecPanel, openMonitorTab } = useNasNav(
  activeWorkspace,
  activeTab,
)

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
} = useNasCredentials(targets, () => loadAll())

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

const helpVisible = ref(false)
const agentVisible = ref(false)

const { filtered, unreadCount, hostCount, monitorCount, configCount, activeAlertCount, sshTargets } = useNasDerived({
  targets,
  alerts,
  opsConfigs,
  activeTab,
})

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

const {
  deployVisible,
  deployConfigId,
  deployForm,
  deployProfiles,
  deployRuns,
  deploySteps,
  openDeployPanel,
  loadDeployProfile,
  saveDeployProfile,
  runDeployCommand,
  buildDeployCommand,
  buildRollbackCommand,
} = useDeploy({
  sshTargets,
  selectedExecTargets,
  opsConfigs,
  commandRuns,
  execTargetIds,
  execCommand,
  execTimeout,
  runBatchCommand,
  focusExecPanel,
  enterDeployWorkspace: () => setWorkspace('deploy'),
})

// 全量加载(设备/告警/凭据/配置/执行历史/最新指标)拆出 useNasLoader
const { error, firstLoad, alertsFailed, alertsTruncated, alertsTotal, runsTruncated, runsTotal, loadAll, retry } =
  useNasLoader({
    isAdmin: auth.isAdmin,
    targets,
    alerts,
    credentials,
    opsConfigs,
    commandRuns,
    latestMap,
    t: (k: string) => t(k),
  })

const assetTabs = computed(() => tabs.filter((t) => ['host', 'nas', 'camera'].includes(t.name)))
const monitorTabs = computed(() => tabs.filter((t) => ['http', 'tcp', 'process', 'script'].includes(t.name)))
const visibleTabs = computed(() => (activeWorkspace.value === 'monitor' ? monitorTabs.value : assetTabs.value))
const {
  formVisible,
  editing,
  formDefaultType,
  detailVisible,
  detailTarget,
  previewVisible,
  previewTarget,
  alertVisible,
  openAddAsset,
  openAddMonitorItem,
  openAdd,
  openEdit,
  onSubmit,
  onProbe,
  onRemove,
  onDetail,
  onPreview,
  onReadAlert,
  onAckAlert,
  onSilenceAlert,
  onDeleteAlert,
  onClearAlerts,
} = useDeviceCrud(auth, { activeWorkspace, activeTab, credentials, reload: loadAll, setWorkspace })

// 15s 轮询(隐藏暂停/可见恢复)拆出 useNasPolling
useNasPolling(loadAll, 15000, loadCommandHistory)
</script>

<template>
  <div class="nas-page">
    <header class="np-head solid-card">
      <div class="np-title">
        <h2>{{ t('nas.title') }}</h2>
      </div>
      <div class="np-actions">
        <el-button :type="unreadCount ? 'danger' : alertsFailed ? 'warning' : 'default'" @click="alertVisible = true">
          {{ t('nas.alerts') }}<span v-if="unreadCount" class="badge">{{ unreadCount }}</span>
          <!-- 读不到 / 被上限截断都要说出来：不能因为拉取失败或只取了最近 200 条就显示成「没有未读」 -->
          <span v-else-if="alertsFailed" class="badge">?</span>
          <span
            v-if="alertsTruncated"
            class="badge"
            :title="t('nas.alertsCapped', { shown: alerts.length, total: alertsTotal })"
            >{{ t('nas.alertsCapped', { shown: alerts.length, total: alertsTotal }) }}</span
          >
        </el-button>
        <!-- 7 个等权头部按钮收敛：仅保留「新增资产」为主 CTA，低频操作进「更多」下拉 -->
        <el-dropdown trigger="click">
          <el-button aria-label="更多操作">更多<SptIcon name="chevron-down" :size="14" aria-hidden="true" /></el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item @click="helpVisible = true">{{ t('nas.help') }}</el-dropdown-item>
              <el-dropdown-item v-if="auth.isAdmin" @click="openCredentialCenter">{{ t('nas.credentials') }}</el-dropdown-item>
              <el-dropdown-item v-if="auth.isAdmin" @click="openConfigCenter">{{ t('trakt.config') }}</el-dropdown-item>
              <el-dropdown-item v-if="auth.isAdmin" @click="agentVisible = true">Agent</el-dropdown-item>
              <el-dropdown-item @click="openAddMonitorItem()">{{ t('nas.addMonitorItem') }}</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <el-button type="primary" @click="openAddAsset">{{ t('nas.addAsset') }}</el-button>
      </div>
    </header>

    <div v-if="error" class="empty solid-card error-state" role="alert">
      <div class="err-text">
        <SptIcon name="alert-triangle" :size="20" aria-hidden="true" /> {{ error }}
      </div>
      <el-button size="small" @click="retry">{{ t('common.retry') }}</el-button>
    </div>

    <NasOpsGrid
      v-if="!error"
      :active-workspace="activeWorkspace"
      :host-count="hostCount"
      :monitor-count="monitorCount"
      :ssh-target-count="sshTargets.length"
      :credential-count="credentials.length"
      :config-count="configCount"
      :active-alert-count="activeAlertCount"
      @workspace="(w: string) => setWorkspace(w as Workspace)"
      @execute="focusExecPanel"
      @go="go"
      @deploy="openDeployPanel"
      @credentials="openCredentialCenter"
      @configs="openConfigCenter"
      @monitor="openMonitorTab"
    />

    <NasExecPanel
      v-if="!error && activeWorkspace === 'execute'"
      :exec-target-ids="execTargetIds"
      :ssh-targets="sshTargets"
      :selected-exec-targets="selectedExecTargets"
      :selected-command-run="selectedCommandRun"
      :exec-command="execCommand"
      :exec-timeout="execTimeout"
      :exec-running="execRunning"
      :command-history="commandHistory"
      :command-runs="commandRuns"
      :runs-truncated="runsTruncated"
      :runs-total="runsTotal"
      :exec-logs="execLogs"
      :is-admin="auth.isAdmin"
      @update:exec-target-ids="execTargetIds = $event"
      @update:exec-command="execCommand = $event"
      @update:exec-timeout="execTimeout = $event"
      @select-online="selectOnlineExecTargets"
      @select-all="selectAllExecTargets"
      @clear-targets="clearExecTargets"
      @apply-command="applyCommand"
      @pick-script-file="pickScriptFile"
      @import-script-file="importScriptFile"
      @run-batch="runBatchCommand()"
      @show-command-run="showCommandRun"
      @terminal="openTerminal"
    />

    <el-tabs
      v-if="!error && (activeWorkspace === 'asset' || activeWorkspace === 'monitor')"
      v-model="activeTab"
      class="np-tabs"
    >
      <el-tab-pane v-for="tab in visibleTabs" :key="tab.name" :name="tab.name">
        <template #label>
          <span class="tab-label"
            >{{ tab.label }}
            <span class="tab-count">{{ targets.filter((t) => t.type === tab.name).length }}</span></span
          >
        </template>
        <div v-if="filtered.length > 1" class="card-batch-bar">
          <el-checkbox
            :model-value="filtered.length > 0 && selectedIds.length === filtered.length"
            :indeterminate="selectedIds.length > 0 && selectedIds.length < filtered.length"
            @change="toggleSelectAll"
            >全选</el-checkbox
          >
          <span v-if="selectedIds.length" class="cb-count">已选 {{ selectedIds.length }} 项</span>
          <el-button v-if="selectedIds.length" size="small" type="danger" plain @click="batchRemove">{{
            t('common.remove')
          }}</el-button>
        </div>
        <div v-if="filtered.length" class="card-grid">
          <div v-for="t in filtered" :key="t.id" class="card-select-item">
            <el-checkbox
              class="card-select-cb"
              :model-value="selectedIds.includes(t.id)"
              :aria-label="'选择 ' + t.name"
              @change="toggleSelect(t.id)"
            />
            <DeviceCard
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
        </div>
        <div v-else class="empty-state solid-card">
          <p style="margin-bottom: 8px"><SptIcon name="inbox" :size="40" aria-hidden="true" /></p>
          <p>
            {{ t('transfer.noRecords') }}{{ tab.label
            }}{{ activeWorkspace === 'monitor' ? t('nas.monitorItem') : t('nas.device') }}
          </p>
          <el-button
            type="primary"
            @click="activeWorkspace === 'monitor' ? openAddMonitorItem(tab.name) : openAddAsset()"
          >
            {{ activeWorkspace === 'monitor' ? t('nas.addMonitorItemAction') : t('nas.addFirst') }}
          </el-button>
        </div>
      </el-tab-pane>
    </el-tabs>

    <NasDeployWorkspace
      v-if="!error && activeWorkspace === 'deploy'"
      :steps="deploySteps"
      :runs="deployRuns"
      @configs="openConfigCenter"
      @open-panel="openDeployPanel"
      @show-run="showCommandRun"
    />

    <DeviceForm
      v-model:visible="formVisible"
      :target="editing"
      :default-type="formDefaultType"
      :credentials="credentials"
      @submit="onSubmit"
    />
    <AgentManager v-model:visible="agentVisible" @changed="loadAll" />
    <DeviceDetail v-model:visible="detailVisible" :target="detailTarget" />

    <NasDeployDrawer
      :visible="deployVisible"
      :config-id="deployConfigId"
      v-model:form="deployForm"
      :profiles="deployProfiles"
      :runs="deployRuns"
      :steps="deploySteps"
      :ssh-targets="sshTargets"
      :deploy-command="buildDeployCommand()"
      @update:visible="deployVisible = $event"
      @update:config-id="deployConfigId = $event"
      @load-profile="() => loadDeployProfile(deployConfigId)"
      @save-profile="saveDeployProfile"
      @run="runDeployCommand"
    />

    <NasTerminalDrawer
      :visible="terminalVisible"
      :target="terminalTarget"
      @update:visible="terminalVisible = $event"
      @closed="terminalVisible = false"
    />

    <NasCredentialDrawer
      :drawer-visible="credentialVisible"
      :credentials="credentials"
      :form-visible="credentialFormVisible"
      :editing="editingCredential"
      v-model:form="credentialForm"
      :credential-usage="credentialUsage"
      @update:drawer-visible="credentialVisible = $event"
      @update:form-visible="credentialFormVisible = $event"
      @add="openCredentialAdd"
      @edit="openCredentialEdit"
      @remove="removeCredential"
      @save="saveCredential"
    />

    <NasConfigDrawer
      :drawer-visible="configVisible"
      :configs="opsConfigs"
      :history="configHistory"
      :history-target="configHistoryTarget"
      :form-visible="configFormVisible"
      :editing="editingConfig"
      v-model:form="configForm"
      @update:drawer-visible="configVisible = $event"
      @update:form-visible="configFormVisible = $event"
      @add="openConfigAdd"
      @edit="openConfigEdit"
      @remove="removeConfig"
      @show-history="showConfigHistory"
      @save="saveConfig"
    />

    <NasHelpDialog v-model:visible="helpVisible" />

    <el-dialog
      v-model="previewVisible"
      :title="previewTarget?.name + ' · ' + t('nas.liveView')"
      width="640px"
      destroy-on-close
    >
      <RtspPlayer v-if="previewTarget" :target="previewTarget" />
    </el-dialog>

    <NasAlertDrawer
      :visible="alertVisible"
      :alerts="alerts"
      @update:visible="alertVisible = $event"
      @read="onReadAlert"
      @ack="onAckAlert"
      @silence="onSilenceAlert"
      @del="onDeleteAlert"
      @clear="onClearAlerts"
    />
  </div>
</template>

<style scoped src="./NasServer.css"></style>
