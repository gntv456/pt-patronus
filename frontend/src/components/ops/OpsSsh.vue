<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { sshApi, type SshServiceStatus, type SshSessionRow, type SshLogRow } from '@/api/ssh_admin'
import { monitorApi, type MonitorTarget } from '@/api/monitor'

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
  const warn = action === 'stop' ? '\n\n⚠️ 停止 SSH 服务可能导致当前连接中断，你将无法远程访问该主机！' : ''
  try {
    await ElMessageBox.confirm(`确定要${label} SSH 服务吗？${warn}`, '确认', {
      type: action === 'stop' ? 'error' : 'warning',
    })
    const res = await sshApi.action(selectedTarget.value, action)
    if (res.ok) { ElMessage.success(`${label}成功`); refresh() }
    else ElMessage.error(res.error || `${label}失败`)
  } catch (e: any) { if (e !== 'cancel') ElMessage.error(e.message) }
}

async function toggleEnable(val: string | number | boolean) {
  const enable = val === true
  try {
    const res = await sshApi.toggleEnable(selectedTarget.value, enable)
    if (res.ok) { ElMessage.success(enable ? '已开启开机自启' : '已关闭开机自启'); refresh() }
    else ElMessage.error(res.error || '操作失败')
  } catch (e: any) { ElMessage.error(e.message) }
}

function openConfig() {
  if (!status.value) return
  configForm.value = { port: status.value.port, listen_addr: status.value.listen_addr, confirm: false }
  configVisible.value = true
}

async function saveConfig() {
  const port = parseInt(configForm.value.port)
  if (isNaN(port) || port < 1 || port > 65535) { ElMessage.warning('端口范围 1-65535'); return }
  try {
    const res = await sshApi.config(selectedTarget.value, {
      port: configForm.value.port,
      listen_addr: configForm.value.listen_addr,
      confirm: true,
    })
    if (res.ok) { ElMessage.success('配置已更新并 reload'); configVisible.value = false; refresh() }
    else ElMessage.error(res.error || '配置失败')
  } catch (e: any) { ElMessage.error(e.message) }
}

async function killSession(s: SshSessionRow) {
  try {
    await ElMessageBox.confirm(`确定要断开会话「${s.user}@${s.tty}」(PID: ${s.pid}) 吗？`, '断开会话', { type: 'warning' })
    const res = await sshApi.killSession(selectedTarget.value, s.pid)
    if (res.ok) { ElMessage.success('会话已断开'); refresh() }
    else ElMessage.error(res.error || '断开失败')
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
        <label class="target-label">目标主机</label>
        <el-select v-model="selectedTarget" placeholder="选择 SSH 主机" style="width: 200px" @change="refresh">
          <el-option v-for="t in sshTargets" :key="t.id" :label="t.name" :value="t.id" />
        </el-select>
        <div class="view-switch">
          <button :class="{ active: view === 'service' }" @click="switchView('service')">服务</button>
          <button :class="{ active: view === 'sessions' }" @click="switchView('sessions')">会话</button>
          <button :class="{ active: view === 'logs' }" @click="switchView('logs')">登录日志</button>
        </div>
      </div>
      <el-button type="primary" :loading="loading" @click="refresh">刷新</el-button>
    </div>

    <div v-if="error" class="ssh-error">{{ error }}</div>
    <div v-if="!sshTargets.length" class="ssh-empty glass">尚未添加 SSH 主机。请在 NAS → 监控目标 中添加。</div>

    <!-- 服务配置 -->
    <div v-if="view === 'service' && sshTargets.length" v-loading="loading" class="ssh-panel glass">
      <div v-if="status" class="service-grid">
        <div class="svc-card">
          <div class="svc-label">运行状态</div>
          <div class="svc-value">
            <span class="status-dot" :class="status.active ? 'on' : 'off'"></span>
            {{ status.active ? '运行中' : '已停止' }} ({{ status.state }})
          </div>
        </div>
        <div class="svc-card">
          <div class="svc-label">开机自启</div>
          <div class="svc-value">
            <el-switch :model-value="status.enabled" @change="toggleEnable" />
          </div>
        </div>
        <div class="svc-card">
          <div class="svc-label">服务名</div>
          <div class="svc-value mono">{{ status.service_name }}</div>
        </div>
        <div class="svc-card">
          <div class="svc-label">监听端口</div>
          <div class="svc-value mono">{{ status.port }}</div>
        </div>
        <div class="svc-card">
          <div class="svc-label">监听地址</div>
          <div class="svc-value mono">{{ status.listen_addr }}</div>
        </div>
        <div class="svc-card">
          <div class="svc-label">版本</div>
          <div class="svc-value mono small">{{ status.version }}</div>
        </div>
      </div>
      <div class="service-actions">
        <el-button type="success" @click="doAction('start', '启动')">启动</el-button>
        <el-button type="danger" @click="doAction('stop', '停止')">停止</el-button>
        <el-button type="warning" @click="doAction('restart', '重启')">重启</el-button>
        <el-button @click="doAction('reload', '重载配置')">重载</el-button>
        <el-button type="primary" @click="openConfig">修改配置</el-button>
      </div>
    </div>

    <!-- 会话列表 -->
    <div v-if="view === 'sessions' && sshTargets.length" class="ssh-panel glass">
      <div class="table-scroll">
        <table class="ssh-table">
          <thead><tr><th>用户</th><th>终端</th><th>来源</th><th>登录时间</th><th>空闲</th><th>当前</th><th>操作</th></tr></thead>
          <tbody>
            <tr v-for="s in sessions" :key="s.pid || s.tty">
              <td>{{ s.user }}</td>
              <td class="mono">{{ s.tty }}</td>
              <td class="mono">{{ s.from || '-' }}</td>
              <td class="mono">{{ s.login }}</td>
              <td>{{ s.idle }}</td>
              <td class="td-what" :title="s.what">{{ s.what || '-' }}</td>
              <td><el-button v-if="s.pid" size="small" type="danger" link @click="killSession(s)">断开</el-button></td>
            </tr>
            <tr v-if="!sessions.length"><td colspan="7" class="td-empty">无活跃会话</td></tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 登录日志 -->
    <div v-if="view === 'logs' && sshTargets.length" class="ssh-panel glass">
      <div class="table-scroll">
        <table class="ssh-table">
          <thead><tr><th>时间</th><th>类型</th><th>用户</th><th>来源 IP</th><th>端口</th><th>认证方式</th></tr></thead>
          <tbody>
            <tr v-for="(l, i) in logs" :key="i">
              <td class="mono small">{{ l.time }}</td>
              <td><span class="log-type" :style="{ color: logTypeColor(l.type) }">{{ l.type }}</span></td>
              <td>{{ l.user || '-' }}</td>
              <td class="mono">{{ l.from || '-' }}</td>
              <td class="mono">{{ l.port || '-' }}</td>
              <td>{{ l.method || '-' }}</td>
            </tr>
            <tr v-if="!logs.length"><td colspan="6" class="td-empty">无登录日志</td></tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 修改配置对话框 -->
    <el-dialog v-model="configVisible" title="修改 SSH 配置" width="440px">
      <el-alert type="warning" :closable="false" show-icon style="margin-bottom: 16px">
        修改后会自动备份原配置并 reload。改端口后需用新端口重连。
      </el-alert>
      <el-form label-width="90px">
        <el-form-item label="监听端口">
          <el-input v-model="configForm.port" placeholder="22" />
        </el-form-item>
        <el-form-item label="监听地址">
          <el-select v-model="configForm.listen_addr" style="width: 100%">
            <el-option value="0.0.0.0" label="0.0.0.0（所有地址）" />
            <el-option value="127.0.0.1" label="127.0.0.1（仅本地）" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="configVisible = false">取消</el-button>
        <el-button type="primary" @click="saveConfig">保存并重载</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.ops-ssh { padding: 0 4px; }
.ssh-toolbar { display: flex; align-items: center; justify-content: space-between; padding: 12px 18px; margin-bottom: 16px; }
.toolbar-left { display: flex; align-items: center; gap: 12px; }
.target-label { font-size: 13px; color: var(--text-secondary, #888); white-space: nowrap; }
.view-switch { display: flex; gap: 4px; margin-left: 8px; }
.view-switch button { padding: 6px 16px; border: 1px solid var(--border, rgba(128,128,128,.25)); background: transparent; border-radius: 6px; cursor: pointer; font-size: 13px; color: var(--text-secondary,#888); }
.view-switch button.active { background: var(--accent, #6c5ce7); color: #fff; border-color: transparent; }
.ssh-error { color: #e74c3c; margin-bottom: 12px; padding: 8px 14px; background: rgba(231,76,60,.08); border-radius: 8px; }
.ssh-empty { padding: 28px; text-align: center; color: var(--text-secondary, #888); }
.ssh-panel { padding: 16px 18px; }
.service-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 12px; margin-bottom: 20px; }
.svc-card { padding: 14px 16px; border-radius: 8px; background: rgba(128,128,128,.06); }
.svc-label { font-size: 12px; color: var(--text-secondary,#888); margin-bottom: 6px; }
.svc-value { font-size: 15px; font-weight: 600; display: flex; align-items: center; gap: 8px; }
.svc-value.mono { font-family: monospace; }
.svc-value.small { font-size: 12px; font-weight: 400; }
.status-dot { width: 10px; height: 10px; border-radius: 50%; display: inline-block; }
.status-dot.on { background: #27ae60; box-shadow: 0 0 6px #27ae60; }
.status-dot.off { background: #e74c3c; }
.service-actions { display: flex; gap: 10px; flex-wrap: wrap; }
.table-scroll { overflow-x: auto; max-height: 65vh; overflow-y: auto; }
.ssh-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.ssh-table th { text-align: left; padding: 8px 10px; font-weight: 600; position: sticky; top: 0; background: var(--bg-th, rgba(0,0,0,.06)); white-space: nowrap; }
.ssh-table td { padding: 6px 10px; border-bottom: 1px solid rgba(128,128,128,.1); white-space: nowrap; }
.ssh-table tbody tr:hover { background: rgba(108,92,231,.06); }
.mono { font-family: monospace; font-size: 12px; }
.small { font-size: 11px; }
.td-what { max-width: 200px; overflow: hidden; text-overflow: ellipsis; }
.td-empty { text-align: center; padding: 24px; color: var(--text-tertiary, #aaa); }
.log-type { font-weight: 600; font-family: monospace; font-size: 12px; }
</style>
