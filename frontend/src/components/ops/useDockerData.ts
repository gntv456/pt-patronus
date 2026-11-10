// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

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
  // 本机 Docker 不可用时后端给的具体原因（含实际地址与出路）；空串=没给。
  const localStatusHint = ref('')
  const loading = ref(false)
  const error = ref('')
  const data = ref<DockerListResp>({ containers: [], images: [], volumes: [], networks: [] })
  const stats = ref<DockerStatsRow[]>([])
  const projectCount = ref(0)

  // 资源监控历史数据（用于 sparkline）
  const cpuHistory = ref<Record<string, number[]>>({})
  const memHistory = ref<Record<string, number[]>>({})
  let statsTimer: number | undefined

  // 资源条的「停更」账面：原因用后端给的中文（503/502 都带 message），界面自己不编造解释。
  const statsError = ref('')
  const statsLastOkAt = ref(0)

  // 统计读失败的留痕口径只写一处：轮询与整页刷新两个入口都走它（同一判定抄第二遍必漂移）。
  function noteStatsFailure(e: unknown) {
    const be = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    statsError.value = be || (e as Error)?.message || ''
    stats.value = []
  }

  // 统计读取的唯一出口：成功 ⇒ 复位停更标记并写下水位；失败 ⇒ 留原因、交回空读数。
  //（整页刷新不该因为统计失败就丢掉容器列表，所以两个入口都走这里，判定只有一份。）
  async function loadStats(): Promise<DockerStatsRow[]> {
    try {
      const st = mode() === 'local' ? await dockerApi.localStats() : await dockerApi.stats(selectedTarget.value)
      statsError.value = ''
      statsLastOkAt.value = Date.now()
      return st.stats || []
    } catch (e: unknown) {
      noteStatsFailure(e)
      return []
    }
  }

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
    // 5 秒轮询失败以前是全静默 ⇒ CPU/内存条停在最后一次成功读数上，看着像实时值（#241/#245）。
    ingestStats(await loadStats())
  }

  async function refresh() {
    loading.value = true
    error.value = ''
    try {
      if (mode() === 'local') {
        const [list, rows] = await Promise.all([dockerApi.localList(), loadStats()])
        data.value = list
        ingestStats(rows)
      } else {
        if (!selectedTarget.value) return
        const [list, rows] = await Promise.all([dockerApi.list(selectedTarget.value), loadStats()])
        data.value = list
        ingestStats(rows)
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
      // 后端探测失败时给的是「实际检查了哪个地址 + 该怎么办」（#240），
      // 从前这里只留 available=false，界面那句泛化提示把唯一的出路整份丢掉。
      localStatusHint.value = st.available ? '' : st.error || ''
    } catch (e: unknown) {
      localAvailable.value = false
      localStatusHint.value = (e as Error)?.message || ''
    }
  }

  return {
    targets,
    selectedTarget,
    localAvailable,
    localStatusHint,
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
    statsError,
    statsLastOkAt,
    statFor,
    cpuHistoryFor,
    memHistoryFor,
    resetHistory,
    initLocalAvailability,
  }
}
