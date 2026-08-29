<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { sshApi, type SshServiceStatus, type SshSessionRow, type SshLogRow } from '@/api/ssh_admin'
import { monitorApi, type MonitorTarget } from '@/api/monitor'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const targets = ref<MonitorTarget[]>([])
const selectedTarget = ref<number>(0)
const loading = ref(false)
const error = ref('')
const view = ref<'service' | 'sessions' | 'logs'>('service')

const status = ref<SshServiceStatus | null>(null)
const sessions = ref<SshSessionRow[]>([])
const logs = ref<SshLogRow[]>([])

// 配置表单
const configForm = ref({ port: '', listen_addr: '', confirm: false })
const configVisible = ref(false)

const sshTargets = computed(() => targets.value.filter((t) => t.type === 'host'))

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
    if (view.value === 'service') {
      status.value = await sshApi.status(selectedTarget.value)
    } else if (view.value === 'sessions') {
      const r = await sshApi.sessions(selectedTarget.value)
      sessions.value = r.sessions || []
    } else {
      const r = await sshApi.logs(selectedTarget.value, 200)
      logs.value = r.logs || []
    }
  } catch (e: any) { error.value = e.message } finally { loading.value = false }
}

async function switchView(v: 'service' | 'sessions' | 'logs') {
  view.value = v
  await refresh()
}

async function doAction(action: 'start' | 'stop' | 'restart' | 'reload', label: string) {
  const warn = action === 'stop' ? '\n\n⚠️ ' + t('ssh.stopWarn') : ''
  try {
    await ElMessageBox.confirm(t('ssh.actionConfirm', { label }), t('common.confirm'), {
      type: action === 'stop' ? 'error' : 'warning',
    })
    const res = await sshApi.action(selectedTarget.value, action)
    if (res.ok) { ElMessage.success(t('ssh.actionOk', { label })); refresh() }
    else ElMessage.error(res.error || t('ssh.actionFailed', { label }))
  } catch (e: any) { if (e !== 'cancel') ElMessage.error(e.message) }
}

async function toggleEnable(val: string | number | boolean) {
  const enable = val === true
  try {
    const res = await sshApi.toggleEnable(selectedTarget.value, enable)
    if (res.ok) { ElMessage.success(enable ? t('ssh.autoStartOn') : t('ssh.autoStartOff')); refresh() }
    else ElMessage.error(res.error || t('common.actionFailed'))
  } catch (e: any) { ElMessage.error(e.message) }
}

function openConfig() {
  if (!status.value) return
  configForm.value = { port: status.value.port, listen_addr: status.value.listen_addr, confirm: false }
  configVisible.value = true
}

async function saveConfig() {
  const port = parseInt(configForm.value.port)
  if (isNaN(port) || port < 1 || port > 65535) { ElMessage.warning(t('ssh.portRange')); return }
  try {
    const res = await sshApi.config(selectedTarget.value, {
      port: configForm.value.port,
      listen_addr: configForm.value.listen_addr,
      confirm: true,
    })
    if (res.ok) { ElMessage.success(t('ssh.cfgUpdated')); configVisible.value = false; refresh() }
    else ElMessage.error(res.error || t('ssh.cfgFailed'))
  } catch (e: any) { ElMessage.error(e.message) }
}

async function killSession(s: SshSessionRow) {
  try {
    await ElMessageBox.confirm(t('ssh.killConfirm', { user: s.user, tty: s.tty, pid: s.pid }), t('ssh.killTitle'), { type: 'warning' })
    const res = await sshApi.killSession(selectedTarget.value, s.pid)
    if (res.ok) { ElMessage.success(t('ssh.killed')); refresh() }
    else ElMessage.error(res.error || t('ssh.killFailed'))
  } catch (e: any) { if (e !== 'cancel') ElMessage.error(e.message) }
}

function logTypeColor(t: string): string {
  if (t === 'Accepted') return '#27ae60'
  if (t === 'Failed') return '#e74c3c'
  return '#95a5a6'
}

onMounted(async () => { await loadTargets(); if (selectedTarget.value) refresh() })
</script>

<template>
  <div class="ops-ssh">
    <div class="ssh-toolbar glass">
      <div class="toolbar-left">
        <label class="target-label">{{ t('ssh.targetHost') }}</label>
        <el-select v-model="selectedTarget" :placeholder="t('ssh.pickHost')" style="width: 200px" @change="refresh">
          <el-option v-for="t in sshTargets" :key="t.id" :label="t.name" :value="t.id" />
        </el-select>
        <div class="view-switch">
          <button :class="{ active: view === 'service' }" @click="switchView('service')">{{ t('ssh.viewService') }}</button>
          <button :class="{ active: view === 'sessions' }" @click="switchView('sessions')">{{ t('ssh.viewSessions') }}</button>
          <button :class="{ active: view === 'logs' }" @click="switchView('logs')">{{ t('ssh.viewLogs') }}</button>
        </div>
      </div>
      <el-button type="primary" :loading="loading" @click="refresh">{{ t('common.refresh') }}</el-button>
    </div>

    <div v-if="error" class="ssh-error">{{ error }}</div>
    <div v-if="!sshTargets.length" class="ssh-empty glass">{{ t('ssh.emptyHosts') }}</div>

    <!-- 服务配置 -->
    <div v-if="view === 'service' && sshTargets.length" v-loading="loading" class="ssh-panel glass">
      <div v-if="status" class="service-grid">
        <div class="svc-card">
          <div class="svc-label">{{ t('opsdocker.runtime') }}</div>
          <div class="svc-value">
            <span class="status-dot" :class="status.active ? 'on' : 'off'"></span>
            {{ status.active ? t('opsdocker.st.running') : t('opsdocker.st.exited') }} ({{ status.state }})
          </div>
        </div>
        <div class="svc-card">
          <div class="svc-label">{{ t('ssh.autoStart') }}</div>
          <div class="svc-value">
            <el-switch :model-value="status.enabled" @change="toggleEnable" />
          </div>
        </div>
        <div class="svc-card">
          <div class="svc-label">{{ t('ssh.svcName') }}</div>
          <div class="svc-value mono">{{ status.service_name }}</div>
        </div>
        <div class="svc-card">
          <div class="svc-label">{{ t('ssh.listenPort') }}</div>
          <div class="svc-value mono">{{ status.port }}</div>
        </div>
        <div class="svc-card">
          <div class="svc-label">{{ t('ssh.listenAddr') }}</div>
          <div class="svc-value mono">{{ status.listen_addr }}</div>
        </div>
        <div class="svc-card">
          <div class="svc-label">{{ t('ssh.version') }}</div>
          <div class="svc-value mono small">{{ status.version }}</div>
        </div>
      </div>
      <div class="service-actions">
        <el-button type="success" @click="doAction('start', t('ssh.act.start'))">{{ t('ssh.act.start') }}</el-button>
        <el-button type="danger" @click="doAction('stop', t('ssh.act.stop'))">{{ t('ssh.act.stop') }}</el-button>
        <el-button type="warning" @click="doAction('restart', t('ssh.act.restart'))">{{ t('ssh.act.restart') }}</el-button>
        <el-button @click="doAction('reload', t('ssh.act.reload'))">{{ t('ssh.act.reloadShort') }}</el-button>
        <el-button type="primary" @click="openConfig">{{ t('ssh.modifyCfg') }}</el-button>
      </div>
    </div>

    <!-- 会话列表 -->
    <div v-if="view === 'sessions' && sshTargets.length" class="ssh-panel glass">
      <div class="table-scroll">
        <table class="ssh-table">
          <thead><tr><th>{{ t('smt.user') }}</th><th>{{ t('ssh.tty') }}</th><th>{{ t('ssh.source') }}</th><th>{{ t('ssh.loginAt') }}</th><th>{{ t('ssh.idle') }}</th><th>{{ t('ssh.current') }}</th><th>{{ t('common.action') }}</th></tr></thead>
          <tbody>
            <tr v-for="s in sessions" :key="s.pid || s.tty">
              <td>{{ s.user }}</td>
              <td class="mono">{{ s.tty }}</td>
              <td class="mono">{{ s.from || '-' }}</td>
              <td class="mono">{{ s.login }}</td>
              <td>{{ s.idle }}</td>
              <td class="td-what" :title="s.what">{{ s.what || '-' }}</td>
              <td><el-button v-if="s.pid" size="small" type="danger" link @click="killSession(s)">{{ t('ssh.kill') }}</el-button></td>
            </tr>
            <tr v-if="!sessions.length"><td colspan="7" class="td-empty">{{ t('ssh.noSessions') }}</td></tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 登录日志 -->
    <div v-if="view === 'logs' && sshTargets.length" class="ssh-panel glass">
      <div class="table-scroll">
        <table class="ssh-table">
          <thead><tr><th>{{ t('pad.time') }}</th><th>{{ t('cloud.kind') }}</th><th>{{ t('smt.user') }}</th><th>{{ t('ssh.srcIp') }}</th><th>{{ t('deviceform.port') }}</th><th>{{ t('ssh.authMethod') }}</th></tr></thead>
          <tbody>
            <tr v-for="(l, i) in logs" :key="i">
              <td class="mono small">{{ l.time }}</td>
              <td><span class="log-type" :style="{ color: logTypeColor(l.type) }">{{ l.type }}</span></td>
              <td>{{ l.user || '-' }}</td>
              <td class="mono">{{ l.from || '-' }}</td>
              <td class="mono">{{ l.port || '-' }}</td>
              <td>{{ l.method || '-' }}</td>
            </tr>
            <tr v-if="!logs.length"><td colspan="6" class="td-empty">{{ t('ssh.noLogs') }}</td></tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 修改配置对话框 -->
    <el-dialog v-model="configVisible" :title="t('ssh.cfgTitle')" width="440px">
      <el-alert type="warning" :closable="false" show-icon style="margin-bottom: 16px">
        {{ t('ssh.cfgTip') }}
      </el-alert>
      <el-form label-width="90px">
        <el-form-item :label="t('ssh.listenPort')">
          <el-input v-model="configForm.port" placeholder="22" />
        </el-form-item>
        <el-form-item :label="t('ssh.listenAddr')">
          <el-select v-model="configForm.listen_addr" style="width: 100%">
            <el-option value="0.0.0.0" :label="t('ssh.allAddrs')" />
            <el-option value="127.0.0.1" :label="t('ssh.localOnly')" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="configVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="saveConfig">{{ t('ssh.saveReload') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.ops-ssh { padding: 0 4px; }
.ssh-toolbar { display: flex; align-items: center; justify-content: space-between; padding: 12px 18px; margin-bottom: 16px; }
.toolbar-left { display: flex; align-items: center; gap: var(--sp-3); }
.target-label { font-size: var(--fs-body); color: var(--text-secondary, #888); white-space: nowrap; }
.view-switch { display: flex; gap: var(--sp-1); margin-left: 8px; }
.view-switch button { padding: 6px 16px; border: 1px solid var(--border, rgba(128,128,128,.25)); background: transparent; border-radius: var(--radius-xs); cursor: pointer; font-size: var(--fs-body); color: var(--text-secondary,#888); }
.view-switch button.active { background: var(--accent, #6c5ce7); color: #fff; border-color: transparent; }
.ssh-error { color: var(--pt-danger); margin-bottom: 12px; padding: 8px 14px; background: var(--pt-danger-bg); border-radius: var(--radius-xs); }
.ssh-empty { padding: 28px; text-align: center; color: var(--text-secondary, #888); }
.ssh-panel { padding: 16px 18px; }
.service-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: var(--sp-3); margin-bottom: 20px; }
.svc-card { padding: 14px 16px; border-radius: var(--radius-xs); background: rgba(128,128,128,.06); }
.svc-label { font-size: var(--fs-xs); color: var(--text-secondary,#888); margin-bottom: 6px; }
.svc-value { font-size: var(--fs-lg); font-weight: 600; display: flex; align-items: center; gap: var(--sp-2); }
.svc-value.mono { font-family: monospace; }
.svc-value.small { font-size: var(--fs-xs); font-weight: 400; }
.status-dot { width: 10px; height: 10px; border-radius: 50%; display: inline-block; }
.status-dot.on { background: #27ae60; box-shadow: 0 0 6px #27ae60; }
.status-dot.off { background: #e74c3c; }
.service-actions { display: flex; gap: var(--sp-2); flex-wrap: wrap; }
.table-scroll { overflow-x: auto; max-height: 65vh; overflow-y: auto; }
.ssh-table { width: 100%; border-collapse: collapse; font-size: var(--fs-body); }
.ssh-table th { text-align: left; padding: 8px 10px; font-weight: 600; position: sticky; top: 0; background: var(--bg-th, rgba(0,0,0,.06)); white-space: nowrap; }
.ssh-table td { padding: 6px 10px; border-bottom: 1px solid rgba(128,128,128,.1); white-space: nowrap; }
.ssh-table tbody tr:hover { background: var(--pt-primary-bg); }
.mono { font-family: monospace; font-size: var(--fs-xs); }
.small { font-size: var(--fs-2xs); }
.td-what { max-width: 200px; overflow: hidden; text-overflow: ellipsis; }
.td-empty { text-align: center; padding: 24px; color: var(--text-tertiary, #aaa); }
.log-type { font-weight: 600; font-family: monospace; font-size: var(--fs-xs); }
</style>
