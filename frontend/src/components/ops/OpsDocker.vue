<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { dockerApi, type DockerContainer, type DockerImage, type DockerListResp, type DockerStatsRow } from '@/api/docker'
import { monitorApi, type MonitorTarget } from '@/api/monitor'
import GlassIcon from '@/components/GlassIcon.vue'
import { useDockerOps } from './useDockerOps'
import DockerExecTerminal from './DockerExecTerminal.vue'
import DockerCreateDialog from './DockerCreateDialog.vue'

const targets = ref<MonitorTarget[]>([])
const selectedTarget = ref<number>(0)
const mode = ref<'local' | 'remote'>('local')
const localAvailable = ref(false)
const loading = ref(false)
const actionLoading = ref(false)
const error = ref('')
const data = ref<DockerListResp>({ containers: [], images: [], volumes: [], networks: [] })
const stats = ref<DockerStatsRow[]>([])
const activeTab = ref<'containers' | 'images' | 'volumes' | 'networks'>('containers')
const logDrawer = ref(false)
const logContainer = ref('')
const logContent = ref('')
const logLoading = ref(false)
const inspectDrawer = ref(false)
const inspectData = ref<any>(null)

const sshTargets = computed(() => targets.value.filter((t) => t.type === 'host'))

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
  } catch (e: any) {
    error.value = e.message
    data.value = { containers: [], images: [], volumes: [], networks: [] }
  } finally {
    loading.value = false
  }
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
      await (dockerApi as any)[action](selectedTarget.value, name)
    }
    ElMessage.success(`${label}成功`)
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
      `确定要删除容器「${name}」吗？此操作不可撤销。`,
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

async function removeImage(img: DockerImage) {
  const ref = `${img.Repository}:${img.Tag}`
  try {
    await ElMessageBox.confirm(`确定要删除镜像「${ref}」吗？`, '危险操作确认', { type: 'warning' })
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

async function showInspect(container: DockerContainer) {
  const name = container.Names.replace(/^\//, '')
  inspectDrawer.value = true
  try {
    inspectData.value = mode.value === 'local'
      ? await dockerApi.localInspect(name)
      : await dockerApi.inspect(selectedTarget.value, name)
  } catch (e: any) {
    ElMessage.error(e.message)
  }
}

const { pullDialog, pullRef, pullLoading, pullResult, openPull, doPull, doPrune, createVolume, removeVolume, createNetwork, removeNetwork, composeVisible, composePath, composeOutput, composeLoading, openCompose, composeAction} = useDockerOps(refresh, () => mode.value, () => selectedTarget.value)

// 创建容器（用独立组件）
const createDialog = ref(false)

// exec 终端（用独立组件）
const execDrawer = ref(false)
const execContainer = ref('')
function openExec(c: DockerContainer) {
  execContainer.value = c.Names.replace(/^\//, '')
  execDrawer.value = true
}

function stateColor(state: string): string {
  const colors: Record<string, string> = {
    running: 'success', exited: 'muted', paused: 'warn', created: 'info', restarting: 'info', dead: 'danger',
  }
  return colors[state] || 'muted'
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
  } else if (selectedTarget.value) {
    mode.value = 'remote'
    refresh()
  }
})
</script>

<template>
  <div class="ops-docker">
    <!-- 主机选择 + 刷新 -->
    <div class="docker-toolbar">
      <div class="mode-switch">
        <button class="mode-btn" :class="{ active: mode === 'local' }" :disabled="!localAvailable"
          @click="mode = 'local'; refresh()">
          🏠 本机
        </button>
        <button class="mode-btn" :class="{ active: mode === 'remote' }"
          @click="mode = 'remote'; refresh()">
          🔗 SSH 远程
        </button>
      </div>
      <el-select v-if="mode === 'remote'" v-model="selectedTarget" placeholder="选择 SSH 主机" @change="refresh" style="width: 200px">
        <el-option v-for="t in sshTargets" :key="t.id" :label="t.name" :value="t.id" />
      </el-select>
      <el-button :loading="loading" @click="refresh">刷新</el-button>
      <span v-if="!localAvailable && mode === 'local'" class="docker-hint">
        Docker socket 未挂载，在 docker-compose 添加：/var/run/docker.sock:/var/run/docker.sock
      </span>
      <span v-if="error" class="docker-error">{{ error }}</span>
    </div>

    <template v-if="!selectedTarget && sshTargets.length === 0">
      <div class="docker-empty glass">
        <GlassIcon name="server" from="#6c5ce7" to="#4285f4" :size="48" />
        <p>请先在 NAS 页面添加 SSH 类型的主机监控目标</p>
      </div>
    </template>

    <template v-else-if="selectedTarget">
      <!-- 概览卡片 -->
      <div class="docker-stats">
        <div class="stat-tile glass">
          <strong>{{ data.containers.filter((c) => c.State === 'running').length }}</strong>
          <span>运行中</span>
        </div>
        <div class="stat-tile glass">
          <strong>{{ data.containers.length }}</strong>
          <span>容器总数</span>
        </div>
        <div class="stat-tile glass">
          <strong>{{ data.images.length }}</strong>
          <span>镜像</span>
        </div>
        <div class="stat-tile glass">
          <strong>{{ data.volumes.length }}</strong>
          <span>卷</span>
        </div>
        <div class="stat-tile glass">
          <strong>{{ data.networks.length }}</strong>
          <span>网络</span>
        </div>
      </div>

      <!-- Tab 切换 -->
      <div class="docker-tabs">
        <button v-for="tab in (['containers','images','volumes','networks'] as const)" :key="tab"
          class="docker-tab" :class="{ active: activeTab === tab }" @click="activeTab = tab">
          {{ { containers: '容器', images: '镜像', volumes: '卷', networks: '网络' }[tab] }}
        </button>
      </div>

      <!-- 容器列表 -->
      <div v-if="activeTab === 'containers'" class="docker-table glass">
        <div v-if="mode === 'local'" class="tab-toolbar">
          <el-button size="small" type="primary" @click="createDialog = true">创建容器</el-button>
          <el-button size="small" type="warning" @click="doPrune('container')">清理已停止容器</el-button>
        </div>
        <table>
          <thead>
            <tr>
              <th>名称</th><th>镜像</th><th>状态</th><th>CPU</th><th>内存</th><th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="c in data.containers" :key="c.ID">
              <td class="td-name" :title="c.Names">{{ c.Names.replace(/^\//, '') }}</td>
              <td class="td-image" :title="c.Image">{{ c.Image }}</td>
              <td>
                <span class="pt-chip" :class="`pt-chip--${stateColor(c.State)}`">{{ c.State }}</span>
                <div class="td-status-sub">{{ c.Status }}</div>
              </td>
              <td>{{ statFor(c.Names)?.CPUPerc || '-' }}</td>
              <td>{{ statFor(c.Names)?.MemUsage || '-' }}</td>
              <td class="td-actions">
                <button v-if="c.State !== 'running'" class="mini-btn" :disabled="actionLoading" @click="doAction(c, 'start', '启动')">▶</button>
                <button v-if="c.State === 'running'" class="mini-btn" :disabled="actionLoading" @click="doAction(c, 'stop', '停止')">■</button>
                <button class="mini-btn" :disabled="actionLoading" @click="doAction(c, 'restart', '重启')">↻</button>
                <button v-if="c.State === 'running' && mode === 'local'" class="mini-btn" title="终端" @click="openExec(c)">⌘</button>
                <button class="mini-btn" title="日志" @click="showLogs(c)">☰</button>
                <button class="mini-btn" title="详情" @click="showInspect(c)">ℹ</button>
                <button class="mini-btn danger" title="删除" @click="removeContainer(c)">✕</button>
              </td>
            </tr>
            <tr v-if="!data.containers.length">
              <td colspan="6" class="td-empty">暂无容器</td>
            </tr>
          </tbody>
        </table>
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
              <td><button class="mini-btn danger" @click="removeImage(img)">✕</button></td>
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
            <tr v-for="(v, i) in data.volumes" :key="i"><td class="td-mono">{{ v.Name }}</td><td>{{ v.Driver }}</td><td v-if="mode === 'local'"><button class="mini-btn danger" @click="removeVolume(v)">✕</button></td></tr>
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
            <tr v-for="(n, i) in data.networks" :key="i"><td>{{ n.Name }}</td><td>{{ n.Driver }}</td><td>{{ n.Scope }}</td><td v-if="mode === 'local'"><button class="mini-btn danger" @click="removeNetwork(n)">✕</button></td></tr>
            <tr v-if="!data.networks.length"><td :colspan="mode === 'local' ? 4 : 3" class="td-empty">暂无网络</td></tr>
          </tbody>
        </table>
      </div>
    </template>

    <!-- 日志抽屉 -->
    <el-drawer v-model="logDrawer" :title="`日志: ${logContainer}`" size="60%" direction="rtl">
      <div v-loading="logLoading" class="log-viewer">
        <pre>{{ logContent }}</pre>
      </div>
    </el-drawer>

    <!-- 详情抽屉 -->
    <el-drawer v-model="inspectDrawer" title="容器详情" size="50%" direction="rtl">
      <pre v-if="inspectData" class="inspect-viewer">{{ JSON.stringify(inspectData, null, 2) }}</pre>
    </el-drawer>

    <!-- Compose 部署对话框 -->
    <el-dialog v-model="composeVisible" title="Docker Compose 部署" width="500px">
      <el-form label-width="60px"><el-form-item label="目录"><el-input v-model="composePath" placeholder="/opt/myapp" /></el-form-item></el-form>
      <div v-if="composeOutput" class="compose-output">
        <pre>{{ composeOutput }}</pre>
      </div>
      <template #footer>
        <el-button @click="composeVisible = false">关闭</el-button><el-button type="danger" :loading="composeLoading" @click="composeAction('down')">Compose Down</el-button><el-button type="primary" :loading="composeLoading" @click="composeAction('up')">Compose Up</el-button>
      </template>
    </el-dialog>

    <!-- 拉取镜像对话框 -->
    <el-dialog v-model="pullDialog" title="拉取镜像" width="500px">
      <el-form label-width="70px"><el-form-item label="镜像名"><el-input v-model="pullRef" placeholder="nginx / redis:7 / ghcr.io/..." @keyup.enter="doPull" /></el-form-item></el-form>
      <div v-if="pullResult" class="pull-output"><pre>{{ pullResult }}</pre></div>
      <template #footer>
        <el-button @click="pullDialog = false">关闭</el-button><el-button type="primary" :loading="pullLoading" @click="doPull">拉取</el-button>
      </template>
    </el-dialog>

    <!-- exec 终端 -->
    <DockerExecTerminal v-model:visible="execDrawer" :container="execContainer" />
    <DockerCreateDialog v-model:visible="createDialog" @created="refresh" />
  </div>
</template>

<style scoped src="../../views/nas-shared/opsDocker.css"></style>
