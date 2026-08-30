<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  dockerApi,
  dockerRemoteActions,
  type DockerContainer,
  type DockerImage,
  type DockerListResp,
  type DockerRemoteAction,
  type DockerStatsRow,
} from '@/api/docker'
import { monitorApi, type MonitorTarget } from '@/api/monitor'
import GlassIcon from '@/components/GlassIcon.vue'
import Sparkline from '@/components/dashboard/Sparkline.vue'
import { useDockerOps } from './useDockerOps'
import { useDockerData } from './useDockerData'
import { useContainerActions } from './useContainerActions'
import ComposeProjects from './ComposeProjects.vue'
import OpsDockerContainers from './OpsDockerContainers.vue'
import { useDockerDialogsState } from './useDockerDialogsState'
import OpsDockerDialogs from './OpsDockerDialogs.vue'
import OpsDockerOverview from './OpsDockerOverview.vue'
import OpsDockerTables from './OpsDockerTables.vue'
import type { DockerTemplate } from '@/data/dockerTemplates'
import type { ComposeTemplate } from '@/data/composeTemplates'

const { t } = useI18n()
const mode = ref<'local' | 'remote'>('local')

// 数据刷新/监控轮询（useDockerData）与容器动作/批量（useContainerActions）拆分自本文件。
const {
  targets,
  selectedTarget,
  localAvailable,
  loading,
  error,
  data,
  stats,
  projectCount,
  sshTargets,
  loadTargets,
  refresh,
  startStatsPolling,
  stopStatsPolling,
  statFor,
  cpuHistoryFor,
  memHistoryFor,
  resetHistory,
  initLocalAvailability,
} = useDockerData(() => mode.value)
const activeTab = ref<'containers' | 'images' | 'volumes' | 'networks' | 'projects'>('containers')
const searchText = ref('')
const stateFilter = ref<'all' | 'running' | 'stopped'>('all')
const viewMode = ref<'card' | 'list'>('card')


const tabKeys = computed<(typeof activeTab.value)[]>(() =>
  mode.value === 'local'
    ? ['containers', 'projects', 'images', 'volumes', 'networks']
    : ['containers', 'images', 'volumes', 'networks'],
)

const filteredContainers = computed(() => {
  const kw = searchText.value.trim().toLowerCase()
  return (data.value.containers || []).filter((c) => {
    if (stateFilter.value === 'running' && c.State !== 'running') return false
    if (stateFilter.value === 'stopped' && c.State === 'running') return false
    if (!kw) return true
    const name = c.Names.replace(/^\//, '').toLowerCase()
    return name.includes(kw) || (c.Image || '').toLowerCase().includes(kw)
  })
})

const {
  actionLoading,
  batchMode,
  selectedIds,
  selectedContainers,
  isAllSelected,
  toggleBatchMode,
  toggleSelect,
  selectAllVisible,
  exitBatchMode,
  doAction,
  removeContainer,
  batchAction,
} = useContainerActions({
  mode: () => mode.value,
  selectedTarget: () => selectedTarget.value,
  refresh,
  filteredContainers: () => filteredContainers.value,
  containers: () => data.value.containers || [],
  t: (k: string, v?: Record<string, unknown>) => t(k, v ?? {}),
})

// 对话框/抽屉状态簇拆出 useDockerDialogsState
const {
  logDrawer,
  logContainer,
  logContent,
  logLoading,
  detailVisible,
  detailContainer,
  createDialog,
  appStoreVisible,
  pendingTemplate,
  pendingCompose,
  execDrawer,
  execContainer,
  removeImage,
  showLogs,
  showDetail,
  openExec,
} = useDockerDialogsState({
  mode: () => mode.value,
  selectedTarget: () => selectedTarget.value,
  refresh,
  actionLoading,
  t: (k: string, v?: Record<string, unknown>) => t(k, v ?? {}),
})




const {
  pullDialog,
  pullRef,
  pullLoading,
  pullResult,
  openPull,
  doPull,
  doPrune,
  createVolume,
  removeVolume,
  createNetwork,
  removeNetwork,
  composeVisible,
  composePath,
  composeOutput,
  composeLoading,
  openCompose,
  composeAction,
  localComposeVisible,
  openLocalCompose,
} = useDockerOps(
  refresh,
  () => mode.value,
  () => selectedTarget.value,
)

// 从应用商店选择模板 → 打开创建向导
function openAppStore() {
  pendingTemplate.value = null
  pendingCompose.value = null
  appStoreVisible.value = true
}

function onStoreComposeSelected(tpl: ComposeTemplate) {
  pendingCompose.value = tpl
  localComposeVisible.value = true
}

function onStoreTemplateSelected(tpl: DockerTemplate) {
  pendingTemplate.value = tpl
  createDialog.value = true
}

function copyDockerSockHint() {
  navigator.clipboard
    .writeText('- /var/run/docker.sock:/var/run/docker.sock')
    .then(() => {
      ElMessage.success(t('opsdocker.copied'))
    })
    .catch(() => {})
}

onMounted(async () => {
  await loadTargets()
  try {
    const st = await dockerApi.localStatus()
    localAvailable.value = st.available
  } catch {
    localAvailable.value = false
  }
  if (localAvailable.value) {
    mode.value = 'local'
    refresh()
    startStatsPolling()
  } else if (selectedTarget.value) {
    mode.value = 'remote'
    refresh()
    startStatsPolling()
  }
})

onBeforeUnmount(() => {
  stopStatsPolling()
})

function switchMode(m: 'local' | 'remote') {
  mode.value = m
  onModeSwitch()
}

function onModeSwitch() {
  resetHistory()
  refresh()
  startStatsPolling()
}
</script>

<template>
  <div class="ops-docker">
    <!-- 主机选择 + 刷新 -->
    <div class="docker-toolbar">
      <div class="mode-switch">
        <button
          class="mode-btn"
          :class="{ active: mode === 'local' }"
          :disabled="!localAvailable"
          @click="switchMode('local')"
        >
          {{ t('opsdocker.localMode') }}
        </button>
        <button class="mode-btn" :class="{ active: mode === 'remote' }" @click="switchMode('remote')">
          {{ t('opsdocker.sshRemote') }}
        </button>
      </div>
      <el-select
        v-if="mode === 'remote'"
        v-model="selectedTarget"
        :placeholder="t('opsdocker.pickHost')"
        @change="refresh"
        style="width: 200px"
      >
        <el-option v-for="t in sshTargets" :key="t.id" :label="t.name" :value="t.id" />
      </el-select>
      <el-button :loading="loading" @click="refresh">{{ t('common.refresh') }}</el-button>
      <span v-if="!localAvailable && mode === 'local'" class="docker-hint">
        {{ t('opsdocker.socketHint') }}
      </span>
      <span v-if="error" class="docker-error">{{ error }}</span>
    </div>

    <template v-if="!selectedTarget && sshTargets.length === 0 && !localAvailable">
      <div class="docker-empty glass">
        <GlassIcon name="server" from="#6c5ce7" to="#4285f4" :size="48" />
        <p>{{ t('opsdocker.addTargetFirst') }}</p>
      </div>
    </template>

    <template v-else>
      <OpsDockerOverview :data="data" :stats="stats" />
      <!-- Tab 切换 -->
      <div class="docker-tabs">
        <button
          v-for="tab in tabKeys"
          :key="tab"
          class="docker-tab"
          :class="{ active: activeTab === tab }"
          @click="activeTab = tab"
        >
          {{
            (
              {
                containers: t('opsdocker.tabContainers'),
                images: t('opsdocker.images'),
                volumes: t('opsdocker.volumes'),
                networks: t('opsdocker.networks'),
                projects: t('opsdocker.projects'),
              } as any
            )[tab]
          }}
          <span v-if="tab === 'projects' && projectCount > 0" class="tab-badge">{{ projectCount }}</span>
        </button>
      </div>

      <OpsDockerContainers
        :active-tab="activeTab"
        :containers-total="data.containers.length"
        :containers-running="(data.containers || []).filter((c) => c.State === 'running').length"
        :containers-stopped="(data.containers || []).filter((c) => c.State !== 'running').length"
        :filtered-containers="filteredContainers"
        :search-text="searchText"
        :selected-ids="[...selectedIds]"
        :selected="Object.fromEntries([...selectedIds].map((id) => [id, true]))"
        :batch-mode="batchMode"
        :action-loading="actionLoading"
        :is-all-selected="isAllSelected"
        :view-mode="viewMode"
        :state-filter="stateFilter"
        :loading="loading"
        :mode="mode"
        :stat-for="statFor"
        :cpu-history-for="cpuHistoryFor"
        :mem-history-for="memHistoryFor"
        @update:search-text="(v: string) => (searchText = v)"
        @update:state-filter="(v: string) => (stateFilter = v as typeof stateFilter)"
        @update:view-mode="(v: 'card' | 'list') => (viewMode = v)"
        @toggle-select="toggleSelect"
        @select-all-visible="selectAllVisible"
        @batch-action="batchAction"
        @do-action="doAction"
        @remove-container="removeContainer"
        @show-logs="showLogs"
        @show-detail="showDetail"
        @open-exec="openExec"
        @open-app-store="appStoreVisible = true"
        @open-local-compose="openLocalCompose"
        @create="createDialog = true"
        @toggle-batch-mode="toggleBatchMode"
        @exit-batch-mode="exitBatchMode"
        @prune-container="doPrune('container')"
      />
      <OpsDockerTables
        :active-tab="activeTab"
        :mode="mode"
        :data="data"
        @open-pull="openPull"
        @prune="doPrune"
        @remove-image="removeImage"
        @create-volume="createVolume"
        @remove-volume="removeVolume"
        @create-network="createNetwork"
        @remove-network="removeNetwork"
      />
      <!-- Compose 项目 -->
      <div v-if="activeTab === 'projects'" class="compose-section">
        <div class="cards-toolbar">
          <span class="compose-hint">{{ t('opsdocker.composeHint') }}</span>
          <div class="cards-actions">
            <el-button size="small" type="primary" @click="openLocalCompose">
              <svg viewBox="0 0 24 24" class="btn-ico-inline"><path d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6v2z" /></svg>
              {{ t('opsdocker.newComposeProject') }}
            </el-button>
          </div>
        </div>
        <ComposeProjects />
      </div>
    </template>

    <OpsDockerDialogs
      v-model:log-drawer="logDrawer"
      v-model:log-container="logContainer"
      v-model:log-content="logContent"
      v-model:log-loading="logLoading"
      v-model:detail-visible="detailVisible"
      v-model:exec-drawer="execDrawer"
      v-model:create-dialog="createDialog"
      v-model:app-store-visible="appStoreVisible"
      v-model:compose-visible="composeVisible"
      v-model:compose-path="composePath"
      v-model:compose-output="composeOutput"
      v-model:compose-loading="composeLoading"
      v-model:pull-dialog="pullDialog"
      v-model:pull-ref="pullRef"
      v-model:pull-result="pullResult"
      v-model:pull-loading="pullLoading"
      :mode="mode"
      :selected-target="selectedTarget"
      :stats="stats"
      :data="data"
      :detail-container="detailContainer"
      :exec-container="execContainer"
      :pending-template="pendingTemplate"
      :pending-compose="pendingCompose"
      @refresh="refresh"
      @compose-action="composeAction" 
      @do-pull="doPull"
      @store-template="onStoreTemplateSelected"
      @store-compose="onStoreComposeSelected"
    />
  </div>
</template>

<style scoped src="../../views/nas-shared/opsDocker.css">
.docker-hint-box {
  padding: 12px 16px;
  margin: 8px 0;
  border-radius: 8px;
  background: rgba(108, 92, 231, 0.08);
  border: 1px solid rgba(108, 92, 231, 0.2);
}
.docker-hint-title {
  font-weight: 700;
  font-size: 14px;
  margin-bottom: 6px;
}
.docker-hint-desc {
  font-size: 13px;
  color: var(--text-muted);
  line-height: 1.6;
  margin-bottom: 6px;
}
.docker-hint-desc code {
  background: rgba(0, 0, 0, 0.1);
  padding: 1px 5px;
  border-radius: 3px;
  font-size: 12px;
}
.docker-hint-code {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  background: rgba(0, 0, 0, 0.15);
  border-radius: 6px;
  cursor: pointer;
  margin-bottom: 6px;
  transition: background var(--dur-fast) var(--ease-out-smooth);
}
.docker-hint-code:hover {
  background: rgba(0, 0, 0, 0.25);
}
.docker-hint-code code {
  font-family: 'Cascadia Code', monospace;
  font-size: 13px;
  flex: 1;
}
.copy-btn {
  font-size: 12px;
  color: var(--pt-primary, #6c5ce7);
  white-space: nowrap;
}
</style>
