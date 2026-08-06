<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { procApi, type ProcRow, type NetConnRow } from '@/api/proc'
import { monitorApi, type MonitorTarget } from '@/api/monitor'

const targets = ref<MonitorTarget[]>([])
const selectedTarget = ref<number>(0)
const loading = ref(false)
const error = ref('')

// 子视图：进程列表 / 网络连接
const view = ref<'process' | 'network'>('process')

// 进程数据 + 筛选 + 排序
const processes = ref<ProcRow[]>([])
const procFilter = ref({ pid: '', name: '', user: '', state: '' })
const sortKey = ref<'cpu' | 'mem' | 'pid' | 'ppid'>('cpu')

// 网络连接数据 + 筛选
const connections = ref<NetConnRow[]>([])
const netFilter = ref({ pid: '', name: '', port: '', state: '' })

const sshTargets = computed(() => targets.value.filter((t) => t.type === 'host'))

const stateLabel: Record<string, string> = {
  R: '运行', S: '睡眠', D: '等待', Z: '僵尸', T: '停止', I: '空闲',
}

const filteredProcesses = computed(() => {
  let list = processes.value
  if (procFilter.value.pid) list = list.filter((p) => String(p.pid).includes(procFilter.value.pid))
  if (procFilter.value.name) list = list.filter((p) => p.name.toLowerCase().includes(procFilter.value.name.toLowerCase()))
  if (procFilter.value.user) list = list.filter((p) => p.user.toLowerCase().includes(procFilter.value.user.toLowerCase()))
  if (procFilter.value.state) list = list.filter((p) => p.state.startsWith(procFilter.value.state))
  // 排序：sortKey 映射到实际字段名
  const fieldMap: Record<string, 'pid' | 'ppid' | 'cpu_perc' | 'mem_perc'> = {
    pid: 'pid', ppid: 'ppid', cpu: 'cpu_perc', mem: 'mem_perc',
  }
  const field = fieldMap[sortKey.value]
  list = [...list].sort((a, b) => {
    if (field === 'pid' || field === 'ppid') return a[field] - b[field]
    return b[field] - a[field] // cpu/mem 降序
  })
  return list
})

const filteredConnections = computed(() => {
  let list = connections.value
  if (netFilter.value.pid) list = list.filter((c) => String(c.pid).includes(netFilter.value.pid))
  if (netFilter.value.name) list = list.filter((c) => (c.process || '').toLowerCase().includes(netFilter.value.name.toLowerCase()))
  if (netFilter.value.port) list = list.filter((c) => c.local_port.includes(netFilter.value.port) || (c.peer_port || '').includes(netFilter.value.port))
  if (netFilter.value.state) list = list.filter((c) => c.state.toUpperCase().includes(netFilter.value.state.toUpperCase()))
  return list
})

async function loadTargets() {
  try {
    targets.value = await monitorApi.list()
    if (sshTargets.value.length && !selectedTarget.value) selectedTarget.value = sshTargets.value[0].id
  } catch (e: any) { error.value = e.message }
}

async function refresh() {
  if (!selectedTarget.value) return
  loading.value = true
  error.value = ''
  try {
    if (view.value === 'process') {
      const r = await procApi.list(selectedTarget.value)
      processes.value = r.processes || []
    } else {
      const r = await procApi.connections(selectedTarget.value)
      connections.value = r.connections || []
    }
  } catch (e: any) {
    error.value = e.message
  } finally { loading.value = false }
}

async function switchView(v: 'process' | 'network') {
  view.value = v
  await refresh()
}

function fmtMem(kb: number): string {
  if (kb > 1048576) return (kb / 1048576).toFixed(1) + ' GB'
  if (kb > 1024) return (kb / 1024).toFixed(1) + ' MB'
  return kb + ' KB'
}

async function killProc(p: ProcRow) {
  try {
    await ElMessageBox.confirm(
      `确定要结束进程「${p.name}」(PID: ${p.pid}, 用户: ${p.user}) 吗？`,
      '结束进程确认',
      { type: 'warning', confirmButtonText: '结束', cancelButtonText: '取消' },
    )
    const res = await procApi.kill(selectedTarget.value, { pid: p.pid, confirm: true })
    if (res.ok) { ElMessage.success(`进程 ${p.pid} 已发送终止信号`); refresh() }
    else ElMessage.error(res.error || '结束失败')
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error(e.message)
  }
}

async function forceKill(p: ProcRow) {
  try {
    await ElMessageBox.confirm(
      `⚠️ 强制结束（SIGKILL）进程「${p.name}」(PID: ${p.pid})？可能导致数据丢失。`,
      '强制结束',
      { type: 'error', confirmButtonText: '强制结束', cancelButtonText: '取消' },
    )
    const res = await procApi.kill(selectedTarget.value, { pid: p.pid, force: true, confirm: true })
    if (res.ok) { ElMessage.success(`进程 ${p.pid} 已强制结束`); refresh() }
    else ElMessage.error(res.error || '结束失败')
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error(e.message)
  }
}

onMounted(async () => { await loadTargets(); if (selectedTarget.value) refresh() })
</script>

<template>
  <div class="ops-proc">
    <!-- 工具栏 -->
    <div class="proc-toolbar glass">
      <div class="toolbar-left">
        <label class="target-label">目标主机</label>
        <el-select v-model="selectedTarget" placeholder="选择 SSH 主机" style="width: 200px" @change="refresh">
          <el-option v-for="t in sshTargets" :key="t.id" :label="t.name" :value="t.id" />
        </el-select>
        <div class="view-switch">
          <button :class="{ active: view === 'process' }" @click="switchView('process')">进程</button>
          <button :class="{ active: view === 'network' }" @click="switchView('network')">网络</button>
        </div>
      </div>
      <el-button type="primary" :loading="loading" @click="refresh">刷新</el-button>
    </div>

    <div v-if="error" class="proc-error">{{ error }}</div>
    <div v-if="!sshTargets.length" class="proc-empty glass">
      尚未添加 SSH 主机。请在 NAS → 监控目标 中添加一台 SSH 主机才能查看进程。
    </div>

    <!-- 进程列表 -->
    <div v-if="view === 'process' && sshTargets.length" class="proc-table-wrap glass">
      <div class="filter-bar">
        <el-input v-model="procFilter.pid" placeholder="PID" style="width: 90px" clearable />
        <el-input v-model="procFilter.name" placeholder="进程名" style="width: 160px" clearable />
        <el-input v-model="procFilter.user" placeholder="用户" style="width: 120px" clearable />
        <el-input v-model="procFilter.state" placeholder="状态" style="width: 80px" clearable />
        <div class="sort-group">
          <span class="sort-label">排序</span>
          <button v-for="k in (['cpu', 'mem', 'pid', 'ppid'] as const)" :key="k"
            class="sort-btn" :class="{ active: sortKey === k }" @click="sortKey = k">
            {{ k === 'cpu' ? 'CPU' : k === 'mem' ? '内存' : k.toUpperCase() }}
          </button>
        </div>
      </div>
      <div class="table-scroll">
        <table class="proc-table">
          <thead><tr><th>PID</th><th>进程名</th><th>用户</th><th>状态</th>
            <th>CPU%</th><th>内存%</th><th>RSS</th><th>累计时间</th><th>操作</th></tr></thead>
          <tbody>
            <tr v-for="p in filteredProcesses" :key="p.pid">
              <td class="td-mono">{{ p.pid }}</td>
              <td class="td-name" :title="p.cmd">{{ p.name }}</td>
              <td>{{ p.user }}</td>
              <td><span class="state-chip" :class="p.state[0]">{{ stateLabel[p.state[0]] || p.state }}</span></td>
              <td :class="{ 'td-hot': p.cpu_perc > 80 }">{{ p.cpu_perc.toFixed(1) }}</td>
              <td :class="{ 'td-hot': p.mem_perc > 50 }">{{ p.mem_perc.toFixed(1) }}</td>
              <td>{{ fmtMem(p.rss) }}</td>
              <td class="td-mono">{{ p.time || '-' }}</td>
              <td class="td-actions">
                <el-button size="small" type="danger" link @click="killProc(p)">结束</el-button>
                <el-button size="small" type="danger" link @click="forceKill(p)">强制</el-button>
              </td>
            </tr>
            <tr v-if="!filteredProcesses.length"><td colspan="9" class="td-empty">无匹配进程</td></tr>
          </tbody>
        </table>
      </div>
      <div class="table-count">共 {{ filteredProcesses.length }} 个进程</div>
    </div>

    <!-- 网络连接列表 -->
    <div v-if="view === 'network' && sshTargets.length" class="proc-table-wrap glass">
      <div class="filter-bar">
        <el-input v-model="netFilter.pid" placeholder="PID" style="width: 90px" clearable />
        <el-input v-model="netFilter.name" placeholder="进程名" style="width: 160px" clearable />
        <el-input v-model="netFilter.port" placeholder="端口" style="width: 100px" clearable />
        <el-input v-model="netFilter.state" placeholder="状态" style="width: 120px" clearable />
      </div>
      <div class="table-scroll">
        <table class="proc-table">
          <thead><tr><th>协议</th><th>状态</th><th>本地地址</th><th>本地端口</th>
            <th>对端地址</th><th>对端端口</th><th>PID</th><th>进程</th></tr></thead>
          <tbody>
            <tr v-for="(c, i) in filteredConnections" :key="i">
              <td><span class="proto-chip">{{ c.proto }}</span></td>
              <td><span class="conn-state" :class="c.state.toUpperCase()">{{ c.state }}</span></td>
              <td class="td-mono">{{ c.local_addr }}</td>
              <td class="td-mono">{{ c.local_port }}</td>
              <td class="td-mono">{{ c.peer_addr || '-' }}</td>
              <td class="td-mono">{{ c.peer_port || '-' }}</td>
              <td class="td-mono">{{ c.pid || '-' }}</td>
              <td>{{ c.process || '-' }}</td>
            </tr>
            <tr v-if="!filteredConnections.length"><td colspan="8" class="td-empty">无连接</td></tr>
          </tbody>
        </table>
      </div>
      <div class="table-count">共 {{ filteredConnections.length }} 条连接</div>
    </div>
  </div>
</template>

<style scoped>
.ops-proc { padding: 0 4px; }
.proc-toolbar { display: flex; align-items: center; justify-content: space-between; padding: 12px 18px; margin-bottom: 16px; }
.toolbar-left { display: flex; align-items: center; gap: 12px; }
.target-label { font-size: 13px; color: var(--text-secondary, #888); white-space: nowrap; }
.view-switch { display: flex; gap: 4px; margin-left: 8px; }
.view-switch button { padding: 6px 16px; border: 1px solid var(--border, rgba(128,128,128,.25)); background: transparent; border-radius: 6px; cursor: pointer; font-size: 13px; color: var(--text-secondary,#888); }
.view-switch button.active { background: var(--accent, #6c5ce7); color: #fff; border-color: transparent; }
.proc-error { color: #e74c3c; margin-bottom: 12px; padding: 8px 14px; background: rgba(231,76,60,.08); border-radius: 8px; }
.proc-empty { padding: 28px; text-align: center; color: var(--text-secondary, #888); }
.proc-table-wrap { padding: 12px 16px; }
.filter-bar { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; margin-bottom: 12px; }
.sort-group { display: flex; align-items: center; gap: 4px; margin-left: auto; }
.sort-label { font-size: 12px; color: var(--text-secondary,#888); }
.sort-btn { padding: 4px 10px; border: 1px solid var(--border, rgba(128,128,128,.2)); background: transparent; border-radius: 4px; cursor: pointer; font-size: 12px; color: var(--text-secondary,#888); }
.sort-btn.active { background: var(--accent, #6c5ce7); color: #fff; border-color: transparent; }
.table-scroll { overflow-x: auto; max-height: 65vh; overflow-y: auto; }
.proc-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.proc-table th { text-align: left; padding: 8px 10px; font-weight: 600; position: sticky; top: 0; background: var(--bg-th, rgba(0,0,0,.06)); backdrop-filter: blur(4px); white-space: nowrap; }
.proc-table td { padding: 6px 10px; border-bottom: 1px solid rgba(128,128,128,.1); white-space: nowrap; }
.proc-table tbody tr:hover { background: rgba(108,92,231,.06); }
.td-mono { font-family: monospace; font-size: 12px; }
.td-name { max-width: 280px; overflow: hidden; text-overflow: ellipsis; }
.td-hot { color: #e74c3c; font-weight: 600; }
.td-actions { white-space: nowrap; }
.td-empty { text-align: center; padding: 24px; color: var(--text-tertiary, #aaa); }
.table-count { font-size: 12px; color: var(--text-secondary,#888); margin-top: 8px; padding: 4px 2px; }
.state-chip { font-size: 11px; padding: 1px 7px; border-radius: 4px; }
.state-chip.R { background: rgba(46,204,113,.18); color: #27ae60; }
.state-chip.S { background: rgba(52,152,219,.18); color: #2980b9; }
.state-chip.Z { background: rgba(231,76,60,.18); color: #c0392b; }
.state-chip.D, .state-chip.T { background: rgba(243,156,18,.18); color: #e67e22; }
.proto-chip { font-size: 11px; padding: 1px 6px; border-radius: 4px; background: rgba(0,0,0,.08); font-family: monospace; }
.conn-state { font-size: 11px; font-family: monospace; }
.conn-state.ESTAB { color: #27ae60; }
.conn-state.LISTEN { color: #2980b9; }
.conn-state.TIME-WAIT, .conn-state.CLOSE-WAIT { color: #95a5a6; }
</style>
