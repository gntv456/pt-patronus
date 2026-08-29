// OpsDocker 的数据刷新与资源监控轮询（从 OpsDocker.vue 拆出，行为不变）。
// 含：目标（SSH host/nas）加载、local/remote 双模式列表+stats 刷新、
// sparkline 历史环形缓冲（20 点）、5s 轮询生命周期。
import { computed, ref } from 'vue'
import { dockerApi, type DockerListResp, type DockerStatsRow } from '@/api/docker'
import { monitorApi, type MonitorTarget } from '@/api/monitor'

const HISTORY_POINTS = 20

export function useDockerData(mode: () => 'local' | 'remote') {
  const targets = ref<MonitorTarget[]>([])
  const selectedTarget = ref<number>(0)
  const localAvailable = ref(false)
  const loading = ref(false)
  const error = ref('')
  const data = ref<DockerListResp>({ containers: [], images: [], volumes: [], networks: [] })
  const stats = ref<DockerStatsRow[]>([])
  const projectCount = ref(0)

  // 资源监控历史数据（用于 sparkline）
  const cpuHistory = ref<Record<string, number[]>>({})
  const memHistory = ref<Record<string, number[]>>({})
  let statsTimer: number | undefined

  // 远程模式支持 host 和 nas 类型（只要有 SSH 连接信息即可管理 Docker）
  const sshTargets = computed(() => targets.value.filter((t) => t.type === 'host' || t.type === 'nas'))

  async function loadTargets() {
    try {
      targets.value = await monitorApi.list()
      if (sshTargets.value.length && !selectedTarget.value) {
        selectedTarget.value = sshTargets.value[0].id
      }
    } catch (e: unknown) {
      error.value = (e as Error).message
    }
  }

  function pushHistory(map: Record<string, number[]>, key: string, val: number) {
    if (!map[key]) map[key] = []
    map[key].push(val)
    if (map[key].length > HISTORY_POINTS) map[key].shift()
  }

  function ingestStats(st: DockerStatsRow[]) {
    stats.value = st
    for (const s of stats.value) {
      const cpu = parseFloat(s.CPUPerc?.replace('%', '') || '0') || 0
      const mem = parseFloat(s.MemPerc?.replace('%', '') || '0') || 0
      pushHistory(cpuHistory.value, s.Name, cpu)
      pushHistory(memHistory.value, s.Name, mem)
    }
  }

  async function refreshStats() {
    try {
      const st = mode() === 'local' ? await dockerApi.localStats() : await dockerApi.stats(selectedTarget.value)
      ingestStats(st.stats || [])
    } catch {
      /* 静默失败 */
    }
  }

  async function refresh() {
    loading.value = true
    error.value = ''
    try {
      if (mode() === 'local') {
        const [list, st] = await Promise.all([
          dockerApi.localList(),
          dockerApi.localStats().catch(() => ({ stats: [] as DockerStatsRow[] })),
        ])
        data.value = list
        ingestStats(st.stats || [])
      } else {
        if (!selectedTarget.value) return
        const [list, st] = await Promise.all([
          dockerApi.list(selectedTarget.value),
          dockerApi.stats(selectedTarget.value).catch(() => ({ stats: [] as DockerStatsRow[] })),
        ])
        data.value = list
        ingestStats(st.stats || [])
      }
    } catch (e: unknown) {
      error.value = (e as Error).message
      data.value = { containers: [], images: [], volumes: [], networks: [] }
    } finally {
      loading.value = false
      if (mode() === 'local') {
        dockerApi
          .localProjects()
          .then((r) => {
            projectCount.value = r.projects?.length || 0
          })
          .catch(() => {})
      }
    }
  }

  function startStatsPolling() {
    stopStatsPolling()
    statsTimer = window.setInterval(refreshStats, 5000)
  }
  function stopStatsPolling() {
    if (statsTimer) {
      clearInterval(statsTimer)
      statsTimer = undefined
    }
  }

  function statFor(name: string): DockerStatsRow | undefined {
    const clean = name.replace(/^\//, '')
    return stats.value.find((s) => s.Name === clean || s.Container === clean.slice(0, 12))
  }

  function cpuHistoryFor(name: string): number[] {
    const clean = name.replace(/^\//, '')
    return cpuHistory.value[clean] || []
  }
  function memHistoryFor(name: string): number[] {
    const clean = name.replace(/^\//, '')
    return memHistory.value[clean] || []
  }

  function resetHistory() {
    cpuHistory.value = {}
    memHistory.value = {}
  }

  async function initLocalAvailability() {
    try {
      const st = await dockerApi.localStatus()
      localAvailable.value = st.available
    } catch {
      localAvailable.value = false
    }
  }

  return {
    targets,
    selectedTarget,
    localAvailable,
    loading,
    error,
    data,
    stats,
    projectCount,
    sshTargets,
    loadTargets,
    refresh,
    refreshStats,
    startStatsPolling,
    stopStatsPolling,
    statFor,
    cpuHistoryFor,
    memHistoryFor,
    resetHistory,
    initLocalAvailability,
  }
}
