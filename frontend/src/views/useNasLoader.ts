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

  async function loadAll() {
    try {
      const [ts, al] = await Promise.all([monitorApi.list(), monitorApi.alerts(undefined, true).catch(() => [])])
      opts.targets.value = ts
      opts.alerts.value = al
      if (opts.isAdmin) {
        const [credList, cfgList, runList] = await Promise.all([
          credentialApi.list().catch(() => opts.credentials.value),
          opsConfigApi.list().catch(() => opts.opsConfigs.value),
          commandRunApi.list(20).catch(() => opts.commandRuns.value),
        ])
        opts.credentials.value = credList
        opts.opsConfigs.value = cfgList
        opts.commandRuns.value = runList
      }
      opts.latestMap.value = {}
      // 每个 target 取最新一条快照作为卡片实时指标（末项才是最新）。
      await Promise.all(
        ts.map(async (t) => {
          const snaps = await monitorApi.metrics(t.id, undefined, 1).catch(() => [])
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

  return { error, firstLoad, loadAll, retry }
}
