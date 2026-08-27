<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { dockerApi, dockerRemoteActions, type DockerContainer, type DockerImage, type DockerListResp, type DockerRemoteAction, type DockerStatsRow } from '@/api/docker'
import { monitorApi, type MonitorTarget } from '@/api/monitor'
import GlassIcon from '@/components/GlassIcon.vue'
import Sparkline from '@/components/dashboard/Sparkline.vue'
import { useDockerOps } from './useDockerOps'
import DockerExecTerminal from './DockerExecTerminal.vue'
import DockerCreateWizard from './DockerCreateWizard.vue'
import DockerDetailDrawer from './DockerDetailDrawer.vue'
import DockerAppStore from './DockerAppStore.vue'
import DockerComposeLocal from './DockerComposeLocal.vue'
import ComposeProjects from './ComposeProjects.vue'
import type { DockerTemplate } from '@/data/dockerTemplates'
import type { ComposeTemplate } from '@/data/composeTemplates'

const targets = ref<MonitorTarget[]>([])
const selectedTarget = ref<number>(0)
const mode = ref<'local' | 'remote'>('local')
const localAvailable = ref(false)
const loading = ref(false)
const actionLoading = ref(false)
const error = ref('')
const data = ref<DockerListResp>({ containers: [], images: [], volumes: [], networks: [] })
const stats = ref<DockerStatsRow[]>([])
const activeTab = ref<'containers' | 'images' | 'volumes' | 'networks' | 'projects'>('containers')
const searchText = ref('')
const stateFilter = ref<'all' | 'running' | 'stopped'>('all')
const viewMode = ref<'card' | 'list'>('card')

// 批量选择
const batchMode = ref(false)
const selectedIds = ref<Set<string>>(new Set())

// 日志抽屉
const logDrawer = ref(false)
const logContainer = ref('')
const logContent = ref('')
const logLoading = ref(false)

// 详情抽屉
const detailVisible = ref(false)
const detailContainer = ref('')

// 创建向导
const createDialog = ref(false)

// 应用商店
const appStoreVisible = ref(false)

// 从应用商店选中模板后传入向导
const pendingTemplate = ref<DockerTemplate | null>(null)

// Compose 项目数量（Tab 角标用）
const projectCount = ref(0)

// 从应用商店选中 Compose 模板后传入编辑器
const pendingCompose = ref<ComposeTemplate | null>(null)

// exec 终端
const execDrawer = ref(false)
const execContainer = ref('')

// 资源监控历史数据（用于 sparkline）
const cpuHistory = ref<Record<string, number[]>>({})
const memHistory = ref<Record<string, number[]>>({})
let statsTimer: number | undefined
const HISTORY_POINTS = 20

// 远程模式支持 host 和 nas 类型（只要有 SSH 连接信息即可管理 Docker）
const sshTargets = computed(() => targets.value.filter((t) => t.type === 'host' || t.type === 'nas'))
const tabKeys = computed<(typeof activeTab.value)[]>(() =>
  mode.value === 'local' ? ['containers', 'projects', 'images', 'volumes', 'networks'] : ['containers', 'images', 'volumes', 'networks'])

async function loadTargets() {
  try {
    targets.value = await monitorApi.list()
    if (sshTargets.value.length && !selectedTarget.value) {
      selectedTarget.value = sshTargets.value[0].id
    }
  } catch (e: any) {
    error.value = e.message
  }
}

function pushHistory(map: typeof cpuHistory.value, key: string, val: number) {
  if (!map[key]) map[key] = []
  map[key].push(val)
  if (map[key].length > HISTORY_POINTS) map[key].shift()
}

async function refreshStats() {
  try {
    const st = mode.value === 'local'
      ? await dockerApi.localStats()
      : await dockerApi.stats(selectedTarget.value)
    stats.value = st.stats || []
    // 更新 sparkline 历史
    for (const s of stats.value) {
      const cpu = parseFloat(s.CPUPerc?.replace('%', '') || '0') || 0
      const mem = parseFloat(s.MemPerc?.replace('%', '') || '0') || 0
      pushHistory(cpuHistory.value, s.Name, cpu)
      pushHistory(memHistory.value, s.Name, mem)
    }
  } catch { /* 静默失败 */ }
}

async function refresh() {
  loading.value = true
  error.value = ''
  try {
    if (mode.value === 'local') {
      const [list, st] = await Promise.all([
        dockerApi.localList(),
        dockerApi.localStats().catch(() => ({ stats: [] })),
      ])
      data.value = list
      stats.value = st.stats || []
    } else {
      if (!selectedTarget.value) return
      const [list, st] = await Promise.all([
        dockerApi.list(selectedTarget.value),
        dockerApi.stats(selectedTarget.value).catch(() => ({ stats: [] })),
      ])
      data.value = list
      stats.value = st.stats || []
    }
    // 初始化历史数据
    for (const s of stats.value) {
      const cpu = parseFloat(s.CPUPerc?.replace('%', '') || '0') || 0
      const mem = parseFloat(s.MemPerc?.replace('%', '') || '0') || 0
      pushHistory(cpuHistory.value, s.Name, cpu)
      pushHistory(memHistory.value, s.Name, mem)
    }
  } catch (e: any) {
    error.value = e.message
    data.value = { containers: [], images: [], volumes: [], networks: [] }
  } finally {
    loading.value = false
    if (mode.value === 'local') {
      dockerApi.localProjects().then((r) => { projectCount.value = r.projects?.length || 0 }).catch(() => {})
    }
  }
}

function startStatsPolling() {
  stopStatsPolling()
  statsTimer = window.setInterval(refreshStats, 5000)
}
function stopStatsPolling() {
  if (statsTimer) { clearInterval(statsTimer); statsTimer = undefined }
}

function statFor(name: string): DockerStatsRow | undefined {
  const clean = name.replace(/^\//, '')
  return stats.value.find((s) => s.Name === clean || s.Container === clean.slice(0, 12))
}

async function doAction(container: DockerContainer, action: string, label: string) {
  actionLoading.value = true
  try {
    const name = container.Names.replace(/^\//, '')
    if (mode.value === 'local') {
      await dockerApi.localAction(name, action)
    } else {
      await dockerRemoteActions[action as DockerRemoteAction](selectedTarget.value, name)
    }
    ElMessage.success(label + '成功')
    refresh()
  } catch (e: any) {
    ElMessage.error(e.message)
  } finally {
    actionLoading.value = false
  }
}

async function removeContainer(container: DockerContainer) {
  const name = container.Names.replace(/^\//, '')
  try {
    await ElMessageBox.confirm(
      '确定要删除容器「' + name + '」吗？此操作不可撤销。',
      '危险操作确认',
      { type: 'warning', confirmButtonText: '强制删除', cancelButtonText: '取消' },
    )
    actionLoading.value = true
    if (mode.value === 'local') {
      await dockerApi.localRemove(name, true, true)
    } else {
      await dockerApi.remove(selectedTarget.value, name, true, true)
    }
    ElMessage.success('容器已删除')
    refresh()
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error(e.message || '取消删除')
  } finally {
    actionLoading.value = false
  }
}

// 批量操作
function toggleBatchMode() {
  batchMode.value = !batchMode.value
  if (!batchMode.value) selectedIds.value.clear()
}

function toggleSelect(id: string) {
  if (selectedIds.value.has(id)) selectedIds.value.delete(id)
  else selectedIds.value.add(id)
}

function selectAllVisible() {
  const allSelected = filteredContainers.value.every((c) => selectedIds.value.has(c.ID))
  if (allSelected) {
    filteredContainers.value.forEach((c) => selectedIds.value.delete(c.ID))
  } else {
    filteredContainers.value.forEach((c) => selectedIds.value.add(c.ID))
  }
}

function exitBatchMode() {
  batchMode.value = false
  selectedIds.value.clear()
}

const selectedContainers = computed(() =>
  (data.value.containers || []).filter((c) => selectedIds.value.has(c.ID)),
)
const isAllSelected = computed(() =>
  filteredContainers.value.length > 0 && filteredContainers.value.every((c) => selectedIds.value.has(c.ID)),
)

async function batchAction(action: 'start' | 'stop' | 'restart' | 'remove') {
  const containers = selectedContainers.value
  if (!containers.length) { ElMessage.warning('请先选择容器'); return }
  const labels: Record<string, string> = { start: '启动', stop: '停止', restart: '重启', remove: '删除' }
  const names = containers.map((c) => c.Names.replace(/^\//, '')).join('、')
  // remove/stop/restart 均为破坏性批量操作，统一确认
  if (action === 'remove' || action === 'stop' || action === 'restart') {
    const verb = action === 'remove' ? '删除' : action === 'stop' ? '停止' : '重启'
    try {
      await ElMessageBox.confirm(
        '确定要批量' + verb + '以下 ' + containers.length + ' 个容器吗？\n' + names + (action === 'remove' ? '\n此操作不可撤销。' : ''),
        '批量' + verb + '确认',
        { type: action === 'remove' ? 'error' : 'warning', confirmButtonText: '确认' + verb, cancelButtonText: '取消' },
      )
    } catch { return }
  }
  actionLoading.value = true
  let ok = 0
  let fail = 0
  for (const c of containers) {
    try {
      const name = c.Names.replace(/^\//, '')
      if (action === 'remove') {
        if (mode.value === 'local') await dockerApi.localRemove(name, true, true)
        else await dockerApi.remove(selectedTarget.value, name, true, true)
      } else {
        if (mode.value === 'local') await dockerApi.localAction(name, action)
        else await dockerRemoteActions[action as DockerRemoteAction](selectedTarget.value, name)
      }
      ok++
    } catch { fail++ }
  }
  actionLoading.value = false
  if (fail === 0) ElMessage.success(`批量${labels[action]}完成，共 ${ok} 个`)
  else ElMessage.warning(`${labels[action]}完成：成功 ${ok}，失败 ${fail}`)
  exitBatchMode()
  refresh()
}

async function removeImage(img: DockerImage) {
  const ref = img.Repository + ':' + img.Tag
  try {
    await ElMessageBox.confirm('确定要删除镜像「' + ref + '」吗？', '危险操作确认', { type: 'warning' })
    actionLoading.value = true
    if (mode.value === 'local') {
      await dockerApi.localRemoveImage(ref, true)
    } else {
      await dockerApi.removeImage(selectedTarget.value, ref, true)
    }
    ElMessage.success('镜像已删除')
    refresh()
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error(e.message)
  } finally {
    actionLoading.value = false
  }
}

async function showLogs(container: DockerContainer) {
  logContainer.value = container.Names.replace(/^\//, '')
  logDrawer.value = true
  logLoading.value = true
  try {
    const res = mode.value === 'local'
      ? await dockerApi.localLogs(logContainer.value, 200)
      : await dockerApi.logs(selectedTarget.value, logContainer.value, 200)
    logContent.value = res.logs || '(无日志)'
  } catch (e: any) {
    logContent.value = '获取日志失败: ' + e.message
  } finally {
    logLoading.value = false
  }
}

function showDetail(container: DockerContainer) {
  detailContainer.value = container.Names.replace(/^\//, '')
  detailVisible.value = true
}

function openExec(c: DockerContainer) {
  execContainer.value = c.Names.replace(/^\//, '')
  execDrawer.value = true
}

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

function stateColor(state: string): string {
  const colors: Record<string, string> = {
    running: 'success', exited: 'muted', paused: 'warn', created: 'info', restarting: 'info', dead: 'danger',
  }
  return colors[state] || 'muted'
}

function stateLabel(state: string): string {
  const labels: Record<string, string> = {
    running: '运行中', exited: '已停止', paused: '已暂停', created: '已创建', restarting: '重启中', dead: '已死亡', removing: '移除中',
  }
  return labels[state] || state
}

function shortPorts(ports: string): string {
  if (!ports) return ''
  const items = ports.split(',').map((s) => s.trim()).filter(Boolean)
  const seen = new Set<string>()
  const out: string[] = []
  for (const raw of items) {
    if (raw.startsWith('::')) continue
    let p = raw.replace(/^0\.0\.0\.0:/, '').replace(/^:::*/, '')
    if (seen.has(p)) continue
    seen.add(p)
    out.push(p)
  }
  return out.join('  ')
}

function netDriverClass(driver: string): string {
  const d = (driver || '').toLowerCase()
  if (['bridge', 'vlan', 'host', 'overlay'].includes(d)) return 'driver-chip--' + d
  return ''
}

function cpuHistoryFor(name: string): number[] {
  const clean = name.replace(/^\//, '')
  return cpuHistory.value[clean] || []
}
function memHistoryFor(name: string): number[] {
  const clean = name.replace(/^\//, '')
  return memHistory.value[clean] || []
}

const { pullDialog, pullRef, pullLoading, pullResult, openPull, doPull, doPrune, createVolume, removeVolume, createNetwork, removeNetwork, composeVisible, composePath, composeOutput, composeLoading, openCompose, composeAction, localComposeVisible, openLocalCompose } = useDockerOps(refresh, () => mode.value, () => selectedTarget.value)

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
  navigator.clipboard.writeText('- /var/run/docker.sock:/var/run/docker.sock').then(() => {
    ElMessage.success('已复制，粘贴到 docker-compose.yml 的 volumes 中')
  }).catch(() => {})
}

onMounted(async () => {
  await loadTargets()
  try {
    const st = await dockerApi.localStatus()
    localAvailable.value = st.available
  } catch { localAvailable.value = false }
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

onBeforeUnmount(() => { stopStatsPolling() })

function onModeSwitch() {
  cpuHistory.value = {}
  memHistory.value = {}
  refresh()
  startStatsPolling()
}
</script>

<template>
  <div class="ops-docker">
    <!-- 主机选择 + 刷新 -->
    <div class="docker-toolbar">
      <div class="mode-switch">
        <button class="mode-btn" :class="{ active: mode === 'local' }" :disabled="!localAvailable"
          @click="mode = 'local'; onModeSwitch()">
          🏠 本机
        </button>
        <button class="mode-btn" :class="{ active: mode === 'remote'}"
          @click="mode = 'remote'; onModeSwitch()">
          🔗 SSH 远程
        </button>
      </div>
      <el-select v-if="mode === 'remote'" v-model="selectedTarget" placeholder="选择主机 / NAS" @change="refresh" style="width: 200px">
        <el-option v-for="t in sshTargets" :key="t.id" :label="t.name" :value="t.id" />
      </el-select>
      <el-button :loading="loading" @click="refresh">刷新</el-button>
      <span v-if="!localAvailable && mode === 'local'" class="docker-hint">
        Docker socket 未挂载，在 docker-compose 添加：/var/run/docker.sock:/var/run/docker.sock
      </span>
      <span v-if="error" class="docker-error">{{ error }}</span>
    </div>

    <template v-if="!selectedTarget && sshTargets.length === 0 && !localAvailable">
      <div class="docker-empty glass">
        <GlassIcon name="server" from="#6c5ce7" to="#4285f4" :size="48" />
        <p>请先在 NAS 页面添加主机或 NAS 类型的监控目标（需支持 SSH 连接）</p>
      </div>
    </template>

    <template v-else>
      <!-- 概览卡片 -->
      <div class="docker-stats">
        <div class="stat-tile glass stat--running">
          <span class="stat-ic"><svg viewBox="0 0 24 24"><path d="M8 5v14l11-7z"/></svg></span>
          <div class="stat-meta"><strong>{{ (data.containers || []).filter((c) => c.State === 'running').length }}</strong><span>运行中</span></div>
        </div>
        <div class="stat-tile glass stat--container">
          <span class="stat-ic"><svg viewBox="0 0 24 24"><path d="M3 6h18v12H3z" fill="none" stroke="currentColor" stroke-width="2"/><path d="M7 6v12M12 6v12M17 6v12" stroke="currentColor" stroke-width="1.5"/></svg></span>
          <div class="stat-meta"><strong>{{ data.containers.length }}</strong><span>容器总数</span></div>
        </div>
        <div class="stat-tile glass stat--image">
          <span class="stat-ic"><svg viewBox="0 0 24 24"><path d="M21 5H3v14h18V5zm-9 2c2.2 0 4 1.8 4 4s-1.8 4-4 4-4-1.8-4-4 1.8-4 4-4z"/></svg></span>
          <div class="stat-meta"><strong>{{ data.images.length }}</strong><span>镜像</span></div>
        </div>
        <div class="stat-tile glass stat--volume">
          <span class="stat-ic"><svg viewBox="0 0 24 24"><path d="M4 6h16l-1 14H5L4 6zm5 0V4h6v2"/></svg></span>
          <div class="stat-meta"><strong>{{ data.volumes.length }}</strong><span>卷</span></div>
        </div>
        <div class="stat-tile glass stat--network">
          <span class="stat-ic"><svg viewBox="0 0 24 24"><path d="M12 2a10 10 0 100 20 10 10 0 000-20zm0 4a6 6 0 016 6M7 9a6 6 0 015-3" fill="none" stroke="currentColor" stroke-width="2"/><circle cx="12" cy="12" r="2"/></svg></span>
          <div class="stat-meta"><strong>{{ data.networks.length }}</strong><span>网络</span></div>
        </div>
      </div>

      <!-- Tab 切换 -->
      <div class="docker-tabs">
        <button v-for="tab in tabKeys" :key="tab"
          class="docker-tab" :class="{ active: activeTab === tab }" @click="activeTab = tab">
          {{ ({ containers: '容器', images: '镜像', volumes: '卷', networks: '网络', projects: '项目' } as any)[tab] }}
          <span v-if="tab === 'projects' && projectCount > 0" class="tab-badge">{{ projectCount }}</span>
        </button>
      </div>

      <!-- 容器列表 -->
      <div v-if="activeTab === 'containers'" class="docker-cards-section">
        <div class="cards-toolbar">
          <el-input v-model="searchText" placeholder="搜索容器名称或镜像…" clearable size="small" class="cards-search">
            <template #prefix><svg viewBox="0 0 24 24" class="search-ico"><path d="M15.5 14h-.79l-.28-.27a6.5 6.5 0 1 0-.7.7l.27.28v.79l5 4.99L20.49 19l-4.99-5zm-6 0A4.5 4.5 0 1 1 14 9.5 4.5 4.5 0 0 1 9.5 14z"/></svg></template>
          </el-input>
          <div class="state-tabs">
            <button class="state-tab" :class="{ active: stateFilter === 'all' }" @click="stateFilter = 'all'">
              全部 <span class="count">{{ data.containers.length }}</span>
            </button>
            <button class="state-tab" :class="{ active: stateFilter === 'running' }" @click="stateFilter = 'running'">
              运行中 <span class="count">{{ (data.containers || []).filter(c => c.State === 'running').length }}</span>
            </button>
            <button class="state-tab" :class="{ active: stateFilter === 'stopped' }" @click="stateFilter = 'stopped'">
              已停止 <span class="count">{{ (data.containers || []).filter(c => c.State !== 'running').length }}</span>
            </button>
          </div>
          <!-- 视图切换 -->
          <div class="view-switch">
            <button class="view-btn" :class="{ active: viewMode === 'card' }" title="卡片视图" @click="viewMode = 'card'">
              <svg viewBox="0 0 24 24"><path d="M3 3h8v8H3V3zm10 0h8v8h-8V3zM3 13h8v8H3v-8zm10 0h8v8h-8v-8z"/></svg>
            </button>
            <button class="view-btn" :class="{ active: viewMode === 'list' }" title="列表视图" @click="viewMode = 'list'">
              <svg viewBox="0 0 24 24"><path d="M3 5h18v3H3V5zm0 5h18v3H3v-3zm0 5h18v3H3v-3z"/></svg>
            </button>
          </div>
          <div class="cards-actions">
            <el-button size="small" type="success" @click="openAppStore">
              <svg viewBox="0 0 24 24" class="btn-ico-inline" style="width:14px;height:14px;margin-right:4px;fill:currentColor"><path d="M7 4V2h10v2h3v2H4V4h3zm-2 4h14l-1 14H6L5 8z"/></svg>
              应用商店
            </el-button>
            <el-button size="small" type="primary" @click="createDialog = true">创建容器</el-button>
            <el-button size="small" @click="openLocalCompose" v-if="mode === 'local'">
              <svg viewBox="0 0 24 24" class="btn-ico-inline" style="width:14px;height:14px;margin-right:4px;fill:currentColor"><path d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6v2z"/></svg>
              新建 Compose
            </el-button>
            <el-button size="small" type="warning" @click="doPrune('container')">清理已停止</el-button>
            <el-button size="small" :type="batchMode ? 'danger' : 'info'" @click="toggleBatchMode">
              {{ batchMode ? '退出批量' : '批量操作' }}
            </el-button>
          </div>
          <!-- 批量操作工具条 -->
          <div v-if="batchMode" class="batch-bar">
            <label class="batch-select-all">
              <input type="checkbox" :checked="isAllSelected" @change="selectAllVisible" />
              <span>全选</span>
            </label>
            <span class="batch-count">已选 {{ selectedIds.size }} 个</span>
            <el-button size="small" type="success" :disabled="!selectedIds.size" :loading="actionLoading" @click="batchAction('start')">批量启动</el-button>
            <el-button size="small" type="warning" :disabled="!selectedIds.size" :loading="actionLoading" @click="batchAction('stop')">批量停止</el-button>
            <el-button size="small" :disabled="!selectedIds.size" :loading="actionLoading" @click="batchAction('restart')">批量重启</el-button>
            <el-button size="small" type="danger" :disabled="!selectedIds.size" :loading="actionLoading" @click="batchAction('remove')">批量删除</el-button>
            <el-button size="small" @click="exitBatchMode">取消</el-button>
          </div>
        </div>

        <!-- 卡片视图 -->
        <div v-if="filteredContainers.length && viewMode === 'card'" class="docker-card-grid">
          <div v-for="c in filteredContainers" :key="c.ID" class="docker-card" :class="['docker-card--' + stateColor(c.State), { 'docker-card--selected': selectedIds.has(c.ID) }]">
            <div v-if="batchMode" class="card-batch-check" @click.stop="toggleSelect(c.ID)">
              <input type="checkbox" :checked="selectedIds.has(c.ID)" @click.stop="toggleSelect(c.ID)" />
            </div>
            <div class="card-header">
              <span class="card-status-dot" :class="'dot--' + stateColor(c.State)"></span>
              <span class="card-name" :title="c.Names.replace(/^\//, '')">{{ c.Names.replace(/^\//, '') }}</span>
              <span class="card-state-chip" :class="'chip--' + stateColor(c.State)">{{ stateLabel(c.State) }}</span>
            </div>
            <div class="card-image" :title="c.Image">
              <svg viewBox="0 0 24 24" class="card-ic-img"><path d="M21 5H3v14h18V5zm-9 2c2.2 0 4 1.8 4 4s-1.8 4-4 4-4-1.8-4-4 1.8-4 4-4z"/></svg>
              <span class="card-image-text">{{ c.Image }}</span>
            </div>
            <div v-if="c.Ports" class="card-ports" :title="c.Ports">
              <svg viewBox="0 0 24 24" class="card-ic-port"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z"/></svg>
              <span class="port-text">{{ shortPorts(c.Ports) }}</span>
            </div>
            <div v-else class="card-ports card-ports--empty">无端口映射</div>
            <!-- 资源监控 sparkline -->
            <div class="card-monitor">
              <div class="card-stat">
                <span class="stat-label">CPU {{ statFor(c.Names)?.CPUPerc || '—' }}</span>
                <Sparkline v-if="cpuHistoryFor(c.Names).length >= 2" :values="cpuHistoryFor(c.Names)" color="#6c5ce7" unit="ratio" class="card-spark" />
              </div>
              <div class="card-stat">
                <span class="stat-label">内存 {{ statFor(c.Names)?.MemUsage || '—' }}</span>
                <Sparkline v-if="memHistoryFor(c.Names).length >= 2" :values="memHistoryFor(c.Names)" color="#11998e" unit="ratio" class="card-spark" />
              </div>
            </div>
            <div class="card-uptime">{{ c.Status }}</div>
            <div class="card-actions">
              <button v-if="c.State !== 'running'" class="card-btn" title="启动" :disabled="actionLoading" @click="doAction(c, 'start', '启动')"><svg viewBox="0 0 24 24" class="btn-ico"><path d="M8 5v14l11-7z"/></svg></button>
              <button v-if="c.State === 'running'" class="card-btn" title="停止" :disabled="actionLoading" @click="doAction(c, 'stop', '停止')"><svg viewBox="0 0 24 24" class="btn-ico"><rect x="6" y="6" width="12" height="12" rx="1.5"/></svg></button>
              <button class="card-btn" title="重启" :disabled="actionLoading" @click="doAction(c, 'restart', '重启')"><svg viewBox="0 0 24 24" class="btn-ico"><path d="M17.65 6.35A7.96 7.96 0 0 0 12 4a8 8 0 1 0 7.73 10h-2.08A6 6 0 1 1 12 6c1.66 0 3.14.69 4.22 1.78L13 11h7V4z"/></svg></button>
              <button v-if="c.State === 'running' && mode === 'local'" class="card-btn" title="终端" @click="openExec(c)"><svg viewBox="0 0 24 24" class="btn-ico"><path d="M4 4h16v16H4V4zm2 2v12h12V6H6zm2 2h2l2 3 2-3h2l-3 4 3 4h-2l-2-3-2 3H8l3-4-3-4z"/></svg></button>
              <button class="card-btn" title="日志" @click="showLogs(c)"><svg viewBox="0 0 24 24" class="btn-ico"><path d="M5 3h14a1 1 0 0 1 1 1v16l-3-2-3 2V8H5V4a1 1 0 0 1 0-1zm1 6h2v2H6V9zm0 3h2v2H6v-2zm0 3h2v2H6v-2z"/></svg></button>
              <button class="card-btn" title="详情" @click="showDetail(c)"><svg viewBox="0 0 24 24" class="btn-ico"><path d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm0 5a1.5 1.5 0 1 1 0 3 1.5 1.5 0 0 1 0-3zm2 10h-4v-1h1v-3h-1v-1h3v4h1v1z"/></svg></button>
              <button class="card-btn danger" title="删除" @click="removeContainer(c)"><svg viewBox="0 0 24 24" class="btn-ico"><path d="M9 3h6v2h5v2H4V5h5V3zm-3 6h12l-1 12H7L6 9zm3 2v8H8v-8h1zm3 0v8h-1v-8h1z"/></svg></button>
            </div>
          </div>
        </div>

        <!-- 列表视图 -->
        <div v-if="filteredContainers.length && viewMode === 'list'" class="docker-table glass">
          <table>
            <thead>
              <tr><th v-if="batchMode" style="width:36px"><input type="checkbox" :checked="isAllSelected" @change="selectAllVisible" /></th><th>状态</th><th>名称</th><th>镜像</th><th>端口</th><th>CPU</th><th>内存</th><th>运行时长</th><th>操作</th></tr>
            </thead>
            <tbody>
              <tr v-for="c in filteredContainers" :key="c.ID" :class="{ 'row-selected': selectedIds.has(c.ID) }">
                <td v-if="batchMode"><input type="checkbox" :checked="selectedIds.has(c.ID)" @change="toggleSelect(c.ID)" /></td>
                <td><span class="card-status-dot" :class="'dot--' + stateColor(c.State)"></span></td>
                <td class="td-name">{{ c.Names.replace(/^\//, '') }}<div class="td-status-sub">{{ stateLabel(c.State) }}</div></td>
                <td class="td-image">{{ c.Image }}</td>
                <td class="td-ports"><span v-if="c.Ports" class="port-chip">{{ shortPorts(c.Ports) }}</span><span v-else class="td-muted">—</span></td>
                <td class="td-stat">{{ statFor(c.Names)?.CPUPerc || '—' }}</td>
                <td class="td-stat">{{ statFor(c.Names)?.MemUsage || '—' }}</td>
                <td class="td-status-sub">{{ c.Status }}</td>
                <td class="td-actions">
                  <button v-if="c.State !== 'running'" class="mini-btn" title="启动" :disabled="actionLoading" @click="doAction(c, 'start', '启动')"><svg viewBox="0 0 24 24" class="btn-ico"><path d="M8 5v14l11-7z"/></svg></button>
                  <button v-if="c.State === 'running'" class="mini-btn" title="停止" :disabled="actionLoading" @click="doAction(c, 'stop', '停止')"><svg viewBox="0 0 24 24" class="btn-ico"><rect x="6" y="6" width="12" height="12" rx="1.5"/></svg></button>
                  <button class="mini-btn" title="重启" :disabled="actionLoading" @click="doAction(c, 'restart', '重启')"><svg viewBox="0 0 24 24" class="btn-ico"><path d="M17.65 6.35A7.96 7.96 0 0 0 12 4a8 8 0 1 0 7.73 10h-2.08A6 6 0 1 1 12 6c1.66 0 3.14.69 4.22 1.78L13 11h7V4z"/></svg></button>
                  <button v-if="c.State === 'running' && mode === 'local'" class="mini-btn" title="终端" @click="openExec(c)"><svg viewBox="0 0 24 24" class="btn-ico"><path d="M4 4h16v16H4V4zm2 2v12h12V6H6zm2 2h2l2 3 2-3h2l-3 4 3 4h-2l-2-3-2 3H8l3-4-3-4z"/></svg></button>
                  <button class="mini-btn" title="日志" @click="showLogs(c)"><svg viewBox="0 0 24 24" class="btn-ico"><path d="M5 3h14a1 1 0 0 1 1 1v16l-3-2-3 2V8H5V4a1 1 0 0 1 0-1zm1 6h2v2H6V9zm0 3h2v2H6v-2zm0 3h2v2H6v-2z"/></svg></button>
                  <button class="mini-btn" title="详情" @click="showDetail(c)"><svg viewBox="0 0 24 24" class="btn-ico"><path d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm0 5a1.5 1.5 0 1 1 0 3 1.5 1.5 0 0 1 0-3zm2 10h-4v-1h1v-3h-1v-1h3v4h1v1z"/></svg></button>
                  <button class="mini-btn danger" title="删除" @click="removeContainer(c)"><svg viewBox="0 0 24 24" class="btn-ico"><path d="M9 3h6v2h5v2H4V5h5V3zm-3 6h12l-1 12H7L6 9zm3 2v8H8v-8h1zm3 0v8h-1v-8h1z"/></svg></button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div v-if="!filteredContainers.length" class="docker-empty glass">
          <GlassIcon name="server" from="#6c5ce7" to="#4285f4" :size="48" />
          <p>{{ searchText || stateFilter !== 'all' ? '没有匹配的容器' : '暂无容器，点击「创建容器」开始' }}</p>
        </div>
      </div>

      <!-- 镜像列表 -->
      <div v-if="activeTab === 'images'" class="docker-table glass">
        <div v-if="mode === 'local'" class="tab-toolbar">
          <el-button size="small" type="primary" @click="openPull">拉取镜像</el-button>
          <el-button size="small" type="warning" @click="doPrune('image')">清理悬空镜像</el-button>
        </div>
        <table>
          <thead><tr><th>仓库</th><th>标签</th><th>ID</th><th>大小</th><th>操作</th></tr></thead>
          <tbody>
            <tr v-for="(img, i) in data.images" :key="i">
              <td>{{ img.Repository }}</td>
              <td><span class="pt-chip">{{ img.Tag }}</span></td>
              <td class="td-mono">{{ img.ID }}</td>
              <td>{{ img.Size }}</td>
              <td><button class="mini-btn danger" @click="removeImage(img)"><svg viewBox="0 0 24 24" class="btn-ico"><path d="M9 3h6v2h5v2H4V5h5V3zm-3 6h12l-1 12H7L6 9zm3 2v8H8v-8h1zm3 0v8h-1v-8h1z"/></svg></button></td>
            </tr>
            <tr v-if="!data.images.length"><td colspan="5" class="td-empty">暂无镜像</td></tr>
          </tbody>
        </table>
      </div>

      <!-- 卷列表 -->
      <div v-if="activeTab === 'volumes'" class="docker-table glass">
        <div v-if="mode === 'local'" class="tab-toolbar">
          <el-button size="small" type="primary" @click="createVolume">创建卷</el-button>
          <el-button size="small" type="warning" @click="doPrune('volume')">清理无用卷</el-button>
        </div>
        <table>
          <thead><tr><th>名称</th><th>驱动</th><th v-if="mode === 'local'">操作</th></tr></thead>
          <tbody>
            <tr v-for="(v, i) in data.volumes" :key="i"><td class="td-mono">{{ v.Name }}</td><td>{{ v.Driver }}</td><td v-if="mode === 'local'"><button class="mini-btn danger" @click="removeVolume(v)"><svg viewBox="0 0 24 24" class="btn-ico"><path d="M9 3h6v2h5v2H4V5h5V3zm-3 6h12l-1 12H7L6 9zm3 2v8H8v-8h1zm3 0v8h-1v-8h1z"/></svg></button></td></tr>
            <tr v-if="!data.volumes.length"><td :colspan="mode === 'local' ? 3 : 2" class="td-empty">暂无卷</td></tr>
          </tbody>
        </table>
      </div>

      <!-- 网络列表 -->
      <div v-if="activeTab === 'networks'" class="docker-table glass">
        <div v-if="mode === 'local'" class="tab-toolbar">
          <el-button size="small" type="primary" @click="createNetwork">创建网络</el-button>
          <el-button size="small" type="warning" @click="doPrune('network')">清理无用网络</el-button>
        </div>
        <table>
          <thead><tr><th>名称</th><th>驱动</th><th>范围</th><th v-if="mode === 'local'">操作</th></tr></thead>
          <tbody>
            <tr v-for="(n, i) in data.networks" :key="i">
              <td class="td-name"><span class="net-name">{{ n.Name }}</span></td>
              <td><span class="driver-chip" :class="netDriverClass(n.Driver)">{{ n.Driver }}</span></td>
              <td><span class="scope-chip">{{ n.Scope }}</span></td>
              <td v-if="mode === 'local'"><button class="mini-btn danger" title="删除网络" @click="removeNetwork(n)"><svg viewBox="0 0 24 24" class="btn-ico"><path d="M9 3h6v2h5v2H4V5h5V3zm-3 6h12l-1 12H7L6 9zm3 2v8H8v-8h1zm3 0v8h-1v-8h1z"/></svg></button></td>
            </tr>
            <tr v-if="!data.networks.length"><td :colspan="mode === 'local' ? 4 : 3" class="td-empty">暂无网络</td></tr>
          </tbody>
        </table>
      </div>

      <!-- Compose 项目 -->
      <div v-if="activeTab === 'projects'" class="compose-section">
        <div class="cards-toolbar">
          <span class="compose-hint">Compose 多容器编排部署</span>
          <div class="cards-actions">
            <el-button size="small" type="primary" @click="openLocalCompose">
              <svg viewBox="0 0 24 24" class="btn-ico-inline"><path d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6v2z"/></svg>
              新建 Compose 项目
            </el-button>
          </div>
        </div>
        <ComposeProjects />
      </div>
    </template>

    <!-- 日志抽屉 -->
    <el-drawer v-model="logDrawer" :title="'日志: ' + logContainer" size="60%" direction="rtl">
      <div v-loading="logLoading" class="log-viewer">
        <pre>{{ logContent }}</pre>
      </div>
    </el-drawer>

    <!-- Compose 部署对话框 -->
    <el-dialog v-model="composeVisible" title="Docker Compose 部署" width="500px">
      <el-form label-width="60px"><el-form-item label="目录"><el-input v-model="composePath" placeholder="/opt/myapp" /></el-form-item></el-form>
      <div v-if="composeOutput" class="compose-output"><pre>{{ composeOutput }}</pre></div>
      <template #footer>
        <el-button @click="composeVisible = false">关闭</el-button>
        <el-button type="danger" :loading="composeLoading" @click="composeAction('down')">Compose Down</el-button>
        <el-button type="primary" :loading="composeLoading" @click="composeAction('up')">Compose Up</el-button>
      </template>
    </el-dialog>

    <!-- 拉取镜像对话框 -->
    <el-dialog v-model="pullDialog" title="拉取镜像" width="500px">
      <el-form label-width="70px"><el-form-item label="镜像名"><el-input v-model="pullRef" placeholder="nginx / redis:7 / ghcr.io/..." @keyup.enter="doPull" /></el-form-item></el-form>
      <div v-if="pullResult" class="pull-output"><pre>{{ pullResult }}</pre></div>
      <template #footer>
        <el-button @click="pullDialog = false">关闭</el-button>
        <el-button type="primary" :loading="pullLoading" @click="doPull">拉取</el-button>
      </template>
    </el-dialog>

    <!-- 详情抽屉 -->
    <DockerDetailDrawer v-model:visible="detailVisible" :container="detailContainer" :mode="mode" :target-id="selectedTarget" :stats="stats" @refresh="refresh" />

    <!-- exec 终端 -->
    <DockerExecTerminal v-model:visible="execDrawer" :container="execContainer" />

    <!-- 创建容器向导 -->
    <DockerCreateWizard v-model:visible="createDialog" :images="data.images" :initial-template="pendingTemplate" @created="refresh" />

    <DockerComposeLocal v-model:visible="localComposeVisible" :initial-content="pendingCompose?.content" :initial-name="pendingCompose?.id" @done="refresh" />

    <!-- 应用商店 -->
    <DockerAppStore v-model:visible="appStoreVisible" @select="onStoreTemplateSelected" @select-compose="onStoreComposeSelected" />
  </div>
</template>

<style scoped src="../../views/nas-shared/opsDocker.css">.docker-hint-box { padding: 12px 16px; margin: 8px 0; border-radius: 8px; background: rgba(108,92,231,.08); border: 1px solid rgba(108,92,231,.2); }
.docker-hint-title { font-weight: 700; font-size: 14px; margin-bottom: 6px; }
.docker-hint-desc { font-size: 13px; color: var(--text-muted); line-height: 1.6; margin-bottom: 6px; }
.docker-hint-desc code { background: rgba(0,0,0,.1); padding: 1px 5px; border-radius: 3px; font-size: 12px; }
.docker-hint-code { display: flex; align-items: center; gap: 8px; padding: 8px 12px; background: rgba(0,0,0,.15); border-radius: 6px; cursor: pointer; margin-bottom: 6px; transition: background var(--dur-fast) var(--ease-out-smooth); }
.docker-hint-code:hover { background: rgba(0,0,0,.25); }
.docker-hint-code code { font-family: 'Cascadia Code', monospace; font-size: 13px; flex: 1; }
.copy-btn { font-size: 12px; color: var(--pt-primary, #6c5ce7); white-space: nowrap; }
</style>
