// 仪表盘逐站同步：进度、日志、失败站点。
// 从 Dashboard.vue 抽出；syncStats 完成后通过回调联动刷新其它卡片。
import { ref, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { dashboardApi } from '@/api/dashboard'
import { siteApi } from '@/api/site'
import { useDashboardData } from '@/composables/useDashboardData'
import { useWs, type TaskProgress } from '@/composables/useWs'

export type SyncLog = {
  id: number
  time: string
  site: string
  status: 'info' | 'running' | 'success' | 'error'
  message: string
}

const syncing = ref(false)
const syncTotal = ref(0)
const syncDone = ref(0)
const syncSuccess = ref(0)
const syncCurrent = ref('')
const syncErrors = ref<{ site: string; error: string }[]>([])
const syncLogs = ref<SyncLog[]>([])
const syncPercent = computed(() => (syncTotal.value ? Math.round((syncDone.value / syncTotal.value) * 100) : 0))

// W4：把逐站进度镜像成 task-progress 形态喂给顶栏 TaskIndicator（与后端 SyncAll
// 发射的 WS 帧同构——指示器不需要区分进度来自前端循环还是后端推送）。
const { mirrorTaskProgress } = useWs()
function mirrorProgress(done: number, total: number, site: string, finished: boolean) {
  mirrorTaskProgress({ kind: 'site-sync', done, total, site, finished })
}
function mirrorFinished(done: number, total: number) {
  mirrorProgress(done, total, '', true)
}

let syncLogSeq = 0
function addSyncLog(status: SyncLog['status'], site: string, message: string) {
  syncLogs.value.unshift({
    id: ++syncLogSeq,
    time: new Date().toLocaleTimeString('zh-CN', { hour12: false }),
    site,
    status,
    message,
  })
}

function errorText(e: unknown): string {
  return (e as Error)?.message || '未知错误'
}

/**
 * 逐站抓取启用站点统计。
 * @param onDone 同步结束后的联动回调（刷新连通性、自动化/AI/任务/时间线卡片）。
 */
async function syncStats(onDone?: () => void) {
  if (syncing.value) return
  const { overview, allSites, trendDays, trend, fmtBytes } = useDashboardData()
  syncing.value = true
  syncTotal.value = 0
  syncDone.value = 0
  syncSuccess.value = 0
  syncCurrent.value = '读取站点列表'
  syncErrors.value = []
  syncLogs.value = []
  addSyncLog('info', '刷新统计', '正在读取启用站点列表')
  try {
    const sites = await siteApi.list()
    allSites.value = sites
    const activeSites = sites.filter((s) => s.is_active)
    syncTotal.value = activeSites.length
    if (!activeSites.length) {
      addSyncLog('error', '刷新统计', '没有启用的站点可同步')
      ElMessage.warning('没有启用的站点可同步')
      return
    }
    addSyncLog('info', '刷新统计', `共 ${activeSites.length} 个启用站点，开始逐站同步`)

    for (const site of activeSites) {
      syncCurrent.value = site.name
      const started = performance.now()
      addSyncLog('running', site.name, '开始同步')
      try {
        const ov = await dashboardApi.syncOne(site.id)
        overview.value = ov
        syncSuccess.value += 1
        const cost = Math.max(0.1, (performance.now() - started) / 1000).toFixed(1)
        const item = ov.sites.find((s) => s.site_id === site.id)
        const stat = item
          ? `上传 ${fmtBytes(item.uploaded)}，下载 ${fmtBytes(item.downloaded)}，做种 ${item.seeding}，分享率 ${item.ratio > 0 ? item.ratio.toFixed(2) : '-'}`
          : '已写入最新统计'
        addSyncLog('success', site.name, `同步成功，用时 ${cost}s，${stat}`)
      } catch (e: unknown) {
        const msg = errorText(e)
        syncErrors.value.push({ site: site.name, error: msg })
        addSyncLog('error', site.name, `同步失败：${msg}`)
      } finally {
        syncDone.value += 1
        mirrorProgress(syncDone.value, syncTotal.value, site.name, false)
      }
    }
    mirrorFinished(syncDone.value, syncTotal.value)

    trend.value = await dashboardApi.trend(trendDays.value).catch(() => trend.value)
    if (syncErrors.value.length) {
      const msg = `同步完成：成功 ${syncSuccess.value} 个，失败 ${syncErrors.value.length} 个`
      addSyncLog(syncSuccess.value ? 'info' : 'error', '刷新统计', msg)
      if (syncSuccess.value) ElMessage.warning(msg)
      else ElMessage.error(`${msg}，请查看下方失败原因`)
    } else {
      addSyncLog('success', '刷新统计', `全部同步成功：${syncSuccess.value} 个站点`)
      ElMessage.success(`统计已同步：${syncSuccess.value} 个站点`)
    }
  } catch (e: unknown) {
    addSyncLog('error', '刷新统计', `同步失败：${errorText(e)}`)
    ElMessage.error(`同步失败：${errorText(e)}`)
  } finally {
    syncing.value = false
    syncCurrent.value = ''
    mirrorFinished(syncDone.value, syncTotal.value)
    onDone?.()
  }
}

export function useDashboardSync() {
  return { syncing, syncTotal, syncDone, syncSuccess, syncCurrent, syncErrors, syncLogs, syncPercent, syncStats }
}
