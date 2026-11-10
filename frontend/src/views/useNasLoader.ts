// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// NAS 运维页的全量数据加载（设备/告警/凭据/配置/执行历史/最新指标）。
// 从 NasServer.vue 的 loadAll 迁出;失败时首屏置错误态,管理员额外拉凭据/配置/命令记录。
import { ref, type Ref } from 'vue'
import {
  monitorApi,
  commandRunApi,
  credentialApi,
  opsConfigApi,
  parseMetrics,
  type MonitorTarget,
  type Metrics,
  type MonitorAlert,
  type CommandRun,
} from '@/api/monitor'
import type { OpsCredential, OpsConfig } from '@/api/monitor'

export function useNasLoader(opts: {
  isAdmin: boolean
  targets: Ref<MonitorTarget[]>
  alerts: Ref<MonitorAlert[]>
  credentials: Ref<OpsCredential[]>
  opsConfigs: Ref<OpsConfig[]>
  commandRuns: Ref<CommandRun[]>
  latestMap: Ref<Record<number, Metrics | null>>
  t: (k: string) => string
}) {
  const error = ref('')
  const firstLoad = ref(true)
  // 告警读不到时必须留痕： badges 不亮 = 运维看着像「没有未读告警」，
  // 而设备可能正挂着。targets 失败会走外层 error，这一条以前被 .catch(() => []) 抹平。
  const alertsFailed = ref(false)
  // 告警列表有单页上限：被截断时必须留痕，否则面板把「取到的 200 条」当成「全部未读」。
  const alertsTruncated = ref(false)
  // 运维命令批次台账同样有单页上限（默认 20 条）：账面与告警清单同构。
  const runsTruncated = ref(false)
  const runsTotal = ref<number | undefined>(undefined)
  const alertsTotal = ref(0)

  async function loadAll() {
    try {
      alertsFailed.value = false
      const [ts, al] = await Promise.all([
        monitorApi.list(),
        monitorApi.alerts(undefined, true).catch(() => {
          alertsFailed.value = true
          return { alerts: [], total: 0, limit: 0, truncated: false }
        }),
      ])
      opts.targets.value = ts
      opts.alerts.value = al.alerts
      alertsTotal.value = al.total
      alertsTruncated.value = al.truncated
      if (opts.isAdmin) {
        const [credList, cfgList, runList] = await Promise.all([
          credentialApi.list().catch(() => opts.credentials.value),
          opsConfigApi.list().catch(() => opts.opsConfigs.value),
          commandRunApi
            .listPage(20)
            .catch(() => ({ runs: opts.commandRuns.value, total: undefined, limit: 20, truncated: true })),
        ])
        opts.credentials.value = credList
        opts.opsConfigs.value = cfgList
        opts.commandRuns.value = runList.runs
        runsTruncated.value = runList.truncated
        runsTotal.value = runList.total
      }
      opts.latestMap.value = {}
      // 每个 target 取最新一条快照作为卡片实时指标（末项才是最新）。
      await Promise.all(
        ts.map(async (t) => {
          const res = await monitorApi.metrics(t.id, undefined, 1).catch(() => null)
          const snaps = res?.snapshots ?? []
          opts.latestMap.value[t.id] = snaps.length ? parseMetrics(snaps[snaps.length - 1]) : null
        }),
      )
      error.value = ''
    } catch (e) {
      if (firstLoad.value) error.value = (e as Error).message || opts.t('common.loadFailed')
    } finally {
      firstLoad.value = false
    }
  }

  async function retry() {
    firstLoad.value = true
    await loadAll()
  }

  return { error, firstLoad, alertsFailed, alertsTruncated, alertsTotal, runsTruncated, runsTotal, loadAll, retry }
}
