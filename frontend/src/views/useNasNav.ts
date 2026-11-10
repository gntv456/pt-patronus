// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// NAS 运维页的工作区切换、终端抽屉、面板定位导航小组。
// 从 NasServer.vue 迁出;setWorkspace 同步校正 activeTab 使其与新工作区匹配。
import { ref, nextTick } from 'vue'
import type { Ref } from 'vue'
import type { MonitorTarget } from '@/api/monitor'
import type { Workspace, MonitorTab } from './nasServerHelpers'

export function useNasNav(activeWorkspace: Ref<Workspace>, activeTab: Ref<MonitorTab>) {
  // 在线终端：抽屉内部自管理 xterm + WebSocket（含重连/心跳），这里仅持有可见性与目标。
  const terminalVisible = ref(false)
  const terminalTarget = ref<MonitorTarget | null>(null)
  function openTerminal(t: MonitorTarget) {
    terminalTarget.value = t
    terminalVisible.value = true
  }

  function setWorkspace(workspace: Workspace) {
    activeWorkspace.value = workspace
    if (workspace === 'asset' && !['host', 'nas', 'camera'].includes(activeTab.value)) activeTab.value = 'host'
    if (workspace === 'monitor' && !['http', 'tcp', 'process', 'script'].includes(activeTab.value))
      activeTab.value = 'http'
  }

  function focusExecPanel() {
    setWorkspace('execute')
    nextTick(() => document.querySelector('.exec-panel')?.scrollIntoView({ behavior: 'smooth', block: 'start' }))
  }
  function openMonitorTab() {
    setWorkspace('monitor')
    activeTab.value = 'http'
    nextTick(() => document.querySelector('.np-tabs')?.scrollIntoView({ behavior: 'smooth', block: 'start' }))
  }

  return { terminalVisible, terminalTarget, openTerminal, setWorkspace, focusExecPanel, openMonitorTab }
}
