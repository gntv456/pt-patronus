import { ref, onBeforeUnmount } from 'vue'
import {
  mediaServerApi,
  type MediaServer,
  type MediaLibrary,
  type PlaybackReport,
  type DuplicateGroup,
} from '@/api/mediaServer'

export function useMediaServerOps() {
  // ---- 库管理 + 封面预览 ----
  const showLibs = ref(false)
  const libs = ref<MediaLibrary[]>([])
  const libsLoading = ref(false)
  const libsServer = ref<MediaServer | null>(null)
  const coverURL = ref<string>('')
  const coverLoading = ref(false)

  async function openLibs(s: MediaServer) {
    libsServer.value = s
    showLibs.value = true
    coverURL.value = ''
    libsLoading.value = true
    try {
      libs.value = await mediaServerApi.libraries(s.id)
    } catch (e: any) {
      ElMessage.error(e?.message || '拉取库失败')
    } finally {
      libsLoading.value = false
    }
  }

  async function scanLib(lib: MediaLibrary | null) {
    if (!libsServer.value) return
    try {
      const res = await mediaServerApi.scan(libsServer.value.id, lib?.id)
      // opResult 风格：失败也是 HTTP 200 + ok:false，不读回执就会把「一个库都没扫动」toast 成成功。
      if (res && res.ok === false) {
        ElMessage.error(res.error || '扫描触发失败')
        return
      }
      if (lib) {
        ElMessage.success(`已触发扫描：${lib.name}`)
      } else {
        ElMessage.success(`已触发全库扫描（${res?.scanned ?? 0} 个库）`)
      }
    } catch (e: any) {
      ElMessage.error(e?.message || '扫描失败')
    }
  }

  async function refreshLib(lib: MediaLibrary) {
    if (!libsServer.value) return
    try {
      const res = await mediaServerApi.refresh(libsServer.value.id, lib.id)
      if (res && res.ok === false) {
        ElMessage.error(res.error || '刷新触发失败')
        return
      }
      ElMessage.success(`已刷新：${lib.name}`)
    } catch (e: any) {
      ElMessage.error(e?.message || '刷新失败')
    }
  }

  async function genCover(lib: MediaLibrary) {
    if (!libsServer.value) return
    if (coverURL.value) {
      URL.revokeObjectURL(coverURL.value)
      coverURL.value = ''
    }
    coverLoading.value = true
    try {
      const blob = await mediaServerApi.coverBlob(libsServer.value.id, lib.id)
      coverURL.value = URL.createObjectURL(blob)
    } catch (e: any) {
      ElMessage.error(e?.message || '封面生成失败')
    } finally {
      coverLoading.value = false
    }
  }

  onBeforeUnmount(() => {
    if (coverURL.value) URL.revokeObjectURL(coverURL.value)
  })

  // ---- 观影报告 ----
  const showReport = ref(false)
  const report = ref<PlaybackReport | null>(null)
  const reportLoading = ref(false)
  const reportDays = ref(30)

  async function openReport(s: MediaServer) {
    libsServer.value = s
    report.value = null
    showReport.value = true
    reportLoading.value = true
    try {
      report.value = await mediaServerApi.playbackReport(s.id, reportDays.value)
    } catch (e: any) {
      ElMessage.error(e?.message || '观影报告拉取失败')
    } finally {
      reportLoading.value = false
    }
  }

  async function reloadReport() {
    if (!libsServer.value) return
    reportLoading.value = true
    try {
      report.value = await mediaServerApi.playbackReport(libsServer.value.id, reportDays.value)
    } catch (e: any) {
      ElMessage.error(e?.message || '观影报告拉取失败')
    } finally {
      reportLoading.value = false
    }
  }

  // ---- 重复检测 ----
  const showDup = ref(false)
  const dupGroups = ref<DuplicateGroup[]>([])
  const dupLoading = ref(false)
  const dupLib = ref<string>('')

  async function openDup(s: MediaServer) {
    libsServer.value = s
    dupGroups.value = []
    dupLib.value = ''
    showDup.value = true
    try {
      libs.value = await mediaServerApi.libraries(s.id)
      if (libs.value.length) dupLib.value = libs.value[0].id
    } catch (e: any) {
      ElMessage.error(e?.message || '拉取库失败')
    }
  }

  async function runDup() {
    if (!libsServer.value || !dupLib.value) return
    dupLoading.value = true
    try {
      dupGroups.value = await mediaServerApi.duplicates(libsServer.value.id, dupLib.value)
    } catch (e: any) {
      ElMessage.error(e?.message || '查重失败')
    } finally {
      dupLoading.value = false
    }
  }

  return {
    showLibs,
    libs,
    libsLoading,
    libsServer,
    coverURL,
    coverLoading,
    openLibs,
    scanLib,
    refreshLib,
    genCover,
    showReport,
    report,
    reportLoading,
    reportDays,
    openReport,
    reloadReport,
    showDup,
    dupGroups,
    dupLoading,
    dupLib,
    openDup,
    runDup,
  }
}

export function fmtDuration(sec: number): string {
  if (!sec || sec <= 0) return '-'
  if (sec < 3600) return `${Math.round(sec / 60)} 分钟`
  return `${(sec / 3600).toFixed(1)} 小时`
}
