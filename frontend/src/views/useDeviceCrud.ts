// NasServer 设备 CRUD（添加/编辑/删除/探活/告警）—— 从 NasServer.vue 抽出。
// 依赖注入：targets（工作区切换）、reload（操作后刷新）、setWorkspace/openAddMonitorItem（工作区联动）。
import { ref, type Ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  monitorApi,
  agentApi,
  type MonitorTarget,
  type TargetInput,
  type MonitorAlert,
} from '@/api/monitor'
import { isSSHAuthError, showSSHAuthFix } from '@/utils/ssh-fix'
import { showAgentSecret } from '@/utils/agent-secret'
import type { OpsCredential } from '@/api/monitor'

type MonitorTab = 'host' | 'nas' | 'camera' | 'http' | 'tcp' | 'process' | 'script'
export type Workspace = 'asset' | 'execute' | 'monitor' | 'deploy'

export function useDeviceCrud(
  auth: { isAdmin: boolean },
  deps: {
    activeWorkspace: Ref<Workspace>
    activeTab: Ref<MonitorTab>
    credentials: Ref<OpsCredential[]>
    reload: () => Promise<void>
    setWorkspace: (w: Workspace) => void
  },
) {
  const formVisible = ref(false)
  const editing = ref<MonitorTarget | null>(null)
  const formDefaultType = ref('host')
  const detailVisible = ref(false)
  const detailTarget = ref<MonitorTarget | null>(null)
  const previewVisible = ref(false)
  const previewTarget = ref<MonitorTarget | null>(null)
  const alertVisible = ref(false)

  function openAddAsset() {
    deps.setWorkspace('asset')
    editing.value = null
    formDefaultType.value = ['host', 'nas', 'camera'].includes(deps.activeTab.value) ? deps.activeTab.value : 'host'
    formVisible.value = true
  }

  function openAddMonitorItem(type: MonitorTab = 'http') {
    deps.setWorkspace('monitor')
    deps.activeTab.value = (['http', 'tcp', 'process', 'script'].includes(type) ? type : 'http') as MonitorTab
    editing.value = null
    formDefaultType.value = deps.activeTab.value
    formVisible.value = true
  }

  function openAdd() {
    if (deps.activeWorkspace.value === 'monitor') openAddMonitorItem(deps.activeTab.value)
    else openAddAsset()
  }

  function openEdit(t: MonitorTarget) {
    editing.value = t
    formVisible.value = true
  }

  async function onSubmit(input: TargetInput) {
    try {
      if (!editing.value && input.protocol === 'agent') {
        // Agent 设备：走 Agent 创建（建 target + 生成 secret），弹安装命令
        const r = await agentApi.create(input.name, input.type === 'nas' ? 'nas' : 'host')
        showAgentSecret(r.agent, r.plain_secret)
      } else if (editing.value) {
        await monitorApi.update(editing.value.id, input)
        ElMessage.success('已更新')
      } else {
        await monitorApi.create(input)
        ElMessage.success('已添加，开始采集')
      }
      await deps.reload()
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
      await deps.reload()
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
      await deps.reload()
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
    await deps.reload()
  }

  async function onAckAlert(a: MonitorAlert) {
    await monitorApi.ackAlert(a.id)
    ElMessage.success('告警已确认')
    await deps.reload()
  }

  async function onSilenceAlert(a: MonitorAlert) {
    await monitorApi.silenceAlert(a.id, 30)
    ElMessage.success('已静默 30 分钟')
    await deps.reload()
  }

  return {
    formVisible,
    editing,
    formDefaultType,
    detailVisible,
    detailTarget,
    previewVisible,
    previewTarget,
    alertVisible,
    openAddAsset,
    openAddMonitorItem,
    openAdd,
    openEdit,
    onSubmit,
    onProbe,
    onRemove,
    onDetail,
    onPreview,
    onReadAlert,
    onAckAlert,
    onSilenceAlert,
  }
}
