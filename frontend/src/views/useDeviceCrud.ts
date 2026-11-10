// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// NasServer 设备 CRUD（添加/编辑/删除/探活/告警）—— 从 NasServer.vue 抽出。
// 依赖注入：targets（工作区切换）、reload（操作后刷新）、setWorkspace/openAddMonitorItem（工作区联动）。
import { ref, type Ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { monitorApi, agentApi, type MonitorTarget, type TargetInput, type MonitorAlert } from '@/api/monitor'
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
      // 探活本身没做成时后端回 {ok:false, online:false, error:'原因'}（monitor.go:99）：
      // 「没测出来」不许说成「设备离线」，否则用户去查设备而不是查凭据/网络（#252）。
      else if (!r.ok) ElMessage.warning(`${t.name} 探活未完成${r.error ? '：' + r.error : ''}`)
      else ElMessage.warning(`${t.name} 离线${r.error ? '：' + r.error : ''}`)
      await deps.reload()
    } catch (e) {
      // 后端给的中文原因在 err.message 里（http 拦截器改写），别压成一句「探活失败」。
      ElMessage.error('探活失败：' + ((e as Error).message || '未知原因'))
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

  // 告警动作的失败出口只写一份。后端这几个端点有真会发生的 400/404（告警已被别人清掉、
  // 静默时间在过去/越界），中文原因就在响应的 error 字段里；不接住异常就逃出 async 的 click
  // handler ⇒ Promise rejection 没人处理：列表没变、也不吭声，用户读到的就是「点了没反应」。
  // 失败后照样 reload 不是把失败演成成功：那条告警可能已经不存在，界面要跟服务端对齐。
  function alertActionFailed(action: string, e: unknown) {
    ElMessage.error(action + '失败：' + (e as Error).message)
  }

  async function onReadAlert(a: MonitorAlert) {
    try {
      await monitorApi.readAlert(a.id)
    } catch (e) {
      alertActionFailed('标记已读', e)
    }
    await deps.reload()
  }

  async function onAckAlert(a: MonitorAlert) {
    try {
      await monitorApi.ackAlert(a.id)
      ElMessage.success('告警已确认')
    } catch (e) {
      alertActionFailed('确认告警', e)
    }
    await deps.reload()
  }

  async function onSilenceAlert(a: MonitorAlert) {
    try {
      await monitorApi.silenceAlert(a.id, 30)
      ElMessage.success('已静默 30 分钟')
    } catch (e) {
      alertActionFailed('静默告警', e)
    }
    await deps.reload()
  }

  async function onDeleteAlert(a: MonitorAlert) {
    try {
      await ElMessageBox.confirm('确定删除该告警记录?', '删除告警', {
        type: 'warning',
        confirmButtonText: '删除',
        cancelButtonText: '取消',
      })
    } catch {
      return
    }
    try {
      await monitorApi.deleteAlert(a.id)
      ElMessage.success('已删除')
    } catch (e) {
      alertActionFailed('删除告警', e)
    }
    await deps.reload()
  }

  async function onClearAlerts() {
    try {
      await ElMessageBox.confirm('确定清空全部告警记录?此操作不可恢复。', '清空告警', {
        type: 'warning',
        confirmButtonText: '清空',
        cancelButtonText: '取消',
      })
    } catch {
      return
    }
    try {
      await monitorApi.clearAlerts(false)
      ElMessage.success('已清空告警记录')
    } catch (e) {
      alertActionFailed('清空告警', e)
    }
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
    onDeleteAlert,
    onClearAlerts,
  }
}
