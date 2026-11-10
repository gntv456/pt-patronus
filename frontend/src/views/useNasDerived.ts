// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// NasServer 的派生计算簇（从 NasServer.vue 拆出；行为不变）。
import { computed, type Ref } from 'vue'
import { isSSHHost } from './nasServerHelpers'
import type { MonitorTarget, MonitorAlert } from '@/api/monitor'

export function useNasDerived(opts: {
  targets: Ref<MonitorTarget[]>
  alerts: Ref<MonitorAlert[]>
  opsConfigs: Ref<unknown[]>
  activeTab: Ref<string>
}) {
const filtered = computed(() => opts.targets.value.filter((t) => t.type === opts.activeTab.value))
const unreadCount = computed(() => opts.alerts.value.filter((a) => !a.read).length)
const hostCount = computed(() => opts.targets.value.filter((t) => t.type === 'host').length)
const monitorCount = computed(
  () => opts.targets.value.filter((t) => ['http', 'tcp', 'process', 'script'].includes(t.type)).length,
)
const configCount = computed(() => opts.opsConfigs.value.length)
const activeAlertCount = computed(() => opts.alerts.value.filter((a) => !a.resolved_at).length)
const sshTargets = computed(() => opts.targets.value.filter(isSSHHost))

  return { filtered, unreadCount, hostCount, monitorCount, configCount, activeAlertCount, sshTargets }
}
