<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import DeviceCard from '@/components/monitor/DeviceCard.vue'
import DeviceForm from '@/components/monitor/DeviceForm.vue'
import DeviceDetail from '@/components/monitor/DeviceDetail.vue'
import AlertList from '@/components/monitor/AlertList.vue'
import RtspPlayer from '@/components/monitor/RtspPlayer.vue'
import AgentManager from '@/components/monitor/AgentManager.vue'
import GlassIcon from '@/components/GlassIcon.vue'
import {
  monitorApi,
  agentApi,
  parseMetrics,
  type MonitorTarget,
  type Metrics,
  type MonitorAlert,
  type TargetInput,
} from '@/api/monitor'
import { ElMessage, ElMessageBox } from 'element-plus'
import { isSSHAuthError, showSSHAuthFix } from '@/utils/ssh-fix'
import { showAgentSecret } from '@/utils/agent-secret'
import { useAuthStore } from '@/stores/auth'

// Agent 创建/删除/重置密钥是 admin 专属（后端 RequireAdmin）；非 admin 隐藏入口。
const auth = useAuthStore()

// NasServer — 设备监控主视图：服务器/NAS/摄像头 三 Tab，复用同一套卡片组件。
const targets = ref<MonitorTarget[]>([])
const latestMap = ref<Record<number, Metrics | null>>({})
const alerts = ref<MonitorAlert[]>([])
const activeTab = ref<'host' | 'nas' | 'camera'>('host')

const formVisible = ref(false)
const editing = ref<MonitorTarget | null>(null)
const formDefaultType = ref('host')
const detailVisible = ref(false)
const detailTarget = ref<MonitorTarget | null>(null)
const previewVisible = ref(false)
const previewTarget = ref<MonitorTarget | null>(null)
const alertVisible = ref(false)
const agentVisible = ref(false)
const helpVisible = ref(false)

const filtered = computed(() => targets.value.filter((t) => t.type === activeTab.value))
const unreadCount = computed(() => alerts.value.filter((a) => !a.read).length)
let timer: number | undefined

async function loadAll() {
  try {
    const [ts, al] = await Promise.all([
      monitorApi.list(),
      monitorApi.alerts(undefined, true).catch(() => []),
    ])
    targets.value = ts
    alerts.value = al
    latestMap.value = {}
    // 每个 target 取最新一条快照作为卡片实时指标
    await Promise.all(
      ts.map(async (t) => {
        const snaps = await monitorApi.metrics(t.id, undefined, 1).catch(() => [])
        latestMap.value[t.id] = snaps.length ? parseMetrics(snaps[0]) : null
      }),
    )
  } catch {
    /* 静默，轮询重试 */
  }
}

function openAdd() {
  editing.value = null
  formDefaultType.value = activeTab.value
  formVisible.value = true
}
function openEdit(t: MonitorTarget) {
  editing.value = t
  formVisible.value = true
}
async function onSubmit(input: TargetInput) {
  try {
    if (!editing.value && input.protocol === 'agent') {
      // Agent 设备：走 Agent 创建（建 target + 生成 secret），弹安装命令
      const r = await agentApi.create(input.name)
      showAgentSecret(r.agent, r.plain_secret)
    } else if (editing.value) {
      await monitorApi.update(editing.value.id, input)
      ElMessage.success('已更新')
    } else {
      await monitorApi.create(input)
      ElMessage.success('已添加，开始采集')
    }
    await loadAll()
  } catch (e) {
    ElMessage.error('保存失败：' + (e as Error).message)
  }
}
async function onProbe(t: MonitorTarget) {
  try {
    const r = await monitorApi.probe(t.id)
    if (r.online) ElMessage.success(`${t.name} 在线`)
    else if (isSSHAuthError(r.error)) showSSHAuthFix()
    else ElMessage.warning(`${t.name} 离线${r.error ? '：' + r.error : ''}`)
    await loadAll()
  } catch {
    ElMessage.error('探活失败')
  }
}
async function onRemove(t: MonitorTarget) {
  try {
    await ElMessageBox.confirm(`确定删除「${t.name}」？关联的指标与告警将一并删除。`, '删除设备', {
      type: 'warning',
    })
    await monitorApi.remove(t.id)
    ElMessage.success('已删除')
    await loadAll()
  } catch {
    /* 用户取消 */
  }
}
function onDetail(t: MonitorTarget) {
  detailTarget.value = t
  detailVisible.value = true
}
function onPreview(t: MonitorTarget) {
  previewTarget.value = t
  previewVisible.value = true
}
async function onReadAlert(a: MonitorAlert) {
  await monitorApi.readAlert(a.id)
  await loadAll()
}

onMounted(() => {
  loadAll()
  timer = window.setInterval(loadAll, 15000)
})
onBeforeUnmount(() => {
  if (timer) window.clearInterval(timer)
})

const tabs = [
  { name: 'host' as const, label: '服务器', icon: 'server', color: ['#0f2027', '#2c5364'] },
  { name: 'nas' as const, label: 'NAS', icon: 'nas', color: ['#f7971e', '#ffd200'] },
  { name: 'camera' as const, label: '摄像头', icon: 'camera', color: ['#ff5f6d', '#ffc371'] },
]
</script>

<template>
  <div class="nas-page">
    <header class="np-head glass-bar">
      <div class="np-title">
        <GlassIcon name="server" from="#0f2027" to="#2c5364" :size="32" />
        <h2>NAS 服务器监控</h2>
      </div>
      <div class="np-actions">
        <el-button @click="helpVisible = true">使用说明</el-button>
        <el-button :type="unreadCount ? 'danger' : 'default'" @click="alertVisible = true">
          告警<span v-if="unreadCount" class="badge">{{ unreadCount }}</span>
        </el-button>
        <el-button v-if="auth.isAdmin" @click="agentVisible = true">Agent</el-button>
        <el-button type="primary" @click="openAdd">+ 添加设备</el-button>
      </div>
    </header>

    <el-tabs v-model="activeTab" class="np-tabs">
      <el-tab-pane v-for="tab in tabs" :key="tab.name" :name="tab.name">
        <template #label>
          <span class="tab-label">
            <GlassIcon :name="tab.icon" :from="tab.color[0]" :to="tab.color[1]" :size="22" />
            {{ tab.label }}
            <span class="tab-count">{{ targets.filter((t) => t.type === tab.name).length }}</span>
          </span>
        </template>
        <div v-if="filtered.length" class="card-grid">
          <DeviceCard
            v-for="t in filtered"
            :key="t.id"
            :target="t"
            :metrics="latestMap[t.id]"
            @edit="openEdit"
            @detail="onDetail"
            @probe="onProbe"
            @remove="onRemove"
            @preview="onPreview"
          />
        </div>
        <div v-else class="empty-state glass">
          <GlassIcon :name="tab.icon" :from="tab.color[0]" :to="tab.color[1]" :size="48" />
          <p>暂无{{ tab.label }}设备</p>
          <el-button type="primary" @click="openAdd">添加第一台</el-button>
        </div>
      </el-tab-pane>
    </el-tabs>

    <DeviceForm v-model:visible="formVisible" :target="editing" :default-type="formDefaultType" @submit="onSubmit" />
    <AgentManager v-model:visible="agentVisible" @changed="loadAll" />
    <DeviceDetail v-model:visible="detailVisible" :target="detailTarget" />

    <el-dialog v-model="helpVisible" title="使用说明" width="620px" class="help">
      <h4>选哪种监控方式？</h4>
      <p><b>服务器</b></p>
      <ul>
        <li><b>本机</b>：监控 PTPatronus 所在机器，无需填地址</li>
        <li><b>SSH 远程</b>：监控其它 Linux，填 IP + 账号 + 密码</li>
        <li><b>Agent 上报</b>：被监控机装 agent 主动上报 —— <b>推荐</b>，免 SSH 认证烦恼</li>
      </ul>
      <p><b>NAS</b>：群晖 / TrueNAS 走厂商 API</p>
      <p><b>摄像头</b>：ONVIF / RTSP</p>

      <h4>SSH 连不上？</h4>
      <p>报 <code>no supported methods remain</code> = root 被禁密码登录（OpenSSH 默认安全策略）。</p>
      <ul>
        <li>点错误旁的「如何修复」→ 复制一条命令到服务器执行（放开 root 密码）</li>
        <li>或改用 <b>Agent 上报</b>，完全绕开 SSH</li>
      </ul>

      <h4>装 Agent（最省事）</h4>
      <ol>
        <li>点「+ 添加设备」→ 类型「服务器」→ 协议「Agent 上报」→ 填名 → 保存</li>
        <li>复制弹出的一条命令</li>
        <li>SSH / 宝塔进被监控机（Linux，root），粘贴执行（自动下载 + 装 systemd + 开机自启）</li>
        <li>回本页，设备自动上线并上报指标</li>
      </ol>

      <h4>其它</h4>
      <ul>
        <li>采集间隔 / 超时、告警阈值（CPU / 内存 / 磁盘 / 温度 / 离线）：编辑设备调整</li>
        <li>SSH 私钥认证：编辑 SSH 设备粘贴私钥（高级，适合禁密码的服务器）</li>
      </ul>
    </el-dialog>

    <el-dialog
      v-model="previewVisible"
      :title="previewTarget?.name + ' · 实时画面'"
      width="640px"
      destroy-on-close
    >
      <RtspPlayer v-if="previewTarget" :target="previewTarget" />
    </el-dialog>

    <el-drawer v-model="alertVisible" title="告警记录" size="380px">
      <AlertList :alerts="alerts" @read="onReadAlert" />
    </el-drawer>
  </div>
</template>

<style scoped>
.nas-page {
  padding: 4px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.np-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 18px;
}
.np-title {
  display: flex;
  align-items: center;
  gap: 12px;
}
.np-title h2 {
  margin: 0;
  font-size: 18px;
}
.np-actions {
  display: flex;
  gap: 8px;
  align-items: center;
}
.badge {
  display: inline-block;
  margin-left: 4px;
  min-width: 18px;
  padding: 0 5px;
  border-radius: 999px;
  background: #fff;
  color: #e74c3c;
  font-size: 11px;
  font-weight: 700;
}
.np-tabs {
  padding: 0 12px;
}
.tab-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.tab-count {
  font-size: 12px;
  color: var(--text-muted);
  background: var(--glass-bg);
  padding: 1px 7px;
  border-radius: 999px;
}
.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
  padding-top: 12px;
}
.empty-state {
  padding: 48px;
  text-align: center;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
}
.empty-state p {
  color: var(--text-muted);
  margin: 4px 0;
}
.help h4 {
  margin: 14px 0 6px;
  font-size: 14px;
  color: var(--text-strong);
}
.help h4:first-child {
  margin-top: 0;
}
.help p,
.help ul,
.help ol {
  margin: 4px 0;
  font-size: 13px;
  line-height: 1.7;
}
.help ul,
.help ol {
  padding-left: 20px;
}
.help code {
  background: var(--glass-bg);
  padding: 1px 5px;
  border-radius: 4px;
  font-size: 12px;
}
</style>
