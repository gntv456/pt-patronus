<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { dockerApi, type DockerContainer, type DockerImage, type DockerListResp, type DockerStatsRow } from '@/api/docker'
import { monitorApi, type MonitorTarget } from '@/api/monitor'
import GlassIcon from '@/components/GlassIcon.vue'
import { useDockerOps } from './useDockerOps'
import DockerExecTerminal from './DockerExecTerminal.vue'
import DockerCreateDialog from './DockerCreateDialog.vue'
import DockerComposeLocal from './DockerComposeLocal.vue'
import ComposeProjects from './ComposeProjects.vue'

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
const logDrawer = ref(false)
const logContainer = ref('')
const logContent = ref('')
const logLoading = ref(false)
const inspectDrawer = ref(false)
const inspectData = ref<any>(null)

const sshTargets = computed(() => targets.value.filter((t) => t.type === 'host'))
const tabKeys = computed<(typeof activeTab.value)[]>(() =>
  mode.value === 'local' ? ['containers', 'images', 'volumes', 'networks', 'projects'] : ['containers', 'images', 'volumes', 'networks'])

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

const { pullDialog, pullRef, pullLoading, pullResult, openPull, doPull, doPrune, createVolume, removeVolume, createNetwork, removeNetwork, composeVisible, composePath, composeOutput, composeLoading, openCompose, composeAction, localComposeVisible, openLocalCompose} = useDockerOps(refresh, () => mode.value, () => selectedTarget.value)

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

function stateLabel(state: string): string {
  const labels: Record<string, string> = {
    running: '运行中', exited: '已停止', paused: '已暂停', created: '已创建', restarting: '重启中', dead: '已死亡', removing: '移除中',
  }
  return labels[state] || state
}

// 端口列简写：取 host:container 协议，去掉 0.0.0.0 前缀，多个用空格分隔
function shortPorts(ports: string): string {
  if (!ports) return ''
  // Docker ports 字段形如 "0.0.0.0:8080->80/tcp, :::8080->80/tcp"
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

// �������� -> CSS �������� opsDocker.css �� .driver-chip--* ��Ӧ��
function netDriverClass(driver: string): string {
  const d = (driver || '').toLowerCase()
  if (['bridge', 'vlan', 'host', 'overlay'].includes(d)) return `driver-chip--${d}`
  return ''
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
      <!-- 概览卡片（1Panel 风格：图标 + 彩色 + 大数字） -->
      <div class="docker-stats">
        <div class="stat-tile glass stat--running">
          <span class="stat-ic"><svg viewBox="0 0 24 24"><path d="M8 5v14l11-7z"/></svg></span>
          <div class="stat-meta"><strong>{{ data.containers.filter((c) => c.State === 'running').length }}</strong><span>运行中</span></div>
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
        </button>
      </div>

      <!-- 容器列表 -->
      <div v-if="activeTab === 'containers'" class="docker-table glass">
        <div class="tab-toolbar">
          <el-button size="small" type="primary" @click="createDialog = true">创建容器</el-button>
          <el-button size="small" type="warning" @click="doPrune('container')">清理已停止容器</el-button>
        </div>
        <table>
          <thead>
            <tr>
              <th>名称</th><th>镜像</th><th>端口</th><th>状态</th><th>CPU</th><th>内存</th><th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="c in data.containers" :key="c.ID">
              <td class="td-name" :title="c.Names">{{ c.Names.replace(/^\//, '') }}</td>
              <td class="td-image" :title="c.Image"><span class="img-tag">{{ c.Image }}</span></td>
              <td class="td-ports">
                <span v-if="c.Ports" class="port-chip" :title="c.Ports">{{ shortPorts(c.Ports) }}</span>
                <span v-else class="td-muted">—</span>
              </td>
              <td>
                <span class="state-badge" :class="`state-badge--${stateColor(c.State)}`">
                  <span class="state-dot"></span>{{ stateLabel(c.State) }}
                </span>
                <div class="td-status-sub">{{ c.Status }}</div>
              </td>
              <td class="td-stat">{{ statFor(c.Names)?.CPUPerc || '-' }}</td>
              <td class="td-stat">{{ statFor(c.Names)?.MemUsage || '-' }}</td>
              <td class="td-actions">
                <button v-if="c.State !== 'running'" class="mini-btn" title="启动" :disabled="actionLoading" @click="doAction(c, 'start', '启动')"><svg viewBox="0 0 24 24" class="btn-ico"><path d="M8 5v14l11-7z"/></svg></button>
                <button v-if="c.State === 'running'" class="mini-btn" title="停止" :disabled="actionLoading" @click="doAction(c, 'stop', '停止')"><svg viewBox="0 0 24 24" class="btn-ico"><rect x="6" y="6" width="12" height="12" rx="1.5"/></svg></button>
                <button class="mini-btn" title="重启" :disabled="actionLoading" @click="doAction(c, 'restart', '重启')"><svg viewBox="0 0 24 24" class="btn-ico"><path d="M17.65 6.35A7.96 7.96 0 0 0 12 4a8 8 0 1 0 7.73 10h-2.08A6 6 0 1 1 12 6c1.66 0 3.14.69 4.22 1.78L13 11h7V4z"/></svg></button>
                <button v-if="c.State === 'running' && mode === 'local'" class="mini-btn" title="终端" @click="openExec(c)"><svg viewBox="0 0 24 24" class="btn-ico"><path d="M4 4h16v16H4V4zm2 2v12h12V6H6zm2 2h2l2 3 2-3h2l-3 4 3 4h-2l-2-3-2 3H8l3-4-3-4z"/></svg></button>
                <button class="mini-btn" title="日志" @click="showLogs(c)"><svg viewBox="0 0 24 24" class="btn-ico"><path d="M5 3h14a1 1 0 0 1 1 1v16l-3-2-3 2V8H5V4a1 1 0 0 1 0-1zm1 6h2v2H6V9zm0 3h2v2H6v-2zm0 3h2v2H6v-2z"/></svg></button>
                <button class="mini-btn" title="详情" @click="showInspect(c)"><svg viewBox="0 0 24 24" class="btn-ico"><path d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm0 5a1.5 1.5 0 1 1 0 3 1.5 1.5 0 0 1 0-3zm2 10h-4v-1h1v-3h-1v-1h3v4h1v1z"/></svg></button>
                <button class="mini-btn danger" title="删除" @click="removeContainer(c)"><svg viewBox="0 0 24 24" class="btn-ico"><path d="M9 3h6v2h5v2H4V5h5V3zm-3 6h12l-1 12H7L6 9zm3 2v8H8v-8h1zm3 0v8h-1v-8h1z"/></svg></button>
              </td>
            </tr>
            <tr v-if="!data.containers.length">
              <td colspan="7" class="td-empty">暂无容器</td>
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
      <ComposeProjects v-if="activeTab === 'projects'" />
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

    <DockerComposeLocal v-model:visible="localComposeVisible" @done="refresh" />
  </div>
</template>

<style scoped src="../../views/nas-shared/opsDocker.css"></style>
