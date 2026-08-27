<script setup lang="ts">
import { ref, watch, computed, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus'
import { dockerApi, dockerRemoteActions, type DockerInspect, type DockerRemoteAction, type DockerStatsRow } from '@/api/docker'
import DockerExecTerminal from './DockerExecTerminal.vue'

const props = defineProps<{
  visible: boolean
  container: string
  mode: 'local' | 'remote'
  targetId: number
  stats?: DockerStatsRow[]
}>()
const emit = defineEmits<{ 'update:visible': [val: boolean]; refresh: [] }>()

const loading = ref(false)
const inspect = ref<DockerInspect | null>(null)
const activeTab = ref<'info' | 'network' | 'storage' | 'env' | 'logs' | 'terminal' | 'raw'>('info')
const logs = ref('')
const logsLoading = ref(false)
const logsTail = ref(200)
const execVisible = ref(false)

const stat = computed(() => {
  const clean = props.container.replace(/^\//, '')
  return props.stats?.find((s) => s.Name === clean || s.Container === clean.slice(0, 12))
})

async function loadInspect() {
  if (!props.container) return
  loading.value = true
  try {
    inspect.value = props.mode === 'local'
      ? await dockerApi.localInspect(props.container)
      : await dockerApi.inspect(props.targetId, props.container)
  } catch (e: any) { ElMessage.error(e.message) }
  finally { loading.value = false }
}

async function loadLogs() {
  if (!props.container) return
  logsLoading.value = true
  try {
    const res = props.mode === 'local'
      ? await dockerApi.localLogs(props.container, logsTail.value)
      : await dockerApi.logs(props.targetId, props.container, logsTail.value)
    logs.value = res.logs || '(无日志)'
  } catch (e: any) { logs.value = '获取日志失败: ' + e.message }
  finally { logsLoading.value = false }
}

watch(() => props.visible, async (v) => {
  if (v) { activeTab.value = 'info'; await loadInspect() }
  else { logs.value = ''; inspect.value = null }
})
watch(() => activeTab.value, (t) => {
  if (t === 'logs' && !logs.value) loadLogs()
})

function cpuPct(s?: DockerStatsRow): number {
  if (!s?.CPUPerc) return 0
  return parseFloat(s.CPUPerc.replace('%', '')) || 0
}
function memPct(s?: DockerStatsRow): number {
  if (!s?.MemPerc) return 0
  return parseFloat(s.MemPerc.replace('%', '')) || 0
}

const networkList = computed(() => {
  if (!inspect.value?.NetworkSettings?.Networks) return []
  return Object.entries(inspect.value.NetworkSettings.Networks).map(([name, net]: [string, any]) => ({
    name, ip: net.IPAddress || '-', gateway: net.Gateway || '-', mac: net.MacAddress || '-',
  }))
})
const mountList = computed(() => {
  if (!inspect.value?.Mounts) return []
  return inspect.value.Mounts.map((m: any) => ({
    type: m.Type || '-', source: m.Source || '-', dest: m.Destination || '-', mode: (m.Mode || (m.RW ? 'rw' : 'ro')),
  }))
})
const envList = computed(() => {
  if (!inspect.value?.Config?.Env) return []
  return inspect.value.Config.Env.map((e: string) => {
    const idx = e.indexOf('=')
    return { key: idx > 0 ? e.substring(0, idx) : e, value: idx > 0 ? e.substring(idx + 1) : '' }
  })
})

async function doAction(action: string, label: string) {
  try {
    if (props.mode === 'local') await dockerApi.localAction(props.container, action)
    else await dockerRemoteActions[action as DockerRemoteAction](props.targetId, props.container)
    ElMessage.success(label + '成功')
    emit('refresh')
    await loadInspect()
  } catch (e: any) { ElMessage.error(e.message) }
}

onBeforeUnmount(() => { inspect.value = null; logs.value = '' })
</script>

<template>
  <el-drawer :model-value="visible" @update:model-value="emit('update:visible', $event)"
    :title="'容器详情: ' + container.replace(/^\//, '')" size="60%" direction="rtl">
    <div v-loading="loading" class="detail-wrap">
      <!-- 快捷操作条 -->
      <div class="detail-actions-bar">
        <el-button size="small" type="success" @click="doAction('start', '启动')">启动</el-button>
        <el-button size="small" type="warning" @click="doAction('stop', '停止')">停止</el-button>
        <el-button size="small" @click="doAction('restart', '重启')">重启</el-button>
        <el-button size="small" @click="doAction('pause', '暂停')" v-if="mode === 'local'">暂停</el-button>
      </div>

      <el-tabs v-model="activeTab" class="detail-tabs">
        <!-- 基本信息 -->
        <el-tab-pane label="基本信息" name="info">
          <div v-if="inspect" class="info-grid">
            <div class="info-row"><span class="ik">容器ID</span><code class="iv mono">{{ inspect.Id?.slice(0, 12) }}</code></div>
            <div class="info-row"><span class="ik">镜像</span><code class="iv mono">{{ inspect.Config?.Image }}</code></div>
            <div class="info-row"><span class="ik">状态</span>
              <span class="state-badge" :class="'state-badge--' + (inspect.State?.Running ? 'success' : 'muted')">
                <span class="state-dot"></span>{{ inspect.State?.Status || 'unknown' }}
              </span>
            </div>
            <div class="info-row"><span class="ik">创建时间</span><span class="iv">{{ inspect.Created }}</span></div>
            <div class="info-row"><span class="ik">重启策略</span><span class="iv">{{ inspect.HostConfig?.RestartPolicy?.Name || '-' }}</span></div>
            <div class="info-row" v-if="inspect.Config?.Cmd"><span class="ik">命令</span><code class="iv mono">{{ inspect.Config.Cmd.join(' ') }}</code></div>
            <div class="info-row" v-if="stat"><span class="ik">CPU</span>
              <div class="resource-bar"><div class="bar-fill bar--cpu" :style="{ width: Math.min(cpuPct(stat), 100) + '%' }"></div><span class="bar-val">{{ stat.CPUPerc }}</span></div>
            </div>
            <div class="info-row" v-if="stat"><span class="ik">内存</span>
              <div class="resource-bar"><div class="bar-fill bar--mem" :style="{ width: Math.min(memPct(stat), 100) + '%' }"></div><span class="bar-val">{{ stat.MemUsage }} ({{ stat.MemPerc }})</span></div>
            </div>
            <div class="info-row" v-if="stat"><span class="ik">网络IO</span><span class="iv mono">{{ stat.NetIO }}</span></div>
            <div class="info-row" v-if="stat"><span class="ik">磁盘IO</span><span class="iv mono">{{ stat.BlockIO }}</span></div>
            <div class="info-row" v-if="stat"><span class="ik">进程数</span><span class="iv mono">{{ stat.PIDs }}</span></div>
          </div>
        </el-tab-pane>

        <!-- 网络 -->
        <el-tab-pane :label="'网络 (' + networkList.length + ')'" name="network">
          <table class="detail-table">
            <thead><tr><th>网络名称</th><th>IP 地址</th><th>网关</th><th>MAC 地址</th></tr></thead>
            <tbody>
              <tr v-for="n in networkList" :key="n.name">
                <td class="td-name">{{ n.name }}</td><td class="td-mono">{{ n.ip }}</td><td class="td-mono">{{ n.gateway }}</td><td class="td-mono">{{ n.mac }}</td>
              </tr>
              <tr v-if="!networkList.length"><td colspan="4" class="td-empty">无网络信息</td></tr>
            </tbody>
          </table>
        </el-tab-pane>

        <!-- 存储 -->
        <el-tab-pane :label="'存储 (' + mountList.length + ')'" name="storage">
          <table class="detail-table">
            <thead><tr><th>类型</th><th>宿主机路径</th><th>容器路径</th><th>模式</th></tr></thead>
            <tbody>
              <tr v-for="(m, i) in mountList" :key="i">
                <td><span class="type-chip">{{ m.type }}</span></td><td class="td-mono">{{ m.source }}</td><td class="td-mono">{{ m.dest }}</td><td>{{ m.mode }}</td>
              </tr>
              <tr v-if="!mountList.length"><td colspan="4" class="td-empty">无挂载</td></tr>
            </tbody>
          </table>
        </el-tab-pane>

        <!-- 环境变量 -->
        <el-tab-pane :label="'环境 (' + envList.length + ')'" name="env">
          <table class="detail-table">
            <thead><tr><th>变量名</th><th>值</th></tr></thead>
            <tbody>
              <tr v-for="(e, i) in envList" :key="i"><td class="td-mono">{{ e.key }}</td><td class="td-mono">{{ e.value }}</td></tr>
              <tr v-if="!envList.length"><td colspan="2" class="td-empty">无环境变量</td></tr>
            </tbody>
          </table>
        </el-tab-pane>

        <!-- 日志 -->
        <el-tab-pane label="日志" name="logs">
          <div class="logs-toolbar">
            <el-input-number v-model="logsTail" :min="50" :max="5000" :step="100" size="small" />
            <el-button size="small" @click="loadLogs" :loading="logsLoading">刷新</el-button>
          </div>
          <pre class="log-viewer">{{ logs || '点击刷新查看日志' }}</pre>
        </el-tab-pane>

        <!-- 终端 -->
        <el-tab-pane label="终端" name="terminal" v-if="mode === 'local'">
          <div class="terminal-hint">
            <el-button type="primary" @click="execVisible = true">打开终端</el-button>
            <span class="field-hint">在容器内执行命令（需要容器正在运行）</span>
          </div>
        </el-tab-pane>

        <!-- 原始 JSON -->
        <el-tab-pane label="原始配置" name="raw">
          <pre class="raw-viewer">{{ JSON.stringify(inspect, null, 2) }}</pre>
        </el-tab-pane>
      </el-tabs>
    </div>

    <DockerExecTerminal v-model:visible="execVisible" :container="container" />
  </el-drawer>
</template>

<style scoped src="./dockerDetail.css"></style>
