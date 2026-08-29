<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { procApi, type ProcRow, type NetConnRow } from '@/api/proc'
import { monitorApi, type MonitorTarget } from '@/api/monitor'

const { t } = useI18n()
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
  R: t('opsproc.st.R'), S: t('opsproc.st.S'), D: t('opsproc.st.D'), Z: t('opsproc.st.Z'), T: t('opsproc.st.T'), I: t('opsproc.st.I'),
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
      t('opsproc.confirmKill', { name: p.name, pid: p.pid, user: p.user }),
      t('opsproc.confirmKillTitle'),
      { type: 'warning', confirmButtonText: t('opsproc.kill'), cancelButtonText: t('common.cancel') },
    )
    const res = await procApi.kill(selectedTarget.value, { pid: p.pid, confirm: true })
    if (res.ok) { ElMessage.success(t('opsproc.killSignalSent', { pid: p.pid })); refresh() }
    else ElMessage.error(res.error || t('opsproc.killFailed'))
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error(e.message)
  }
}

async function forceKill(p: ProcRow) {
  try {
    await ElMessageBox.confirm(
      t('opsproc.confirmForceKill', { name: p.name, pid: p.pid }),
      t('opsproc.forceKill'),
      { type: 'error', confirmButtonText: t('opsproc.forceKill'), cancelButtonText: t('common.cancel') },
    )
    const res = await procApi.kill(selectedTarget.value, { pid: p.pid, force: true, confirm: true })
    if (res.ok) { ElMessage.success(t('opsproc.forceKilled', { pid: p.pid })); refresh() }
    else ElMessage.error(res.error || t('opsproc.killFailed'))
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
        <label class="target-label">{{ t('opsproc.targetHost') }}</label>
        <el-select v-model="selectedTarget" :placeholder="t('opsproc.selectHost')" style="width: 200px" @change="refresh">
          <el-option v-for="t in sshTargets" :key="t.id" :label="t.name" :value="t.id" />
        </el-select>
        <div class="view-switch">
          <button :class="{ active: view === 'process' }" @click="switchView('process')">{{ t('ops.tab.process') }}</button>
          <button :class="{ active: view === 'network' }" @click="switchView('network')">{{ t('opsproc.network') }}</button>
        </div>
      </div>
      <el-button type="primary" :loading="loading" @click="refresh">{{ t('common.refresh') }}</el-button>
    </div>

    <div v-if="error" class="proc-error">{{ error }}</div>
    <div v-if="!sshTargets.length" class="proc-empty glass">
      {{ t('opsproc.noHostHint') }}
    </div>

    <!-- 进程列表 -->
    <div v-if="view === 'process' && sshTargets.length" class="proc-table-wrap glass">
      <div class="filter-bar">
        <el-input v-model="procFilter.pid" placeholder="PID" style="width: 90px" clearable />
        <el-input v-model="procFilter.name" :placeholder="t('opsproc.procName')" style="width: 160px" clearable />
        <el-input v-model="procFilter.user" :placeholder="t('mediaserver.col.user')" style="width: 120px" clearable />
        <el-input v-model="procFilter.state" :placeholder="t('reseed.col.status')" style="width: 80px" clearable />
        <div class="sort-group">
          <span class="sort-label">{{ t('opsproc.sort') }}</span>
          <button v-for="k in (['cpu', 'mem', 'pid', 'ppid'] as const)" :key="k"
            class="sort-btn" :class="{ active: sortKey === k }" @click="sortKey = k">
            {{ k === 'cpu' ? 'CPU' : k === 'mem' ? t('opsmonitor.mem') : k.toUpperCase() }}
          </button>
        </div>
      </div>
      <div class="table-scroll">
        <table class="proc-table">
          <thead><tr><th>PID</th><th>{{ t('opsproc.procNameCol') }}</th><th>{{ t('mediaserver.col.user') }}</th><th>{{ t('reseed.col.status') }}</th>
            <th>CPU%</th><th>{{ t('opsmonitor.mem') }}%</th><th>RSS</th><th>{{ t('opsproc.totalTime') }}</th><th>{{ t('reseed.col.actions') }}</th></tr></thead>
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
                <el-button size="small" type="danger" link @click="killProc(p)">{{ t('opsproc.kill') }}</el-button>
                <el-button size="small" type="danger" link @click="forceKill(p)">{{ t('opsproc.force') }}</el-button>
              </td>
            </tr>
            <tr v-if="!filteredProcesses.length"><td colspan="9" class="td-empty">{{ t('opsproc.noMatchProc') }}</td></tr>
          </tbody>
        </table>
      </div>
      <div class="table-count">{{ t('opsproc.procCount', { n: filteredProcesses.length }) }}</div>
    </div>

    <!-- 网络连接列表 -->
    <div v-if="view === 'network' && sshTargets.length" class="proc-table-wrap glass">
      <div class="filter-bar">
        <el-input v-model="netFilter.pid" placeholder="PID" style="width: 90px" clearable />
        <el-input v-model="netFilter.name" :placeholder="t('opsproc.procName')" style="width: 160px" clearable />
        <el-input v-model="netFilter.port" :placeholder="t('opsproc.port')" style="width: 100px" clearable />
        <el-input v-model="netFilter.state" :placeholder="t('reseed.col.status')" style="width: 120px" clearable />
      </div>
      <div class="table-scroll">
        <table class="proc-table">
          <thead><tr><th>{{ t('opsproc.proto') }}</th><th>{{ t('reseed.col.status') }}</th><th>{{ t('opsproc.localAddr') }}</th><th>{{ t('opsproc.localPort') }}</th>
            <th>{{ t('opsproc.peerAddr') }}</th><th>{{ t('opsproc.peerPort') }}</th><th>PID</th><th>{{ t('ops.tab.process') }}</th></tr></thead>
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
            <tr v-if="!filteredConnections.length"><td colspan="8" class="td-empty">{{ t('opsproc.noConns') }}</td></tr>
          </tbody>
        </table>
      </div>
      <div class="table-count">{{ t('opsproc.connCount', { n: filteredConnections.length }) }}</div>
    </div>
  </div>
</template>

<style scoped>
.ops-proc { padding: 0 4px; }
.proc-toolbar { display: flex; align-items: center; justify-content: space-between; padding: 12px 18px; margin-bottom: 16px; }
.toolbar-left { display: flex; align-items: center; gap: var(--sp-3); }
.target-label { font-size: var(--fs-body); color: var(--text-secondary, #888); white-space: nowrap; }
.view-switch { display: flex; gap: var(--sp-1); margin-left: 8px; }
.view-switch button { padding: 6px 16px; border: 1px solid var(--border, rgba(128,128,128,.25)); background: transparent; border-radius: var(--radius-xs); cursor: pointer; font-size: var(--fs-body); color: var(--text-secondary,#888); }
.view-switch button.active { background: var(--accent, #6c5ce7); color: #fff; border-color: transparent; }
.proc-error { color: var(--pt-danger); margin-bottom: 12px; padding: 8px 14px; background: var(--pt-danger-bg); border-radius: var(--radius-xs); }
.proc-empty { padding: 28px; text-align: center; color: var(--text-secondary, #888); }
.proc-table-wrap { padding: 12px 16px; }
.filter-bar { display: flex; flex-wrap: wrap; gap: var(--sp-2); align-items: center; margin-bottom: 12px; }
.sort-group { display: flex; align-items: center; gap: var(--sp-1); margin-left: auto; }
.sort-label { font-size: var(--fs-xs); color: var(--text-secondary,#888); }
.sort-btn { padding: 4px 10px; border: 1px solid var(--border, rgba(128,128,128,.2)); background: transparent; border-radius: var(--radius-2xs); cursor: pointer; font-size: var(--fs-xs); color: var(--text-secondary,#888); }
.sort-btn.active { background: var(--accent, #6c5ce7); color: #fff; border-color: transparent; }
.table-scroll { overflow-x: auto; max-height: 65vh; overflow-y: auto; }
.proc-table { width: 100%; border-collapse: collapse; font-size: var(--fs-body); }
.proc-table th { text-align: left; padding: 8px 10px; font-weight: 600; position: sticky; top: 0; background: var(--bg-th, rgba(0,0,0,.06)); backdrop-filter: blur(4px); white-space: nowrap; }
.proc-table td { padding: 6px 10px; border-bottom: 1px solid rgba(128,128,128,.1); white-space: nowrap; }
.proc-table tbody tr:hover { background: var(--pt-primary-bg); }
.td-mono { font-family: monospace; font-size: var(--fs-xs); }
.td-name { max-width: 280px; overflow: hidden; text-overflow: ellipsis; }
.td-hot { color: var(--pt-danger); font-weight: 600; }
.td-actions { white-space: nowrap; }
.td-empty { text-align: center; padding: 24px; color: var(--text-tertiary, #aaa); }
.table-count { font-size: var(--fs-xs); color: var(--text-secondary,#888); margin-top: 8px; padding: 4px 2px; }
.state-chip { font-size: var(--fs-2xs); padding: 1px 7px; border-radius: var(--radius-2xs); }
.state-chip.R { background: var(--pt-success-bg); color: var(--pt-success); }
.state-chip.S { background: rgba(52,152,219,.18); color: var(--pt-primary-2); }
.state-chip.Z { background: var(--pt-danger-bg); color: var(--pt-danger); }
.state-chip.D, .state-chip.T { background: var(--pt-warn-bg); color: var(--pt-warn); }
.proto-chip { font-size: var(--fs-2xs); padding: 1px 6px; border-radius: var(--radius-2xs); background: rgba(0,0,0,.08); font-family: monospace; }
.conn-state { font-size: var(--fs-2xs); font-family: monospace; }
.conn-state.ESTAB { color: var(--pt-success); }
.conn-state.LISTEN { color: var(--pt-primary-2); }
.conn-state.TIME-WAIT, .conn-state.CLOSE-WAIT { color: var(--text-muted); }
</style>
